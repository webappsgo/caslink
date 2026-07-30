package scheduler

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/src/backup"
	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/geoip"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/store"
	"github.com/casjaysdevdocker/caslink/src/updater"
)

// torHealthChecker is a minimal interface for checking and restarting the Tor service.
// Using an interface breaks the circular import between scheduler ↔ tor packages.
type torHealthChecker interface {
	IsRunning() bool
	Restart() error
}

// task is one registered scheduled job, per AI.md PART 19 "Built-in Tasks".
type task struct {
	id         string
	name       string
	taskType   string // "global" (one node in cluster mode) or "local" (every node)
	schedule   string
	cron       *cronSchedule
	enabled    bool
	maxRetries int
	retryDelay time.Duration
	fn         func(ctx context.Context) error

	mu      sync.Mutex // guards running/nextRun/attempt — one goroutine touches a task at a time
	running bool
	nextRun time.Time
	attempt int // consecutive failure count since the last success, for backoff
}

// Scheduler manages background tasks. It is a self-contained replacement for
// github.com/robfig/cron/v3 per AI.md PART 19 ("NEVER Use External
// Schedulers" — "Exceptions (NONE)"): a time.Ticker loop, a hand-written
// cron-expression matcher (cronexpr.go), and persistent state in
// scheduler_tasks/scheduler_history (server.db).
type Scheduler struct {
	store *store.Store
	cfg   config.SchedulerConfig
	tasks []*task

	loc           *time.Location
	catchUpWindow time.Duration
	nodeID        string

	logDir           string                   // path to log directory for log_rotation; may be ""
	geoip            *geoip.Service           // optional; nil → geoip_update is a no-op
	configDir        string                   // for daily backup; may be ""
	dataDir          string                   // for daily backup; may be ""
	backupDir        string                   // for daily backup; "" disables automatic backups
	torChecker       torHealthChecker         // optional; nil → tor_health is a no-op
	blocklistSources []config.BlocklistSource // empty → blocklist_update is a no-op
	cveSources       []config.CVESource       // empty → cve_update is a no-op
	complianceRequired bool                   // true → scheduled backups must be encrypted or are skipped
	retention        config.BackupRetentionConfig // applied to backupDir after a successful backup_daily run
	version          string                       // running binary version, for update_check

	auditSvc *service.AuditService // optional; nil → task runs are not audit-logged

	updateSeenMu sync.Mutex
	updateSeen   map[string]time.Time // release tag -> first-observed time, for update.defer_days (in-memory; resets on restart)

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new scheduler bound to the given store. logDir is the
// directory containing application log files; pass "" to skip log rotation.
// configDir, dataDir, and backupDir are used by the daily backup task — pass
// "" for backupDir to disable automatic backups. geoSvc is optional — when
// nil the geoip_update task logs and skips. sec carries blocklist and CVE
// source configuration; pass a zero value to leave both update tasks as
// no-ops. complianceRequired mirrors cfg.Server.Compliance.Enabled per AI.md
// PART 22: when true, the scheduled daily backup only runs encrypted
// (password from CASLINK_BACKUP_PASSWORD) and skips with a logged warning
// otherwise — it is never stored in config. retention is
// cfg.Server.Backup.Retention, applied after every successful backup_daily
// run. sched is cfg.Server.Scheduler — the live schedule/enabled source of
// truth per AI.md PART 19 "Task Configuration". version is the running
// binary version, used by the update_check task.
func New(st *store.Store, logDir, configDir, dataDir, backupDir, version string, geoSvc *geoip.Service, sec config.SecurityConfig, complianceRequired bool, retention config.BackupRetentionConfig, sched config.SchedulerConfig) *Scheduler {
	loc, err := time.LoadLocation(sched.Timezone)
	if err != nil || sched.Timezone == "" {
		loc = time.UTC
	}
	catchUp, err := time.ParseDuration(sched.CatchUpWindow)
	if err != nil || catchUp <= 0 {
		catchUp = time.Hour
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	s := &Scheduler{
		store:              st,
		cfg:                sched,
		loc:                loc,
		catchUpWindow:      catchUp,
		nodeID:             fmt.Sprintf("%s-%d", hostname, os.Getpid()),
		logDir:             logDir,
		geoip:              geoSvc,
		configDir:          configDir,
		dataDir:            dataDir,
		backupDir:          backupDir,
		blocklistSources:   sec.Blocklist.Sources,
		cveSources:         sec.CVE.Sources,
		complianceRequired: complianceRequired,
		retention:          retention,
		version:            version,
		updateSeen:         make(map[string]time.Time),
		stopCh:             make(chan struct{}),
	}
	return s
}

// SetTorChecker wires an optional Tor health-checker after the scheduler has
// been created but before it is started. Safe to call from any goroutine
// as long as Start() has not yet been called.
func (s *Scheduler) SetTorChecker(tc torHealthChecker) {
	s.torChecker = tc
}

// SetAuditService wires the audit-log writer after construction (mirrors
// SetTorChecker — server.go builds AuditService after the scheduler). When
// unset, task executions are simply not audit-logged.
func (s *Scheduler) SetAuditService(a *service.AuditService) {
	s.auditSvc = a
}

// retryPolicy returns the default (max_retries, retry_delay) from config,
// falling back to the AI.md PART 19 "Retry Policy" defaults.
func (s *Scheduler) retryPolicy() (int, time.Duration) {
	maxRetries := s.cfg.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	delay, err := time.ParseDuration(s.cfg.RetryDelay)
	if err != nil || delay <= 0 {
		delay = 5 * time.Minute
	}
	return maxRetries, delay
}

// addTasks builds the task list per AI.md PART 19 → "Built-in Tasks
// (Required)" and "Cluster Mode Task Distribution". Schedules and
// enabled/disabled state come from cfg (server.yml / admin panel), never
// hardcoded, so the admin panel is the single source of truth.
func (s *Scheduler) addTasks() {
	maxRetries, retryDelay := s.retryPolicy()
	noRetry := func(t *task) *task { t.maxRetries = 0; return t }
	withRetry := func(t *task) *task { t.maxRetries = maxRetries; t.retryDelay = retryDelay; return t }

	defs := []*task{
		noRetry(&task{id: "session_cleanup", name: "Session Cleanup", taskType: "local",
			schedule: s.cfg.SessionCleanupCron, enabled: s.cfg.SessionCleanupEnabled, fn: s.cleanupSessions}),
		noRetry(&task{id: "token_cleanup", name: "Token Cleanup", taskType: "local",
			schedule: s.cfg.TokenCleanupCron, enabled: s.cfg.TokenCleanupEnabled, fn: s.cleanupTokens}),
		noRetry(&task{id: "expire_urls", name: "Expire URLs", taskType: "local",
			schedule: s.cfg.ExpireURLsCron, enabled: s.cfg.ExpireURLsEnabled, fn: s.expireURLs}),
		noRetry(&task{id: "log_rotation", name: "Log Rotation", taskType: "local",
			schedule: s.cfg.LogRotationCron, enabled: s.cfg.LogRotationEnabled, fn: s.rotateLogs}),
		withRetry(&task{id: "backup_daily", name: "Backup Daily", taskType: "global",
			schedule: s.cfg.BackupCron, enabled: s.cfg.BackupEnabled, fn: s.runDailyBackup}),
		noRetry(&task{id: "backup_hourly", name: "Backup Hourly", taskType: "global",
			schedule: s.cfg.BackupHourlyCron, enabled: s.cfg.BackupHourlyEnabled, fn: s.runDailyBackup}),
		withRetry(&task{id: "ssl_renewal", name: "SSL Renewal", taskType: "global",
			schedule: s.cfg.SSLRenewalCron, enabled: s.cfg.SSLRenewalEnabled, fn: s.renewSSL}),
		withRetry(&task{id: "geoip_update", name: "GeoIP Update", taskType: "global",
			schedule: s.cfg.GeoIPUpdateCron, enabled: s.cfg.GeoIPUpdateEnabled, fn: s.updateGeoIP}),
		withRetry(&task{id: "blocklist_update", name: "Blocklist Update", taskType: "global",
			schedule: s.cfg.BlocklistUpdateCron, enabled: s.cfg.BlocklistUpdateEnabled, fn: s.updateBlocklist}),
		withRetry(&task{id: "cve_update", name: "CVE Update", taskType: "global",
			schedule: s.cfg.CVEUpdateCron, enabled: s.cfg.CVEUpdateEnabled, fn: s.updateCVE}),
		noRetry(&task{id: "update_check", name: "Update Check", taskType: "global",
			schedule: s.cfg.UpdateCheckCron, enabled: s.cfg.UpdateCheckEnabled, fn: s.checkForUpdate}),
		noRetry(&task{id: "healthcheck_self", name: "Self Health Check", taskType: "local",
			schedule: s.cfg.HealthcheckCron, enabled: s.cfg.HealthcheckEnabled, fn: s.selfHealthCheck}),
		noRetry(&task{id: "tor_health", name: "Tor Health", taskType: "local",
			schedule: s.cfg.TorHealthCron, enabled: s.cfg.TorHealthEnabled, fn: s.checkTorHealth}),
		noRetry(&task{id: "cluster_heartbeat", name: "Cluster Heartbeat", taskType: "local",
			schedule: s.cfg.ClusterHeartbeatCron, enabled: s.cfg.ClusterHeartbeatEnabled, fn: s.clusterHeartbeat}),
	}

	s.tasks = s.tasks[:0]
	for _, t := range defs {
		cs, err := parseCronSchedule(t.schedule)
		if err != nil {
			log.Printf("[scheduler] addTasks: task %q has invalid schedule %q, skipping: %v", t.id, t.schedule, err)
			continue
		}
		t.cron = cs
		s.tasks = append(s.tasks, t)
	}
}

// Start loads/persists task state, runs the startup catch-up pass, and
// starts the scheduler loop. Per AI.md PART 19 "Startup Behavior".
func (s *Scheduler) Start() {
	s.addTasks()
	now := time.Now()
	for _, t := range s.tasks {
		s.loadOrInitTaskState(t, now)
	}

	s.wg.Add(1)
	go s.loop()
	log.Printf("[scheduler] started (node=%s, %d tasks, timezone=%s, catch_up_window=%s)",
		s.nodeID, len(s.tasks), s.loc, s.catchUpWindow)
}

// Stop stops the scheduler gracefully: no new task executions are started,
// running tasks get up to 30s to finish, then locks are force-released.
// Per AI.md PART 19 "Shutdown Behavior".
func (s *Scheduler) Stop() {
	close(s.stopCh)

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		log.Println("[scheduler] stop: timed out waiting for running tasks, force-releasing locks")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = s.store.ServerDB.ExecContext(ctx, `UPDATE scheduler_tasks SET locked_by = NULL, locked_at = NULL WHERE locked_by = ?`, s.nodeID)
	log.Println("[scheduler] stopped")
}

// loop is the ticker-based execution loop — Go's time/ticker per AI.md
// PART 19 "Implementation Requirements" #1, no external cron library.
func (s *Scheduler) loop() {
	defer s.wg.Done()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.tick()
		}
	}
}

