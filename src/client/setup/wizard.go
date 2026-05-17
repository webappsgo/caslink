// Package setup provides the first-run TUI setup wizard for caslink-cli.
package setup

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/casjaysdevdocker/caslink/src/client/config"
)

// step tracks which input field is active.
type step int

const (
	stepServer step = iota
	stepToken
	stepDone
)

// model is the bubbletea model for the setup wizard.
type model struct {
	step       step
	serverInput textinput.Model
	tokenInput  textinput.Model
	status      string
	err         string
	done        bool
	saved       bool
	cfg         *config.CLIConfig
}

// initialModel returns a freshly initialised wizard model.
func initialModel(cfg *config.CLIConfig) model {
	server := textinput.New()
	server.Placeholder = "https://link.example.com"
	server.Focus()
	server.CharLimit = 512
	server.Width = 60

	token := textinput.New()
	token.Placeholder = "Optional — press Enter to skip"
	token.CharLimit = 256
	token.Width = 60
	token.EchoMode = textinput.EchoPassword
	token.EchoCharacter = '•'

	if cfg.Server != "" {
		server.SetValue(cfg.Server)
	}

	return model{
		step:        stepServer,
		serverInput: server,
		tokenInput:  token,
		cfg:         cfg,
	}
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit

		case "enter":
			switch m.step {
			case stepServer:
				srv := strings.TrimRight(strings.TrimSpace(m.serverInput.Value()), "/")
				if srv == "" {
					m.err = "Server URL cannot be empty."
					return m, nil
				}
				if !strings.HasPrefix(srv, "http://") && !strings.HasPrefix(srv, "https://") {
					m.err = "Server URL must start with http:// or https://"
					return m, nil
				}
				m.err = ""
				m.cfg.Server = srv
				m.step = stepToken
				m.tokenInput.Focus()
				m.serverInput.Blur()
				return m, textinput.Blink

			case stepToken:
				tok := strings.TrimSpace(m.tokenInput.Value())
				if tok != "" {
					m.cfg.Token = tok
				}
				m.status = "Testing connection..."
				return m, testConnectionCmd(m.cfg.Server)
			}

		case "tab":
			if m.step == stepServer {
				m.step = stepToken
				m.tokenInput.Focus()
				m.serverInput.Blur()
				return m, textinput.Blink
			}
		}

	case connResultMsg:
		if msg.err != nil {
			m.err = fmt.Sprintf("Connection failed: %v", msg.err)
			m.status = ""
			return m, nil
		}
		m.status = "Connection successful. Saving configuration..."
		if err := config.SaveCLIConfig(m.cfg); err != nil {
			m.err = fmt.Sprintf("Failed to save config: %v", err)
			m.status = ""
			return m, nil
		}
		m.saved = true
		m.done = true
		return m, tea.Quit
	}

	switch m.step {
	case stepServer:
		m.serverInput, cmd = m.serverInput.Update(msg)
	case stepToken:
		m.tokenInput, cmd = m.tokenInput.Update(msg)
	}

	return m, cmd
}

func (m model) View() string {
	if m.done && m.saved {
		return fmt.Sprintf(
			"\n  Configuration saved.\n  Server: %s\n\n",
			m.cfg.Server,
		)
	}

	var b strings.Builder
	b.WriteString("\n  caslink-cli — First Run Setup\n")
	b.WriteString("  ─────────────────────────────────────\n\n")
	b.WriteString("  Enter the URL of your caslink server:\n")
	b.WriteString("  ")
	b.WriteString(m.serverInput.View())
	b.WriteString("\n\n")
	b.WriteString("  API token (leave empty to authenticate later):\n")
	b.WriteString("  ")
	b.WriteString(m.tokenInput.View())
	b.WriteString("\n\n")

	if m.err != "" {
		b.WriteString(fmt.Sprintf("  Error: %s\n\n", m.err))
	}
	if m.status != "" {
		b.WriteString(fmt.Sprintf("  %s\n\n", m.status))
	}

	b.WriteString("  [Enter] confirm  [Esc] cancel\n\n")
	return b.String()
}

// connResultMsg is sent back to the model after a connection test.
type connResultMsg struct{ err error }

// testConnectionCmd returns a bubbletea command that pings the server healthz endpoint.
func testConnectionCmd(server string) tea.Cmd {
	return func() tea.Msg {
		url := strings.TrimRight(server, "/") + "/server/healthz"
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(url) //nolint:noctx
		if err != nil {
			return connResultMsg{err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return connResultMsg{err: fmt.Errorf("server returned HTTP %d", resp.StatusCode)}
		}
		return connResultMsg{}
	}
}

// Run starts the TUI setup wizard. It returns when the user saves or cancels.
// The updated cfg is returned; callers should check cfg.Server to detect cancellation.
func Run(cfg *config.CLIConfig) (*config.CLIConfig, error) {
	m := initialModel(cfg)
	p := tea.NewProgram(m)
	result, err := p.Run()
	if err != nil {
		return cfg, fmt.Errorf("setup wizard: %w", err)
	}
	finalModel, ok := result.(model)
	if !ok {
		return cfg, nil
	}
	return finalModel.cfg, nil
}
