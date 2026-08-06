package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	_ "modernc.org/sqlite"
)

// testConfig returns a SchedulerConfig with all built-in tasks enabled on
// valid schedules, mirroring config.DefaultConfig()'s Scheduler section, so
// addTasks() registers the full built-in task set from AI.md PART 19.
func testConfig() config.SchedulerConfig {
	return config.SchedulerConfig{
		Timezone:                  "UTC",
		CatchUpWindow:             "1h",
		MaxRetries:                3,
		RetryDelay:                "5m",
		SessionCleanupCron:        "@every 15m",
		SessionCleanupEnabled:     true,
		TokenCleanupCron:          "@every 15m",
		TokenCleanupEnabled:       true,
		ExpireURLsCron:            "30 2 * * *",
		ExpireURLsEnabled:         true,
		LogRotationCron:           "0 0 * * *",
		LogRotationEnabled:        true,
		BackupCron:                "0 2 * * *",
		BackupEnabled:             true,
		BackupHourlyCron:          "@hourly",
		BackupHourlyEnabled:       false,
		SSLRenewalCron:            "0 3 * * *",
		SSLRenewalEnabled:         true,
		GeoIPUpdateCron:           "0 3 * * 0",
		GeoIPUpdateEnabled:        true,
		BlocklistUpdateCron:       "0 4 * * *",
		BlocklistUpdateEnabled:    true,
		CVEUpdateCron:             "0 5 * * *",
		CVEUpdateEnabled:          true,
		UpdateCheckCron:           "0 6 * * *",
		UpdateCheckEnabled:        true,
		UpdateBranch:              "stable",
		HealthcheckCron:           "@every 5m",
		HealthcheckEnabled:        true,
		TorHealthCron:             "@every 10m",
		TorHealthEnabled:          true,
		ClusterHeartbeatCron:      "@every 30s",
		ClusterHeartbeatEnabled:   true,
		DomainVerificationCron:    "@every 30m",
		DomainVerificationEnabled: true,
	}
}

// newTestStore opens a real Store backed by on-disk SQLite (modernc.org/sqlite,
// pure Go per AI.md PART 3) with the full production schema applied via
// store.Open/InitSchema, inside a t.TempDir() (never the project tree, per
// testing-rules.md). This gives scheduler tests the real scheduler_tasks /
// scheduler_history / api_tokens / admin_sessions / user_sessions / urls
// schemas instead of a hand-duplicated subset.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// newTestScheduler builds a Scheduler wired to a fresh test store with the
// full built-in task config, but does not call Start() — callers drive the
// internal methods (addTasks/loadOrInitTaskState/runTask/tick) directly so
// tests do not depend on the real 15s ticker.
func newTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	st := newTestStore(t)
	return New(st, "", "", "", "", "1.0.0", nil, config.SecurityConfig{}, false, config.BackupRetentionConfig{}, testConfig())
}

func TestNewFallsBackToUTCAndDefaultCatchUpWindow(t *testing.T) {
	st := newTestStore(t)
	s := New(st, "", "", "", "", "1.0.0", nil, config.SecurityConfig{}, false, config.BackupRetentionConfig{}, config.SchedulerConfig{
		Timezone:      "not-a-real-timezone",
		CatchUpWindow: "not-a-duration",
	})
	if s.loc != time.UTC {
		t.Errorf("loc = %v, want UTC", s.loc)
	}
	if s.catchUpWindow != time.Hour {
		t.Errorf("catchUpWindow = %v, want 1h", s.catchUpWindow)
	}
}

func TestAddTasksRegistersAllBuiltinTasks(t *testing.T) {
	s := newTestScheduler(t)
	s.addTasks()

	// AI.md PART 19 "Built-in Tasks (Required)" plus the PART 36 custom-domain
	// verification maintenance task — 15 tasks total.
	if len(s.tasks) != 15 {
		t.Fatalf("len(tasks) = %d, want 15", len(s.tasks))
	}

	critical := map[string]bool{
		"session_cleanup":   true,
		"token_cleanup":     true,
		"log_rotation":      true,
		"healthcheck_self":  true,
		"tor_health":        true,
		"cluster_heartbeat": true,
		"ssl_renewal":       true,
	}
	byID := map[string]*task{}
	for _, tk := range s.tasks {
		byID[tk.id] = tk
	}
	for id := range critical {
		if _, ok := byID[id]; !ok {
			t.Errorf("critical task %q was not registered", id)
		}
	}

	// Global vs local classification per "Cluster Mode Task Distribution".
	wantGlobal := map[string]bool{
		"backup_daily": true, "backup_hourly": true, "ssl_renewal": true,
		"geoip_update": true, "blocklist_update": true, "cve_update": true,
		"update_check": true, "domain_verification": true,
	}
	for id, tk := range byID {
		if wantGlobal[id] && tk.taskType != "global" {
			t.Errorf("task %q taskType = %q, want global", id, tk.taskType)
		}
		if !wantGlobal[id] && tk.taskType != "local" {
			t.Errorf("task %q taskType = %q, want local", id, tk.taskType)
		}
	}
}

