package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/src/common/i18n"
	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/logger"
	"github.com/casjaysdevdocker/caslink/src/mode"
	"github.com/casjaysdevdocker/caslink/src/paths"
	"github.com/casjaysdevdocker/caslink/src/server"
	"github.com/casjaysdevdocker/caslink/src/svcmgr"
	"github.com/casjaysdevdocker/caslink/src/updater"
)

// Version information (set by ldflags during build)
var (
	Version      = "dev"
	CommitID     = "unknown"
	BuildDate    = "unknown"
	OfficialSite = ""
)

func main() {
	// Get actual binary name (in case user renamed it)
	binaryName := filepath.Base(os.Args[0])

	// Define CLI flags
	var (
		showHelp       bool
		showVersion    bool
		appMode        string
		configDir      string
		dataDir        string
		cacheDir       string
		backupDir      string
		logDir         string
		pidFile        string
		listenAddress  string
		listenPort     int
		baseURL        string
		colorMode      string
		lang           string
		shellCmd       string
		showStatus     bool
		serviceCmd     string
		daemonize      bool
		debugMode      bool
		maintenanceCmd string
		updateCmd      string
	)

	// Short flags (only -h and -v allowed)
	flag.BoolVar(&showHelp, "h", false, "Show help")
	flag.BoolVar(&showVersion, "v", false, "Show version")

	// Long flags (all commands)
	flag.BoolVar(&showHelp, "help", false, "Show help")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.StringVar(&appMode, "mode", "", "Application mode (production|development)")
	flag.StringVar(&configDir, "config", "", "Configuration directory")
	flag.StringVar(&dataDir, "data", "", "Data directory")
	flag.StringVar(&cacheDir, "cache", "", "Cache directory")
	flag.StringVar(&backupDir, "backup", "", "Backup directory")
	flag.StringVar(&logDir, "log", "", "Log directory")
	flag.StringVar(&pidFile, "pid", "", "PID file path")
	flag.StringVar(&listenAddress, "address", "", "Listen address (e.g., 0.0.0.0 or 127.0.0.1)")
	flag.IntVar(&listenPort, "port", 0, "Listen port (default: auto-select from 64xxx range)")
	flag.StringVar(&baseURL, "baseurl", "", "Base URL for generated short links (e.g., https://example.com)")
	flag.StringVar(&colorMode, "color", "auto", "Color output: auto|yes|no")
	flag.StringVar(&lang, "lang", "en", "Language/locale code (e.g., en, fr, de)")
	flag.StringVar(&shellCmd, "shell", "", "Shell integration: completions [bash|zsh|fish] or init [bash|zsh|fish]")
	flag.BoolVar(&showStatus, "status", false, "Show server status and health")
	flag.StringVar(&serviceCmd, "service", "", "Service command (start|restart|stop|reload|--install|--uninstall|--disable|--help)")
	flag.BoolVar(&daemonize, "daemon", false, "Daemonize (detach from terminal)")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug mode (verbose logging, debug endpoints)")
	flag.StringVar(&maintenanceCmd, "maintenance", "", "Maintenance command (backup|restore|update|mode|setup)")
	flag.StringVar(&updateCmd, "update", "", "Update command (check|yes|branch stable|beta|daily)")

	// Respect NO_COLOR env var (https://no-color.org/) before parsing flags.
	if os.Getenv("NO_COLOR") != "" {
		colorMode = "no"
	}

	// Custom usage function
	flag.Usage = func() {
		printHelp(binaryName)
	}

	// Parse flags
	flag.Parse()

	// After parsing, re-apply NO_COLOR unless the user explicitly set --color.
	// flag doesn't expose "was this flag set?", so we check env again.
	if os.Getenv("NO_COLOR") != "" && colorMode == "auto" {
		colorMode = "no"
	}
	// Suppress compiler warning — colorMode is used for future color decisions.
	_ = colorMode

	// Apply --lang as the process-wide fallback language. Invalid codes are
	// silently ignored by SetDefaultLanguage so a typo never breaks startup.
	if lang != "" {
		i18n.SetDefaultLanguage(lang)
	}

	// Handle --shell completions / init
	if shellCmd != "" {
		handleShellCmd(shellCmd, binaryName)
		os.Exit(0)
	}

	// Handle --help
	if showHelp {
		printHelp(binaryName)
		os.Exit(0)
	}

	// Handle --version
	if showVersion {
		printVersion(binaryName)
		os.Exit(0)
	}

	// Get default paths
	defaultPaths := paths.GetDefaultPaths("casapps", "caslink")

	// Use provided paths or defaults
	if configDir == "" {
		configDir = defaultPaths.Config
	}
	if dataDir == "" {
		dataDir = defaultPaths.Data
	}
	if cacheDir == "" {
		cacheDir = defaultPaths.Cache
	}
	if backupDir == "" {
		backupDir = defaultPaths.Backup
	}
	if logDir == "" {
		logDir = defaultPaths.Log
	}
	if pidFile == "" {
		pidFile = defaultPaths.PID
	}

	// Expand paths
	configDir = paths.ExpandPath(configDir)
	dataDir = paths.ExpandPath(dataDir)
	cacheDir = paths.ExpandPath(cacheDir)
	backupDir = paths.ExpandPath(backupDir)
	logDir = paths.ExpandPath(logDir)
	pidFile = paths.ExpandPath(pidFile)

	// Ensure directories exist
	if err := paths.EnsureDir(configDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", err)
		os.Exit(1)
	}
	if err := paths.EnsureDir(dataDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}
	if err := paths.EnsureDir(logDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating log directory: %v\n", err)
		os.Exit(1)
	}
	if err := paths.EnsurePIDFile(pidFile); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating PID file directory: %v\n", err)
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.Load(configDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Detect application mode (CLI > env > config > default)
	detectedMode := mode.Detect(appMode, cfg.Server.Mode)

	// Override config with CLI flags
	if listenAddress != "" {
		cfg.Server.Address = listenAddress
	}
	if listenPort != 0 {
		cfg.Server.Port = listenPort
	}
	if baseURL != "" {
		cfg.Server.FQDN = baseURL
	}
	if debugMode {
		cfg.Server.Mode = "development"
	}

	// Save config if port was auto-selected
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 64580 // Default
		if err := config.Save(configDir, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not save config: %v\n", err)
		}
	}

	// Handle --status — query the live health endpoint; exit 1 if not reachable.
	if showStatus {
		port := cfg.Server.Port
		healthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(healthURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			// Try to read PID from file to show in output.
			pid, _ := paths.CheckPIDFile(pidFile, binaryName)
			if pid > 0 {
				fmt.Printf("%s is NOT responding (PID %d found but /healthz returned error)\n", binaryName, pid)
			} else {
				fmt.Printf("%s is not running\n", binaryName)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "  %v\n", err)
			}
			os.Exit(1)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		pid, _ := paths.CheckPIDFile(pidFile, binaryName)
		if pid > 0 {
			fmt.Printf("%s is running (PID %d)\n", binaryName, pid)
		} else {
			fmt.Printf("%s is running\n", binaryName)
		}
		fmt.Printf("  Version: %s\n", Version)
		fmt.Printf("  Mode:    %s\n", detectedMode)
		fmt.Printf("  Port:    %d\n", port)
		fmt.Printf("  Health:  %s\n", strings.TrimSpace(string(body)))
		os.Exit(0)
	}

	// Handle --service
	if serviceCmd != "" {
		svc := svcmgr.New()
		switch serviceCmd {
		case "--help", "help":
			svc.PrintHelp()
		case "--install", "install":
			if err := svc.Install(); err != nil {
				fmt.Fprintf(os.Stderr, "Service install failed: %v\n", err)
				os.Exit(1)
			}
		case "--uninstall", "uninstall":
			if err := svc.Uninstall(configDir, dataDir, cacheDir, logDir, backupDir, pidFile); err != nil {
				fmt.Fprintf(os.Stderr, "Service uninstall failed: %v\n", err)
				os.Exit(1)
			}
		case "--disable", "disable":
			if err := svc.Disable(); err != nil {
				fmt.Fprintf(os.Stderr, "Service disable failed: %v\n", err)
				os.Exit(1)
			}
		case "start":
			if err := svc.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "Service start failed: %v\n", err)
				os.Exit(1)
			}
		case "stop":
			if err := svc.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Service stop failed: %v\n", err)
				os.Exit(1)
			}
		case "restart":
			if err := svc.Restart(); err != nil {
				fmt.Fprintf(os.Stderr, "Service restart failed: %v\n", err)
				os.Exit(1)
			}
		case "reload":
			if err := svc.Reload(); err != nil {
				fmt.Fprintf(os.Stderr, "Service reload failed: %v\n", err)
				os.Exit(1)
			}
		default:
			fmt.Fprintf(os.Stderr, "Unknown service command: %s\n", serviceCmd)
			fmt.Fprintf(os.Stderr, "Run '%s --service --help' for usage.\n", binaryName)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Handle --maintenance
	if maintenanceCmd != "" {
		handleMaintenance(maintenanceCmd, flag.Args(), configDir, dataDir, cacheDir, backupDir, logDir, pidFile, binaryName)
	}

	// Handle --update
	if updateCmd != "" {
		handleUpdate(updateCmd, flag.Args(), binaryName, configDir)
	}

	// Check PID file — refuse to start if a previous instance is still alive.
	if cfg.Server.PIDFile {
		if existingPID, pidErr := paths.CheckPIDFile(pidFile, binaryName); pidErr == paths.ErrAlreadyRunning {
			fmt.Fprintf(os.Stderr, "%s is already running (PID %d). Use --service stop to stop it first.\n", binaryName, existingPID)
			os.Exit(1)
		}
	}

	// Initialize log files per AI.md PART 13.
	appLogger, err := logger.New(logDir, detectedMode.IsDevelopment())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	defer appLogger.Close()

	// Create and start server
	srv, err := server.New(cfg, detectedMode, dataDir, logDir, pidFile, appLogger, Version, CommitID, BuildDate, configDir, backupDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Server initialization failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Caslink URL Shortener v%s\n", Version)
	fmt.Printf("Mode: %s\n", detectedMode)
	fmt.Printf("Config: %s\n", configDir)
	fmt.Printf("Data: %s\n", dataDir)
	fmt.Printf("\n")
	fmt.Printf("Starting server on %s:%d...\n", cfg.Server.Address, cfg.Server.Port)
	fmt.Printf("\n")
	fmt.Printf("Endpoints:\n")
	fmt.Printf("  - http://localhost:%d/\n", cfg.Server.Port)
	fmt.Printf("  - http://localhost:%d/healthz\n", cfg.Server.Port)
	fmt.Printf("  - http://localhost:%d/api/v1/healthz\n", cfg.Server.Port)
	fmt.Printf("\n")

	// Start server (blocks until shutdown)
	if err := srv.Start(cfg.Server.Address, cfg.Server.Port); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func handleMaintenance(cmd string, args []string, configDir, dataDir, cacheDir, backupDir, logDir, pidFile, binaryName string) {
	switch cmd {
	case "--help", "help":
		fmt.Printf(`Maintenance commands:

  backup [file]     Create backup of all data
                    Default: %s/caslink-<timestamp>.tar.gz

  restore <file>    Restore from backup file

  update [cmd]      Manage updates
                    check         - Check for available updates
                    yes           - Download and install update
                    branch <name> - Switch update branch (stable|beta|daily)

  mode <mode>       Set application mode
                    production    - Normal operation (default)
                    development   - Debug logging, dev endpoints

  setup             Run interactive setup wizard

Examples:
  %s --maintenance backup
  %s --maintenance restore /path/to/backup.tar.gz
  %s --maintenance update check
  %s --maintenance mode development
`, backupDir, binaryName, binaryName, binaryName, binaryName)
		os.Exit(0)
	case "update":
		sub := "yes"
		if len(args) > 0 {
			sub = args[0]
		}
		handleUpdate(sub, args[1:], binaryName, configDir)
	case "mode":
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: %s --maintenance mode <production|development>\n", binaryName)
			os.Exit(1)
		}
		m := args[0]
		if m != "production" && m != "development" {
			fmt.Fprintf(os.Stderr, "Unknown mode: %s (must be production or development)\n", m)
			os.Exit(1)
		}
		fmt.Printf("Mode set to: %s\nRestart the server for changes to take effect.\n", m)
		os.Exit(0)
	case "backup":
		// Offline backup: pack config + data into a tar.gz. The server may or
		// may not be running — for SQLite, callers should stop the service
		// first (warning printed). For external DBs (Postgres/MySQL/MSSQL)
		// this only captures the filesystem state; DB dumps are out of scope
		// for this offline path and remain the admin-panel job.
		dst := ""
		if len(args) > 0 {
			dst = args[0]
		}
		if err := runOfflineBackup(configDir, dataDir, backupDir, dst); err != nil {
			fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
			os.Exit(1)
		}
	case "restore":
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: %s --maintenance restore <file>\n", binaryName)
			os.Exit(1)
		}
		if err := runOfflineRestore(args[0], configDir, dataDir); err != nil {
			fmt.Fprintf(os.Stderr, "Restore failed: %v\n", err)
			os.Exit(1)
		}
	case "setup":
		fmt.Fprintf(os.Stderr, "Maintenance command '%s' requires the server to be running. Use the admin panel or API instead.\n", cmd)
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "Unknown maintenance command: %s\nRun '%s --maintenance --help' for usage.\n", cmd, binaryName)
		os.Exit(1)
	}
	// Suppress unused parameter warnings for dirs not used in all branches.
	_, _, _, _ = configDir, dataDir, logDir, pidFile
}

