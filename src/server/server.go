package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/graphql"
	"github.com/casjaysdevdocker/caslink/src/mode"
	"github.com/casjaysdevdocker/caslink/src/server/handler"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/store"
	"github.com/casjaysdevdocker/caslink/src/swagger"
)

// Server represents the HTTP server
type Server struct {
	router *chi.Mux
	server *http.Server
	config *config.Config
	mode   mode.Mode
	store  *store.Store

	// Version information
	Version   string
	CommitID  string
	BuildDate string
}

// New creates a new server instance
func New(cfg *config.Config, appMode mode.Mode, dataDir, version, commitID, buildDate string) (*Server, error) {
	// Open database
	db, err := store.Open(dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	s := &Server{
		router:    chi.NewRouter(),
		config:    cfg,
		mode:      appMode,
		store:     db,
		Version:   version,
		CommitID:  commitID,
		BuildDate: buildDate,
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s, nil
}

// setupMiddleware configures HTTP middleware
func (s *Server) setupMiddleware() {
	// Recovery middleware (always enabled)
	s.router.Use(middleware.Recoverer)

	// Request ID middleware
	s.router.Use(middleware.RequestID)

	// Real IP middleware
	s.router.Use(middleware.RealIP)

	// Logging middleware
	if s.mode.IsDevelopment() {
		s.router.Use(middleware.Logger)
	}

	// Timeout middleware (30 second timeout)
	s.router.Use(middleware.Timeout(30 * time.Second))

	// CORS middleware
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   []string{s.config.Web.CORS},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	})
	s.router.Use(corsHandler.Handler)
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Create services
	urlService := service.NewURLService(s.store)
	authService := service.NewAuthService(s.store)
	totpService := service.NewTOTPService(s.store)
	emailService := service.NewEmailService(s.config)
	qrService := service.NewQRService(s.store)
	orgService := service.NewOrgService(s.store)
	domainService := service.NewDomainService(s.store)

	// Create handlers
	urlHandler := handler.NewURLHandler(urlService)
	qrHandler := handler.NewQRHandler(qrService, urlService)
	adminHandler := handler.NewAdminHandler(authService, s.Version, s.mode.String())
	setupHandler := handler.NewSetupHandler(authService, s.Version)
	authUserHandler := handler.NewAuthUserHandler(authService)
	twoFactorHandler := handler.NewTwoFactorHandler(authService, totpService)
	passwordHandler := handler.NewPasswordHandler(authService, emailService)
	userHandler := handler.NewUserHandler(authService)
	orgHandler := handler.NewOrgHandler(orgService, authService)
	domainHandler := handler.NewDomainHandler(domainService, authService)

	// Health endpoints
	s.router.Get("/healthz", handler.HealthHandler(s.Version, s.mode.String()))
	s.router.Get("/version", handler.VersionHandler(s.Version, s.CommitID, s.BuildDate))

	// Swagger/OpenAPI documentation
	s.router.Get("/swagger", swagger.Handler(s.Version))
	s.router.Get("/swagger/spec.json", swagger.SpecHandler(s.Version))

	// GraphQL API
	s.router.Get("/graphiql", graphql.Handler(s.Version))
	s.router.Get("/graphql/schema", graphql.SchemaHandler())
	s.router.Post("/graphql", graphql.QueryHandler())

	// Setup wizard (first-run only)
	s.router.Get("/setup", setupHandler.SetupPage)
	s.router.Post("/setup", setupHandler.Setup)

	// Auth routes per PART 23
	s.router.Route("/auth", func(r chi.Router) {
		r.Get("/login", authUserHandler.LoginPage)
		r.Post("/login", authUserHandler.Login)
		r.Get("/logout", authUserHandler.Logout)
		r.Get("/register", authUserHandler.RegisterPage)
		r.Post("/register", authUserHandler.Register)

		// Password reset per PART 23 and PART 26
		r.Get("/password/forgot", passwordHandler.ForgotPasswordPage)
		r.Post("/password/forgot", passwordHandler.ForgotPassword)
		r.Get("/password/reset/{token}", passwordHandler.ResetPasswordPage)
		r.Post("/password/reset/{token}", passwordHandler.ResetPassword)

		// 2FA verification per PART 23 line 20217-20280
		r.Get("/2fa", twoFactorHandler.VerifyPage)
		r.Post("/2fa", twoFactorHandler.Verify)
		r.Get("/2fa/recovery", twoFactorHandler.RecoveryPage)
		r.Post("/2fa/recovery", twoFactorHandler.Recovery)
		r.Get("/2fa/recovery/options", twoFactorHandler.RecoveryOptionsPage)
	})

	// User routes (requires auth per PART 23)
	s.router.Route("/user", func(r chi.Router) {
		// Authentication middleware - validates user_session cookie
		r.Use(UserAuthMiddleware(authService))

		// Profile and settings per PART 23
		r.Get("/profile", userHandler.Profile)
		r.Get("/settings", userHandler.Settings)
		r.Get("/tokens", userHandler.Tokens)
		r.Get("/security", userHandler.Security)

		// Security sub-routes per PART 23
		userSecurityHandler := handler.NewUserSecurityHandler(authService, totpService, qrService, emailService)
		r.HandleFunc("/security/password", userSecurityHandler.Password)
		r.HandleFunc("/security/sessions", userSecurityHandler.Sessions)
		r.HandleFunc("/security/2fa", userSecurityHandler.TwoFactor)
		r.HandleFunc("/security/passkeys", userSecurityHandler.Passkeys)
		r.HandleFunc("/security/recovery", userSecurityHandler.Recovery)

		// Custom domain management per PART 35
		r.Get("/domains", domainHandler.ListUserDomains)
		r.Post("/domains/add", domainHandler.AddUserDomain)
		r.Post("/domains/{domain}/verify", domainHandler.VerifyUserDomain)
	})

	// Organization routes per PART 23 (requires auth)
	s.router.Route("/org", func(r chi.Router) {
		// Authentication middleware - validates user_session cookie
		r.Use(UserAuthMiddleware(authService))

		r.Get("/", orgHandler.ListOrgs)
		r.Get("/new", orgHandler.CreateOrgPage)
		r.Post("/new", orgHandler.CreateOrg)

		// Organization-specific routes (requires org membership)
		r.Route("/{slug}", func(sr chi.Router) {
			// Verify org membership per PART 23
			sr.Use(OrgMemberMiddleware(orgService))

			sr.Get("/", orgHandler.OrgDashboard)
			sr.Get("/settings", orgHandler.OrgSettings)
			sr.Get("/members", orgHandler.OrgMembers)

			// Custom domain management per PART 35
			sr.Get("/domains", domainHandler.ListOrgDomains)
			sr.Post("/domains/add", domainHandler.AddOrgDomain)
			// TODO: Add more org/domain routes per PART 35
		})
	})

	// Admin panel routes (isolated from public routes)
	s.router.Route("/admin", func(r chi.Router) {
		// Login/logout (no auth required)
		r.Get("/", adminHandler.LoginPage)
		r.Post("/login", adminHandler.Login)
		r.Get("/logout", adminHandler.Logout)

		// Authenticated admin routes (require admin_session cookie per PART 23)
		r.Group(func(ar chi.Router) {
			ar.Use(AdminAuthMiddleware(authService))
			ar.Get("/dashboard", adminHandler.Dashboard)
			// TODO: Add /admin/server/* routes per PART 23
			// TODO: Add /admin/server/moderation/* routes per PART 23
		})
	})

	// API v1
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Get("/healthz", handler.APIHealthHandler(s.Version, s.mode.String()))
		r.Get("/version", handler.VersionHandler(s.Version, s.CommitID, s.BuildDate))

		// URL management endpoints
		r.Post("/urls", urlHandler.CreateURL)
		r.Get("/urls/{code}", urlHandler.GetURL)

		// QR code endpoints
		r.Get("/qr/{code}", qrHandler.GenerateQR)
	})

	// Root handler
	s.router.Get("/", s.handleRoot)

	// Short URL redirect (must be last to not catch other routes)
	s.router.Get("/{code}", urlHandler.RedirectURL)
}