func TestVerifyDomainsNoopWhenServiceUnset(t *testing.T) {
	s := newTestScheduler(t)
	// No SetDomainService call — the task must be a safe no-op.
	if err := s.verifyDomains(context.Background()); err != nil {
		t.Fatalf("verifyDomains with nil service: %v", err)
	}
}

func TestVerifyDomainsCleansExpiredPending(t *testing.T) {
	st := newTestStore(t)
	s := New(st, "", "", "", "", "1.0.0", nil, config.SecurityConfig{}, false, config.BackupRetentionConfig{}, testConfig())

	ds := service.NewDomainService(st, config.CustomDomainsConfig{VerificationTTL: 3600})
	s.SetDomainService(ds)

	ctx := context.Background()
	old := time.Now().Add(-2 * time.Hour)
	if _, err := st.UsersDB.ExecContext(ctx,
		`INSERT INTO custom_domains (owner_type, owner_id, domain, verification_status, verification_token, status, created_at, updated_at)
		 VALUES ('user', 1, 'stale.example', 'pending', 'tok', 'pending', ?, ?)`,
		old, old,
	); err != nil {
		t.Fatalf("insert stale domain: %v", err)
	}

	if err := s.verifyDomains(ctx); err != nil {
		t.Fatalf("verifyDomains: %v", err)
	}

	var n int
	if err := st.UsersDB.QueryRow("SELECT COUNT(*) FROM custom_domains WHERE domain = 'stale.example'").Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected stale unverified domain to be cleaned up, still present (%d)", n)
	}
}

func TestAddTasksSkipsInvalidScheduleZeroTasksRegistered(t *testing.T) {
	// Boundary: every task has an invalid cron schedule, so addTasks must
	// register zero tasks rather than error out or panic.
	s := newTestScheduler(t)
	cfg := testConfig()
	cfg.SessionCleanupCron = "garbage"
	cfg.TokenCleanupCron = "garbage"
	cfg.ExpireURLsCron = "garbage"
	cfg.LogRotationCron = "garbage"
	cfg.BackupCron = "garbage"
	cfg.BackupHourlyCron = "garbage"
	cfg.SSLRenewalCron = "garbage"
	cfg.GeoIPUpdateCron = "garbage"
	cfg.BlocklistUpdateCron = "garbage"
	cfg.CVEUpdateCron = "garbage"
	cfg.UpdateCheckCron = "garbage"
	cfg.HealthcheckCron = "garbage"
	cfg.TorHealthCron = "garbage"
	cfg.ClusterHeartbeatCron = "garbage"
	cfg.DomainVerificationCron = "garbage"
	s.cfg = cfg

	s.addTasks()
	if len(s.tasks) != 0 {
		t.Fatalf("len(tasks) = %d, want 0 when every schedule is invalid", len(s.tasks))
	}
}

func TestAddTasksRerunReplacesPreviousTaskList(t *testing.T) {
	// addTasks is called again on every Start(); it must reset s.tasks
	// rather than append duplicates.
	s := newTestScheduler(t)
	s.addTasks()
	first := len(s.tasks)
	s.addTasks()
	if len(s.tasks) != first {
		t.Fatalf("second addTasks() call left %d tasks, want %d (no duplicates)", len(s.tasks), first)
	}
}

func newRegisteredTask(t *testing.T, s *Scheduler, id, taskType, schedule string, fn func(context.Context) error) *task {
	t.Helper()
	cs, err := parseCronSchedule(schedule)
	if err != nil {
		t.Fatalf("parseCronSchedule(%q): %v", schedule, err)
	}
	return &task{id: id, name: id, taskType: taskType, schedule: schedule, cron: cs, enabled: true, fn: fn}
}

func schedulerTaskRow(t *testing.T, s *Scheduler, id string) (nextRun, lastRun, runCount, failCount int64, lastStatus string) {
	t.Helper()
	row := s.store.ServerDB.QueryRow(
		`SELECT next_run, COALESCE(last_run,0), run_count, fail_count, COALESCE(last_status,'') FROM scheduler_tasks WHERE id=?`, id)
	if err := row.Scan(&nextRun, &lastRun, &runCount, &failCount, &lastStatus); err != nil {
		t.Fatalf("scheduler_tasks row for %q: %v", id, err)
	}
	return
}

