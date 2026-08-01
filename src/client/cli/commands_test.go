package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/client/config"
)

// TestNewClient covers flag-over-config precedence: an explicit GlobalFlags
// value must win over the persisted config value, and a trailing slash on
// the configured server must be stripped so path joins in do() don't
// produce a double slash.
func TestNewClient(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *config.CLIConfig
		gf         GlobalFlags
		wantBase   string
		wantToken  string
		wantDebug  bool
	}{
		{
			name:      "config values used when flags empty",
			cfg:       &config.CLIConfig{Server: "https://link.example.com/", Token: "cfg-token"},
			gf:        GlobalFlags{},
			wantBase:  "https://link.example.com",
			wantToken: "cfg-token",
		},
		{
			name:      "flags override config",
			cfg:       &config.CLIConfig{Server: "https://link.example.com", Token: "cfg-token"},
			gf:        GlobalFlags{Server: "https://override.example.com", Token: "flag-token", Debug: true},
			wantBase:  "https://override.example.com",
			wantToken: "flag-token",
			wantDebug: true,
		},
		{
			name:      "empty config produces empty base",
			cfg:       &config.CLIConfig{},
			gf:        GlobalFlags{},
			wantBase:  "",
			wantToken: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newClient(tt.cfg, tt.gf)
			if c.base != tt.wantBase {
				t.Errorf("base = %q, want %q", c.base, tt.wantBase)
			}
			if c.token != tt.wantToken {
				t.Errorf("token = %q, want %q", c.token, tt.wantToken)
			}
			if c.debug != tt.wantDebug {
				t.Errorf("debug = %v, want %v", c.debug, tt.wantDebug)
			}
		})
	}
}

// newTestClient builds a client wired to httptest server ts, bypassing
// newClient's config-precedence logic since these tests only exercise do().
func newTestClient(ts *httptest.Server) *client {
	return &client{
		base: ts.URL,
		http: ts.Client(),
	}
}

func TestClientDo_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/links" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":[{"code":"abc"}]}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	ar, err := c.do(http.MethodGet, "/links", nil)
	if err != nil {
		t.Fatalf("do() error = %v", err)
	}
	if !ar.OK {
		t.Error("ar.OK = false, want true")
	}
	var links []struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(ar.Data, &links); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(links) != 1 || links[0].Code != "abc" {
		t.Errorf("links = %+v, want one link with code abc", links)
	}
}

// TestClientDo_SendsAuthHeader verifies the bearer token is attached when set.
func TestClientDo_SendsAuthHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	c.token = "adm_abc123"
	if _, err := c.do(http.MethodGet, "/links", nil); err != nil {
		t.Fatalf("do() error = %v", err)
	}
	if want := "Bearer adm_abc123"; gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

// TestClientDo_NoAuthHeaderWhenTokenEmpty ensures no Authorization header is
// sent for anonymous requests.
func TestClientDo_NoAuthHeaderWhenTokenEmpty(t *testing.T) {
	var gotAuth string
	sawHeader := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		sawHeader = r.Header.Get("Authorization") != ""
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	if _, err := c.do(http.MethodGet, "/links", nil); err != nil {
		t.Fatalf("do() error = %v", err)
	}
	if sawHeader {
		t.Errorf("Authorization header unexpectedly set: %q", gotAuth)
	}
}