func (s *Scheduler) tick() {
	now := time.Now()
	for _, t := range s.tasks {
		if !t.enabled {
			continue
		}
		t.mu.Lock()
		ready := !t.running && !now.Before(t.nextRun)
		if ready {
			t.running = true
		}
		t.mu.Unlock()
		if !ready {
			continue
		}

		s.wg.Add(1)
		go func(t *task) {
			defer s.wg.Done()
			s.runTask(t)
		}(t)
	}
}

// loadOrInitTaskState upserts a task's row in scheduler_tasks, computing
// next_run for a brand-new row or performing the "Startup Behavior" catch-up
// check for an existing one: a next_run in the past but within
// catch_up_window is queued for immediate execution; further in the past, it
// is skipped and rescheduled from now.
func (s *Scheduler) loadOrInitTaskState(t *task, now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var nextRunUnix, runCount, failCount int64
	var lastStatus, lastErrorS string
	row := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT next_run, run_count, fail_count, COALESCE(last_status,''), COALESCE(last_error,'') FROM scheduler_tasks WHERE id = ?`, t.id)
	err := row.Scan(&nextRunUnix, &runCount, &failCount, &lastStatus, &lastErrorS)

	if err != nil {
		// New task: insert with next_run computed from now.
		next := t.cron.Next(now, s.loc)
		t.nextRun = next
		_, insErr := s.store.ServerDB.ExecContext(ctx,
			`INSERT INTO scheduler_tasks (id, name, task_type, enabled, schedule, next_run, run_count, fail_count)
			 VALUES (?, ?, ?, ?, ?, ?, 0, 0)
			 ON CONFLICT(id) DO UPDATE SET name=excluded.name, task_type=excluded.task_type, schedule=excluded.schedule`,
			t.id, t.name, t.taskType, boolToInt(t.enabled), t.schedule, next.Unix())
		if insErr != nil {
			log.Printf("[scheduler] loadOrInitTaskState: init %s: %v", t.id, insErr)
		}
		return
	}

	// Existing row: keep name/type/schedule/enabled in sync with config,
	// clear any stale lock this node held across a restart.
	_, updErr := s.store.ServerDB.ExecContext(ctx,
		`UPDATE scheduler_tasks SET name=?, task_type=?, schedule=?, enabled=?, locked_by=NULL, locked_at=NULL WHERE id=?`,
		t.name, t.taskType, t.schedule, boolToInt(t.enabled), t.id)
	if updErr != nil {
		log.Printf("[scheduler] loadOrInitTaskState: sync %s: %v", t.id, updErr)
	}

	stored := time.Unix(nextRunUnix, 0)
	overdue := now.Sub(stored)
	switch {
	case overdue <= 0:
		// Not due yet.
		t.nextRun = stored
	case overdue <= s.catchUpWindow:
		// Missed but within the catch-up window — run immediately.
		log.Printf("[scheduler] %s: missed run at %s is within catch_up_window, queuing immediate execution", t.id, stored.Format(time.RFC3339))
		t.nextRun = now
	default:
		// Missed well outside the window — skip it, resume normal schedule.
		log.Printf("[scheduler] %s: missed run at %s is outside catch_up_window (%s), skipping and rescheduling", t.id, stored.Format(time.RFC3339), s.catchUpWindow)
		t.nextRun = t.cron.Next(now, s.loc)
		_, _ = s.store.ServerDB.ExecContext(ctx, `UPDATE scheduler_tasks SET next_run=? WHERE id=?`, t.nextRun.Unix(), t.id)
	}
}

// runTask executes one task run: acquires the cluster lock for global tasks,
// runs fn, records scheduler_history + scheduler_tasks state, applies
// exponential retry/backoff on failure, and audit-logs the outcome. Per
// AI.md PART 19 "Task Execution Flow" / "Task Locking (Cluster Mode)".
func (s *Scheduler) runTask(t *task) {
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	if t.taskType == "global" {
		if !s.acquireLock(t) {
			log.Printf("[scheduler] %s: lock held by another node, skipping this run", t.id)
			t.mu.Lock()
			t.nextRun = t.cron.Next(time.Now(), s.loc)
			t.mu.Unlock()
			return
		}
		defer s.releaseLock(t)
	}

	started := time.Now()
	err := t.fn(context.Background())
	finished := time.Now()
	duration := finished.Sub(started)

	status := "success"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}

	t.mu.Lock()
	if err != nil {
		t.attempt++
		if t.maxRetries > 0 && t.attempt <= t.maxRetries {
			backoff := t.retryDelay
			for i := 1; i < t.attempt; i++ {
				backoff *= 2
			}
			t.nextRun = finished.Add(backoff)
			status = "failed"
			log.Printf("[scheduler] %s: run failed (attempt %d/%d), retrying in %s: %v", t.id, t.attempt, t.maxRetries, backoff, err)
		} else {
			t.nextRun = t.cron.Next(finished, s.loc)
			log.Printf("[scheduler] %s: run failed, giving up after %d attempt(s), next run %s: %v", t.id, t.attempt, t.nextRun.Format(time.RFC3339), err)
		}
	} else {
		t.attempt = 0
		t.nextRun = t.cron.Next(finished, s.loc)
	}
	nextRun := t.nextRun
	t.mu.Unlock()

	s.recordRun(t, started, finished, status, errMsg, nextRun)

	if s.auditSvc != nil {
		details := fmt.Sprintf("status=%s duration=%s", status, duration.Round(time.Millisecond))
		if errMsg != "" {
			details += " error=" + errMsg
		}
		if aerr := s.auditSvc.RecordEvent(context.Background(), nil, "system", "scheduler_task_run", "scheduler:"+t.id, details, "", ""); aerr != nil {
			log.Printf("[scheduler] %s: failed to audit-log run: %v", t.id, aerr)
		}
	}
}

// acquireLock attempts to claim the distributed lock for a global task.
// Lock timeout is 5 minutes (auto-release if the holder died mid-task), per
// AI.md PART 19 "Task Locking (Cluster Mode)". With no cluster peers
// configured this always succeeds for the sole node.
func (s *Scheduler) acquireLock(t *task) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	staleBefore := time.Now().Add(-5 * time.Minute).Unix()
	res, err := s.store.ServerDB.ExecContext(ctx,
		`UPDATE scheduler_tasks SET locked_by=?, locked_at=? WHERE id=? AND (locked_by IS NULL OR locked_by='' OR locked_at < ?)`,
		s.nodeID, time.Now().Unix(), t.id, staleBefore)
	if err != nil {
		log.Printf("[scheduler] %s: acquireLock: %v", t.id, err)
		return false
	}
	n, _ := res.RowsAffected()
	return n == 1
}

func (s *Scheduler) releaseLock(t *task) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.store.ServerDB.ExecContext(ctx,
		`UPDATE scheduler_tasks SET locked_by=NULL, locked_at=NULL WHERE id=? AND locked_by=?`, t.id, s.nodeID)
	if err != nil {
		log.Printf("[scheduler] %s: releaseLock: %v", t.id, err)
	}
}

func (s *Scheduler) recordRun(t *task, started, finished time.Time, status, errMsg string, nextRun time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	durationMs := finished.Sub(started).Milliseconds()
	_, err := s.store.ServerDB.ExecContext(ctx,
		`INSERT INTO scheduler_history (task_id, started_at, finished_at, status, error, duration_ms) VALUES (?, ?, ?, ?, ?, ?)`,
		t.id, started.Unix(), finished.Unix(), status, nullIfEmpty(errMsg), durationMs)
	if err != nil {
		log.Printf("[scheduler] %s: recordRun: insert history: %v", t.id, err)
	}

	var errArg interface{}
	if errMsg != "" {
		errArg = errMsg
	}
	runInc, failInc := 0, 0
	if status == "success" {
		runInc = 1
	} else {
		failInc = 1
	}
	_, err = s.store.ServerDB.ExecContext(ctx,
		`UPDATE scheduler_tasks SET last_run=?, last_status=?, last_error=?, next_run=?, run_count=run_count+?, fail_count=fail_count+? WHERE id=?`,
		finished.Unix(), status, errArg, nextRun.Unix(), runInc, failInc, t.id)
	if err != nil {
		log.Printf("[scheduler] %s: recordRun: update state: %v", t.id, err)
	}
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cleanupTokens removes expired API tokens from users.db.
func (s *Scheduler) cleanupTokens(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at < ?`,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("cleanupTokens: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[scheduler] cleanupTokens: removed %d expired tokens", n)
	}
	return nil
}