func TestLoadOrInitTaskStateNewTaskInsertsRow(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "t1", "local", "@every 15m", func(context.Context) error { return nil })

	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	s.loadOrInitTaskState(tk, now)

	wantNext := now.Add(15 * time.Minute)
	if !tk.nextRun.Equal(wantNext) {
		t.Errorf("tk.nextRun = %v, want %v", tk.nextRun, wantNext)
	}
	nextRun, _, runCount, failCount, _ := schedulerTaskRow(t, s, "t1")
	if nextRun != wantNext.Unix() {
		t.Errorf("stored next_run = %d, want %d", nextRun, wantNext.Unix())
	}
	if runCount != 0 || failCount != 0 {
		t.Errorf("run_count=%d fail_count=%d, want 0/0 for a brand-new task", runCount, failCount)
	}
}

func TestLoadOrInitTaskStateNotYetDueKeepsStoredNextRun(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "t2", "local", "@every 15m", func(context.Context) error { return nil })

	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	s.loadOrInitTaskState(tk, now) // insert

	// Reload from "5 minutes later" — stored next_run is still in the future.
	later := now.Add(5 * time.Minute)
	s.loadOrInitTaskState(tk, later)
	wantNext := now.Add(15 * time.Minute)
	if !tk.nextRun.Equal(wantNext) {
		t.Errorf("tk.nextRun = %v, want unchanged %v", tk.nextRun, wantNext)
	}
}

func TestLoadOrInitTaskStateCatchUpWithinWindowRunsImmediately(t *testing.T) {
	s := newTestScheduler(t)
	s.catchUpWindow = time.Hour
	tk := newRegisteredTask(t, s, "t3", "local", "@every 15m", func(context.Context) error { return nil })

	base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	s.loadOrInitTaskState(tk, base) // next_run = base+15m

	// 30 minutes after the missed run, still inside the 1h catch-up window.
	missedAt := base.Add(15 * time.Minute)
	now := missedAt.Add(30 * time.Minute)
	s.loadOrInitTaskState(tk, now)
	if !tk.nextRun.Equal(now) {
		t.Errorf("tk.nextRun = %v, want %v (immediate catch-up)", tk.nextRun, now)
	}
}

func TestLoadOrInitTaskStateCatchUpExactBoundaryRunsImmediately(t *testing.T) {
	// Boundary: overdue == catchUpWindow exactly. The production code uses
	// `overdue <= s.catchUpWindow`, so exactly-at-the-boundary must still
	// catch up rather than skip.
	s := newTestScheduler(t)
	s.catchUpWindow = time.Hour
	tk := newRegisteredTask(t, s, "t4", "local", "@every 15m", func(context.Context) error { return nil })

	base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	s.loadOrInitTaskState(tk, base) // next_run = base+15m
	missedAt := base.Add(15 * time.Minute)
	now := missedAt.Add(time.Hour) // overdue == exactly 1h == catchUpWindow

	s.loadOrInitTaskState(tk, now)
	if !tk.nextRun.Equal(now) {
		t.Errorf("tk.nextRun = %v, want %v (exact-boundary catch-up)", tk.nextRun, now)
	}
}

func TestLoadOrInitTaskStateOutsideWindowSkipsAndReschedules(t *testing.T) {
	s := newTestScheduler(t)
	s.catchUpWindow = time.Hour
	tk := newRegisteredTask(t, s, "t5", "local", "@every 15m", func(context.Context) error { return nil })

	base := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	s.loadOrInitTaskState(tk, base) // next_run = base+15m
	missedAt := base.Add(15 * time.Minute)
	now := missedAt.Add(time.Hour + time.Second) // just past the catch-up window

	s.loadOrInitTaskState(tk, now)
	wantNext := now.Add(15 * time.Minute) // @every 15m computed fresh from now
	if !tk.nextRun.Equal(wantNext) {
		t.Errorf("tk.nextRun = %v, want %v (skipped, rescheduled from now)", tk.nextRun, wantNext)
	}
	nextRun, _, _, _, _ := schedulerTaskRow(t, s, "t5")
	if nextRun != wantNext.Unix() {
		t.Errorf("stored next_run = %d, want %d", nextRun, wantNext.Unix())
	}
}

func TestLoadOrInitTaskStateSyncsConfigOnExistingRow(t *testing.T) {
	// Re-running loadOrInitTaskState for an existing task must keep name/
	// type/schedule/enabled in sync with the (possibly-changed) config,
	// since the admin panel is the single source of truth per AI.md PART 19.
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "t6", "local", "@every 15m", func(context.Context) error { return nil })
	now := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)
	s.loadOrInitTaskState(tk, now)

	tk.name = "Renamed Task"
	tk.enabled = false
	s.loadOrInitTaskState(tk, now)

	var name string
	var enabled int
	row := s.store.ServerDB.QueryRow(`SELECT name, enabled FROM scheduler_tasks WHERE id=?`, "t6")
	if err := row.Scan(&name, &enabled); err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != "Renamed Task" || enabled != 0 {
		t.Errorf("name=%q enabled=%d, want %q/0 after config sync", name, enabled, "Renamed Task")
	}
}