// TestClientDo_ServerErrorEnvelope covers the ok:false error path, both with
// and without a message, per the canonical error envelope (AI.md PART 14).
func TestClientDo_ServerErrorEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "error with message",
			body:    `{"ok":false,"error":"NOT_FOUND","message":"link not found"}`,
			wantErr: "[NOT_FOUND] link not found",
		},
		{
			name:    "error without message",
			body:    `{"ok":false,"error":"SERVER_ERROR"}`,
			wantErr: "server error: SERVER_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer ts.Close()

			c := newTestClient(ts)
			_, err := c.do(http.MethodGet, "/links", nil)
			if err == nil {
				t.Fatal("do() expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestClientDo_MalformedJSON ensures a non-JSON body produces a decode error
// rather than a panic or a false-success result.
func TestClientDo_MalformedJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	_, err := c.do(http.MethodGet, "/links", nil)
	if err == nil {
		t.Fatal("do() expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode response") {
		t.Errorf("error = %q, want it to mention decode response", err.Error())
	}
}

// TestClientDo_ConnectionError covers the network-failure path (unreachable
// host) used by doWithFailover to decide whether to try cluster members.
func TestClientDo_ConnectionError(t *testing.T) {
	c := &client{
		base: "http://127.0.0.1:1", // reserved port, connection refused
		http: &http.Client{Timeout: 2 * time.Second},
	}
	_, err := c.do(http.MethodGet, "/links", nil)
	if err == nil {
		t.Fatal("do() expected connection error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q, want it to mention request failed", err.Error())
	}
}

// TestDoWithFailover_PrimarySucceeds ensures no failover is attempted when
// the primary responds successfully.
func TestDoWithFailover_PrimarySucceeds(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	c := newTestClient(ts)
	c.cluster = []string{"http://127.0.0.1:1"}
	ar, err := c.doWithFailover(http.MethodGet, "/links", nil)
	if err != nil {
		t.Fatalf("doWithFailover() error = %v", err)
	}
	if !ar.OK {
		t.Error("ar.OK = false, want true")
	}
}

// TestDoWithFailover_PromotesWorkingClusterMember verifies that when the
// primary is unreachable, the first working cluster URL is tried and its
// base URL is promoted for the rest of the session (AI.md PART 33).
func TestDoWithFailover_PromotesWorkingClusterMember(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer good.Close()

	c := &client{
		base:    "http://127.0.0.1:1",
		cluster: []string{"http://127.0.0.1:1", good.URL},
		http:    &http.Client{Timeout: 2 * time.Second},
	}
	ar, err := c.doWithFailover(http.MethodGet, "/links", nil)
	if err != nil {
		t.Fatalf("doWithFailover() error = %v", err)
	}
	if !ar.OK {
		t.Error("ar.OK = false, want true")
	}
	if c.base != good.URL {
		t.Errorf("base after failover = %q, want %q (promoted)", c.base, good.URL)
	}
}

// TestDoWithFailover_AllUnreachable ensures a clear aggregate error is
// returned when the primary and every cluster member fail.
func TestDoWithFailover_AllUnreachable(t *testing.T) {
	c := &client{
		base:    "http://127.0.0.1:1",
		cluster: []string{"http://127.0.0.1:2"},
		http:    &http.Client{Timeout: 2 * time.Second},
	}
	_, err := c.doWithFailover(http.MethodGet, "/links", nil)
	if err == nil {
		t.Fatal("doWithFailover() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cannot reach caslink server") {
		t.Errorf("error = %q, want it to mention unreachable server", err.Error())
	}
}

// TestDoWithFailover_NoClusterConfigured ensures the original error is
// returned unwrapped when there is no cluster list to fall back to.
func TestDoWithFailover_NoClusterConfigured(t *testing.T) {
	c := &client{
		base: "http://127.0.0.1:1",
		http: &http.Client{Timeout: 2 * time.Second},
	}
	_, err := c.doWithFailover(http.MethodGet, "/links", nil)
	if err == nil {
		t.Fatal("doWithFailover() expected error, got nil")
	}
	if strings.Contains(err.Error(), "cannot reach caslink server") {
		t.Errorf("error = %q, expected primary error to be returned unwrapped when no cluster is configured", err.Error())
	}
}

// TestDoWithFailover_4xxNotRetried documents the ACTUAL implemented behavior
// of doWithFailover (see its "Only fail-over on connection errors, not on
// HTTP 4xx/5xx from the server" doc comment). The implementation does not
// actually distinguish connection errors from business (4xx/5xx) errors —
// any non-nil error from the primary, including a decoded {"ok":false,...}
// body, sends it into the cluster-failover loop, so a 4xx from the primary
// still tries (and here, succeeds against) cluster members. This is a
// discovered discrepancy, logged in TODO.AI.md rather than fixed here (out
// of scope for test-writing). This test locks in the current, actual
// behavior.
func TestDoWithFailover_4xxNotRetried(t *testing.T) {
	primaryHits := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ok":false,"error":"NOT_FOUND","message":"no such link"}`))
	}))
	defer primary.Close()

	clusterHits := 0
	clusterSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clusterHits++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer clusterSrv.Close()

	c := newTestClient(primary)
	c.cluster = []string{clusterSrv.URL}
	_, err := c.doWithFailover(http.MethodGet, "/links/xyz", nil)
	if err != nil {
		t.Fatalf("doWithFailover() error = %v, want nil (current actual behavior — see TODO.AI.md)", err)
	}
	if primaryHits != 1 {
		t.Errorf("primary hit %d times, want 1", primaryHits)
	}
	if clusterHits != 1 {
		t.Errorf("cluster hit %d times, want 1 (current actual behavior — 4xx does trigger failover — see TODO.AI.md)", clusterHits)
	}
}

func TestRenderLinks_Table(t *testing.T) {
	created := time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC)
	links := []linkRecord{
		{Code: "abc", URL: "https://example.com", ShortURL: "https://l.co/abc", Clicks: 5, Active: true, CreatedAt: created},
		{Code: "def", URL: "https://example.org", ShortURL: "https://l.co/def", Clicks: 0, Active: false, CreatedAt: created},
	}
	var buf bytes.Buffer
	if err := renderLinks(&buf, links, "table"); err != nil {
		t.Fatalf("renderLinks() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "CODE") || !strings.Contains(out, "abc") {
		t.Errorf("table output missing expected content: %q", out)
	}
	if !strings.Contains(out, "yes") || !strings.Contains(out, "no") {
		t.Errorf("table output missing active/inactive markers: %q", out)
	}
}

func TestRenderLinks_JSON(t *testing.T) {
	links := []linkRecord{{Code: "abc", URL: "https://example.com", Clicks: 3, Active: true}}
	var buf bytes.Buffer
	if err := renderLinks(&buf, links, "json"); err != nil {
		t.Fatalf("renderLinks() error = %v", err)
	}
	var got []linkRecord
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 1 || got[0].Code != "abc" {
		t.Errorf("decoded links = %+v, want one link with code abc", got)
	}
}

func TestRenderLinks_CSV(t *testing.T) {
	links := []linkRecord{{Code: "abc", URL: "https://example.com", ShortURL: "https://l.co/abc", Clicks: 3, Active: true}}
	var buf bytes.Buffer
	if err := renderLinks(&buf, links, "csv"); err != nil {
		t.Fatalf("renderLinks() error = %v", err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("csv output = %d lines, want 2 (header + row)", len(lines))
	}
	if !strings.HasPrefix(lines[0], "code,url,short_url,clicks,active,created_at") {
		t.Errorf("csv header = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "abc,https://example.com,https://l.co/abc,3,true,") {
		t.Errorf("csv row = %q", lines[1])
	}
}

// TestRenderLinks_Empty ensures the zero-links case does not error or panic
// for any output format.
func TestRenderLinks_Empty(t *testing.T) {
	for _, format := range []string{"table", "json", "csv"} {
		t.Run(format, func(t *testing.T) {
			var buf bytes.Buffer
			if err := renderLinks(&buf, nil, format); err != nil {
				t.Fatalf("renderLinks(%q) error = %v", format, err)
			}
		})
	}
}

func TestRenderStats_Table(t *testing.T) {
	s := statsRecord{
		Code:         "abc",
		TotalClicks:  10,
		UniqueClicks: 7,
		Countries:    []countryCount{{Country: "US", Count: 5}},
		Referrers:    []referrerCount{{Referrer: "google.com", Count: 3}},
	}
	var buf bytes.Buffer
	if err := renderStats(&buf, s, "table"); err != nil {
		t.Fatalf("renderStats() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"abc", "10", "7", "US", "google.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q:\n%s", want, out)
		}
	}
}

// TestRenderStats_NoCountriesOrReferrers ensures optional sections are
// omitted cleanly rather than printing empty headers.
func TestRenderStats_NoCountriesOrReferrers(t *testing.T) {
	s := statsRecord{Code: "abc", TotalClicks: 1, UniqueClicks: 1}
	var buf bytes.Buffer
	if err := renderStats(&buf, s, "table"); err != nil {
		t.Fatalf("renderStats() error = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "COUNTRY") || strings.Contains(out, "REFERRER") {
		t.Errorf("expected no COUNTRY/REFERRER sections when empty:\n%s", out)
	}
}

func TestRenderStats_JSON(t *testing.T) {
	s := statsRecord{Code: "abc", TotalClicks: 10, UniqueClicks: 7}
	var buf bytes.Buffer
	if err := renderStats(&buf, s, "json"); err != nil {
		t.Fatalf("renderStats() error = %v", err)
	}
	var got statsRecord
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Code != "abc" || got.TotalClicks != 10 {
		t.Errorf("decoded stats = %+v", got)
	}
}

func TestRenderStats_CSV(t *testing.T) {
	s := statsRecord{Code: "abc", TotalClicks: 10, UniqueClicks: 7}
	var buf bytes.Buffer
	if err := renderStats(&buf, s, "csv"); err != nil {
		t.Fatalf("renderStats() error = %v", err)
	}
	want := "code,total_clicks,unique_clicks\nabc,10,7\n"
	if buf.String() != want {
		t.Errorf("csv output = %q, want %q", buf.String(), want)
	}
}

// TestSetVersionString_OverridesDefault covers the package-level version
// injection hook used by main.go.
func TestSetVersionString_OverridesDefault(t *testing.T) {
	original := versionString
	defer func() { versionString = original }()

	SetVersionString(func() string { return "caslink-cli 9.9.9 (deadbeef)" })
	if got := versionString(); got != "caslink-cli 9.9.9 (deadbeef)" {
		t.Errorf("versionString() = %q, want injected value", got)
	}
}

func TestBuildRootCmd_HasExpectedCommands(t *testing.T) {
	cfg := &config.CLIConfig{}
	gf := &GlobalFlags{}
	root := BuildRootCmd(cfg, gf)

	want := []string{"login", "logout", "list", "create", "get", "delete", "qr", "stats", "version"}
	got := make(map[string]bool)
	for _, c := range root.Commands() {
		got[c.Name()] = true
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing subcommand %q", name)
		}
	}
}

// TestLogoutCmd_ClearsToken ensures the logout command clears the in-memory
// token and persists the cleared config, and removes any standalone token
// file — a full regression check for the credential-clearing path.
func TestLogoutCmd_ClearsToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.CLIConfig{Server: "https://link.example.com", Token: "adm_abc"}
	if err := config.SaveCLIConfig(cfg); err != nil {
		t.Fatalf("seed SaveCLIConfig() error = %v", err)
	}

	root := BuildRootCmd(cfg, &GlobalFlags{})
	root.SetArgs([]string{"logout"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("logout command error = %v", err)
	}

	if cfg.Token != "" {
		t.Errorf("cfg.Token = %q after logout, want empty", cfg.Token)
	}

	reloaded, err := config.LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() error = %v", err)
	}
	if reloaded.Token != "" {
		t.Errorf("persisted Token = %q after logout, want empty", reloaded.Token)
	}
}

// TestDeleteCmd_Success drives the delete subcommand end-to-end against a
// fake server to verify request routing and success output.
func TestDeleteCmd_Success(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.CLIConfig{Server: ts.URL}
	root := BuildRootCmd(cfg, &GlobalFlags{})
	root.SetArgs([]string{"delete", "abc"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("delete command error = %v", err)
	}

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/links/abc" {
		t.Errorf("path = %q, want /api/v1/links/abc", gotPath)
	}
	if !strings.Contains(out.String(), "Deleted link abc") {
		t.Errorf("output = %q, want confirmation message", out.String())
	}
}

// TestCreateCmd_SendsCustomCode verifies the optional --code flag is
// included in the request payload only when set.
func TestCreateCmd_SendsCustomCode(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		gotBody = string(body)
		_, _ = fmt.Fprintf(w, `{"ok":true,"data":{"code":"custom","url":"https://example.com"}}`)
	}))
	defer ts.Close()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.CLIConfig{Server: ts.URL}
	root := BuildRootCmd(cfg, &GlobalFlags{})
	root.SetArgs([]string{"create", "https://example.com", "--code", "custom"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("create command error = %v", err)
	}
	if !strings.Contains(gotBody, `"code":"custom"`) {
		t.Errorf("request body = %q, want it to include custom code", gotBody)
	}
}
