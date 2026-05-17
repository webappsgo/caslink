// caslink-cli is the companion CLI/TUI client for the caslink URL shortener server.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/casjaysdevdocker/caslink/src/client/cli"
	clientcfg "github.com/casjaysdevdocker/caslink/src/client/config"
	"github.com/casjaysdevdocker/caslink/src/client/setup"
	"github.com/casjaysdevdocker/caslink/src/client/tui"
	"github.com/casjaysdevdocker/caslink/src/common/display"
)

// Version information — populated by ldflags at build time.
var (
	Version      = "dev"
	CommitID     = "unknown"
	BuildDate    = "unknown"
	OfficialSite = ""
)

func main() {
	args := os.Args[1:]

	// Fast-path: -h / --help
	if hasFlag(args, "-h", "--help") {
		printHelp()
		os.Exit(0)
	}

	// Fast-path: -v / --version
	if hasFlag(args, "-v", "--version") {
		printVersion()
		os.Exit(0)
	}

	// --shell <bash|zsh|fish>: print shell completions and exit.
	if shellFlag := flagValue(args, "--shell"); shellFlag != "" {
		printCompletions(shellFlag)
		os.Exit(0)
	}

	// --update [check|yes|branch <channel>]: update operations.
	if idx := flagIndex(args, "--update"); idx >= 0 {
		handleUpdate(args[idx:])
		os.Exit(0)
	}

	// Load or create config.
	cfg, err := clientcfg.LoadCLIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Apply flag overrides for server / token before any other logic.
	applyFlagOverrides(cfg, args)

	// Detect display environment.
	denv := display.DetectDisplayEnv()

	// When no server is configured and we have an interactive terminal,
	// run the first-run setup wizard.
	if cfg.Server == "" && denv.IsTerminal {
		cfg, err = setup.Run(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Setup failed: %v\n", err)
			os.Exit(1)
		}
		if cfg.Server == "" {
			fmt.Fprintln(os.Stderr, "No server configured. Run again to set one up.")
			os.Exit(1)
		}
	}

	// Determine effective output color setting.
	colorMode := cfg.Color
	if c := flagValue(args, "--color"); c != "" {
		colorMode = c
	}
	_ = colorMode

	// Determine effective output format.
	outputFmt := "table"
	if o := flagValue(args, "--output"); o != "" {
		outputFmt = o
	}

	// Determine debug flag.
	debug := hasFlag(args, "--debug")

	// If no subcommand tokens remain (after stripping known global flags),
	// decide between TUI and CLI mode.
	nonFlagArgs := stripGlobalFlags(args)

	if len(nonFlagArgs) == 0 {
		if denv.IsTerminal && !denv.IsAutoDetectDisplayModeHeadless() {
			// Interactive terminal, no command → launch TUI.
			p := tui.NewApp(cfg)
			if _, err := p.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		// Non-interactive, no command → print help.
		printHelp()
		os.Exit(0)
	}

	// CLI mode: hand off to cobra.
	gf := &cli.GlobalFlags{
		Output: outputFmt,
		Debug:  debug,
	}
	// Re-apply explicit flags from CLI.
	if s := flagValue(args, "--server"); s != "" {
		gf.Server = s
	}
	if t := flagValue(args, "--token"); t != "" {
		gf.Token = t
	} else {
		gf.Token = resolveToken(cfg, args)
	}

	rootCmd := cli.BuildRootCmd(cfg, gf)
	cli.SetVersionString(func() string { return fullVersionString() })

	// Pass only the non-global-flag arguments to cobra.
	rootCmd.SetArgs(nonFlagArgs)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// applyFlagOverrides overlays CLI flag values onto cfg before further use.
func applyFlagOverrides(cfg *clientcfg.CLIConfig, args []string) {
	if v := flagValue(args, "--lang"); v != "" {
		cfg.Lang = v
	}
	if v := flagValue(args, "--color"); v != "" {
		cfg.Color = v
	}
	// --server and --token are handled per-command via globalFlags.
}

// resolveToken returns the token to use, in priority order:
//  1. --token flag (already handled by caller)
//  2. --token-file flag
//  3. CASLINK_TOKEN env var
//  4. cli.yml token field
//  5. ~/.config/casapps/caslink/token file
func resolveToken(cfg *clientcfg.CLIConfig, args []string) string {
	// --token-file
	if tf := flagValue(args, "--token-file"); tf != "" {
		data, err := os.ReadFile(tf)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	// Env var
	if t := os.Getenv("CASLINK_TOKEN"); t != "" {
		return t
	}
	// Config file token
	if cfg.Token != "" {
		return cfg.Token
	}
	// Token file
	tokenFile, err := clientcfg.GetTokenFile()
	if err == nil {
		data, err := os.ReadFile(tokenFile)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return ""
}

// printHelp prints the CLI usage to stdout.
func printHelp() {
	bin := filepath.Base(os.Args[0])
	fmt.Printf(`Usage: %s [flags] [command] [args]

Flags:
  -h, --help               Show this help
  -v, --version            Show version
  --server URL             caslink server URL
  --token TOKEN            API bearer token
  --token-file FILE        Read token from file
  --output FORMAT          Output format: table|json|csv (default: table)
  --user NAME              User/org context (@NAME=user, +NAME=org)
  --debug                  Enable debug output
  --color on|off|auto      Color output (default: auto)
  --lang LANG              Language code (default: en)
  --shell bash|zsh|fish    Print shell completions
  --update [check|yes|branch <stable|beta|daily>]  Update operations

Commands:
  login                    Authenticate with the server
  logout                   Clear saved credentials
  list                     List all links
  create <url>             Create a new short link
  get <code>               Get link details
  delete <code>            Delete a link
  qr <code>                Display QR code in terminal
  stats <code>             Show analytics for a link
  version                  Show version information

When run interactively without a command, launches the TUI dashboard.
`, bin)
}

// printVersion prints the version string to stdout.
func printVersion() {
	fmt.Println(fullVersionString())
}

// fullVersionString returns the complete version string.
func fullVersionString() string {
	s := fmt.Sprintf("caslink-cli %s", Version)
	if CommitID != "unknown" && CommitID != "" {
		s += fmt.Sprintf(" (%s)", CommitID)
	}
	if BuildDate != "unknown" && BuildDate != "" {
		s += fmt.Sprintf(" built %s", BuildDate)
	}
	if OfficialSite != "" {
		s += fmt.Sprintf(" — %s", OfficialSite)
	}
	return s
}

// printCompletions writes shell completion script to stdout.
func printCompletions(shell string) {
	bin := filepath.Base(os.Args[0])
	switch shell {
	case "bash":
		fmt.Printf(`# bash completion for %[1]s
_%[1]s() {
    local cur prev commands
    commands="login logout list create get delete qr stats version"
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    case "$prev" in
        --output) COMPREPLY=($(compgen -W "table json csv" -- "$cur")) ; return ;;
        --color)  COMPREPLY=($(compgen -W "on off auto" -- "$cur")) ; return ;;
        --shell)  COMPREPLY=($(compgen -W "bash zsh fish" -- "$cur")) ; return ;;
        --update) COMPREPLY=($(compgen -W "check yes branch" -- "$cur")) ; return ;;
    esac
    if [[ "$cur" == -* ]]; then
        COMPREPLY=($(compgen -W "--help --version --server --token --token-file --output --user --debug --color --lang --shell --update" -- "$cur"))
    else
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
    fi
}
complete -F _%[1]s %[1]s
`, bin)

	case "zsh":
		fmt.Printf(`# zsh completion for %[1]s
_%[1]s() {
    local -a cmds flags
    cmds=('login:Authenticate' 'logout:Clear credentials' 'list:List links'
          'create:Create link' 'get:Get link' 'delete:Delete link'
          'qr:Show QR code' 'stats:Show stats' 'version:Show version')
    flags=(
        '--help:Show help' '--version:Show version'
        '--server:Server URL' '--token:API token'
        '--token-file:Token file' '--output:Output format'
        '--user:User context' '--debug:Debug mode'
        '--color:Color mode' '--lang:Language'
        '--shell:Shell completions' '--update:Update'
    )
    _describe 'commands' cmds
    _describe 'flags' flags
}
compdef _%[1]s %[1]s
`, bin)

	case "fish":
		fmt.Printf(`# fish completion for %[1]s
complete -c %[1]s -f
complete -c %[1]s -n __fish_use_subcommand -a login     -d 'Authenticate'
complete -c %[1]s -n __fish_use_subcommand -a logout    -d 'Clear credentials'
complete -c %[1]s -n __fish_use_subcommand -a list      -d 'List links'
complete -c %[1]s -n __fish_use_subcommand -a create    -d 'Create link'
complete -c %[1]s -n __fish_use_subcommand -a get       -d 'Get link'
complete -c %[1]s -n __fish_use_subcommand -a delete    -d 'Delete link'
complete -c %[1]s -n __fish_use_subcommand -a qr        -d 'Show QR code'
complete -c %[1]s -n __fish_use_subcommand -a stats     -d 'Show stats'
complete -c %[1]s -n __fish_use_subcommand -a version   -d 'Show version'
complete -c %[1]s -l output  -a 'table json csv' -d 'Output format'
complete -c %[1]s -l color   -a 'on off auto'    -d 'Color mode'
complete -c %[1]s -l shell   -a 'bash zsh fish'  -d 'Print completions'
`, bin)

	default:
		fmt.Fprintf(os.Stderr, "Unknown shell: %s (supported: bash, zsh, fish)\n", shell)
		os.Exit(1)
	}
}

// handleUpdate processes the --update flag.
func handleUpdate(args []string) {
	// args[0] is "--update"; args[1] is the subcommand if present.
	sub := ""
	if len(args) > 1 && !strings.HasPrefix(args[1], "--") {
		sub = args[1]
	}
	switch sub {
	case "check", "":
		fmt.Println("Checking for updates... (not implemented in this build)")
	case "yes":
		fmt.Println("Applying update... (not implemented in this build)")
	case "branch":
		channel := "stable"
		if len(args) > 2 {
			channel = args[2]
		}
		switch channel {
		case "stable", "beta", "daily":
			fmt.Printf("Switching to %s channel... (not implemented in this build)\n", channel)
		default:
			fmt.Fprintf(os.Stderr, "Unknown branch: %s (stable|beta|daily)\n", channel)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown --update subcommand: %s\n", sub)
		os.Exit(1)
	}
}

// stripGlobalFlags removes known global flags and their values from args,
// returning only the subcommand tokens.
func stripGlobalFlags(args []string) []string {
	valuedFlags := map[string]bool{
		"--server":     true,
		"--token":      true,
		"--token-file": true,
		"--output":     true,
		"--user":       true,
		"--color":      true,
		"--lang":       true,
		"--shell":      true,
	}
	boolFlags := map[string]bool{
		"--debug": true,
	}

	var out []string
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if valuedFlags[a] {
			skip = true
			continue
		}
		if boolFlags[a] {
			continue
		}
		// --flag=value style
		isValued := false
		for f := range valuedFlags {
			if strings.HasPrefix(a, f+"=") {
				isValued = true
				break
			}
		}
		if isValued {
			continue
		}
		out = append(out, a)
	}
	return out
}

// hasFlag returns true if any of the given flags appear in args.
func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// flagValue returns the value of a flag from args, supporting "--flag value" and "--flag=value".
func flagValue(args []string, flag string) string {
	prefix := flag + "="
	for i, a := range args {
		if strings.HasPrefix(a, prefix) {
			return a[len(prefix):]
		}
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// flagIndex returns the index of flag in args, or -1 if not found.
func flagIndex(args []string, flag string) int {
	for i, a := range args {
		if a == flag {
			return i
		}
	}
	return -1
}