// handleRoot handles the root endpoint
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// Check if setup is needed
	ctx := r.Context()
	authService := service.NewAuthService(s.store)
	needsSetup, err := authService.NeedsSetup(ctx)
	if err == nil && needsSetup {
		// Redirect to setup wizard
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>%s</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            margin: 0;
            padding: 40px;
            background: #0d1117;
            color: #c9d1d9;
            line-height: 1.6;
        }
        .container { max-width: 800px; margin: 0 auto; }
        h1 { color: #58a6ff; margin-bottom: 10px; }
        .version { color: #8b949e; font-size: 14px; margin-bottom: 30px; }
        .info { background: #161b22; padding: 20px; border-radius: 6px; border: 1px solid #30363d; }
        a { color: #58a6ff; text-decoration: none; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="container">
        <h1>%s</h1>
        <div class="version">Version %s</div>
        <div class="info">
            <p><strong>Server Status:</strong> Running</p>
            <p><strong>Mode:</strong> %s</p>
            <p><strong>Endpoints:</strong></p>
            <ul>
                <li><a href="/healthz">Health Check</a> (HTML)</li>
                <li><a href="/api/v1/healthz">Health Check</a> (JSON)</li>
                <li><a href="/version">Version Info</a> (JSON)</li>
            </ul>
            <p><strong>API:</strong></p>
            <ul>
                <li>POST /api/v1/urls - Create short URL</li>
                <li>GET /api/v1/urls/{code} - Get URL details</li>
                <li>GET /{code} - Redirect to long URL</li>
            </ul>
            <p style="margin-top: 20px; color: #8b949e;">
                <strong>Note:</strong> This is the API server. Create URLs via POST /api/v1/urls.
            </p>
        </div>
    </div>
</body>
</html>`, s.config.Server.Branding.Title, s.config.Server.Branding.Title, s.Version, s.mode.String())

	fmt.Fprint(w, html)
}

// Start starts the HTTP server
func (s *Server) Start(address string, port int) error {
	// Auto-select port if not specified
	if port == 0 {
		port = selectRandomPort()
		log.Printf("Auto-selected port: %d", port)
	}

	addr := fmt.Sprintf("%s:%d", address, port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on %s (mode: %s)", addr, s.mode)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit

	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Println("Server stopped")
	return nil
}

// Stop stops the HTTP server gracefully
func (s *Server) Stop() error {
	// Close database connections
	if s.store != nil {
		if err := s.store.Close(); err != nil {
			log.Printf("Warning: database close error: %v", err)
		}
	}

	if s.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.server.Shutdown(ctx)
}

// selectRandomPort selects a random unused port in the 64xxx range
func selectRandomPort() int {
	// Try ports in 64xxx range
	for port := 64580; port < 65000; port++ {
		if isPortAvailable(port) {
			return port
		}
	}

	// Fallback to any available port
	return 0
}

// isPortAvailable checks if a port is available
func isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
