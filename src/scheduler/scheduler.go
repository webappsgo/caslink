package scheduler

import (
	"context"
	"log"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// Scheduler manages background tasks.
type Scheduler struct {
	cron  *cron.Cron
	store *store.Store
}

// New creates a new scheduler bound to the given store.
func New(st *store.Store) *Scheduler {
	return &Scheduler{
		cron:  cron.New(),
		store: st,
	}
}

// Start starts the scheduler.
func (s *Scheduler) Start() {
	s.addTasks()
	s.cron.Start()
	log.Println("Scheduler started")
}

// Stop stops the scheduler gracefully.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(10 * time.Second):
		log.Println("Scheduler stop timed out")
	}
	log.Println("Scheduler stopped")
}

// addTasks registers all cron jobs.
func (s *Scheduler) addTasks() {
	// Expire URLs daily at midnight.
	s.cron.AddFunc("@daily", func() {
		log.Println("[scheduler] running expired-URL cleanup")
		s.expireURLs()
	})

	// GeoIP update placeholder — every 6 hours.
	s.cron.AddFunc("@every 6h", func() {
		log.Println("[scheduler] GeoIP update: not yet implemented")
	})

	// SSL renewal placeholder — every 6 hours.
	s.cron.AddFunc("@every 6h", func() {
		log.Println("[scheduler] SSL renewal check: not yet implemented")
	})

	// Session cleanup — hourly.
	s.cron.AddFunc("@hourly", func() {
		log.Println("[scheduler] running expired-session cleanup")
		s.cleanupSessions()
	})
}

// expireURLs deactivates or deletes URLs whose expires_at has passed.
func (s *Scheduler) expireURLs() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.store.ServerDB.ExecContext(ctx,
		`DELETE FROM urls WHERE expires_at IS NOT NULL AND expires_at < ?`,
		time.Now().UTC(),
	)
	if err != nil {
		log.Printf("[scheduler] expireURLs error: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[scheduler] expireURLs: removed %d expired URLs", n)
	}
}

// cleanupSessions removes expired sessions.
func (s *Scheduler) cleanupSessions() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`,
		time.Now().UTC(),
	)
	if err != nil {
		log.Printf("[scheduler] cleanupSessions error: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[scheduler] cleanupSessions: removed %d expired sessions", n)
	}
}