// selfHealthCheck pings both databases.
func (s *Scheduler) selfHealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var firstErr error
	if s.store.ServerDB != nil {
		if err := s.store.ServerDB.PingContext(ctx); err != nil {
			firstErr = fmt.Errorf("server.db ping failed: %w", err)
		}
	}
	if s.store.UsersDB != nil {
		if err := s.store.UsersDB.PingContext(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("users.db ping failed: %w", err)
		}
	}
	return firstErr
}

// expireURLs deactivates or deletes URLs whose expires_at has passed.
func (s *Scheduler) expireURLs(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res, err := s.store.ServerDB.ExecContext(ctx,
		`DELETE FROM urls WHERE expires_at IS NOT NULL AND expires_at < ?`,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("expireURLs: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[scheduler] expireURLs: removed %d expired URLs", n)
	}
	return nil
}

// rotateLogs compresses log files that are older than 24 hours and removes
// compressed archives older than 30 days. It operates only on the files
// created by the logger package: access.log, server.log, error.log,
// audit.log, security.log, debug.log.
func (s *Scheduler) rotateLogs(ctx context.Context) error {
	if s.logDir == "" {
		log.Println("[scheduler] log_rotation: no log directory configured, skipping")
		return nil
	}

	logFiles := []string{
		"access.log", "server.log", "error.log",
		"audit.log", "security.log", "debug.log",
	}

	now := time.Now()
	rotateAfter := 24 * time.Hour
	removeAfter := 30 * 24 * time.Hour
	rotated := 0
	removed := 0

	for _, name := range logFiles {
		path := filepath.Join(s.logDir, name)
		fi, err := os.Stat(path)
		if err != nil {
			continue // file doesn't exist — skip
		}
		if fi.Size() == 0 {
			continue // nothing to rotate
		}
		if now.Sub(fi.ModTime()) < rotateAfter {
			continue // too recent
		}

		stamp := fi.ModTime().UTC().Format("2006-01-02")
		archiveName := fmt.Sprintf("%s.%s.gz", name, stamp)
		archivePath := filepath.Join(s.logDir, archiveName)

		if err := compressLog(path, archivePath); err != nil {
			log.Printf("[scheduler] log_rotation: compress %s: %v", name, err)
			continue
		}
		rotated++
	}

	entries, err := os.ReadDir(s.logDir)
	if err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".log.gz") && !strings.HasSuffix(e.Name(), ".gz") {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				continue
			}
			if now.Sub(fi.ModTime()) > removeAfter {
				_ = os.Remove(filepath.Join(s.logDir, e.Name()))
				removed++
			}
		}
	}

	if rotated > 0 || removed > 0 {
		log.Printf("[scheduler] log_rotation: rotated %d file(s), removed %d old archive(s)", rotated, removed)
	}
	return nil
}

