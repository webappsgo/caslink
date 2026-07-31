package store

import (
	"net/url"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func TestBuildPostgresDSNEscapesSpecialCharacters(t *testing.T) {
	dsn := buildPostgresDSN("db.example.com", 5432, "caslink", "app_user", `p@ss w'ord\`, "require")

	// The raw password must never appear unescaped, since the single quote
	// and backslash would otherwise terminate the quoted value early.
	if strings.Contains(dsn, `password='p@ss w'ord\'`) {
		t.Fatalf("password was not escaped, DSN is malformed: %s", dsn)
	}
	if !strings.Contains(dsn, `password='p@ss w\'ord\\'`) {
		t.Fatalf("expected escaped password segment, got: %s", dsn)
	}
	if !strings.Contains(dsn, "host='db.example.com'") {
		t.Fatalf("expected quoted host, got: %s", dsn)
	}
	if !strings.Contains(dsn, "dbname='caslink'") {
		t.Fatalf("expected quoted dbname, got: %s", dsn)
	}
}

func TestBuildPostgresDSNDefaults(t *testing.T) {
	dsn := buildPostgresDSN("localhost", 0, "caslink", "app", "secret", "")
	if !strings.Contains(dsn, "port=5432") {
		t.Fatalf("expected default port 5432, got: %s", dsn)
	}
	if !strings.Contains(dsn, "sslmode='require'") {
		t.Fatalf("expected default sslmode require, got: %s", dsn)
	}
}

func TestBuildMySQLDSNEscapesSpecialCharacters(t *testing.T) {
	dsn := buildMySQLDSN("db.example.com", 3306, "caslink", "app_user", "p@ss:w/ord")

	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("FormatDSN produced an unparseable DSN: %v (%s)", err, dsn)
	}
	if cfg.Passwd != "p@ss:w/ord" {
		t.Fatalf("password round-trip mismatch: got %q", cfg.Passwd)
	}
	if cfg.User != "app_user" {
		t.Fatalf("user round-trip mismatch: got %q", cfg.User)
	}
	if cfg.DBName != "caslink" {
		t.Fatalf("dbname round-trip mismatch: got %q", cfg.DBName)
	}
}

func TestBuildMySQLDSNDefaultPort(t *testing.T) {
	dsn := buildMySQLDSN("localhost", 0, "caslink", "app", "secret")
	if !strings.Contains(dsn, "localhost:3306") {
		t.Fatalf("expected default port 3306, got: %s", dsn)
	}
}

func TestBuildSQLServerDSNEscapesSpecialCharacters(t *testing.T) {
	dsn := buildSQLServerDSN("db.example.com", 1433, "caslink", "app_user", "p@ss w:ord/x")

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("dsn is not a valid URL: %v (%s)", err, dsn)
	}
	if u.Scheme != "sqlserver" {
		t.Fatalf("expected sqlserver scheme, got: %s", u.Scheme)
	}
	pass, ok := u.User.Password()
	if !ok || pass != "p@ss w:ord/x" {
		t.Fatalf("password round-trip mismatch: got %q (ok=%v)", pass, ok)
	}
	if u.User.Username() != "app_user" {
		t.Fatalf("user round-trip mismatch: got %q", u.User.Username())
	}
	if u.Query().Get("database") != "caslink" {
		t.Fatalf("expected database query param, got: %s", u.Query().Get("database"))
	}
}

func TestBuildSQLServerDSNDefaultPort(t *testing.T) {
	dsn := buildSQLServerDSN("localhost", 0, "caslink", "app", "secret")
	if !strings.Contains(dsn, "localhost:1433") {
		t.Fatalf("expected default port 1433, got: %s", dsn)
	}
}