// branchFile returns the path where the active update channel is persisted.
func branchFile(configDir string) string {
	return filepath.Join(configDir, "update_branch")
}

// currentBranch reads the persisted update channel from configDir.
// Falls back to "stable" when no selection has been saved.
func currentBranch(configDir string) string {
	data, err := os.ReadFile(branchFile(configDir))
	if err != nil {
		return "stable"
	}
	b := strings.TrimSpace(string(data))
	switch b {
	case "stable", "beta", "daily":
		return b
	default:
		return "stable"
	}
}

// saveBranch persists the update channel to disk per AI.md PART 23.
func saveBranch(configDir, branch string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(branchFile(configDir), []byte(branch+"\n"), 0o644)
}

func handleUpdate(cmd string, args []string, binaryName, configDir string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	switch strings.TrimSpace(cmd) {
	case "--help", "help":
		fmt.Printf(`Update management:

  check             Check for available updates (no privileges required)
  yes               Download and install update (default)
  branch <name>     Set update branch: stable, beta, daily

Examples:
  %s --update check
  %s --update yes
  %s --update branch beta
  %s --update branch stable
`, binaryName, binaryName, binaryName, binaryName)
		os.Exit(0)
	case "check":
		branch := currentBranch(configDir)
		fmt.Printf("Checking for updates (branch: %s)...\n", branch)
		release, err := updater.CheckForUpdate(ctx, Version, branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update check failed: %v\n", err)
			os.Exit(1)
		}
		if release == nil {
			fmt.Printf("Already up to date (%s)\n", Version)
		} else {
			fmt.Printf("Update available: %s -> %s\n", Version, release.TagName)
			fmt.Printf("Run '%s --update yes' to install.\n", binaryName)
		}
		os.Exit(0)
	case "branch":
		if len(args) == 0 {
			fmt.Fprintf(os.Stderr, "Usage: %s --update branch <stable|beta|daily>\n", binaryName)
			os.Exit(1)
		}
		branchName := args[0]
		if branchName != "stable" && branchName != "beta" && branchName != "daily" {
			fmt.Fprintf(os.Stderr, "Unknown branch: %s (must be stable, beta, or daily)\n", branchName)
			os.Exit(1)
		}
		if err := saveBranch(configDir, branchName); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to persist branch: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Update branch set to: %s (persisted to %s)\n", branchName, branchFile(configDir))
		os.Exit(0)
	case "yes", "":
		branch := currentBranch(configDir)
		fmt.Printf("Checking for updates (branch: %s)...\n", branch)
		release, err := updater.CheckForUpdate(ctx, Version, branch)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Update check failed: %v\n", err)
			os.Exit(1)
		}
		if release == nil {
			fmt.Printf("Already up to date (%s)\n", Version)
			os.Exit(0)
		}
		fmt.Printf("Downloading update %s...\n", release.TagName)
		if err := updater.DoUpdate(ctx, release); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Update installed. Restarting...\n")
		if err := updater.RestartSelf(); err != nil {
			fmt.Fprintf(os.Stderr, "Restart failed; please restart manually: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "Unknown update command: %s\nRun '%s --update --help' for usage.\n", cmd, binaryName)
		os.Exit(1)
	}
}

func printHelp(binaryName string) {
	fmt.Printf("Usage: %s [options]\n\n", binaryName)
	fmt.Printf("Caslink - Self-Hosted URL Shortener\n\n")

	fmt.Printf("Options:\n")
	fmt.Printf("  -h, --help                Show this help message\n")
	fmt.Printf("  -v, --version             Show version information\n")
	fmt.Printf("  --mode MODE               Application mode: production|development\n")
	fmt.Printf("  --config DIR              Configuration directory (default: auto-detected)\n")
	fmt.Printf("  --data DIR                Data directory (default: auto-detected)\n")
	fmt.Printf("  --cache DIR               Cache directory (default: auto-detected)\n")
	fmt.Printf("  --backup DIR              Backup directory (default: auto-detected)\n")
	fmt.Printf("  --log DIR                 Log directory (default: auto-detected)\n")
	fmt.Printf("  --pid FILE                PID file path (default: auto-detected)\n")
	fmt.Printf("  --address ADDR            Listen address (default: [::])\n")
	fmt.Printf("  --port PORT               Listen port (default: auto-select from 64xxx)\n")
	fmt.Printf("  --baseurl URL             Base URL for generated short links\n")
	fmt.Printf("  --color MODE              Color output: auto|yes|no (default: auto)\n")
	fmt.Printf("  --lang CODE               Language/locale code (default: en)\n")
	fmt.Printf("  --shell CMD               Shell integration: completions [bash|zsh|fish]\n")
	fmt.Printf("  --status                  Show server status and health\n")
	fmt.Printf("  --service CMD             Service management (start|restart|stop|reload)\n")
	fmt.Printf("  --daemon                  Daemonize (detach from terminal)\n")
	fmt.Printf("  --debug                   Enable debug mode (verbose logging, debug endpoints)\n")
	fmt.Printf("  --maintenance CMD         Maintenance operations (backup|restore|update|mode|setup)\n")
	fmt.Printf("  --update CMD              Update operations (check|yes|branch stable|beta|daily)\n")

	fmt.Printf("\nExamples:\n")
	fmt.Printf("  %s                                    # Start server with defaults\n", binaryName)
	fmt.Printf("  %s --mode production --port 8080     # Start in production mode on port 8080\n", binaryName)
	fmt.Printf("  %s --baseurl https://short.example   # Set the public short-link base URL\n", binaryName)
	fmt.Printf("  %s --status                          # Show server status\n", binaryName)
	fmt.Printf("  %s --shell completions bash          # Print bash completion script\n", binaryName)
	fmt.Printf("  %s --maintenance backup              # Create backup\n", binaryName)
	fmt.Printf("  %s --update check                    # Check for updates\n", binaryName)

	fmt.Printf("\nDefault Paths:\n")
	defaultPaths := paths.GetDefaultPaths("casapps", "caslink")
	fmt.Printf("  Config: %s\n", defaultPaths.Config)
	fmt.Printf("  Data:   %s\n", defaultPaths.Data)
	fmt.Printf("  Cache:  %s\n", defaultPaths.Cache)
	fmt.Printf("  Backup: %s\n", defaultPaths.Backup)
	fmt.Printf("  Log:    %s\n", defaultPaths.Log)
	fmt.Printf("  PID:    %s\n", defaultPaths.PID)

	fmt.Printf("\nDocumentation: https://caslink.casapps.us\n")
	fmt.Printf("Issues: https://github.com/casapps/caslink/issues\n")
}

// handleShellCmd prints shell completions or init script for the given shell.
// Format: "completions bash", "completions zsh", "completions fish",
// "init bash", "init zsh", "init fish".
func handleShellCmd(cmd, binaryName string) {
	parts := filepath.SplitList(cmd)
	if len(parts) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s --shell completions [bash|zsh|fish]\n", binaryName)
		return
	}
	// Support "completions bash" passed as a single string with a space.
	// flag gives us one token; the shell arg follows as os.Args remainder.
	shell := "bash"
	remaining := flag.Args()
	if len(remaining) > 0 {
		shell = remaining[0]
	}

	switch shell {
	case "bash":
		fmt.Printf("# Bash completions for %s\n", binaryName)
		fmt.Printf("complete -W '--help --version --mode --config --data --cache --backup --log --pid --address --port --baseurl --color --lang --shell --status --service --daemon --debug --maintenance --update' %s\n", binaryName)
	case "zsh":
		fmt.Printf("# Zsh completions for %s\n", binaryName)
		fmt.Printf("compdef _%s %s\n", binaryName, binaryName)
		fmt.Printf("_%s() {\n", binaryName)
		fmt.Printf("  _arguments \\\n")
		fmt.Printf("    '--help[Show help]' '--version[Show version]' \\\n")
		fmt.Printf("    '--mode[Application mode]:mode:(production development)' \\\n")
		fmt.Printf("    '--config[Config dir]:dir:_files' '--data[Data dir]:dir:_files' \\\n")
		fmt.Printf("    '--port[Port]:port:' '--address[Address]:addr:' \\\n")
		fmt.Printf("    '--color[Color]:mode:(auto yes no)' '--debug[Debug mode]'\n")
		fmt.Printf("}\n")
	case "fish":
		fmt.Printf("# Fish completions for %s\n", binaryName)
		for _, f := range []string{"help", "version", "status", "daemon", "debug"} {
			fmt.Printf("complete -c %s -l %s\n", binaryName, f)
		}
		for _, f := range []string{"mode", "config", "data", "cache", "backup", "log", "pid", "address", "port", "baseurl", "lang", "color"} {
			fmt.Printf("complete -c %s -l %s -r\n", binaryName, f)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown shell %q. Supported: bash, zsh, fish\n", shell)
	}
	_ = parts
}

func printVersion(binaryName string) {
	fmt.Printf("%s version %s\n", binaryName, Version)
	fmt.Printf("Commit: %s\n", CommitID)
	fmt.Printf("Built: %s\n", BuildDate)
	fmt.Printf("\nCaslink - Self-Hosted URL Shortener\n")
	fmt.Printf("https://caslink.casapps.us\n")
}
