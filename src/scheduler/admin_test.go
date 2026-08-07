package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newAdminTestScheduler builds a scheduler with the full built-in task set
// registered and each task's row initialised in scheduler_tasks (the state the
// admin control surface reads and mutates), without starting the ticker loop.
func newAdminTestScheduler(t *testing.T) *Scheduler {
	t.Helper()
	s := newTestScheduler(t)
	s.addTasks()
	now := time.Now()
	for _, tk := range s.tasks {
		s.loadOrInitTaskState(tk, now)
	}
	return s
}

func TestListTasksReturnsEveryRegisteredTask(t *testing.T) {
	s := newAdminTestScheduler(t)
	views := s.ListTasks(context.Background())
	if len(views) != len(s.tasks) {
		t.Fatalf("ListTasks returned %d views, want %d", len(views), len(s.tasks))
	}

	byID := map[string]TaskView{}
	for _, v := range views {
		byID[v.ID] = v
	}

	// A non-skippable critical task must be flagged and enabled.
	sc, ok := byID["session_cleanup"]
	if !ok {
		t.Fatal("session_cleanup missing from ListTasks")
	}
	if !sc.NonSkippable {
		t.Error("session_cleanup should be NonSkippable")
	}
	if !sc.Enabled {
		t.Error("session_cleanup should be enabled")
	}
	if sc.NextRun.IsZero() {
		t.Error("session_cleanup NextRun should be set after loadOrInitTaskState")
	}

	// A skippable task must not be flagged non-skippable.
	if bh, ok := byID["backup_hourly"]; ok && bh.NonSkippable {
		t.Error("backup_hourly should be skippable")
	}
}

func TestTaskByIDUnknownReturnsErrTaskNotFound(t *testing.T) {
	s := newAdminTestScheduler(t)
	if _, err := s.TaskByID(context.Background(), "does_not_exist"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("TaskByID(unknown) err = %v, want ErrTaskNotFound", err)
	}
}

func TestSetTaskEnabledPersistsAndRefusesCriticalDisable(t *testing.T) {
	s := newAdminTestScheduler(t)
	ctx := context.Background()

	// Disabling a skippable task succeeds and persists to scheduler_tasks.
	if err := s.SetTaskEnabled(ctx, "backup_daily", false); err != nil {
		t.Fatalf("SetTaskEnabled(backup_daily,false) err = %v", err)
	}
	v, err := s.TaskByID(ctx, "backup_daily")
	if err != nil {
		t.Fatalf("TaskByID(backup_daily) err = %v", err)
	}
	if v.Enabled {
		t.Error("backup_daily should be disabled after SetTaskEnabled(false)")
	}
	// Confirm durability: the row's enabled column is 0.
	var enabled int
	if err := s.store.ServerDB.QueryRow(
		`SELECT enabled FROM scheduler_tasks WHERE id=?`, "backup_daily").Scan(&enabled); err != nil {
		t.Fatalf("query enabled: %v", err)
	}
	if enabled != 0 {
		t.Errorf("persisted enabled = %d, want 0", enabled)
	}

	// Re-enabling works.
	if err := s.SetTaskEnabled(ctx, "backup_daily", true); err != nil {
		t.Fatalf("SetTaskEnabled(backup_daily,true) err = %v", err)
	}

	// Disabling a non-skippable critical task is refused and changes nothing.
	if err := s.SetTaskEnabled(ctx, "session_cleanup", false); !errors.Is(err, ErrTaskNotSkippable) {
		t.Fatalf("SetTaskEnabled(session_cleanup,false) err = %v, want ErrTaskNotSkippable", err)
	}
	v, err = s.TaskByID(ctx, "session_cleanup")
	if err != nil {
		t.Fatalf("TaskByID(session_cleanup) err = %v", err)
	}
	if !v.Enabled {
		t.Error("session_cleanup must remain enabled after refused disable")
	}

	// Unknown task id.
	if err := s.SetTaskEnabled(ctx, "nope", true); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("SetTaskEnabled(nope) err = %v, want ErrTaskNotFound", err)
	}
}

func TestRunNowRejectsAlreadyRunning(t *testing.T) {
	s := newAdminTestScheduler(t)

	tk := s.findTask("healthcheck_self")
	if tk == nil {
		t.Fatal("healthcheck_self not registered")
	}
	// Simulate an in-flight execution.
	tk.mu.Lock()
	tk.running = true
	tk.mu.Unlock()

	if err := s.RunNow("healthcheck_self"); !errors.Is(err, ErrTaskRunning) {
		t.Fatalf("RunNow(running) err = %v, want ErrTaskRunning", err)
	}

	tk.mu.Lock()
	tk.running = false
	tk.mu.Unlock()

	if err := s.RunNow("nope"); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("RunNow(unknown) err = %v, want ErrTaskNotFound", err)
	}
}

func TestRunNowExecutesTaskAndRecordsHistory(t *testing.T) {
	s := newAdminTestScheduler(t)

	// Replace a task's function with a fast, deterministic one so RunNow's
	// async execution completes quickly and writes a history row.
	tk := s.findTask("healthcheck_self")
	if tk == nil {
		t.Fatal("healthcheck_self not registered")
	}
	tk.fn = func(ctx context.Context) error { return nil }

	if err := s.RunNow("healthcheck_self"); err != nil {
		t.Fatalf("RunNow err = %v", err)
	}
	// Wait for the async execution launched by RunNow to finish.
	s.wg.Wait()

	entries, err := s.TaskHistory(context.Background(), "healthcheck_self", 10)
	if err != nil {
		t.Fatalf("TaskHistory err = %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one history entry after RunNow")
	}
	if entries[0].Status != "success" {
		t.Errorf("history status = %q, want success", entries[0].Status)
	}
}

func TestTaskHistoryUnknownTask(t *testing.T) {
	s := newAdminTestScheduler(t)
	if _, err := s.TaskHistory(context.Background(), "nope", 10); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("TaskHistory(unknown) err = %v, want ErrTaskNotFound", err)
	}
}
