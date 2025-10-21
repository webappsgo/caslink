package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// Scheduler manages scheduled tasks using cron
type Scheduler struct {
	cron     *cron.Cron
	db       *db.DB
	config   *config.Config
	logger   *logrus.Logger
	tasks    map[string]*ScheduledTask
	mutex    sync.RWMutex
	running  bool
	ctx      context.Context
	cancel   context.CancelFunc
}

// ScheduledTask represents a scheduled task
type ScheduledTask struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schedule    string                 `json:"schedule"`
	Enabled     bool                   `json:"enabled"`
	Handler     TaskHandler            `json:"-"`
	LastRun     *time.Time             `json:"last_run,omitempty"`
	NextRun     *time.Time             `json:"next_run,omitempty"`
	RunCount    int64                  `json:"run_count"`
	ErrorCount  int64                  `json:"error_count"`
	LastError   string                 `json:"last_error,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	CronEntryID cron.EntryID           `json:"-"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// TaskHandler is the function signature for task handlers
type TaskHandler func(ctx context.Context, task *ScheduledTask) error

// TaskStatus represents the execution status of a task
type TaskStatus struct {
	TaskID      string     `json:"task_id"`
	Status      string     `json:"status"` // running, completed, failed
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Duration    string     `json:"duration,omitempty"`
	Error       string     `json:"error,omitempty"`
	Output      string     `json:"output,omitempty"`
}

// SchedulerStats represents scheduler statistics
type SchedulerStats struct {
	TotalTasks      int   `json:"total_tasks"`
	EnabledTasks    int   `json:"enabled_tasks"`
	RunningTasks    int   `json:"running_tasks"`
	TotalRuns       int64 `json:"total_runs"`
	SuccessfulRuns  int64 `json:"successful_runs"`
	FailedRuns      int64 `json:"failed_runs"`
	LastRunTime     *time.Time `json:"last_run_time,omitempty"`
	NextRunTime     *time.Time `json:"next_run_time,omitempty"`
	UptimeSeconds   int64 `json:"uptime_seconds"`
}

// NewScheduler creates a new scheduler instance
func NewScheduler(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*Scheduler, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Create cron scheduler with second precision
	cronScheduler := cron.New(cron.WithSeconds(), cron.WithLogger(cron.VerbosePrintfLogger(logger)))

	scheduler := &Scheduler{
		cron:   cronScheduler,
		db:     database,
		config: cfg,
		logger: logger,
		tasks:  make(map[string]*ScheduledTask),
		ctx:    ctx,
		cancel: cancel,
	}

	return scheduler, nil
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	// Load persisted tasks from database
	if err := s.loadPersistedTasks(); err != nil {
		s.logger.WithError(err).Warn("Failed to load persisted tasks")
	}

	// Register default system tasks
	if err := s.registerDefaultTasks(); err != nil {
		return fmt.Errorf("failed to register default tasks: %w", err)
	}

	// Start the cron scheduler
	s.cron.Start()
	s.running = true

	s.logger.Info("Scheduler started successfully")
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.running {
		return fmt.Errorf("scheduler is not running")
	}

	// Stop the cron scheduler
	s.cron.Stop()
	s.cancel()
	s.running = false

	s.logger.Info("Scheduler stopped successfully")
	return nil
}

// AddTask adds a new scheduled task
func (s *Scheduler) AddTask(task *ScheduledTask) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if task.ID == "" {
		task.ID = s.generateTaskID()
	}

	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	task.UpdatedAt = time.Now()

	// Validate cron schedule
	if _, err := cron.ParseStandard(task.Schedule); err != nil {
		return fmt.Errorf("invalid cron schedule: %w", err)
	}

	// Add to cron if enabled
	if task.Enabled && s.running {
		entryID, err := s.cron.AddFunc(task.Schedule, func() {
			s.executeTask(task)
		})
		if err != nil {
			return fmt.Errorf("failed to add task to cron: %w", err)
		}
		task.CronEntryID = entryID

		// Set next run time
		entry := s.cron.Entry(entryID)
		task.NextRun = &entry.Next
	}

	// Store in memory
	s.tasks[task.ID] = task

	// Persist to database
	if err := s.persistTask(task); err != nil {
		s.logger.WithError(err).WithField("task_id", task.ID).Error("Failed to persist task")
	}

	s.logger.WithFields(logrus.Fields{
		"task_id":  task.ID,
		"name":     task.Name,
		"schedule": task.Schedule,
		"enabled":  task.Enabled,
	}).Info("Task added to scheduler")

	return nil
}

// RemoveTask removes a scheduled task
func (s *Scheduler) RemoveTask(taskID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Remove from cron
	if task.CronEntryID != 0 {
		s.cron.Remove(task.CronEntryID)
	}

	// Remove from memory
	delete(s.tasks, taskID)

	// Remove from database
	if err := s.removePersistedTask(taskID); err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("Failed to remove persisted task")
	}

	s.logger.WithField("task_id", taskID).Info("Task removed from scheduler")
	return nil
}