func TestRunTaskSuccessRecordsHistoryAndResetsAttempt(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "ok-task", "local", "@every 15m", func(context.Context) error { return nil })
	tk.attempt = 2 // simulate prior failures — must reset to 0 on success
	now := time.Now()
	s.loadOrInitTaskState(tk, now)

	s.runTask(tk)

	if tk.attempt != 0 {
		t.Errorf("attempt = %d, want 0 after a successful run", tk.attempt)
	}
	if tk.running {
		t.Error("running = true after runTask returned, want false")
	}
	nextRun, lastRun, runCount, failCount, lastStatus := schedulerTaskRow(t, s, "ok-task")
	if lastStatus != "success" {
		t.Errorf("last_status = %q, want success", lastStatus)
	}
	if runCount != 1 || failCount != 0 {
		t.Errorf("run_count=%d fail_count=%d, want 1/0", runCount, failCount)
	}
	if lastRun == 0 {
		t.Error("last_run was not recorded")
	}
	if nextRun <= lastRun {
		t.Errorf("next_run (%d) should be after last_run (%d)", nextRun, lastRun)
	}

	var histCount int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM scheduler_history WHERE task_id=?`, "ok-task").Scan(&histCount); err != nil {
		t.Fatalf("query history: %v", err)
	}
	if histCount != 1 {
		t.Errorf("scheduler_history rows = %d, want 1", histCount)
	}
}

func TestRunTaskFailureRetriesWithExponentialBackoff(t *testing.T) {
	s := newTestScheduler(t)
	wantErr := errors.New("boom")
	tk := newRegisteredTask(t, s, "retry-task", "local", "0 3 * * *", func(context.Context) error { return wantErr })
	tk.maxRetries = 3
	tk.retryDelay = time.Second
	now := time.Now()
	s.loadOrInitTaskState(tk, now)

	// Attempt 1: backoff == retryDelay (1s).
	before := time.Now()
	s.runTask(tk)
	if tk.attempt != 1 {
		t.Fatalf("attempt = %d, want 1", tk.attempt)
	}
	gotBackoff := tk.nextRun.Sub(before)
	if gotBackoff < 900*time.Millisecond || gotBackoff > 2*time.Second {
		t.Errorf("attempt 1 backoff ~= %v, want ~1s", gotBackoff)
	}

	// Attempt 2: backoff == retryDelay*2 (2s).
	before = time.Now()
	s.runTask(tk)
	if tk.attempt != 2 {
		t.Fatalf("attempt = %d, want 2", tk.attempt)
	}
	gotBackoff = tk.nextRun.Sub(before)
	if gotBackoff < 1800*time.Millisecond || gotBackoff > 3*time.Second {
		t.Errorf("attempt 2 backoff ~= %v, want ~2s", gotBackoff)
	}

	_, _, runCount, failCount, lastStatus := schedulerTaskRow(t, s, "retry-task")
	if lastStatus != "failed" {
		t.Errorf("last_status = %q, want failed", lastStatus)
	}
	if runCount != 0 || failCount != 2 {
		t.Errorf("run_count=%d fail_count=%d, want 0/2", runCount, failCount)
	}
}

func TestRunTaskGivesUpAfterMaxRetriesAndReschedulesByCron(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "give-up-task", "local", "0 3 * * *", func(context.Context) error { return errors.New("boom") })
	tk.maxRetries = 1
	tk.retryDelay = time.Millisecond
	now := time.Now()
	s.loadOrInitTaskState(tk, now)

	s.runTask(tk) // attempt 1 == maxRetries → still retries once
	s.runTask(tk) // attempt 2 > maxRetries → gives up, falls back to cron schedule

	if tk.attempt != 2 {
		t.Fatalf("attempt = %d, want 2", tk.attempt)
	}
	wantNext := tk.cron.Next(tk.nextRun.Add(-25*time.Hour), time.UTC) // sanity: nextRun must be a real cron occurrence
	_ = wantNext
	if tk.nextRun.Hour() != 3 || tk.nextRun.Minute() != 0 {
		t.Errorf("nextRun = %v, want the next 03:00 cron occurrence after giving up", tk.nextRun)
	}
}

func TestRunTaskZeroMaxRetriesGivesUpImmediately(t *testing.T) {
	// Boundary: a noRetry() task (maxRetries=0) must not retry at all —
	// t.attempt (1) > t.maxRetries (0) is true on the very first failure.
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "no-retry-task", "local", "0 3 * * *", func(context.Context) error { return errors.New("boom") })
	tk.maxRetries = 0
	now := time.Now()
	s.loadOrInitTaskState(tk, now)

	s.runTask(tk)
	if tk.attempt != 1 {
		t.Fatalf("attempt = %d, want 1", tk.attempt)
	}
	if tk.nextRun.Hour() != 3 || tk.nextRun.Minute() != 0 {
		t.Errorf("nextRun = %v, want next 03:00 cron occurrence (no retry attempted)", tk.nextRun)
	}
}

func TestRunTaskZeroRegisteredTasksTickIsNoop(t *testing.T) {
	// Boundary: an empty task list must not panic tick().
	s := newTestScheduler(t)
	s.tasks = nil
	s.tick() // must not panic
}

func TestRunTaskIdempotentRepeatedRuns(t *testing.T) {
	// Running the same task twice in a row (e.g. after a crash-recovery
	// restart re-queues it) must be safe: history accumulates, counters
	// increment, no duplicate-row conflicts.
	s := newTestScheduler(t)
	var calls int32
	tk := newRegisteredTask(t, s, "idempotent-task", "local", "@every 15m", func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	now := time.Now()
	s.loadOrInitTaskState(tk, now)

	s.runTask(tk)
	s.runTask(tk)

	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("fn called %d times, want 2", calls)
	}
	_, _, runCount, failCount, _ := schedulerTaskRow(t, s, "idempotent-task")
	if runCount != 2 || failCount != 0 {
		t.Errorf("run_count=%d fail_count=%d, want 2/0", runCount, failCount)
	}
	var histCount int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM scheduler_history WHERE task_id=?`, "idempotent-task").Scan(&histCount); err != nil {
		t.Fatalf("query history: %v", err)
	}
	if histCount != 2 {
		t.Errorf("scheduler_history rows = %d, want 2", histCount)
	}
}

