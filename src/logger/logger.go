// Package logger implements the log file subsystem required by AI.md PART 13.
//
// Log files created:
//   - access.log  — one line per HTTP request (Apache common log format)
//   - server.log  — application lifecycle events (text)
//   - error.log   — errors and warnings (text)
//   - audit.log   — security-relevant events (JSON Lines — machine-parseable)
//   - security.log — fail2ban-compatible auth failure records (text)
//   - debug.log   — verbose debug output (text; written only in dev mode)
//
// All log files are plain ASCII; no ANSI codes.
// Rotation trigger: each file is capped at 50 MB; weekly rotation for most,
// daily for audit.log. Full rotation is handled by the external OS log
// rotation tooling (logrotate) or the scheduler's log_rotation task.
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger holds handles for each log channel.
type Logger struct {
	mu       sync.Mutex
	logDir   string
	devMode  bool

	accessFile   *os.File
	serverFile   *os.File
	errorFile    *os.File
	auditFile    *os.File
	securityFile *os.File
	debugFile    *os.File

	// stdlib loggers backed by the files above.
	Server   *log.Logger
	Error    *log.Logger
	Security *log.Logger
	Debug    *log.Logger
}

// New opens (or creates) all required log files under logDir and returns a
// Logger. Caller must call Close() when the process exits.
func New(logDir string, devMode bool) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("creating log directory: %w", err)
	}

	l := &Logger{logDir: logDir, devMode: devMode}

	open := func(name string) (*os.File, error) {
		return os.OpenFile(filepath.Join(logDir, name),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	}

	var err error
	if l.accessFile, err = open("access.log"); err != nil {
		return nil, err
	}
	if l.serverFile, err = open("server.log"); err != nil {
		return nil, err
	}
	if l.errorFile, err = open("error.log"); err != nil {
		return nil, err
	}
	if l.auditFile, err = open("audit.log"); err != nil {
		return nil, err
	}
	if l.securityFile, err = open("security.log"); err != nil {
		return nil, err
	}
	if devMode {
		if l.debugFile, err = open("debug.log"); err != nil {
			return nil, err
		}
	}

	flags := log.Ldate | log.Ltime | log.LUTC

	l.Server = log.New(io.MultiWriter(os.Stderr, l.serverFile), "server ", flags)
	l.Error = log.New(io.MultiWriter(os.Stderr, l.errorFile), "error  ", flags)
	l.Security = log.New(l.securityFile, "security ", flags)

	if devMode && l.debugFile != nil {
		l.Debug = log.New(io.MultiWriter(os.Stderr, l.debugFile), "debug  ", flags)
	} else {
		// Discard debug output in production.
		l.Debug = log.New(io.Discard, "", 0)
	}

	// Replace the default stdlib logger so that log.Printf() from third-party
	// packages goes to server.log rather than bare stderr.
	log.SetOutput(io.MultiWriter(os.Stderr, l.serverFile))
	log.SetFlags(flags)
	log.SetPrefix("server ")

	return l, nil
}

// Access writes a single access log line in Apache common log format.
// Fields: ip - - [time] "METHOD path HTTP/1.1" status bytes
func (l *Logger) Access(ip, method, path, proto string, status, bytes int, duration time.Duration) {
	if l.accessFile == nil {
		return
	}
	ts := time.Now().UTC().Format("02/Jan/2006:15:04:05 -0700")
	line := fmt.Sprintf("%s - - [%s] \"%s %s %s\" %d %d %.3f\n",
		ip, ts, method, path, proto, status, bytes, duration.Seconds())
	l.mu.Lock()
	_, _ = l.accessFile.WriteString(line)
	l.mu.Unlock()
}

// AuditEvent is the JSON Lines record written to audit.log per AI.md PART 13.
// Fields are fixed; callers fill what is known and leave the rest zero.
type AuditEvent struct {
	Time      string `json:"time"`
	Action    string `json:"action"`
	Actor     string `json:"actor,omitempty"`
	ActorType string `json:"actor_type,omitempty"`
	Resource  string `json:"resource,omitempty"`
	IP        string `json:"ip,omitempty"`
	Result    string `json:"result"` // "ok" | "denied" | "error"
	Details   string `json:"details,omitempty"`
}

// Audit writes a JSON Lines record to audit.log. It is always written in
// JSON regardless of the server mode — audit logs must be machine-parseable.
func (l *Logger) Audit(ev AuditEvent) {
	if l.auditFile == nil {
		return
	}
	if ev.Time == "" {
		ev.Time = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	l.mu.Lock()
	_, _ = l.auditFile.WriteString(string(data) + "\n")
	l.mu.Unlock()
}

// SecurityEvent writes a fail2ban-compatible line to security.log.
// Format: TIMESTAMP [LEVEL] Message  (e.g., "Failed login for user foo from 1.2.3.4")
func (l *Logger) SecurityEvent(level, msg string) {
	if l.securityFile == nil {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s [%s] %s\n", ts, level, msg)
	l.mu.Lock()
	_, _ = l.securityFile.WriteString(line)
	l.mu.Unlock()
}

// Close closes all open log file handles.
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, f := range []*os.File{
		l.accessFile, l.serverFile, l.errorFile,
		l.auditFile, l.securityFile, l.debugFile,
	} {
		if f != nil {
			_ = f.Close()
		}
	}
}
