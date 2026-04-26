package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/mode"
	"github.com/casjaysdevdocker/caslink/src/paths"
	"github.com/casjaysdevdocker/caslink/src/server"
)

// Version information (set by ldflags during build)
var (
	Version   = "dev"
	CommitID  = "unknown"
	BuildDate = "unknown"
)

func main() {
	// Get actual binary name (in case user renamed it)
	binaryName := filepath.Base(os.Args[0])

	// Define CLI flags
	var (
		showHelp        bool
		showVersion     bool
		appMode         string
		configDir       string
		dataDir         string
		logDir          string
		pidFile         string
		listenAddress   string
		listenPort      int
		showStatus      bool
		serviceCmd      string
		daemonize       bool
		debugMode       bool
		maintenanceCmd  string
		updateCmd       string
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
	flag.StringVar(&logDir, "log", "", "Log directory")
	flag.StringVar(&pidFile, "pid", "", "PID file path")
	flag.StringVar(&listenAddress, "address", "", "Listen address (e.g., 0.0.0.0 or 127.0.0.1)")
	flag.IntVar(&listenPort, "port", 0, "Listen port (default: auto-select from 64xxx range)")
	flag.BoolVar(&showStatus, "status", false, "Show server status and health")
	flag.StringVar(&serviceCmd, "service", "", "Service command (start|restart|stop|reload|--install|--uninstall|--disable|--help)")
	flag.BoolVar(&daemonize, "daemon", false, "Daemonize (detach from terminal)")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug mode (verbose logging, debug endpoints)")
	flag.StringVar(&maintenanceCmd, "maintenance", "", "Maintenance command (backup|restore|update|mode|setup)")
	flag.StringVar(&updateCmd, "update", "", "Update command (check|yes|branch stable|beta|daily)")

	// Custom usage function
	flag.Usage = func() {
		printHelp(binaryName)
	}

	// Parse flags
	flag.Parse()

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
	if logDir == "" {
		logDir = defaultPaths.Log
	}
	if pidFile == "" {
		pidFile = defaultPaths.PID
	}

	// Expand paths
	configDir = paths.ExpandPath(configDir)
	dataDir = paths.ExpandPath(dataDir)
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

	// Save config if port was auto-selected
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 64580 // Default
		if err := config.Save(configDir, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Could not save config: %v\n", err)
		}
	}

	// Handle --status
	if showStatus {
		fmt.Printf("Caslink v%s\n", Version)
		fmt.Printf("Status: Running\n")
		fmt.Printf("Mode: %s\n", detectedMode)
		fmt.Printf("Config: %s\n", configDir)
		fmt.Printf("Data: %s\n", dataDir)
		fmt.Printf("Log: %s\n", logDir)
		fmt.Printf("PID: %s\n", pidFile)
		fmt.Printf("Port: %d\n", cfg.Server.Port)
		fmt.Printf("Address: %s\n", cfg.Server.Address)
		os.Exit(0)
	}

	// Handle --service
	if serviceCmd != "" {
		fmt.Printf("Service command not yet implemented: %s\n", serviceCmd)
		os.Exit(0)
	}

	// Handle --maintenance
	if maintenanceCmd != "" {
		fmt.Printf("Maintenance command not yet implemented: %s\n", maintenanceCmd)
		os.Exit(0)
	}

	// Handle --update
	if updateCmd != "" {
		fmt.Printf("Update command not yet implemented: %s\n", updateCmd)
		os.Exit(0)
	}

	// Create and start server
	srv, err := server.New(cfg, detectedMode, dataDir, Version, CommitID, BuildDate)
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

func printHelp(binaryName string) {
	fmt.Printf("Usage: %s [options]\n\n", binaryName)
	fmt.Printf("Caslink - Self-Hosted URL Shortener\n\n")

	fmt.Printf("Options:\n")
	fmt.Printf("  -h, --help                Show this help message\n")
	fmt.Printf("  -v, --version             Show version information\n")
	fmt.Printf("  --mode MODE               Set application mode (production|development)\n")
	fmt.Printf("  --config DIR              Configuration directory (default: auto-detected)\n")
	fmt.Printf("  --data DIR                Data directory (default: auto-detected)\n")
	fmt.Printf("  --log DIR                 Log directory (default: auto-detected)\n")
	fmt.Printf("  --pid FILE                PID file path (default: auto-detected)\n")
	fmt.Printf("  --address ADDR            Listen address (default: 0.0.0.0)\n")
	fmt.Printf("  --port PORT               Listen port (default: auto-select from 64xxx)\n")
	fmt.Printf("  --status                  Show server status and health\n")
	fmt.Printf("  --service CMD             Service management (start|restart|stop|reload)\n")
	fmt.Printf("  --daemon                  Daemonize (detach from terminal)\n")
	fmt.Printf("  --debug                   Enable debug mode\n")
	fmt.Printf("  --maintenance CMD         Maintenance operations (backup|restore|update|mode|setup)\n")
	fmt.Printf("  --update [CMD]            Update operations (check|yes|branch stable|beta|daily)\n")

	fmt.Printf("\nExamples:\n")
	fmt.Printf("  %s                                    # Start server with defaults\n", binaryName)
	fmt.Printf("  %s --mode production --port 8080     # Start in production mode on port 8080\n", binaryName)
	fmt.Printf("  %s --status                          # Show server status\n", binaryName)
	fmt.Printf("  %s --maintenance backup              # Create backup\n", binaryName)
	fmt.Printf("  %s --update check                    # Check for updates\n", binaryName)

	fmt.Printf("\nDefault Paths:\n")
	defaultPaths := paths.GetDefaultPaths("casapps", "caslink")
	fmt.Printf("  Config: %s\n", defaultPaths.Config)
	fmt.Printf("  Data:   %s\n", defaultPaths.Data)
	fmt.Printf("  Log:    %s\n", defaultPaths.Log)
	fmt.Printf("  PID:    %s\n", defaultPaths.PID)

	fmt.Printf("\nDocumentation: https://caslink.casapps.us\n")
	fmt.Printf("Issues: https://github.com/casapps/caslink/issues\n")
}

func printVersion(binaryName string) {
	fmt.Printf("%s version %s\n", binaryName, Version)
	fmt.Printf("Commit: %s\n", CommitID)
	fmt.Printf("Built: %s\n", BuildDate)
	fmt.Printf("\nCaslink - Self-Hosted URL Shortener\n")
	fmt.Printf("https://caslink.casapps.us\n")
}