func TestAcquireLockSucceedsWhenUnlocked(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "lock-task", "global", "0 3 * * *", func(context.Context) error { return nil })
	s.loadOrInitTaskState(tk, time.Now())

	if !s.acquireLock(tk) {
		t.Fatal("acquireLock() = false, want true on an unlocked row")
	}
}

func TestAcquireLockFailsWhenHeldByAnotherLiveNode(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "lock-task2", "global", "0 3 * * *", func(context.Context) error { return nil })
	s.loadOrInitTaskState(tk, time.Now())

	_, err := s.store.ServerDB.Exec(`UPDATE scheduler_tasks SET locked_by=?, locked_at=? WHERE id=?`,
		"other-node-123", time.Now().Unix(), tk.id)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	if s.acquireLock(tk) {
		t.Fatal("acquireLock() = true, want false — lock is held by a live node")
	}
}

func TestAcquireLockReclaimsStaleLockAfterCrash(t *testing.T) {
	// Crash-recovery: a lock older than the 5-minute timeout must be
	// reclaimable by another node per AI.md PART 19 "Task Locking".
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "lock-task3", "global", "0 3 * * *", func(context.Context) error { return nil })
	s.loadOrInitTaskState(tk, time.Now())

	staleAt := time.Now().Add(-6 * time.Minute).Unix()
	_, err := s.store.ServerDB.Exec(`UPDATE scheduler_tasks SET locked_by=?, locked_at=? WHERE id=?`,
		"dead-node", staleAt, tk.id)
	if err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	if !s.acquireLock(tk) {
		t.Fatal("acquireLock() = false, want true — a >5m-old lock must be reclaimable")
	}
	var lockedBy string
	if err := s.store.ServerDB.QueryRow(`SELECT locked_by FROM scheduler_tasks WHERE id=?`, tk.id).Scan(&lockedBy); err != nil {
		t.Fatalf("query: %v", err)
	}
	if lockedBy != s.nodeID {
		t.Errorf("locked_by = %q, want this node's id %q", lockedBy, s.nodeID)
	}
}

func TestReleaseLockOnlyReleasesOwnLock(t *testing.T) {
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "lock-task4", "global", "0 3 * * *", func(context.Context) error { return nil })
	s.loadOrInitTaskState(tk, time.Now())

	_, err := s.store.ServerDB.Exec(`UPDATE scheduler_tasks SET locked_by=?, locked_at=? WHERE id=?`,
		"other-node", time.Now().Unix(), tk.id)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	// s owns node ID s.nodeID, not "other-node" — releaseLock must be a no-op.
	s.releaseLock(tk)

	var lockedBy string
	if err := s.store.ServerDB.QueryRow(`SELECT locked_by FROM scheduler_tasks WHERE id=?`, tk.id).Scan(&lockedBy); err != nil {
		t.Fatalf("query: %v", err)
	}
	if lockedBy != "other-node" {
		t.Errorf("locked_by = %q, want unchanged %q", lockedBy, "other-node")
	}
}

