package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/webappsgo/caslink/src/client/config"
)

func newTestModel() model {
	return model{
		cfg:        &config.CLIConfig{Server: "https://link.example.com", Token: "sometoken12345"},
		activeView: viewLinks,
		loading:    true,
		styles:     newStyles(),
	}
}

func TestUpdate_WindowSizeMsgSetsDimensions(t *testing.T) {
	m := newTestModel()
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	got := newM.(model)
	if got.width != 100 || got.height != 40 {
		t.Errorf("width/height = %d/%d, want 100/40", got.width, got.height)
	}
}

func TestUpdate_FetchLinksMsg_Success(t *testing.T) {
	m := newTestModel()
	links := []linkRecord{{Code: "abc", URL: "https://example.com", Clicks: 3, Active: true}}

	newM, _ := m.Update(fetchLinksMsg{links: links})
	got := newM.(model)

	if got.loading {
		t.Error("loading = true, want false after fetch completes")
	}
	if got.err != "" {
		t.Errorf("err = %q, want empty", got.err)
	}
	if len(got.links) != 1 || got.links[0].Code != "abc" {
		t.Errorf("links = %+v", got.links)
	}
}

func TestUpdate_FetchLinksMsg_Error(t *testing.T) {
	m := newTestModel()
	newM, _ := m.Update(fetchLinksMsg{err: fmt.Errorf("boom")})
	got := newM.(model)

	if got.loading {
		t.Error("loading = true, want false after fetch completes")
	}
	if got.err != "boom" {
		t.Errorf("err = %q, want %q", got.err, "boom")
	}
}

func TestUpdate_QuitKeys(t *testing.T) {
	for _, k := range []tea.KeyType{tea.KeyCtrlC} {
		m := newTestModel()
		_, cmd := m.Update(tea.KeyMsg{Type: k})
		if cmd == nil {
			t.Fatalf("expected quit command for key %v", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("cmd() = %T, want tea.QuitMsg", cmd())
		}
	}

	m := newTestModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit command for 'q'")
	}
}

func TestUpdate_ViewSwitching(t *testing.T) {
	tests := []struct {
		key  string
		want view
	}{
		{"1", viewLinks},
		{"2", viewStats},
		{"3", viewSettings},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := newTestModel()
			m.activeView = viewSettings // start somewhere else to prove it actually changes
			if tt.want == viewSettings {
				m.activeView = viewLinks
			}
			newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			got := newM.(model)
			if got.activeView != tt.want {
				t.Errorf("activeView = %v, want %v", got.activeView, tt.want)
			}
		})
	}
}

func TestUpdate_RefreshOnlyWhenViewingLinks(t *testing.T) {
	m := newTestModel()
	m.activeView = viewLinks
	m.loading = false

	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got := newM.(model)
	if !got.loading {
		t.Error("loading = false, want true after 'r' on links view")
	}
	if cmd == nil {
		t.Error("expected fetchLinksCmd to be returned")
	}

	m2 := newTestModel()
	m2.activeView = viewSettings
	m2.loading = false
	newM2, cmd2 := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	got2 := newM2.(model)
	if got2.loading {
		t.Error("loading = true, want false — 'r' should be a no-op outside links view")
	}
	if cmd2 != nil {
		t.Error("expected no command when 'r' pressed outside links view")
	}
}

func TestUpdate_CursorNavigation(t *testing.T) {
	m := newTestModel()
	m.links = []linkRecord{{Code: "a"}, {Code: "b"}, {Code: "c"}}
	m.cursor = 0

	// Up at the top stays at 0.
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	got := newM.(model)
	if got.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", got.cursor)
	}

	// Down moves forward.
	newM, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = newM.(model)
	if got.cursor != 1 {
		t.Errorf("cursor = %d, want 1", got.cursor)
	}

	// Down at the bottom stays clamped.
	got.cursor = 2
	newM, _ = got.Update(tea.KeyMsg{Type: tea.KeyDown})
	got = newM.(model)
	if got.cursor != 2 {
		t.Errorf("cursor = %d, want 2 (clamped at len-1)", got.cursor)
	}
}

func TestUpdate_CursorNavigation_EmptyLinks(t *testing.T) {
	m := newTestModel()
	m.links = nil
	m.cursor = 0

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	got := newM.(model)
	if got.cursor != 0 {
		t.Errorf("cursor = %d, want 0 when there are no links", got.cursor)
	}
}

func TestView_LoadingBeforeWindowSize(t *testing.T) {
	m := newTestModel()
	if got := m.View(); got != "Loading..." {
		t.Errorf("View() = %q, want %q before a WindowSizeMsg", got, "Loading...")
	}
}

