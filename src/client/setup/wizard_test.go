package setup

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/webappsgo/caslink/src/client/config"
)

// typeText feeds each rune of s into the model as a sequence of KeyRunes
// messages, mirroring what bubbletea delivers for keyboard input.
func typeText(m model, s string) model {
	for _, r := range s {
		newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = newM.(model)
	}
	return m
}

func TestInitialModel_PrefillsServerFromConfig(t *testing.T) {
	cfg := &config.CLIConfig{Server: "https://existing.example.com"}
	m := initialModel(cfg)
	if got := m.serverInput.Value(); got != cfg.Server {
		t.Errorf("serverInput prefilled = %q, want %q", got, cfg.Server)
	}
	if m.step != stepServer {
		t.Errorf("step = %v, want stepServer", m.step)
	}
}

func TestUpdate_EmptyServerRejected(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := newM.(model)

	if got.step != stepServer {
		t.Errorf("step = %v, want stepServer (should not advance)", got.step)
	}
	if got.err == "" {
		t.Error("expected an error message for empty server URL")
	}
}

func TestUpdate_ServerMissingSchemeRejected(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)
	m = typeText(m, "link.example.com")

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := newM.(model)

	if got.step != stepServer {
		t.Errorf("step = %v, want stepServer (should not advance)", got.step)
	}
	if !strings.Contains(got.err, "http://") {
		t.Errorf("err = %q, want mention of required scheme", got.err)
	}
}

func TestUpdate_ValidServerAdvancesToToken(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)
	m = typeText(m, "https://link.example.com/")

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := newM.(model)

	if got.step != stepToken {
		t.Errorf("step = %v, want stepToken", got.step)
	}
	if got.err != "" {
		t.Errorf("err = %q, want empty", got.err)
	}
	// Trailing slash must be stripped before being persisted to cfg.
	if cfg.Server != "https://link.example.com" {
		t.Errorf("cfg.Server = %q, want trailing slash stripped", cfg.Server)
	}
}

func TestUpdate_TabAdvancesFromServerToToken(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)

	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	got := newM.(model)

	if got.step != stepToken {
		t.Errorf("step = %v, want stepToken", got.step)
	}
}

func TestUpdate_EscQuits(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)

	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	got := newM.(model)

	if !got.done {
		t.Error("done = false, want true after Esc")
	}
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("cmd() = %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdate_CtrlCQuits(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)

	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := newM.(model)

	if !got.done {
		t.Error("done = false, want true after Ctrl+C")
	}
	if cmd == nil {
		t.Fatal("expected a quit command, got nil")
	}
}

func TestUpdate_TokenStepEnterTriggersConnectionTest(t *testing.T) {
	cfg := &config.CLIConfig{Server: "https://link.example.com"}
	m := initialModel(cfg)
	// Advance to token step first.
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(model)
	m = typeText(m, "sometoken")

	newM2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := newM2.(model)

	if got.status != "Testing connection..." {
		t.Errorf("status = %q, want %q", got.status, "Testing connection...")
	}
	if cmd == nil {
		t.Fatal("expected testConnectionCmd to be returned")
	}
	if cfg.Token != "sometoken" {
		t.Errorf("cfg.Token = %q, want %q", cfg.Token, "sometoken")
	}
}

func TestUpdate_ConnResultMsg_Error(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)
	m.step = stepToken
	m.status = "Testing connection..."

	newM, _ := m.Update(connResultMsg{err: fmt.Errorf("connection refused")})
	got := newM.(model)

	if got.status != "" {
		t.Errorf("status = %q, want cleared on error", got.status)
	}
	if !strings.Contains(got.err, "connection refused") {
		t.Errorf("err = %q, want it to mention the underlying error", got.err)
	}
	if got.done {
		t.Error("done = true, want false on connection error")
	}
}

func TestUpdate_ConnResultMsg_SuccessSavesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	cfg := &config.CLIConfig{Server: "https://link.example.com"}
	m := initialModel(cfg)
	m.step = stepToken

	newM, cmd := m.Update(connResultMsg{})
	got := newM.(model)

	if !got.saved {
		t.Error("saved = false, want true after successful connection test")
	}
	if !got.done {
		t.Error("done = false, want true after successful connection test")
	}
	if cmd == nil {
		t.Fatal("expected a quit command after save")
	}

	reloaded, err := config.LoadCLIConfig()
	if err != nil {
		t.Fatalf("LoadCLIConfig() error = %v", err)
	}
	if reloaded.Server != cfg.Server {
		t.Errorf("persisted Server = %q, want %q", reloaded.Server, cfg.Server)
	}
}

func TestView_DoneAndSavedShowsSummary(t *testing.T) {
	cfg := &config.CLIConfig{Server: "https://link.example.com"}
	m := initialModel(cfg)
	m.done = true
	m.saved = true

	out := m.View()
	if !strings.Contains(out, "Configuration saved.") {
		t.Errorf("View() = %q, want save confirmation", out)
	}
	if !strings.Contains(out, cfg.Server) {
		t.Errorf("View() = %q, want it to include the server URL", out)
	}
}

func TestView_ShowsErrorMessage(t *testing.T) {
	cfg := &config.CLIConfig{}
	m := initialModel(cfg)
	m.err = "Server URL cannot be empty."

	out := m.View()
	if !strings.Contains(out, "Server URL cannot be empty.") {
		t.Errorf("View() = %q, want it to include the error", out)
	}
}