func TestRunTaskGlobalSkipsWhenLockHeldElsewhere(t *testing.T) {
	s := newTestScheduler(t)
	var calls int32
	tk := newRegisteredTask(t, s, "global-locked", "global", "0 3 * * *", func(context.Context) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	s.loadOrInitTaskState(tk, time.Now())

	_, err := s.store.ServerDB.Exec(`UPDATE scheduler_tasks SET locked_by=?, locked_at=? WHERE id=?`,
		"other-live-node", time.Now().Unix(), tk.id)
	if err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	s.runTask(tk)

	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("fn was called %d time(s), want 0 — a locked global task must not run", calls)
	}
	// No history row should have been written for a skipped run.
	var histCount int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM scheduler_history WHERE task_id=?`, "global-locked").Scan(&histCount); err != nil {
		t.Fatalf("query history: %v", err)
	}
	if histCount != 0 {
		t.Errorf("scheduler_history rows = %d, want 0 for a lock-skipped run", histCount)
	}
}

// TestConcurrentGlobalTaskLockOnlyOneNodeWins is a real concurrency test:
// two Scheduler instances representing two cluster nodes share the same
// on-disk store and race to acquire the same global task's lock. Per AI.md
// PART 19 "NEVER let more than one cluster node run a Global Task
// simultaneously", exactly one of them must win.
func TestConcurrentGlobalTaskLockOnlyOneNodeWins(t *testing.T) {
	st := newTestStore(t)
	nodeA := New(st, "", "", "", "", "1.0.0", nil, config.SecurityConfig{}, false, config.BackupRetentionConfig{}, testConfig())
	nodeB := New(st, "", "", "", "", "1.0.0", nil, config.SecurityConfig{}, false, config.BackupRetentionConfig{}, testConfig())
	nodeA.nodeID = "node-a"
	nodeB.nodeID = "node-b"

	cs, err := parseCronSchedule("0 3 * * *")
	if err != nil {
		t.Fatalf("parseCronSchedule: %v", err)
	}
	nodeA.store.ServerDB.Exec(`INSERT INTO scheduler_tasks (id, name, task_type, enabled, schedule, next_run, run_count, fail_count) VALUES (?,?,?,?,?,?,0,0)`,
		"shared-global-task", "Shared", "global", 1, "0 3 * * *", time.Now().Unix())

	tkA := &task{id: "shared-global-task", name: "Shared", taskType: "global", schedule: "0 3 * * *", cron: cs, enabled: true, fn: func(context.Context) error { return nil }}
	tkB := &task{id: "shared-global-task", name: "Shared", taskType: "global", schedule: "0 3 * * *", cron: cs, enabled: true, fn: func(context.Context) error { return nil }}

	var wg sync.WaitGroup
	results := make([]bool, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results[0] = nodeA.acquireLock(tkA) }()
	go func() { defer wg.Done(); results[1] = nodeB.acquireLock(tkB) }()
	wg.Wait()

	if results[0] == results[1] {
		t.Fatalf("acquireLock results = %v, %v — exactly one node must win, not zero or both", results[0], results[1])
	}
}

func TestCleanupTokensRemovesOnlyExpired(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, user_type, token_hash, expires_at) VALUES (1,'admin','h1',?)`, now.Add(-time.Hour)); err != nil {
		t.Fatalf("seed expired token: %v", err)
	}
	if _, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, user_type, token_hash, expires_at) VALUES (1,'admin','h2',?)`, now.Add(time.Hour)); err != nil {
		t.Fatalf("seed valid token: %v", err)
	}
	if _, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO api_tokens (user_id, user_type, token_hash) VALUES (1,'admin','h3')`); err != nil {
		t.Fatalf("seed no-expiry token: %v", err)
	}

	if err := s.cleanupTokens(ctx); err != nil {
		t.Fatalf("cleanupTokens: %v", err)
	}

	var remaining int
	if err := s.store.UsersDB.QueryRow(`SELECT COUNT(*) FROM api_tokens`).Scan(&remaining); err != nil {
		t.Fatalf("query: %v", err)
	}
	if remaining != 2 {
		t.Errorf("remaining tokens = %d, want 2 (valid + no-expiry kept, expired removed)", remaining)
	}
}