func TestRenderLinks_States(t *testing.T) {
	base := newTestModel()
	base.width = 100

	t.Run("loading", func(t *testing.T) {
		m := base
		m.loading = true
		if out := m.renderLinks(); !strings.Contains(out, "Loading links") {
			t.Errorf("renderLinks() = %q, want loading message", out)
		}
	})

	t.Run("error", func(t *testing.T) {
		m := base
		m.loading = false
		m.err = "connection refused"
		if out := m.renderLinks(); !strings.Contains(out, "connection refused") {
			t.Errorf("renderLinks() = %q, want error message", out)
		}
	})

	t.Run("empty", func(t *testing.T) {
		m := base
		m.loading = false
		m.err = ""
		m.links = nil
		if out := m.renderLinks(); !strings.Contains(out, "No links found") {
			t.Errorf("renderLinks() = %q, want empty-state message", out)
		}
	})

	t.Run("populated with long URL truncation", func(t *testing.T) {
		m := base
		m.loading = false
		m.err = ""
		m.links = []linkRecord{
			{Code: "abc", URL: strings.Repeat("x", 200), Clicks: 1, CreatedAt: time.Now()},
		}
		out := m.renderLinks()
		if !strings.Contains(out, "abc") {
			t.Errorf("renderLinks() = %q, want it to include the code", out)
		}
		if strings.Contains(out, strings.Repeat("x", 200)) {
			t.Error("renderLinks() did not truncate an overlong URL")
		}
	})
}

func TestRenderStatsPanel_States(t *testing.T) {
	m := newTestModel()
	m.width = 100

	t.Run("no links", func(t *testing.T) {
		mm := m
		mm.links = nil
		if out := mm.renderStatsPanel(); !strings.Contains(out, "No links available") {
			t.Errorf("renderStatsPanel() = %q", out)
		}
	})

	t.Run("cursor beyond range", func(t *testing.T) {
		mm := m
		mm.links = []linkRecord{{Code: "a"}}
		mm.cursor = 5
		if out := mm.renderStatsPanel(); !strings.Contains(out, "Select a link") {
			t.Errorf("renderStatsPanel() = %q", out)
		}
	})

	t.Run("selected link shown", func(t *testing.T) {
		mm := m
		mm.links = []linkRecord{{Code: "abc", URL: "https://example.com", ShortURL: "https://l.co/abc", Clicks: 5, Active: true}}
		mm.cursor = 0
		out := mm.renderStatsPanel()
		for _, want := range []string{"abc", "https://example.com", "https://l.co/abc", "5", "true"} {
			if !strings.Contains(out, want) {
				t.Errorf("renderStatsPanel() = %q, want it to contain %q", out, want)
			}
		}
	})
}

func TestRenderSettings_MasksToken(t *testing.T) {
	m := newTestModel()
	out := m.renderSettings()
	if strings.Contains(out, "sometoken12345") {
		t.Error("renderSettings() leaked the raw token")
	}
	if !strings.Contains(out, m.cfg.Server) {
		t.Errorf("renderSettings() = %q, want it to include the server", out)
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "(none)"},
		{"short (<=8 chars) fully masked", "abcd", "••••"},
		{"exactly 8 chars fully masked", "abcdefgh", "••••••••"},
		{"long token shows prefix and suffix", "abcd1234567890wxyz", "abcd" + strings.Repeat("•", len("abcd1234567890wxyz")-8) + "wxyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskToken(tt.in); got != tt.want {
				t.Errorf("maskToken(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMaskToken_NeverExposesMiddle(t *testing.T) {
	tok := "adm_verysecrettoken1234567890"
	masked := maskToken(tok)
	middle := tok[4 : len(tok)-4]
	if strings.Contains(masked, middle) {
		t.Errorf("maskToken(%q) = %q leaked the middle portion", tok, masked)
	}
}

func TestMin(t *testing.T) {
	tests := []struct{ a, b, want int }{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{-1, 3, -1},
	}
	for _, tt := range tests {
		if got := min(tt.a, tt.b); got != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestView_FullRenderContainsExpectedSections(t *testing.T) {
	m := newTestModel()
	m.width = 100
	m.height = 40
	m.loading = false
	m.links = []linkRecord{{Code: "abc", URL: "https://example.com", Clicks: 1}}

	out := m.View()
	if !strings.Contains(out, "CASLINK") {
		t.Error("View() missing title")
	}
	if !strings.Contains(out, "quit") {
		t.Error("View() missing help bar")
	}
}
