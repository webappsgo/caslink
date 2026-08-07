package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Admin-facing control surface for the scheduler, backing the
// /server/{admin_path}/config/scheduler admin page and the AI.md PART 19
// "API Endpoints" (list, run, enable, disable, history). These methods are
// the only supported way to mutate live task state at runtime — the ticker
// loop (tick) and these methods coordinate through each task's mutex.

var (
	// ErrTaskNotFound is returned when no registered task has the given id.
	ErrTaskNotFound = errors.New("scheduler: task not found")
	// ErrTaskRunning is returned by RunNow when the task is already executing.
	ErrTaskRunning = errors.New("scheduler: task is already running")
	// ErrTaskNotSkippable is returned when disabling a critical task that
	// AI.md PART 19 marks non-skippable.
	ErrTaskNotSkippable = errors.New("scheduler: task is critical and cannot be disabled")
)

// TaskView is a read-only snapshot of one task's live and persisted state,
// combining the in-memory schedule/run flags with the durable counters from
// scheduler_tasks. Times are zero when not yet set.
type TaskView struct {
	ID           string
	Name         string
	Type         string // "global" or "local"
	Schedule     string
	Enabled      bool
	Running      bool
	NonSkippable bool // true → the enable/disable toggle is locked on
	NextRun      time.Time
	LastRun      time.Time
	LastStatus   string
	LastError    string
	RunCount     int64
	FailCount    int64
}

// HistoryEntry is one row of a task's execution history (scheduler_history).
type HistoryEntry struct {
	StartedAt  time.Time
	FinishedAt time.Time
	Status     string
	Error      string
	DurationMs int64
}

// findTask returns the registered task with the given id, or nil.
func (s *Scheduler) findTask(id string) *task {
	for _, t := range s.tasks {
		if t.id == id {
			return t
		}
	}
	return nil
}

// snapshot builds a TaskView for t, reading its live flags under t.mu and
// joining the persisted counters from scheduler_tasks.
func (s *Scheduler) snapshot(ctx context.Context, t *task) TaskView {
	t.mu.Lock()
	v := TaskView{
		ID:           t.id,
		Name:         t.name,
		Type:         t.taskType,
		Schedule:     t.schedule,
		Enabled:      t.enabled,
		Running:      t.running,
		NonSkippable: nonSkippableTasks[t.id],
		NextRun:      t.nextRun,
	}
	t.mu.Unlock()

	var lastRunUnix sql.NullInt64
	var lastStatus, lastError sql.NullString
	var runCount, failCount int64
	row := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT last_run, last_status, last_error, run_count, fail_count FROM scheduler_tasks WHERE id = ?`, t.id)
	if err := row.Scan(&lastRunUnix, &lastStatus, &lastError, &runCount, &failCount); err == nil {
		if lastRunUnix.Valid && lastRunUnix.Int64 > 0 {
			v.LastRun = time.Unix(lastRunUnix.Int64, 0)
		}
		v.LastStatus = lastStatus.String
		v.LastError = lastError.String
		v.RunCount = runCount
		v.FailCount = failCount
	}
	return v
}

// ListTasks returns a snapshot of every registered task in registration order,
// for the admin scheduler overview and the list API endpoint.
func (s *Scheduler) ListTasks(ctx context.Context) []TaskView {
	views := make([]TaskView, 0, len(s.tasks))
	for _, t := range s.tasks {
		views = append(views, s.snapshot(ctx, t))
	}
	return views
}

// TaskByID returns a snapshot of a single task, or ErrTaskNotFound.
func (s *Scheduler) TaskByID(ctx context.Context, id string) (TaskView, error) {
	t := s.findTask(id)
	if t == nil {
		return TaskView{}, ErrTaskNotFound
	}
	return s.snapshot(ctx, t), nil
}

// SetTaskEnabled enables or disables a task at runtime and persists the change
// to scheduler_tasks so it survives a restart (AI.md PART 19 persistent state).
// Disabling a non-skippable critical task returns ErrTaskNotSkippable and makes
// no change.
func (s *Scheduler) SetTaskEnabled(ctx context.Context, id string, enabled bool) error {
	if !enabled && nonSkippableTasks[id] {
		return ErrTaskNotSkippable
	}
	t := s.findTask(id)
	if t == nil {
		return ErrTaskNotFound
	}

	t.mu.Lock()
	t.enabled = enabled
	t.mu.Unlock()

	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, err := s.store.ServerDB.ExecContext(dbCtx,
		`UPDATE scheduler_tasks SET enabled=? WHERE id=?`, boolToInt(enabled), id)
	return err
}

// RunNow triggers an immediate execution of a task, respecting the same
// running-lock and cluster-lock path as a scheduled tick. It returns
// ErrTaskRunning if the task is already executing, or ErrTaskNotFound. A
// disabled task can still be run on demand (an explicit admin action), matching
// the AI.md PART 19 "Run Now" button semantics.
func (s *Scheduler) RunNow(id string) error {
	t := s.findTask(id)
	if t == nil {
		return ErrTaskNotFound
	}

	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return ErrTaskRunning
	}
	t.running = true
	t.mu.Unlock()

	if s.metrics != nil {
		s.metrics.SchedulerTasksRunning.WithLabelValues(t.id).Set(1)
	}

	s.wg.Add(1)
	go func(t *task) {
		defer s.wg.Done()
		s.runTask(t)
	}(t)
	return nil
}

// TaskHistory returns up to limit recent execution records for a task, newest
// first (AI.md PART 19 "History — Last 100 executions per task"). limit is
// clamped to 1..100.
func (s *Scheduler) TaskHistory(ctx context.Context, id string, limit int) ([]HistoryEntry, error) {
	if s.findTask(id) == nil {
		return nil, ErrTaskNotFound
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := s.store.ServerDB.QueryContext(dbCtx,
		`SELECT started_at, finished_at, status, COALESCE(error,''), COALESCE(duration_ms,0)
		 FROM scheduler_history WHERE task_id=? ORDER BY started_at DESC, id DESC LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []HistoryEntry
	for rows.Next() {
		var startedUnix, finishedUnix, durationMs int64
		var status, errStr string
		if err := rows.Scan(&startedUnix, &finishedUnix, &status, &errStr, &durationMs); err != nil {
			return nil, err
		}
		e := HistoryEntry{
			StartedAt:  time.Unix(startedUnix, 0),
			Status:     status,
			Error:      errStr,
			DurationMs: durationMs,
		}
		if finishedUnix > 0 {
			e.FinishedAt = time.Unix(finishedUnix, 0)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