func TestCleanupSessionsRemovesFromBothTables(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()
	now := time.Now().Unix()

	if _, err := s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO admin_sessions (id, admin_id, expires_at) VALUES ('a1',1,?)`, now-100); err != nil {
		t.Fatalf("seed admin session: %v", err)
	}
	if _, err := s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO admin_sessions (id, admin_id, expires_at) VALUES ('a2',1,?)`, now+3600); err != nil {
		t.Fatalf("seed admin session: %v", err)
	}
	if _, err := s.store.UsersDB.ExecContext(ctx,
		`INSERT INTO user_sessions (id, user_id, expires_at) VALUES ('u1',1,?)`, now-100); err != nil {
		t.Fatalf("seed user session: %v", err)
	}

	if err := s.cleanupSessions(ctx); err != nil {
		t.Fatalf("cleanupSessions: %v", err)
	}

	var adminCount, userCount int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM admin_sessions`).Scan(&adminCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if err := s.store.UsersDB.QueryRow(`SELECT COUNT(*) FROM user_sessions`).Scan(&userCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if adminCount != 1 {
		t.Errorf("admin_sessions remaining = %d, want 1", adminCount)
	}
	if userCount != 0 {
		t.Errorf("user_sessions remaining = %d, want 0", userCount)
	}
}

func TestExpireURLsRemovesOnlyExpired(t *testing.T) {
	s := newTestScheduler(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if _, err := s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO urls (short_code, long_url, expires_at) VALUES ('exp','https://a.example',?)`, now.Add(-time.Hour)); err != nil {
		t.Fatalf("seed expired url: %v", err)
	}
	if _, err := s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO urls (short_code, long_url, expires_at) VALUES ('ok','https://b.example',?)`, now.Add(time.Hour)); err != nil {
		t.Fatalf("seed valid url: %v", err)
	}
	if _, err := s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO urls (short_code, long_url) VALUES ('perm','https://c.example')`); err != nil {
		t.Fatalf("seed permanent url: %v", err)
	}

	if err := s.expireURLs(ctx); err != nil {
		t.Fatalf("expireURLs: %v", err)
	}

	var remaining int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM urls`).Scan(&remaining); err != nil {
		t.Fatalf("query: %v", err)
	}
	if remaining != 2 {
		t.Errorf("remaining urls = %d, want 2", remaining)
	}
}

func TestSelfHealthCheckHealthyStore(t *testing.T) {
	s := newTestScheduler(t)
	if err := s.selfHealthCheck(context.Background()); err != nil {
		t.Errorf("selfHealthCheck() = %v, want nil for a live store", err)
	}
}

func TestSelfHealthCheckClosedStoreReturnsError(t *testing.T) {
	s := newTestScheduler(t)
	_ = s.store.Close()
	if err := s.selfHealthCheck(context.Background()); err == nil {
		t.Error("selfHealthCheck() = nil, want an error for a closed store")
	}
}

func TestRenewSSLIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	if err := s.renewSSL(context.Background()); err != nil {
		t.Errorf("renewSSL() = %v, want nil (no-op)", err)
	}
}

func TestClusterHeartbeatIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	if err := s.clusterHeartbeat(context.Background()); err != nil {
		t.Errorf("clusterHeartbeat() = %v, want nil (no-op — no cluster peer tracking yet)", err)
	}
}

func TestUpdateGeoIPNilServiceIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	s.geoip = nil
	if err := s.updateGeoIP(context.Background()); err != nil {
		t.Errorf("updateGeoIP() = %v, want nil when geoip service is unset", err)
	}
}

func TestUpdateBlocklistNoSourcesIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	s.blocklistSources = nil
	if err := s.updateBlocklist(context.Background()); err != nil {
		t.Errorf("updateBlocklist() = %v, want nil with no sources configured", err)
	}
}

func TestUpdateCVENoSourcesIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	s.cveSources = nil
	if err := s.updateCVE(context.Background()); err != nil {
		t.Errorf("updateCVE() = %v, want nil with no sources configured", err)
	}
}

func TestRunDailyBackupNoBackupDirIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	s.backupDir = ""
	if err := s.runDailyBackup(context.Background()); err != nil {
		t.Errorf("runDailyBackup() = %v, want nil when backupDir is empty", err)
	}
}

func TestRunDailyBackupComplianceRequiredWithoutPasswordSkips(t *testing.T) {
	// AI.md PART 22: scheduled backups must never run unencrypted under
	// compliance mode — the run must be skipped (not failed) when no
	// backup password is available.
	t.Setenv("CASLINK_BACKUP_PASSWORD", "")
	s := newTestScheduler(t)
	s.backupDir = t.TempDir()
	s.complianceRequired = true

	if err := s.runDailyBackup(context.Background()); err != nil {
		t.Errorf("runDailyBackup() = %v, want nil (skipped, not failed)", err)
	}
}

type fakeTorChecker struct {
	running     bool
	restartErr  error
	restartCall int32
}

func (f *fakeTorChecker) IsRunning() bool { return f.running }
func (f *fakeTorChecker) Restart() error {
	atomic.AddInt32(&f.restartCall, 1)
	return f.restartErr
}

func TestCheckTorHealthNilCheckerIsANoop(t *testing.T) {
	s := newTestScheduler(t)
	if err := s.checkTorHealth(context.Background()); err != nil {
		t.Errorf("checkTorHealth() = %v, want nil when no checker is registered", err)
	}
}

func TestCheckTorHealthRunningSkipsRestart(t *testing.T) {
	s := newTestScheduler(t)
	fc := &fakeTorChecker{running: true}
	s.SetTorChecker(fc)
	if err := s.checkTorHealth(context.Background()); err != nil {
		t.Errorf("checkTorHealth() = %v, want nil", err)
	}
	if atomic.LoadInt32(&fc.restartCall) != 0 {
		t.Error("Restart() was called even though Tor is running")
	}
}

func TestCheckTorHealthDownRestartsSuccessfully(t *testing.T) {
	s := newTestScheduler(t)
	fc := &fakeTorChecker{running: false}
	s.SetTorChecker(fc)
	if err := s.checkTorHealth(context.Background()); err != nil {
		t.Errorf("checkTorHealth() = %v, want nil after a successful restart", err)
	}
	if atomic.LoadInt32(&fc.restartCall) != 1 {
		t.Errorf("Restart() called %d times, want 1", fc.restartCall)
	}
}

func TestCheckTorHealthDownRestartFails(t *testing.T) {
	s := newTestScheduler(t)
	fc := &fakeTorChecker{running: false, restartErr: errors.New("exec failed")}
	s.SetTorChecker(fc)
	if err := s.checkTorHealth(context.Background()); err == nil {
		t.Error("checkTorHealth() = nil, want an error when restart fails")
	}
}

func TestDownloadToFileSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "payload")
	}))
	defer srv.Close()

	s := newTestScheduler(t)
	dest := t.TempDir() + "/out.txt"
	if err := s.downloadToFile(srv.URL, dest); err != nil {
		t.Fatalf("downloadToFile: %v", err)
	}
}

func TestDownloadToFileNon200ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := newTestScheduler(t)
	dest := t.TempDir() + "/out.txt"
	if err := s.downloadToFile(srv.URL, dest); err == nil {
		t.Fatal("downloadToFile() = nil, want an error on HTTP 404")
	}
}

func TestUpdateBlocklistDownloadsEnabledSourcesOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "1.2.3.4")
	}))
	defer srv.Close()

	s := newTestScheduler(t)
	s.dataDir = t.TempDir()
	s.blocklistSources = []config.BlocklistSource{
		{Name: "enabled-src", URL: srv.URL, Enabled: true},
		{Name: "disabled-src", URL: "http://127.0.0.1:1/unreachable", Enabled: false},
	}

	if err := s.updateBlocklist(context.Background()); err != nil {
		t.Fatalf("updateBlocklist: %v", err)
	}
	if _, err := os.Stat(s.dataDir + "/security/blocklists/enabled-src.txt"); err != nil {
		t.Errorf("expected downloaded file for the enabled source: %v", err)
	}
	if _, err := os.Stat(s.dataDir + "/security/blocklists/disabled-src.txt"); err == nil {
		t.Error("disabled source must not be downloaded")
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := []struct{ in, want string }{
		{"simple-name", "simple-name"},
		{"has spaces/slashes", "has_spaces_slashes"},
		{"", "source"},
		{"UPPER_lower123", "UPPER_lower123"},
	}
	for _, c := range cases {
		if got := sanitizeFilename(c.in); got != c.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNullIfEmpty(t *testing.T) {
	if got := nullIfEmpty(""); got != nil {
		t.Errorf("nullIfEmpty(\"\") = %v, want nil", got)
	}
	if got := nullIfEmpty("x"); got != "x" {
		t.Errorf("nullIfEmpty(\"x\") = %v, want \"x\"", got)
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) != 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) != 0")
	}
}

func TestStartStopLifecycle(t *testing.T) {
	// End-to-end: Start() must initialize every task's DB row and Stop()
	// must release this node's locks and return promptly (loop's ticker is
	// 15s, well inside the 30s force-release timeout, but stopCh closes the
	// loop immediately so this test does not wait on the ticker at all).
	s := newTestScheduler(t)
	s.Start()
	s.Stop()

	var count int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM scheduler_tasks`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 15 {
		t.Errorf("scheduler_tasks rows after Start = %d, want 15", count)
	}
	var locked int
	if err := s.store.ServerDB.QueryRow(`SELECT COUNT(*) FROM scheduler_tasks WHERE locked_by IS NOT NULL`).Scan(&locked); err != nil {
		t.Fatalf("query: %v", err)
	}
	if locked != 0 {
		t.Errorf("locked tasks after Stop = %d, want 0", locked)
	}
}

func TestSetAuditServiceAndMetricsAreOptional(t *testing.T) {
	// Both setters must be safe to skip entirely (nil auditSvc/metrics), and
	// runTask must not panic when they're unset.
	s := newTestScheduler(t)
	tk := newRegisteredTask(t, s, "no-observability-task", "local", "@every 15m", func(context.Context) error { return nil })
	s.loadOrInitTaskState(tk, time.Now())
	s.runTask(tk) // must not panic with auditSvc == nil and metrics == nil
}
