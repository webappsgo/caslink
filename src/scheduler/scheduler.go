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

// addTasks registers all cron jobs. The schedule follows AI.md PART 19
// → "Built-in Tasks (Required)". Tasks whose feature is not yet
// implemented log a placeholder message — they remain registered so the
// admin panel surfaces the task once UI lands.
func (s *Scheduler) addTasks() {
	// session_cleanup — every 15 minutes (PART 19).
	s.cron.AddFunc("@every 15m", func() {
		s.cleanupSessions()
	})

	// token_cleanup — every 15 minutes (PART 19). Removes expired API
	// tokens so revoked sessions can never be replayed.
	s.cron.AddFunc("@every 15m", func() {
		s.cleanupTokens()
	})

	// healthcheck_self — every 5 minutes (PART 19). Pings both DBs so a
	// degraded backend surfaces in logs without waiting for an external
	// monitor to notice.
	s.cron.AddFunc("@every 5m", func() {
		s.selfHealthCheck()
	})

	// Expired-URL cleanup — daily at 02:30 (after backup window).
	s.cron.AddFunc("30 2 * * *", func() {
		s.expireURLs()
	})

	// ssl_renewal — daily at 03:00. Placeholder until Let's Encrypt
	// integration lands (TODO.AI.md → Custom domains).
	s.cron.AddFunc("0 3 * * *", func() {
		log.Println("[scheduler] ssl_renewal: not yet implemented")
	})

	// geoip_update — weekly Sunday 03:00. Placeholder until GeoIP
	// enrichment lands (TODO.AI.md → Analytics).
	s.cron.AddFunc("0 3 * * 0", func() {
		log.Println("[scheduler] geoip_update: not yet implemented")
	})

	// backup_daily — daily at 02:00. Placeholder until backup subsystem
	// lands.
	s.cron.AddFunc("0 2 * * *", func() {
		log.Println("[scheduler] backup_daily: not yet implemented")
	})
}

// cleanupTokens removes expired API tokens from users.db.
func (s *Scheduler) cleanupTokens() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := s.store.UsersDB.ExecContext(ctx,
		`DELETE FROM api_tokens WHERE expires_at IS NOT NULL AND expires_at < ?`,
		time.Now().UTC(),
	)
	if err != nil {
		log.Printf("[scheduler] cleanupTokens error: %v", err)
		return
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log.Printf("[scheduler] cleanupTokens: removed %d expired tokens", n)
	}
}

// selfHealthCheck pings both databases. Failures are logged at WARN — the
// scheduler does not restart the process; an external monitor / operator
// reacts to the log line.
func (s *Scheduler) selfHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.store.ServerDB != nil {
		if err := s.store.ServerDB.PingContext(ctx); err != nil {
			log.Printf("[scheduler] healthcheck_self: server.db ping failed: %v", err)
		}
	}
	if s.store.UsersDB != nil {
		if err := s.store.UsersDB.PingContext(ctx); err != nil {
			log.Printf("[scheduler] healthcheck_self: users.db ping failed: %v", err)
		}
	}
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