// EnableTask enables a scheduled task
func (s *Scheduler) EnableTask(taskID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if task.Enabled {
		return nil // Already enabled
	}

	task.Enabled = true
	task.UpdatedAt = time.Now()

	// Add to cron if running
	if s.running {
		entryID, err := s.cron.AddFunc(task.Schedule, func() {
			s.executeTask(task)
		})
		if err != nil {
			task.Enabled = false
			return fmt.Errorf("failed to add task to cron: %w", err)
		}
		task.CronEntryID = entryID

		// Set next run time
		entry := s.cron.Entry(entryID)
		task.NextRun = &entry.Next
	}

	// Update in database
	if err := s.persistTask(task); err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("Failed to update task in database")
	}

	s.logger.WithField("task_id", taskID).Info("Task enabled")
	return nil
}

// DisableTask disables a scheduled task
func (s *Scheduler) DisableTask(taskID string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if !task.Enabled {
		return nil // Already disabled
	}

	task.Enabled = false
	task.UpdatedAt = time.Now()
	task.NextRun = nil

	// Remove from cron
	if task.CronEntryID != 0 {
		s.cron.Remove(task.CronEntryID)
		task.CronEntryID = 0
	}

	// Update in database
	if err := s.persistTask(task); err != nil {
		s.logger.WithError(err).WithField("task_id", taskID).Error("Failed to update task in database")
	}

	s.logger.WithField("task_id", taskID).Info("Task disabled")
	return nil
}

// GetTask retrieves a task by ID
func (s *Scheduler) GetTask(taskID string) (*ScheduledTask, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	task, exists := s.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	// Update next run time if running
	if task.Enabled && task.CronEntryID != 0 {
		entry := s.cron.Entry(task.CronEntryID)
		task.NextRun = &entry.Next
	}

	return task, nil
}

// ListTasks returns all scheduled tasks
func (s *Scheduler) ListTasks() []*ScheduledTask {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	tasks := make([]*ScheduledTask, 0, len(s.tasks))
	for _, task := range s.tasks {
		// Update next run time if running
		if task.Enabled && task.CronEntryID != 0 {
			entry := s.cron.Entry(task.CronEntryID)
			task.NextRun = &entry.Next
		}
		tasks = append(tasks, task)
	}

	return tasks
}

// RunTaskNow executes a task immediately
func (s *Scheduler) RunTaskNow(taskID string) error {
	s.mutex.RLock()
	task, exists := s.tasks[taskID]
	s.mutex.RUnlock()

	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	go s.executeTask(task)
	return nil
}

// GetStats returns scheduler statistics
func (s *Scheduler) GetStats() *SchedulerStats {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	stats := &SchedulerStats{
		TotalTasks:   len(s.tasks),
		EnabledTasks: 0,
		RunningTasks: 0,
	}

	var lastRun, nextRun *time.Time

	for _, task := range s.tasks {
		if task.Enabled {
			stats.EnabledTasks++
		}

		stats.TotalRuns += task.RunCount
		stats.SuccessfulRuns += task.RunCount - task.ErrorCount
		stats.FailedRuns += task.ErrorCount

		if task.LastRun != nil && (lastRun == nil || task.LastRun.After(*lastRun)) {
			lastRun = task.LastRun
		}

		if task.NextRun != nil && (nextRun == nil || task.NextRun.Before(*nextRun)) {
			nextRun = task.NextRun
		}
	}

	stats.LastRunTime = lastRun
	stats.NextRunTime = nextRun

	return stats
}

// executeTask executes a scheduled task
func (s *Scheduler) executeTask(task *ScheduledTask) {
	if task.Handler == nil {
		s.logger.WithField("task_id", task.ID).Error("Task has no handler")
		return
	}

	startTime := time.Now()
	s.logger.WithFields(logrus.Fields{
		"task_id": task.ID,
		"name":    task.Name,
	}).Info("Executing scheduled task")

	// Create task context with timeout
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
	defer cancel()

	// Execute task
	err := task.Handler(ctx, task)

	// Update task statistics
	s.mutex.Lock()
	task.LastRun = &startTime
	task.RunCount++
	if err != nil {
		task.ErrorCount++
		task.LastError = err.Error()
		s.logger.WithError(err).WithField("task_id", task.ID).Error("Task execution failed")
	} else {
		task.LastError = ""
		s.logger.WithFields(logrus.Fields{
			"task_id":  task.ID,
			"duration": time.Since(startTime),
		}).Info("Task executed successfully")
	}
	task.UpdatedAt = time.Now()
	s.mutex.Unlock()

	// Persist updated task
	if err := s.persistTask(task); err != nil {
		s.logger.WithError(err).WithField("task_id", task.ID).Error("Failed to persist task update")
	}

	// Log task execution
	s.logTaskExecution(task.ID, startTime, time.Now(), err)
}

// generateTaskID generates a unique task ID
func (s *Scheduler) generateTaskID() string {
	return fmt.Sprintf("task_%d", time.Now().UnixNano())
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.running
}