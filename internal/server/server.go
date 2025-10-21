package server

import (
	"context"
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/casjaysdevdocker/caslink/internal/db/migrations"
	"github.com/casjaysdevdocker/caslink/internal/proxy"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
)

//go:embed static/css/*.css static/js/*.js templates/*.html
var embeddedFiles embed.FS

// Server represents the HTTP server
type Server struct {
	config     *config.Config
	db         *db.DB
	router     *mux.Router
	httpServer *http.Server
	logger     *logrus.Logger
	proxyDetector *proxy.Detector
}

// New creates a new server instance
func New(cfg *config.Config) (*Server, error) {
	logger := logrus.New()
	logger.SetLevel(getLogLevel(cfg.Logging.Level))
	if cfg.Logging.Format == "json" {
		logger.SetFormatter(&logrus.JSONFormatter{})
	}

	// Initialize database
	database, err := db.New(&cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Run migrations if enabled
	if cfg.Database.AutoMigrate {
		migrationService, err := migrations.NewMigrationService(database, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize migration service: %w", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Database.MigrationTimeout)
		defer cancel()

		if err := migrationService.Migrate(ctx); err != nil {
			return nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	// Initialize proxy detector
	proxyDetector := proxy.NewDetector(&cfg.Server, logger)

	// Determine server port
	port := cfg.Server.Port
	if port == 0 {
		port, err = selectAndPersistPort(database, cfg.Server.Host)
		if err != nil {
			return nil, fmt.Errorf("failed to select port: %w", err)
		}
		cfg.Server.Port = port
	}

	server := &Server{
		config:        cfg,
		db:            database,
		logger:        logger,
		proxyDetector: proxyDetector,
	}

	// Initialize router
	server.setupRouter()

	// Create HTTP server
	server.httpServer = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", cfg.Server.Host, port),
		Handler:        server.router,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}

	return server, nil
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	// Display startup information
	s.displayStartupInfo()

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		s.logger.WithFields(logrus.Fields{
			"host": s.config.Server.Host,
			"port": s.config.Server.Port,
		}).Info("Starting HTTP server")

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down server...")

		// Create shutdown context
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Graceful shutdown
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.WithError(err).Error("Failed to gracefully shutdown server")
			return err
		}

		// Close database connection
		if err := s.db.Close(); err != nil {
			s.logger.WithError(err).Error("Failed to close database connection")
		}

		s.logger.Info("Server stopped")
		return nil

	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}

// setupRouter configures the router with routes and middleware
func (s *Server) setupRouter() {
	s.router = mux.NewRouter()

	// Apply global middleware
	s.router.Use(s.loggingMiddleware)
	s.router.Use(s.proxyHeadersMiddleware)
	s.router.Use(s.securityHeadersMiddleware)
	s.router.Use(s.corsMiddleware)
	s.router.Use(s.rateLimitMiddleware)

	// Static files
	s.setupStaticRoutes()

	// API routes
	s.setupAPIRoutes()

	// Web UI routes
	s.setupWebRoutes()

	// Short URL redirect route (must be last)
	s.router.PathPrefix("/").HandlerFunc(s.handleRedirect)
}

// setupStaticRoutes sets up static file serving
func (s *Server) setupStaticRoutes() {
	// Serve static files from embedded filesystem
	staticFS := http.FS(embeddedFiles)
	staticHandler := http.StripPrefix("/static/", http.FileServer(staticFS))
	s.router.PathPrefix("/static/").Handler(staticHandler)

	// Serve favicon
	s.router.HandleFunc("/favicon.ico", s.handleFavicon).Methods("GET")
}

// setupAPIRoutes sets up API routes
func (s *Server) setupAPIRoutes() {
	api := s.router.PathPrefix("/api/v1").Subrouter()

	// URL management
	api.HandleFunc("/urls", s.handleCreateURL).Methods("POST")
	api.HandleFunc("/urls", s.handleListURLs).Methods("GET")
	api.HandleFunc("/urls/{id}", s.handleGetURL).Methods("GET")
	api.HandleFunc("/urls/{id}", s.handleUpdateURL).Methods("PUT", "PATCH")
	api.HandleFunc("/urls/{id}", s.handleDeleteURL).Methods("DELETE")
	api.HandleFunc("/urls/{id}/analytics", s.handleURLAnalytics).Methods("GET")
	api.HandleFunc("/urls/{id}/qr", s.handleGenerateQR).Methods("GET")

	// Bulk operations
	api.HandleFunc("/urls/bulk", s.handleBulkCreateURLs).Methods("POST")
	api.HandleFunc("/urls/export", s.handleExportURLs).Methods("GET")
	api.HandleFunc("/urls/import", s.handleImportURLs).Methods("POST")

	// Suggestions
	api.HandleFunc("/suggestions", s.handleGetSuggestions).Methods("GET")

	// Health check
	api.HandleFunc("/health", s.handleHealthCheck).Methods("GET")

	// Metrics (if enabled)
	if s.config.Monitoring.EnableMetrics {
		api.HandleFunc("/metrics", s.handleMetrics).Methods("GET")
	}

	// Authentication routes
	api.HandleFunc("/auth/login", s.handleLogin).Methods("POST")
	api.HandleFunc("/auth/logout", s.handleLogout).Methods("POST")
	api.HandleFunc("/auth/register", s.handleRegister).Methods("POST")
	api.HandleFunc("/auth/profile", s.handleGetProfile).Methods("GET")
	api.HandleFunc("/auth/profile", s.handleUpdateProfile).Methods("PUT", "PATCH")

	// Admin routes
	admin := api.PathPrefix("/admin").Subrouter()
	admin.Use(s.adminAuthMiddleware)
	admin.HandleFunc("/users", s.handleListUsers).Methods("GET")
	admin.HandleFunc("/users/{id}", s.handleGetUser).Methods("GET")
	admin.HandleFunc("/users/{id}", s.handleUpdateUser).Methods("PUT", "PATCH")
	admin.HandleFunc("/users/{id}", s.handleDeleteUser).Methods("DELETE")
	admin.HandleFunc("/config", s.handleGetConfig).Methods("GET")
	admin.HandleFunc("/config", s.handleUpdateConfig).Methods("PUT", "PATCH")
	admin.HandleFunc("/analytics", s.handleAdminAnalytics).Methods("GET")
}

// setupWebRoutes sets up web UI routes
func (s *Server) setupWebRoutes() {
	// Setup/first-run routes
	s.router.HandleFunc("/setup", s.handleSetupPage).Methods("GET")
	s.router.HandleFunc("/setup/admin", s.handleSetupAdmin).Methods("GET", "POST")
	s.router.HandleFunc("/setup/first-url", s.handleSetupFirstURL).Methods("GET", "POST")
	s.router.HandleFunc("/setup/customize", s.handleSetupCustomize).Methods("GET", "POST")

	// Authentication pages
	s.router.HandleFunc("/login", s.handleLoginPage).Methods("GET")
	s.router.HandleFunc("/register", s.handleRegisterPage).Methods("GET")
	s.router.HandleFunc("/logout", s.handleLogoutPage).Methods("GET")

	// Health check (standalone for Docker health checks)
	s.router.HandleFunc("/health", s.handleHealthCheck).Methods("GET")

	// Main application pages
	s.router.HandleFunc("/", s.handleHomePage).Methods("GET")
	s.router.HandleFunc("/dashboard", s.handleDashboard).Methods("GET")
	s.router.HandleFunc("/analytics", s.handleAnalyticsPage).Methods("GET")
	s.router.HandleFunc("/bulk", s.handleBulkPage).Methods("GET")
	s.router.HandleFunc("/qr", s.handleQRPage).Methods("GET")
	s.router.HandleFunc("/profile", s.handleProfilePage).Methods("GET")

	// URL-specific pages
	s.router.HandleFunc("/url/{id}", s.handleURLPage).Methods("GET")
	s.router.HandleFunc("/url/{id}/edit", s.handleURLEditPage).Methods("GET")
	s.router.HandleFunc("/url/{id}/analytics", s.handleURLAnalyticsPage).Methods("GET")

	// Admin pages
	admin := s.router.PathPrefix("/admin").Subrouter()
	admin.Use(s.webAuthMiddleware)
	admin.Use(s.adminAuthMiddleware)
	admin.HandleFunc("", s.handleAdminDashboard).Methods("GET")
	admin.HandleFunc("/", s.handleAdminDashboard).Methods("GET")
	admin.HandleFunc("/users", s.handleAdminUsersPage).Methods("GET")
	admin.HandleFunc("/settings", s.handleAdminSettingsPage).Methods("GET")
	admin.HandleFunc("/database", s.handleAdminDatabasePage).Methods("GET")
	admin.HandleFunc("/migrations", s.handleAdminMigrationsPage).Methods("GET")
	admin.HandleFunc("/analytics", s.handleAdminAnalyticsPage).Methods("GET")

	if s.config.Billing.Enabled {
		admin.HandleFunc("/billing", s.handleAdminBillingPage).Methods("GET")
	}
}

// displayStartupInfo displays startup information in the terminal
func (s *Server) displayStartupInfo() {
	host := s.config.Server.Host
	if host == "0.0.0.0" {
		host = "localhost"
	}

	port := s.config.Server.Port
	externalIP := s.detectExternalIP()

	// ASCII box for visual appeal
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│                     Caslink URL Shortener                  │")
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Version: %-50s │\n", "1.0.0") // TODO: Get from build flags
	fmt.Printf("│ Local:   http://%-42s │\n", fmt.Sprintf("%s:%d", host, port))
	if externalIP != "127.0.0.1" && externalIP != host {
		fmt.Printf("│ Network: http://%-42s │\n", fmt.Sprintf("%s:%d", externalIP, port))
	}
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	fmt.Println("│ First-time setup:                                          │")
	fmt.Println("│ 1. Open the URL above in your browser                     │")
	fmt.Println("│ 2. Create your admin account                              │")
	fmt.Println("│ 3. Create your first short URL                           │")
	fmt.Println("│                                                            │")
	fmt.Println("│ Press Ctrl+C to stop the server                          │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()
}

// detectExternalIP tries to detect the external IP address
func (s *Server) detectExternalIP() string {
	// Method 1: Check network interfaces
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					return ipNet.IP.String()
				}
			}
		}
	}

	// Method 2: Check common environment variables
	if ip := os.Getenv("SERVER_IP"); ip != "" {
		return ip
	}
	if ip := os.Getenv("HOST_IP"); ip != "" {
		return ip
	}

	// Fallback to localhost
	return "127.0.0.1"
}