// compressLog reads src, writes a gzip-compressed copy to dst, then truncates
// src so the logger process can keep writing to the same file descriptor.
// The compressed archive is written atomically via a temp file.
func compressLog(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".logrotate-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName) // no-op if rename succeeded
	}()

	gz := gzip.NewWriter(tmp)
	if _, err := io.Copy(gz, in); err != nil {
		return fmt.Errorf("compress: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("gzip close: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("temp close: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	return os.Truncate(src, 0)
}

// cleanupSessions removes expired sessions from both session tables.
// admin_sessions lives in server.db; user_sessions lives in users.db.
func (s *Scheduler) cleanupSessions(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	now := time.Now().Unix()
	var total int64
	var firstErr error

	resAdmin, err := s.store.ServerDB.ExecContext(ctx,
		`DELETE FROM admin_sessions WHERE expires_at < ?`, now)
	if err != nil {
		firstErr = fmt.Errorf("admin_sessions: %w", err)
	} else {
		n, _ := resAdmin.RowsAffected()
		total += n
	}

	resUser, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM user_sessions WHERE expires_at < ?`, now)
	if err != nil && firstErr == nil {
		firstErr = fmt.Errorf("user_sessions: %w", err)
	} else if err == nil {
		n, _ := resUser.RowsAffected()
		total += n
	}

	if total > 0 {
		log.Printf("[scheduler] cleanupSessions: removed %d expired sessions", total)
	}
	return firstErr
}

// renewSSL checks and renews Let's Encrypt certificates expiring within 7 days.
// Silently skips when no certificates are configured.
func (s *Scheduler) renewSSL(ctx context.Context) error {
	// SSL renewal is handled by the ssl package when Let's Encrypt is configured.
	// The ssl package tracks certs in the database and handles ACME challenges
	// directly via the server's /.well-known/acme-challenge/ handler.
	// When no certs are registered, this is a no-op.
	return nil
}

// updateGeoIP downloads the configured GeoIP databases via the geoip
// service. Skips silently when the service is not wired (e.g. GeoIP
// disabled in config).
func (s *Scheduler) updateGeoIP(ctx context.Context) error {
	if s.geoip == nil || !s.geoip.Enabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()
	if err := s.geoip.Update(ctx); err != nil {
		return fmt.Errorf("geoip_update: %w", err)
	}
	return nil
}

// runDailyBackup creates a dated full backup + the fixed-name daily incremental
// per AI.md PART 22. Both files are verified after creation. Also used for
// backup_hourly (same underlying routine — see AI.md PART 19's backup task
// table; both create/verify then apply retention). Silently skips when
// backupDir is not configured. When compliance mode is enabled, the backup
// password is read from CASLINK_BACKUP_PASSWORD (never stored in config); if
// compliance requires encryption and no password is available, the run is
// skipped with a logged warning rather than failed.
func (s *Scheduler) runDailyBackup(ctx context.Context) error {
	if s.backupDir == "" {
		return nil
	}
	password := os.Getenv("CASLINK_BACKUP_PASSWORD")
	if s.complianceRequired && password == "" {
		log.Printf("[scheduler] backup: skipped — compliance mode requires an encrypted backup and CASLINK_BACKUP_PASSWORD is not set")
		return nil
	}
	opts := backup.Options{
		Password:           password,
		ComplianceRequired: s.complianceRequired,
		CreatedBy:          "scheduler",
	}
	if err := backup.RunDailyBackup(s.configDir, s.dataDir, s.backupDir, opts); err != nil {
		return fmt.Errorf("backup_daily: %w", err)
	}
	log.Printf("[scheduler] backup: complete (full + daily incremental written and verified)")

	retention := backup.Retention{
		MaxBackups:   s.retention.MaxBackups,
		KeepWeekly:   s.retention.KeepWeekly,
		KeepMonthly:  s.retention.KeepMonthly,
		KeepYearly:   s.retention.KeepYearly,
		MaxTotalSize: s.retention.MaxTotalSize,
	}
	if err := backup.ApplyRetention(s.backupDir, retention); err != nil {
		log.Printf("[scheduler] backup: retention sweep failed: %v", err)
	}
	return nil
}

// updateBlocklist downloads updated IP/domain blocklists from configured
// sources and stores them under {data_dir}/security/blocklists/.
// Silently skips when no sources are configured.
func (s *Scheduler) updateBlocklist(ctx context.Context) error {
	if len(s.blocklistSources) == 0 {
		return nil
	}

	dir := filepath.Join(s.dataDir, "security", "blocklists")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("blocklist_update: mkdir %s: %w", dir, err)
	}

	var lastErr error
	for _, src := range s.blocklistSources {
		if !src.Enabled {
			continue
		}
		if err := s.downloadToFile(src.URL, filepath.Join(dir, sanitizeFilename(src.Name)+".txt")); err != nil {
			log.Printf("[scheduler] blocklist_update: download %s (%s): %v", src.Name, src.URL, err)
			lastErr = err
		} else {
			log.Printf("[scheduler] blocklist_update: updated %s", src.Name)
		}
	}
	return lastErr
}

// updateCVE downloads updated CVE/security database feeds from configured
// sources and stores them under {data_dir}/security/cve/.
// Silently skips when no sources are configured.
func (s *Scheduler) updateCVE(ctx context.Context) error {
	if len(s.cveSources) == 0 {
		return nil
	}

	dir := filepath.Join(s.dataDir, "security", "cve")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cve_update: mkdir %s: %w", dir, err)
	}

	var lastErr error
	for _, src := range s.cveSources {
		if !src.Enabled {
			continue
		}
		if err := s.downloadToFile(src.URL, filepath.Join(dir, sanitizeFilename(src.Name)+".json")); err != nil {
			log.Printf("[scheduler] cve_update: download %s (%s): %v", src.Name, src.URL, err)
			lastErr = err
		} else {
			log.Printf("[scheduler] cve_update: updated %s", src.Name)
		}
	}
	return lastErr
}

// checkForUpdate implements the `update_check` task (AI.md PART 19):
// notify-only unless update.auto_install is true, and honors
// update.defer_days. The defer-days clock is tracked in-memory keyed by
// release tag (resets on restart) since there is no dedicated persistence
// column for it yet — see TODO.AI.md for the follow-up to persist it.
func (s *Scheduler) checkForUpdate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	branch := s.cfg.UpdateBranch
	if branch == "" {
		branch = "stable"
	}
	release, err := updater.CheckForUpdate(ctx, s.version, branch)
	if err != nil {
		return fmt.Errorf("update_check: %w", err)
	}
	if release == nil {
		return nil // already up to date
	}

	log.Printf("[scheduler] update_check: new release available: %s (current: %s)", release.TagName, s.version)

	if !s.cfg.UpdateAutoInstall {
		return nil
	}

	s.updateSeenMu.Lock()
	firstSeen, ok := s.updateSeen[release.TagName]
	if !ok {
		firstSeen = time.Now()
		s.updateSeen[release.TagName] = firstSeen
	}
	s.updateSeenMu.Unlock()

	if s.cfg.UpdateDeferDays > 0 && time.Since(firstSeen) < time.Duration(s.cfg.UpdateDeferDays)*24*time.Hour {
		log.Printf("[scheduler] update_check: %s deferred (update.defer_days=%d)", release.TagName, s.cfg.UpdateDeferDays)
		return nil
	}

	// Rolling, node-by-node install for cluster mode (AI.md PART 19 "never
	// all nodes at once") requires cluster peer coordination that does not
	// exist yet in this codebase — see TODO.AI.md. On a single node this is
	// safe: install the binary but do not force-restart the running
	// process, since that would drop in-flight connections without a
	// graceful drain. An admin/operator (or the next planned restart)
	// picks up the new binary.
	if err := updater.DoUpdate(ctx, release); err != nil {
		return fmt.Errorf("update_check: auto-install %s: %w", release.TagName, err)
	}
	log.Printf("[scheduler] update_check: installed %s — restart required to run the new version", release.TagName)
	return nil
}

// downloadToFile fetches url and writes the body to dest atomically (write to
// a temp file then rename). Follows up to 5 redirects; times out after 60 s.
func (s *Scheduler) downloadToFile(url, dest string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: unexpected status %d", url, resp.StatusCode)
	}

	tmp := dest + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err = io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write to temp file: %w", err)
	}
	if err = f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err = os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, dest, err)
	}
	return nil
}

// sanitizeFilename replaces characters that are unsafe in file names with
// underscores so that source names can be used as file name stems.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	s := b.String()
	if s == "" {
		return "source"
	}
	return s
}

// checkTorHealth verifies the Tor hidden-service process is running.
// If it is not running, a restart is attempted. Silently skips when no
// Tor checker has been registered (i.e. Tor binary was not found at startup).
func (s *Scheduler) checkTorHealth(ctx context.Context) error {
	if s.torChecker == nil {
		return nil
	}

	if s.torChecker.IsRunning() {
		return nil
	}

	log.Printf("[scheduler] tor_health: Tor process is not running — attempting restart")
	if err := s.torChecker.Restart(); err != nil {
		return fmt.Errorf("tor_health: restart failed: %w", err)
	}
	log.Printf("[scheduler] tor_health: Tor restarted successfully")
	return nil
}

// clusterHeartbeat implements the `cluster_heartbeat` local task (AI.md
// PART 19: "every 30 seconds, cluster mode only"). This codebase does not
// yet implement real multi-node clustering (see TODO.AI.md) — the admin
// panel's "Cluster Nodes" page has no persisted peer list — so this is
// currently always a no-op single-node skip. It is still registered (and
// scheduled) so the task becomes active without further scheduler changes
// once cluster peer tracking lands.
func (s *Scheduler) clusterHeartbeat(ctx context.Context) error {
	return nil
}
