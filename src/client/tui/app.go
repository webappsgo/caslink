// Package tui implements the interactive TUI for caslink-cli.
package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/casjaysdevdocker/caslink/src/client/config"
)

// view identifies the active panel.
type view int

const (
	viewLinks view = iota
	viewStats
	viewSettings
)

// linkRecord is a simplified link for display.
type linkRecord struct {
	Code      string    `json:"code"`
	URL       string    `json:"url"`
	ShortURL  string    `json:"short_url"`
	Clicks    int64     `json:"clicks"`
	CreatedAt time.Time `json:"created_at"`
	Active    bool      `json:"active"`
}

// apiResponse is the server envelope.
type apiResponse struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data,omitempty"`
}

// model is the root bubbletea model for the TUI.
type model struct {
	cfg         *config.CLIConfig
	activeView  view
	links       []linkRecord
	cursor      int
	loading     bool
	err         string
	width       int
	height      int
	styles      styles
}

// styles holds pre-built lipgloss styles.
type styles struct {
	title     lipgloss.Style
	selected  lipgloss.Style
	normal    lipgloss.Style
	muted     lipgloss.Style
	tab       lipgloss.Style
	activeTab lipgloss.Style
	border    lipgloss.Style
	help      lipgloss.Style
	error     lipgloss.Style
}

func newStyles() styles {
	return styles{
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75")).
			MarginBottom(1),
		selected: lipgloss.NewStyle().
			Background(lipgloss.Color("24")).
			Foreground(lipgloss.Color("255")).
			PaddingLeft(1).PaddingRight(1),
		normal: lipgloss.NewStyle().
			PaddingLeft(1).PaddingRight(1),
		muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")),
		tab: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			PaddingLeft(1).PaddingRight(1),
		activeTab: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("75")).
			Underline(true).
			PaddingLeft(1).PaddingRight(1),
		border: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240")),
		help: lipgloss.NewStyle().
			Foreground(lipgloss.Color("244")).
			MarginTop(1),
		error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true),
	}
}

// fetchLinksMsg carries the result of fetching links.
type fetchLinksMsg struct {
	links []linkRecord
	err   error
}

// fetchLinksCmd fires an HTTP request to list links.
func fetchLinksCmd(cfg *config.CLIConfig) tea.Cmd {
	return func() tea.Msg {
		url := strings.TrimRight(cfg.Server, "/") + "/api/v1/links"
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fetchLinksMsg{err: err}
		}
		if cfg.Token != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.Token)
		}
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fetchLinksMsg{err: fmt.Errorf("request failed: %w", err)}
		}
		defer resp.Body.Close()

		var ar apiResponse
		if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
			return fetchLinksMsg{err: fmt.Errorf("decode: %w", err)}
		}
		if !ar.OK {
			return fetchLinksMsg{err: fmt.Errorf("server error (HTTP %d)", resp.StatusCode)}
		}
		var links []linkRecord
		if err := json.Unmarshal(ar.Data, &links); err != nil {
			return fetchLinksMsg{err: fmt.Errorf("parse links: %w", err)}
		}
		return fetchLinksMsg{links: links}
	}
}

// NewApp returns a configured bubbletea program for the TUI.
func NewApp(cfg *config.CLIConfig) *tea.Program {
	m := model{
		cfg:        cfg,
		activeView: viewLinks,
		loading:    true,
		styles:     newStyles(),
	}
	return tea.NewProgram(m, tea.WithAltScreen())
}

func (m model) Init() tea.Cmd {
	return fetchLinksCmd(m.cfg)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case fetchLinksMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.links = msg.links
			m.err = ""
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "1":
			m.activeView = viewLinks
		case "2":
			m.activeView = viewStats
		case "3":
			m.activeView = viewSettings

		case "r":
			if m.activeView == viewLinks {
				m.loading = true
				return m, fetchLinksCmd(m.cfg)
			}

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.links)-1 {
				m.cursor++
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	var b strings.Builder

	// Title bar
	b.WriteString(m.styles.title.Render("CASLINK"))
	b.WriteString("\n")

	// Navigation tabs
	tabs := []string{"1: Links", "2: Stats", "3: Settings"}
	for i, t := range tabs {
		if view(i) == m.activeView {
			b.WriteString(m.styles.activeTab.Render(t))
		} else {
			b.WriteString(m.styles.tab.Render(t))
		}
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", min(m.width, 80)))
	b.WriteString("\n")

	// Content area
	switch m.activeView {
	case viewLinks:
		b.WriteString(m.renderLinks())
	case viewStats:
		b.WriteString(m.renderStatsPanel())
	case viewSettings:
		b.WriteString(m.renderSettings())
	}

	// Help bar
	b.WriteString(m.styles.help.Render("j/k: navigate  r: refresh  q: quit"))

	return b.String()
}

func (m model) renderLinks() string {
	var b strings.Builder

	if m.loading {
		return m.styles.muted.Render("\n  Loading links...")
	}
	if m.err != "" {
		return m.styles.error.Render("\n  Error: " + m.err)
	}
	if len(m.links) == 0 {
		return m.styles.muted.Render("\n  No links found. Use 'caslink-cli create <url>' to add one.")
	}

	// Column header
	b.WriteString(m.styles.muted.Render(
		fmt.Sprintf("  %-12s %-40s %8s\n", "CODE", "URL", "CLICKS"),
	))

	maxURL := m.width - 30
	if maxURL < 20 {
		maxURL = 20
	}
	if maxURL > 60 {
		maxURL = 60
	}

	for i, l := range m.links {
		urlDisplay := l.URL
		if len(urlDisplay) > maxURL {
			urlDisplay = urlDisplay[:maxURL-1] + "…"
		}
		line := fmt.Sprintf("  %-12s %-*s %8d", l.Code, maxURL, urlDisplay, l.Clicks)
		if i == m.cursor {
			b.WriteString(m.styles.selected.Render(line))
		} else {
			b.WriteString(m.styles.normal.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (m model) renderStatsPanel() string {
	if len(m.links) == 0 {
		return m.styles.muted.Render("\n  No links available for stats.")
	}
	if m.cursor >= len(m.links) {
		return m.styles.muted.Render("\n  Select a link from the Links tab first.")
	}
	l := m.links[m.cursor]
	return fmt.Sprintf(
		"\n  Link:   %s\n  URL:    %s\n  Short:  %s\n  Clicks: %d\n  Active: %v\n",
		l.Code, l.URL, l.ShortURL, l.Clicks, l.Active,
	)
}

func (m model) renderSettings() string {
	return fmt.Sprintf(
		"\n  Server: %s\n  Token:  %s\n  Lang:   %s\n  Color:  %s\n",
		m.cfg.Server,
		maskToken(m.cfg.Token),
		m.cfg.Lang,
		m.cfg.Color,
	)
}

// maskToken returns a safe display version of a token.
func maskToken(tok string) string {
	if tok == "" {
		return "(none)"
	}
	if len(tok) <= 8 {
		return strings.Repeat("•", len(tok))
	}
	return tok[:4] + strings.Repeat("•", len(tok)-8) + tok[len(tok)-4:]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
