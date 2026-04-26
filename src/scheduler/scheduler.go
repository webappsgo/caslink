package scheduler

import (
	"log"

	"github.com/robfig/cron/v3"
)

// Scheduler manages background tasks
type Scheduler struct {
	cron *cron.Cron
}

// New creates a new scheduler
func New() *Scheduler {
	return &Scheduler{
		cron: cron.New(),
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	// Add scheduled tasks
	s.addTasks()

	// Start cron
	s.cron.Start()
	log.Println("Scheduler started")
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Scheduler stopped")
}

// addTasks adds scheduled tasks
func (s *Scheduler) addTasks() {
	// Session cleanup - hourly
	s.cron.AddFunc("@hourly", func() {
		log.Println("Running session cleanup task")
		// Implementation will be added later
	})

	// Daily backup - 2am
	s.cron.AddFunc("0 2 * * *", func() {
		log.Println("Running daily backup task")
		// Implementation will be added later
	})

	// GeoIP update - weekly Sunday 3am
	s.cron.AddFunc("0 3 * * 0", func() {
		log.Println("Running GeoIP update task")
		// Implementation will be added later
	})

	// Log rotation - daily midnight
	s.cron.AddFunc("0 0 * * *", func() {
		log.Println("Running log rotation task")
		// Implementation will be added later
	})
}
