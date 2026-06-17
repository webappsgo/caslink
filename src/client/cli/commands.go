// Package cli implements the cobra command tree for caslink-cli.
package cli

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/casjaysdevdocker/caslink/src/client/config"
)

// autodiscoverResponse mirrors the /api/autodiscover JSON shape (AI.md PART 14/33).
type autodiscoverResponse struct {
	Primary    string   `json:"primary"`
	Cluster    []string `json:"cluster"`
	APIVersion string   `json:"api_version"`
}

// GlobalFlags holds values parsed from persistent root flags.
// Exported so main.go can populate it before handing off to cobra.
type GlobalFlags struct {
	Server string
	Token  string
	Output string
	Debug  bool
}

// apiResponse mirrors the server's standard JSON envelope.
type apiResponse struct {
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   string          `json:"error,omitempty"`
	Message string          `json:"message,omitempty"`
}

// linkRecord is a single link as returned by the API.
type linkRecord struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ShortURL  string    `json:"short_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Active    bool      `json:"active"`
}

// statsRecord holds per-link analytics.
type statsRecord struct {
	Code       string         `json:"code"`
	TotalClicks int64         `json:"total_clicks"`
	UniqueClicks int64        `json:"unique_clicks"`
	Countries  []countryCount `json:"countries,omitempty"`
	Referrers  []referrerCount `json:"referrers,omitempty"`
}

type countryCount struct {
	Country string `json:"country"`
	Count   int64  `json:"count"`
}

type referrerCount struct {
	Referrer string `json:"referrer"`
	Count    int64  `json:"count"`
}

// client wraps an http.Client with auth, base URL, and cluster failover
// per AI.md PART 33.
type client struct {
	base    string
	cluster []string // additional URLs to try on primary failure
	token   string
	http    *http.Client
	output  string
	debug   bool
	cfg     *config.CLIConfig // kept for cluster persistence
}

func newClient(cfg *config.CLIConfig, gf GlobalFlags) *client {
	base := cfg.Server
	if gf.Server != "" {
		base = gf.Server
	}
	tok := cfg.Token
	if gf.Token != "" {
		tok = gf.Token
	}
	out := cfg.Color
	if gf.Output != "" {
		out = gf.Output
	}
	_ = out
	return &client{
		base:    strings.TrimRight(base, "/"),
		cluster: cfg.Cluster,
		token:   tok,
		http:    &http.Client{Timeout: 30 * time.Second},
		output:  gf.Output,
		debug:   gf.Debug,
		cfg:     cfg,
	}
}

// refreshCluster calls /api/autodiscover against the primary URL and updates
// the cluster list in the config when the cached data is stale (>30 minutes).
// Errors are non-fatal — the existing cluster list is kept.
func (c *client) refreshCluster() {
	if c.cfg == nil {
		return
	}
	// Skip refresh if cache is fresh (< 30 minutes old).
	if c.cfg.ClusterRefreshedAt > 0 &&
		time.Now().Unix()-c.cfg.ClusterRefreshedAt < 30*60 {
		return
	}
	resp, err := c.http.Get(c.base + "/api/autodiscover")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	var disc autodiscoverResponse
	if json.NewDecoder(resp.Body).Decode(&disc) != nil {
		return
	}
	if len(disc.Cluster) > 0 {
		c.cluster = disc.Cluster
		c.cfg.Cluster = disc.Cluster
	}
	c.cfg.ClusterRefreshedAt = time.Now().Unix()
	_ = config.SaveCLIConfig(c.cfg) // non-fatal if save fails
}

// doWithFailover wraps do() with cluster failover per AI.md PART 33.
// On a connection-level error to the primary, each cluster URL is tried in
// order. The first success promotes that URL for the rest of the session.
// 4xx errors (including 401) are never retried — they are caller errors.
func (c *client) doWithFailover(method, path string, body io.Reader) (*apiResponse, error) {
	ar, err := c.do(method, path, body)
	if err == nil {
		return ar, nil
	}
	// Only fail-over on connection errors, not on HTTP 4xx/5xx from the server.
	if len(c.cluster) == 0 {
		return nil, err
	}
	lastErr := err
	for _, alt := range c.cluster {
		altBase := strings.TrimRight(alt, "/")
		if altBase == c.base {
			continue
		}
		saved := c.base
		c.base = altBase
		ar2, err2 := c.do(method, path, body)
		if err2 == nil {
			if c.debug {
				fmt.Fprintf(os.Stderr, "cluster failover: promoted %s (was %s)\n", altBase, saved)
			}
			if c.cfg != nil {
				c.cfg.Server = altBase
				_ = config.SaveCLIConfig(c.cfg)
			}
			return ar2, nil
		}
		c.base = saved
		lastErr = err2
	}
	return nil, fmt.Errorf("cannot reach caslink server at any of %d configured URLs (last error: %w)",
		1+len(c.cluster), lastErr)
}

func (c *client) do(method, path string, body io.Reader) (*apiResponse, error) {
	url := c.base + "/api/v1" + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.debug {
		fmt.Fprintf(os.Stderr, ">> %s %s\n", method, url)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var ar apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return nil, fmt.Errorf("decode response (HTTP %d): %w", resp.StatusCode, err)
	}

	// Handle token revocation per AI.md PART 33.
	// On TOKEN_REVOKED or TOKEN_EXPIRED the CLI clears the cached token
	// so the next invocation prompts for fresh credentials.
	if resp.StatusCode == http.StatusUnauthorized &&
		(ar.Error == "TOKEN_REVOKED" || ar.Error == "TOKEN_EXPIRED") {
		// Clear the cached token from the in-memory config (callers must
		// persist via config.SaveCLIConfig if needed).
		if c.token != "" {
			c.token = ""
		}
		fmt.Fprintln(os.Stderr, "error: your API token has been revoked or has expired. Run 'caslink-cli login' to re-authenticate.")
		os.Exit(4)
	}

	if !ar.OK {
		if ar.Message != "" {
			return nil, fmt.Errorf("[%s] %s", ar.Error, ar.Message)
		}
		return nil, fmt.Errorf("server error: %s", ar.Error)
	}
	return &ar, nil
}

// BuildRootCmd constructs and returns the root cobra command.
func BuildRootCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	root := &cobra.Command{
		Use:           "caslink-cli",
		Short:         "CLI client for caslink URL shortener",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&gf.Server, "server", "", "caslink server URL (overrides config)")
	root.PersistentFlags().StringVar(&gf.Token, "token", "", "API bearer token (overrides config)")
	root.PersistentFlags().StringVar(&gf.Output, "output", "table", "output format: table|json|csv")
	root.PersistentFlags().BoolVar(&gf.Debug, "debug", false, "enable debug output")

	root.AddCommand(
		loginCmd(cfg, gf),
		logoutCmd(cfg),
		listCmd(cfg, gf),
		createCmd(cfg, gf),
		getCmd(cfg, gf),
		deleteCmd(cfg, gf),
		qrCmd(cfg, gf),
		statsCmd(cfg, gf),
		versionCmd(),
	)
	return root
}

// loginCmd authenticates and saves the token to config.
func loginCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with the caslink server",
		RunE: func(cmd *cobra.Command, args []string) error {
			tok := gf.Token
			if tok == "" {
				fmt.Print("Token: ")
				if _, err := fmt.Scan(&tok); err != nil {
					return fmt.Errorf("read token: %w", err)
				}
			}
			cfg.Token = strings.TrimSpace(tok)
			// Refresh autodiscover to populate cluster list (PART 33).
			c := newClient(cfg, *gf)
			c.refreshCluster()
			if err := config.SaveCLIConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged in. Token saved to config.")
			return nil
		},
	}
}

// logoutCmd clears the saved token from config.
func logoutCmd(cfg *config.CLIConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear saved credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg.Token = ""
			if err := config.SaveCLIConfig(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			// Also remove token file if present.
			tf, err := config.GetTokenFile()
			if err == nil {
				_ = os.Remove(tf)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
			return nil
		},
	}
}

// listCmd fetches and displays all links.
func listCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all links",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg, *gf)
			ar, err := c.doWithFailover(http.MethodGet, "/links", nil)
			if err != nil {
				return err
			}
			var links []linkRecord
			if err := json.Unmarshal(ar.Data, &links); err != nil {
				return fmt.Errorf("parse links: %w", err)
			}
			return renderLinks(cmd.OutOrStdout(), links, c.output)
		},
	}
}

// createCmd creates a new short link.
func createCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	var code string
	cmd := &cobra.Command{
		Use:   "create <url>",
		Short: "Create a new short link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg, *gf)
			payload := fmt.Sprintf(`{"url":%q`, args[0])
			if code != "" {
				payload += fmt.Sprintf(`,"code":%q`, code)
			}
			payload += "}"
			ar, err := c.doWithFailover(http.MethodPost, "/links", strings.NewReader(payload))
			if err != nil {
				return err
			}
			var link linkRecord
			if err := json.Unmarshal(ar.Data, &link); err != nil {
				return fmt.Errorf("parse link: %w", err)
			}
			return renderLinks(cmd.OutOrStdout(), []linkRecord{link}, c.output)
		},
	}
	cmd.Flags().StringVar(&code, "code", "", "custom short code (auto-generated if empty)")
	return cmd
}

// getCmd retrieves a single link by code.
func getCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "get <code>",
		Short: "Get link details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg, *gf)
			ar, err := c.doWithFailover(http.MethodGet, "/links/"+args[0], nil)
			if err != nil {
				return err
			}
			var link linkRecord
			if err := json.Unmarshal(ar.Data, &link); err != nil {
				return fmt.Errorf("parse link: %w", err)
			}
			return renderLinks(cmd.OutOrStdout(), []linkRecord{link}, c.output)
		},
	}
}

// deleteCmd removes a link by code.
func deleteCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <code>",
		Short: "Delete a link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg, *gf)
			_, err := c.doWithFailover(http.MethodDelete, "/links/"+args[0], nil)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted link %s\n", args[0])
			return nil
		},
	}
}

// qrCmd prints the QR code for a link to the terminal.
func qrCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "qr <code>",
		Short: "Display QR code in terminal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg, *gf)
			ar, err := c.doWithFailover(http.MethodGet, "/links/"+args[0]+"/qr", nil)
			if err != nil {
				return err
			}
			var result struct {
				ASCII string `json:"ascii"`
				URL   string `json:"url"`
			}
			if err := json.Unmarshal(ar.Data, &result); err != nil {
				return fmt.Errorf("parse qr: %w", err)
			}
			if result.ASCII != "" {
				fmt.Fprintln(cmd.OutOrStdout(), result.ASCII)
			} else if result.URL != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "QR URL: %s\n", result.URL)
			}
			return nil
		},
	}
}

// statsCmd shows click analytics for a link.
func statsCmd(cfg *config.CLIConfig, gf *GlobalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "stats <code>",
		Short: "Show analytics for a link",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := newClient(cfg, *gf)
			ar, err := c.doWithFailover(http.MethodGet, "/links/"+args[0]+"/stats", nil)
			if err != nil {
				return err
			}
			var s statsRecord
			if err := json.Unmarshal(ar.Data, &s); err != nil {
				return fmt.Errorf("parse stats: %w", err)
			}
			return renderStats(cmd.OutOrStdout(), s, c.output)
		},
	}
}

// versionCmd prints version information.
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), versionString())
		},
	}
}

// versionString is filled by the main package via a package-level variable.
var versionString = func() string { return "caslink-cli dev" }

// SetVersionString allows main.go to inject the full version string.
func SetVersionString(fn func() string) { versionString = fn }

// renderLinks writes link records in the requested format.
func renderLinks(w io.Writer, links []linkRecord, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(links)
	case "csv":
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"code", "url", "short_url", "clicks", "active", "created_at"})
		for _, l := range links {
			_ = cw.Write([]string{
				l.Code, l.URL, l.ShortURL,
				fmt.Sprintf("%d", l.Clicks),
				fmt.Sprintf("%v", l.Active),
				l.CreatedAt.Format(time.RFC3339),
			})
		}
		cw.Flush()
		return cw.Error()
	default: // table
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CODE\tURL\tCLICKS\tACTIVE\tCREATED")
		for _, l := range links {
			active := "yes"
			if !l.Active {
				active = "no"
			}
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
				l.Code, l.URL, l.Clicks, active, l.CreatedAt.Format("2006-01-02 15:04"))
		}
		return tw.Flush()
	}
}

// renderStats writes analytics in the requested format.
func renderStats(w io.Writer, s statsRecord, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(s)
	case "csv":
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"code", "total_clicks", "unique_clicks"})
		_ = cw.Write([]string{s.Code, fmt.Sprintf("%d", s.TotalClicks), fmt.Sprintf("%d", s.UniqueClicks)})
		cw.Flush()
		return cw.Error()
	default: // table
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "Code:\t%s\n", s.Code)
		fmt.Fprintf(tw, "Total clicks:\t%d\n", s.TotalClicks)
		fmt.Fprintf(tw, "Unique clicks:\t%d\n", s.UniqueClicks)
		if len(s.Countries) > 0 {
			fmt.Fprintln(tw, "")
			fmt.Fprintln(tw, "COUNTRY\tCLICKS")
			for _, c := range s.Countries {
				fmt.Fprintf(tw, "%s\t%d\n", c.Country, c.Count)
			}
		}
		if len(s.Referrers) > 0 {
			fmt.Fprintln(tw, "")
			fmt.Fprintln(tw, "REFERRER\tCLICKS")
			for _, r := range s.Referrers {
				fmt.Fprintf(tw, "%s\t%d\n", r.Referrer, r.Count)
			}
		}
		return tw.Flush()
	}
}
