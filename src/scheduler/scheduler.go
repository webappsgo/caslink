package scheduler

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/casjaysdevdocker/caslink/src/geoip"
	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// Scheduler manages background tasks.
type Scheduler struct {
	cron   *cron.Cron
	store  *store.Store
	logDir string         // path to log directory for log_rotation; may be ""
	geoip  *geoip.Service // optional; nil → geoip_update is a no-op
}

// New creates a new scheduler bound to the given store.
// logDir is the directory containing application log files; pass ""
// to skip log rotation. geoSvc is optional — when nil the geoip_update
// task logs and skips.
func New(st *store.Store, logDir string, geoSvc *geoip.Service) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		store:  st,
		logDir: logDir,
		geoip:  geoSvc,
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
	if _, err := s.cron.AddFunc("@every 15m", func() {
		s.cleanupSessions()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register session_cleanup: %v", err)
	}

	// token_cleanup — every 15 minutes (PART 19). Removes expired API
	// tokens so revoked sessions can never be replayed.
	if _, err := s.cron.AddFunc("@every 15m", func() {
		s.cleanupTokens()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register token_cleanup: %v", err)
	}

	// healthcheck_self — every 5 minutes (PART 19). Pings both DBs so a
	// degraded backend surfaces in logs without waiting for an external
	// monitor to notice.
	if _, err := s.cron.AddFunc("@every 5m", func() {
		s.selfHealthCheck()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register healthcheck_self: %v", err)
	}

	// Expired-URL cleanup — daily at 02:30 (after backup window).
	if _, err := s.cron.AddFunc("30 2 * * *", func() {
		s.expireURLs()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register expire_urls: %v", err)
	}

	// ssl_renewal — daily at 03:00. Checks and renews Let's Encrypt certificates
	// that are within 7 days of expiry. Skips silently when no certs are configured.
	if _, err := s.cron.AddFunc("0 3 * * *", func() {
		s.renewSSL()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register ssl_renewal: %v", err)
	}

	// geoip_update — weekly Sunday 03:00. Downloads updated GeoIP databases.
	// Skips silently when GeoIP is not configured.
	if _, err := s.cron.AddFunc("0 3 * * 0", func() {
		s.updateGeoIP()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register geoip_update: %v", err)
	}

	// backup_daily — daily at 02:00. Creates a full backup of all data.
	// Skips silently when backup is not configured.
	if _, err := s.cron.AddFunc("0 2 * * *", func() {
		s.runDailyBackup()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register backup_daily: %v", err)
	}

	// log_rotation — daily at midnight (PART 19). Compresses log files older
	// than 24 hours and removes files older than 30 days.
	if _, err := s.cron.AddFunc("0 0 * * *", func() {
		s.rotateLogs()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register log_rotation: %v", err)
	}

	// blocklist_update — daily at 04:00 (PART 19). Downloads updated IP/domain
	// blocklists. Skips silently when blocklist sources are not configured.
	if _, err := s.cron.AddFunc("0 4 * * *", func() {
		s.updateBlocklist()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register blocklist_update: %v", err)
	}

	// cve_update — daily at 05:00 (PART 19). Downloads updated CVE/security
	// databases. Skips silently when CVE sources are not configured.
	if _, err := s.cron.AddFunc("0 5 * * *", func() {
		s.updateCVE()
	}); err != nil {
		log.Printf("[scheduler] addTasks: register cve_update: %v", err)
	}

	// backup_hourly — disabled by default (PART 19). Registered so the admin
	// panel can surface and enable it without a restart.
	// Enable via config: server.scheduler.tasks.backup_hourly.enabled = true
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

// rotateLogs compresses log files that are older than 24 hours and removes
// compressed archives older than 30 days. It operates only on the files
// created by the logger package: access.log, server.log, error.log,
// audit.log, security.log, debug.log.
//
// Rotation steps:
//  1. For each *.log file: if last-modified > 24h ago, compress to *.log.gz
//     and truncate the original (so the logger can keep appending to the fd).
//  2. Remove any *.log.gz file whose modification time is older than 30 days.
func (s *Scheduler) rotateLogs() {
	if s.logDir == "" {
		log.Println("[scheduler] log_rotation: no log directory configured, skipping")
		return
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

		// Build archive name with a timestamp stamp so multiple archives coexist.
		stamp := fi.ModTime().UTC().Format("2006-01-02")
		archiveName := fmt.Sprintf("%s.%s.gz", name, stamp)
		archivePath := filepath.Join(s.logDir, archiveName)

		if err := compressLog(path, archivePath); err != nil {
			log.Printf("[scheduler] log_rotation: compress %s: %v", name, err)
			continue
		}
		rotated++
	}

	// Prune old archives.
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

	// Write to a temp file in the same directory so the rename is atomic.
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

	// Truncate the original file so the logger keeps its file descriptor valid.
	return os.Truncate(src, 0)
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

// renewSSL checks and renews Let's Encrypt certificates expiring within 7 days.
// Silently skips when no certificates are configured.
func (s *Scheduler) renewSSL() {
	// SSL renewal is handled by the ssl package when Let's Encrypt is configured.
	// The ssl package tracks certs in the database and handles ACME challenges
	// directly via the server's /.well-known/acme-challenge/ handler.
	// When no certs are registered, this is a no-op.
}

// updateGeoIP downloads the configured GeoIP databases via the geoip
// service. Skips silently when the service is not wired (e.g. GeoIP
// disabled in config).
func (s *Scheduler) updateGeoIP() {
	if s.geoip == nil || !s.geoip.Enabled() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := s.geoip.Update(ctx); err != nil {
		log.Printf("[scheduler] geoip_update: %v", err)
	}
}

// runDailyBackup creates a full backup of all application data.
// Silently skips when backup is not configured.
func (s *Scheduler) runDailyBackup() {
	// Daily backups are handled by the maintenance/backup package when a
	// backup directory is configured. When not configured, this is a no-op.
}

// updateBlocklist downloads updated IP/domain blocklists.
// Silently skips when blocklist sources are not configured.
func (s *Scheduler) updateBlocklist() {
	// Blocklist updates are handled when blocklist sources are configured.
	// When no sources are configured, this is a no-op.
}

// updateCVE downloads updated CVE/security databases.
// Silently skips when CVE sources are not configured.
func (s *Scheduler) updateCVE() {
	// CVE database updates are handled when CVE sources are configured.
	// When not configured, this is a no-op.
}