// selectAndPersistPort selects an available port and persists it to the database
func selectAndPersistPort(database *db.DB, host string) (int, error) {
	// First, try to get the previously used port from the database
	ctx := context.Background()

	query := "SELECT value FROM server_config WHERE key = 'server_port'"
	var portStr string
	err := database.QueryRow(ctx, query).Scan(&portStr)
	if err == nil {
		// Port found in database, validate it's still available
		if port, err := strconv.Atoi(portStr); err == nil {
			if isPortAvailable(host, port) {
				return port, nil
			}
		}
	}

	// No port in database or port unavailable, select a new one
	port, err := selectAvailablePort(host, 64000, 65535)
	if err != nil {
		return 0, err
	}

	// Persist the port to database
	persistQuery := `
		INSERT OR REPLACE INTO server_config (key, value, type, description, updated_at)
		VALUES ('server_port', ?, 'integer', 'Auto-selected server port', ?)
	`
	if database.Type() == "postgres" {
		persistQuery = `
			INSERT INTO server_config (key, value, type, description, updated_at)
			VALUES ('server_port', $1, 'integer', 'Auto-selected server port', $2)
			ON CONFLICT (key) DO UPDATE SET
				value = EXCLUDED.value,
				updated_at = EXCLUDED.updated_at
		`
	} else if database.Type() == "mysql" {
		persistQuery = `
			INSERT INTO server_config (` + "`key`" + `, ` + "`value`" + `, ` + "`type`" + `, description, updated_at)
			VALUES ('server_port', ?, 'integer', 'Auto-selected server port', ?)
			ON DUPLICATE KEY UPDATE
				` + "`value`" + ` = VALUES(` + "`value`" + `),
				updated_at = VALUES(updated_at)
		`
	}

	_, err = database.Exec(ctx, persistQuery, strconv.Itoa(port), time.Now())
	if err != nil {
		// Log error but don't fail - port selection still worked
		logrus.WithError(err).Warn("Failed to persist port to database")
	}

	return port, nil
}

// selectAvailablePort finds an available port in the given range
func selectAvailablePort(host string, startPort, endPort int) (int, error) {
	for port := startPort; port <= endPort; port++ {
		if isPortAvailable(host, port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", startPort, endPort)
}

// isPortAvailable checks if a port is available for binding
func isPortAvailable(host string, port int) bool {
	addr := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// getLogLevel converts string log level to logrus level
func getLogLevel(level string) logrus.Level {
	switch level {
	case "debug":
		return logrus.DebugLevel
	case "info":
		return logrus.InfoLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	default:
		return logrus.InfoLevel
	}
}