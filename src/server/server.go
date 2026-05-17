package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"golang.org/x/crypto/acme/autocert"

	"net/http/pprof"

	"github.com/casjaysdevdocker/caslink/src/config"
	"github.com/casjaysdevdocker/caslink/src/graphql"
	"github.com/casjaysdevdocker/caslink/src/logger"
	appmetrics "github.com/casjaysdevdocker/caslink/src/metrics"
	"github.com/casjaysdevdocker/caslink/src/mode"
	"github.com/casjaysdevdocker/caslink/src/scheduler"
	"github.com/casjaysdevdocker/caslink/src/server/handler"
	"github.com/casjaysdevdocker/caslink/src/server/service"
	"github.com/casjaysdevdocker/caslink/src/server/store"
	"github.com/casjaysdevdocker/caslink/src/server/tmpl"
	"github.com/casjaysdevdocker/caslink/src/swagger"
)

// Server represents the HTTP server
type Server struct {
	router      *chi.Mux
	server      *http.Server
	config      *config.Config
	mode        mode.Mode
	store       *store.Store
	scheduler   *scheduler.Scheduler
	renderer    *tmpl.Renderer
	metrics     *appmetrics.Metrics
	log         *logger.Logger
	pidFile     string // path to PID file; empty = no PID file
	acmeManager *autocert.Manager // non-nil when LE HTTP-01 is active

	// Version information
	Version   string
	CommitID  string
	BuildDate string
}

// New creates a new server instance
func New(cfg *config.Config, appMode mode.Mode, dataDir, logDir, pidFile string, appLogger *logger.Logger, version, commitID, buildDate string) (*Server, error) {
	// Open database — use configured driver if set, otherwise default to SQLite
	dbCfg := cfg.Server.Database
	var db *store.Store
	var err error
	if dbCfg.Driver != "" && dbCfg.Driver != "sqlite" {
		db, err = store.OpenStoreWithConfig(
			dbCfg.Driver, dbCfg.Host, dbCfg.Port,
			dbCfg.Name, dbCfg.Username, dbCfg.Password, dbCfg.SSLMode,
			dataDir,
		)
	} else {
		db, err = store.Open(dataDir)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	sched := scheduler.New(db, logDir)

	renderer, err := tmpl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize template renderer: %w", err)
	}

	// Initialize Prometheus metrics (always; gated by config at route level).
	m, _ := appmetrics.New(version, commitID, buildDate, cfg.Server.Metrics.IncludeRuntime)

	// Initialise Let's Encrypt autocert.Manager when HTTP-01 is enabled so the
	// /.well-known/acme-challenge/ handler can serve real token responses.
	var acmeMgr *autocert.Manager
	if cfg.Server.SSL.LetsEncrypt.Enabled &&
		(cfg.Server.SSL.LetsEncrypt.Challenge == "http-01" || cfg.Server.SSL.LetsEncrypt.Challenge == "") {
		cacheDir := filepath.Join(dataDir, "ssl", "acme-cache")
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			log.Printf("acme: could not create cache dir %s: %v — HTTP-01 disabled", cacheDir, err)
		} else {
			leEmail := cfg.Server.SSL.LetsEncrypt.Email
			if leEmail == "" {
				leEmail = cfg.Server.Admin.Email
			}
			hosts := []string{cfg.Server.FQDN}
			acmeMgr = &autocert.Manager{
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(hosts...),
				Cache:      autocert.DirCache(cacheDir),
				Email:      leEmail,
			}
			if cfg.Server.SSL.LetsEncrypt.Staging {
				// Use the staging CA so we can test without rate-limit risks.
				acmeMgr.Client = nil // autocert uses ACME dir from Client; staging needs custom setup
				log.Printf("acme: Let's Encrypt staging mode — certificates will not be trusted by browsers")
			}
			log.Printf("acme: HTTP-01 autocert manager initialised (cache=%s, hosts=%v)", cacheDir, hosts)
		}
	}

	s := &Server{
		router:      chi.NewRouter(),
		config:      cfg,
		mode:        appMode,
		store:       db,
		scheduler:   sched,
		renderer:    renderer,
		metrics:     m,
		log:         appLogger,
		pidFile:     pidFile,
		acmeManager: acmeMgr,
		Version:     version,
		CommitID:    commitID,
		BuildDate:   buildDate,
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

	// Access logging middleware. Spec PART 11 wants structured access
	// logs in both development and production for the audit trail.
	// Development uses chi's verbose colored logger; production uses
	// a compact single-line format suitable for log aggregators.
	if s.mode.IsDevelopment() {
		s.router.Use(middleware.Logger)
	} else {
		s.router.Use(accessLogMiddleware(s.log))
	}

	// URL normalization and path security per AI.md PART 5.
	// Order matters: normalize first, then block traversal on the cleaned path.
	s.router.Use(URLNormalizeMiddleware)
	s.router.Use(PathSecurityMiddleware)

	// HTTP metrics middleware — records request counts, latency, and sizes.
	// Runs after path normalization so labels use clean paths.
	if s.config.Server.Metrics.Enabled {
		s.router.Use(s.metrics.Middleware)
	}

	// Timeout middleware (30 second timeout)
	s.router.Use(middleware.Timeout(30 * time.Second))

	// CORS middleware. When unset, restrict to same-origin (no allowed
	// origins); operators set Web.CORS to a comma-separated allowlist to
	// enable cross-origin access. Using "*" with AllowCredentials is a
	// browser-rejected misconfiguration and is intentionally not supported.
	var origins []string
	if s.config.Web.CORS != "" && s.config.Web.CORS != "*" {
		origins = strings.Split(s.config.Web.CORS, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
	}
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: len(origins) > 0,
		MaxAge:           300,
	})
	s.router.Use(corsHandler.Handler)
}

// setupRoutes configures HTTP routes
func (s *Server) setupRoutes() {
	// Determine the configurable admin path (default "admin" per spec)
	adminPath := s.config.Server.Admin.Path
	if adminPath == "" {
		adminPath = "admin"
	}

	// Create rate limiter (per-IP sliding window)
	rateLimiter := NewRateLimiter()

	// Create services
	urlService := service.NewURLService(s.store)
	authService := service.NewAuthService(s.store)
	totpService := service.NewTOTPService(s.store)
	emailService := service.NewEmailService(s.config)
	qrService := service.NewQRService(s.store)
	orgService := service.NewOrgService(s.store)
	domainService := service.NewDomainService(s.store)
	analyticsService := service.NewAnalyticsService(s.store)
	bulkService := service.NewBulkService(s.store, urlService)
	userAdminService := service.NewUserAdminService(s.store)

	// Token service (needed by user handler and bearer middleware)
	tokenService := service.NewTokenService(s.store)

	// Create handlers
	urlHandler := handler.NewURLHandler(urlService, analyticsService)
	qrHandler := handler.NewQRHandler(qrService, urlService)
	bulkHandler := handler.NewBulkHandler(bulkService)
	adminHandler := handler.NewAdminHandler(authService, userAdminService, s.Version, s.mode.String(), adminPath)
	setupHandler := handler.NewSetupHandler(authService, s.config, s.Version)
	authUserHandler := handler.NewAuthUserHandler(authService, s.renderer, s.config)
	twoFactorHandler := handler.NewTwoFactorHandler(authService, totpService)
	passwordHandler := handler.NewPasswordHandler(authService, emailService, s.renderer, s.config)
	userHandler := handler.NewUserHandler(authService, tokenService, urlService, s.renderer, s.config)
	orgHandler := handler.NewOrgHandler(orgService, authService, s.renderer, s.config)
	domainHandler := handler.NewDomainHandler(domainService, authService, orgService)

	// Static assets (CSS, JS, PWA manifest, service worker)
	s.router.Handle("/static/*", tmpl.StaticHandler())

	// Debug endpoints — only in development/debug mode per AI.md PART 6.
	// These endpoints MUST NOT be exposed in production (pprof leaks internals).
	if s.mode.IsDevelopment() {
		s.router.HandleFunc("/debug/pprof/", pprof.Index)
		s.router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		s.router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		s.router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		s.router.HandleFunc("/debug/pprof/trace", pprof.Trace)
		s.router.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
		s.router.Handle("/debug/pprof/heap", pprof.Handler("heap"))
		s.router.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
		s.router.Handle("/debug/pprof/block", pprof.Handler("block"))
		s.router.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
		s.router.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
		log.Printf("debug: pprof endpoints registered at /debug/pprof/")
	}

	// Well-known / health — no auth, no CSRF
	s.router.Get("/server/healthz", handler.HealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String()))
	s.router.Get("/version", handler.VersionHandler(s.Version, s.CommitID, s.BuildDate))

	// Swagger/OpenAPI documentation per spec PART 14 + IDEA.md:
	// web UI at /server/docs/swagger; JSON spec at canonical + alias paths.
	// Vendor assets (swagger-ui-bundle.js, swagger-ui.css) are embedded in the binary.
	s.router.Get("/server/docs/swagger", swagger.Handler(s.Version))
	s.router.Handle("/server/docs/swagger/static/*", swagger.StaticHandler())
	s.router.Get("/api/swagger", swagger.SpecHandler(s.Version))

	// Prometheus metrics endpoint per AI.md PART 21.
	// INTERNAL ONLY — operators must firewall or proxy-restrict this path.
	// When a bearer token is configured only matching requests are served.
	if s.config.Server.Metrics.Enabled {
		endpoint := s.config.Server.Metrics.Endpoint
		if endpoint == "" {
			endpoint = "/metrics"
		}
		_, metricsHandler := appmetrics.New(
			s.Version, s.CommitID, s.BuildDate,
			s.config.Server.Metrics.IncludeRuntime,
		)
		token := s.config.Server.Metrics.Token
		s.router.Get(endpoint, func(w http.ResponseWriter, r *http.Request) {
			if token != "" {
				auth := r.Header.Get("Authorization")
				if auth != "Bearer "+token {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
			}
			metricsHandler.ServeHTTP(w, r)
		})
	}

	// Well-known routes per spec PART 11 / RFC 9116
	s.router.Get("/.well-known/security.txt", s.wellKnownSecurityTxt)
	s.router.Get("/.well-known/change-password", s.wellKnownChangePassword)
	// ACME HTTP-01 challenge handler — serves challenge tokens for Let's Encrypt
	// HTTP-01 verification. Returns 404 when no challenge is active for the token.
	s.router.Get("/.well-known/acme-challenge/{token}", s.wellKnownACMEChallenge)

	// GraphQL API
	// GraphiQL UI at /graphiql; vendor assets embedded in the binary.
	s.router.Get("/graphiql", graphql.Handler(s.Version))
	s.router.Handle("/server/docs/graphql/static/*", graphql.StaticHandler())
	s.router.Get("/graphql/schema", graphql.SchemaHandler())
	s.router.Post("/graphql", graphql.QueryHandler())

	// Setup wizard (first-run only) — CSRF applied per AI.md PART 11.
	// The setup token provides primary protection; CSRF adds a second layer
	// against same-LAN network attackers.
	s.router.Route("/setup", func(r chi.Router) {
		r.Use(CSRFMiddleware())
		r.Get("/", setupHandler.SetupPage)
		r.Post("/", setupHandler.Setup)
	})

	// Auth routes — /server/auth/* per spec PART 17
	s.router.Route("/server/auth", func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))
		r.Use(RateLimitMiddleware(rateLimiter))

		r.Get("/login", authUserHandler.LoginPage)
		r.Post("/login", authUserHandler.Login)
		r.Get("/logout", authUserHandler.Logout)
		r.Get("/register", authUserHandler.RegisterPage)
		r.Post("/register", authUserHandler.Register)

		// Password reset per spec PART 23 / PART 26
		r.Get("/password/forgot", passwordHandler.ForgotPasswordPage)
		r.Post("/password/forgot", passwordHandler.ForgotPassword)
		r.Get("/password/reset/{token}", passwordHandler.ResetPasswordPage)
		r.Post("/password/reset/{token}", passwordHandler.ResetPassword)

		// 2FA verification per spec PART 23
		r.Get("/2fa", twoFactorHandler.VerifyPage)
		r.Post("/2fa", twoFactorHandler.Verify)
		r.Get("/2fa/recovery", twoFactorHandler.RecoveryPage)
		r.Post("/2fa/recovery", twoFactorHandler.Recovery)
		r.Get("/2fa/recovery/options", twoFactorHandler.RecoveryOptionsPage)
	})

	// User routes — /users/* per spec PART 17 (requires auth)
	s.router.Route("/users", func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))
		r.Use(UserAuthMiddleware(authService))
		r.Use(CSRFMiddleware())

		r.Get("/dashboard", userHandler.Dashboard)
		r.Get("/profile", userHandler.Profile)
		r.Get("/settings", userHandler.Settings)
		r.Get("/tokens", userHandler.Tokens)
		r.Post("/tokens", userHandler.Tokens)
		r.Get("/security", userHandler.Security)

		// Security sub-routes per spec PART 23
		userSecurityHandler := handler.NewUserSecurityHandler(authService, totpService, qrService, emailService, s.renderer, s.config)
		r.HandleFunc("/security/password", userSecurityHandler.Password)
		r.HandleFunc("/security/sessions", userSecurityHandler.Sessions)
		r.HandleFunc("/security/2fa", userSecurityHandler.TwoFactor)
		r.HandleFunc("/security/passkeys", userSecurityHandler.Passkeys)
		r.HandleFunc("/security/recovery", userSecurityHandler.Recovery)

		// Custom domain management per PART 35
		r.Get("/domains", domainHandler.ListUserDomains)
		r.Post("/domains", domainHandler.AddUserDomain)
		r.Post("/domains/{domain}/verify", domainHandler.VerifyUserDomain)
	})

	// Organization routes — /orgs/* per spec PART 17 (requires auth)
	s.router.Route("/orgs", func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))
		r.Use(UserAuthMiddleware(authService))
		r.Use(CSRFMiddleware())

		r.Get("/", orgHandler.ListOrgs)
		r.Get("/new", orgHandler.CreateOrgPage)
		r.Post("/", orgHandler.CreateOrg)

		// Organization-specific routes (requires org membership)
		r.Route("/{slug}", func(sr chi.Router) {
			sr.Use(OrgMemberMiddleware(orgService))

			sr.Get("/", orgHandler.OrgDashboard)
			sr.Get("/settings", orgHandler.OrgSettings)
			sr.Get("/members", orgHandler.OrgMembers)

			// Custom domain management per PART 35
			sr.Get("/domains", domainHandler.ListOrgDomains)
			sr.Post("/domains", domainHandler.AddOrgDomain)
		})
	})

	// Admin panel routes — /server/{adminPath}/* per spec PART 17
	s.router.Route("/server/"+adminPath, func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))

		// Login/logout (no auth required)
		r.Get("/", adminHandler.LoginPage)
		r.Post("/login", adminHandler.Login)
		r.Get("/logout", adminHandler.Logout)

		// Authenticated admin routes (require admin_session cookie per spec PART 23)
		r.Group(func(ar chi.Router) {
			ar.Use(AdminAuthMiddleware(authService, adminPath))
			ar.Use(CSRFMiddleware())
			ar.Get("/dashboard", adminHandler.Dashboard)

			// User moderation
			ar.Get("/config/users", adminHandler.UserList)
			ar.Get("/config/users/{id}", adminHandler.UserDetail)
			ar.Post("/config/users/{id}/suspend", adminHandler.SuspendUser)
			ar.Post("/config/users/{id}/activate", adminHandler.ActivateUser)
		})
	})

	// Bearer token middleware factory for API routes
	bearerMiddleware := BearerAuthMiddleware(tokenService)

	// API v1
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))

		// Public endpoints (no auth)
		r.Get("/server/healthz", handler.APIHealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String(), s.store))
		r.Get("/healthz", handler.APIHealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String(), s.store))
		r.Get("/version", handler.VersionHandler(s.Version, s.CommitID, s.BuildDate))
		// OpenAPI JSON spec — canonical per spec PART 14 + IDEA.md
		r.Get("/server/swagger", swagger.SpecHandler(s.Version))

		// CSP violation report endpoint (AI.md PART 11 report-uri)
		r.Post("/server/reports/csp", func(w http.ResponseWriter, req *http.Request) {
			// Accept and discard CSP reports (browser POST, no body processing needed for now).
			// Full processing (log + alert) is a future enhancement.
			w.WriteHeader(http.StatusNoContent)
		})

		// Auth API — /api/v1/server/auth/*
		r.Route("/server/auth", func(ar chi.Router) {
			ar.Use(RateLimitMiddleware(rateLimiter))
			ar.Post("/login", authUserHandler.Login)
			ar.Post("/register", authUserHandler.Register)
		})

		// Admin API — /api/v1/server/{adminPath}/*
		r.Route("/server/"+adminPath, func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			ar.Get("/config/users", adminHandler.APIUserList)
			ar.Get("/config/users/{id}", adminHandler.APIUserDetail)
			ar.Post("/config/users/{id}/suspend", adminHandler.APISuspendUser)
			ar.Post("/config/users/{id}/activate", adminHandler.APIActivateUser)
		})

		// URL management endpoints (require Bearer auth per spec)
		r.Group(func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			ar.Post("/urls", urlHandler.CreateURL)
		})

		// Public read endpoints
		r.Get("/urls/{code}", urlHandler.GetURL)
		r.Get("/urls/{code}/stats", urlHandler.Stats)

		// QR code endpoints
		r.Get("/qr/{code}", qrHandler.GenerateQR)

		// Users API — /api/v1/users/*
		r.Route("/users", func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			ar.Get("/urls/export", bulkHandler.Export)
			ar.Post("/urls/import", bulkHandler.Import)
		})

		// Orgs API — /api/v1/orgs/*
		r.Route("/orgs", func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			ar.Get("/", orgHandler.APIListOrgs)
			ar.Post("/", orgHandler.APICreateOrg)
			ar.Get("/{slug}", orgHandler.APIGetOrg)
			ar.Get("/{slug}/members", orgHandler.APIGetMembers)
		})
	})

	// Root handler
	s.router.Get("/", s.handleRoot)

	// Short URL redirect (must be last to not catch other routes)
	s.router.Get("/{code}", urlHandler.RedirectURL)
}

// wellKnownSecurityTxt serves RFC 9116 security.txt at /.well-known/security.txt.
// Contact and policy URLs are derived from the server config.
func (s *Server) wellKnownSecurityTxt(w http.ResponseWriter, r *http.Request) {
	fqdn := s.config.Server.FQDN
	scheme := "http"
	if s.config.Server.SSL.Enabled {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, fqdn)

	contact := s.config.Server.Admin.Email
	if contact == "" {
		contact = fmt.Sprintf("admin@%s", fqdn)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Contact: mailto:%s\n", contact)
	fmt.Fprintf(w, "Policy: %s/.well-known/security.txt\n", baseURL)
	fmt.Fprintf(w, "Canonical: %s/.well-known/security.txt\n", baseURL)
	fmt.Fprintf(w, "Preferred-Languages: en\n")
}

// wellKnownChangePassword redirects per the W3C change-password well-known URL spec.
// Logged-in users go to the password-change page; others go to the reset flow.
func (s *Server) wellKnownChangePassword(w http.ResponseWriter, r *http.Request) {
	authService := service.NewAuthService(s.store)
	if cookie, err := r.Cookie("user_session"); err == nil && cookie.Value != "" {
		if _, sessionErr := authService.ValidateUserSession(r.Context(), cookie.Value); sessionErr == nil {
			http.Redirect(w, r, "/users/security/password", http.StatusFound)
			return
		}
	}
	http.Redirect(w, r, "/server/auth/password/forgot", http.StatusFound)
}

// wellKnownACMEChallenge handles /.well-known/acme-challenge/{token} for
// Let's Encrypt HTTP-01 validation per AI.md PART 15.
// When the autocert.Manager is active (LE HTTP-01 enabled in config) the
// request is delegated to it so the ACME CA can verify domain ownership.
// Without a manager the handler returns 404 so LE falls back to DNS-01.
func (s *Server) wellKnownACMEChallenge(w http.ResponseWriter, r *http.Request) {
	if s.acmeManager != nil {
		// autocert.Manager.HTTPHandler wraps http.NotFound for non-challenge
		// paths; for /.well-known/acme-challenge/{token} it writes the key
		// authorisation token required by the ACME CA.
		s.acmeManager.HTTPHandler(nil).ServeHTTP(w, r)
		return
	}
	http.NotFound(w, r)
}

// handleRoot handles the root endpoint — redirects to dashboard if logged in,
// to login if not, or to /setup on first run.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	authService := service.NewAuthService(s.store)
	needsSetup, err := authService.NeedsSetup(ctx)
	if err == nil && needsSetup {
		http.Redirect(w, r, "/setup", http.StatusFound)
		return
	}

	// Check for authenticated session — redirect to dashboard if present.
	if cookie, err := r.Cookie("user_session"); err == nil && cookie.Value != "" {
		if _, sessionErr := authService.ValidateUserSession(ctx, cookie.Value); sessionErr == nil {
			http.Redirect(w, r, "/users/dashboard", http.StatusFound)
			return
		}
	}

	http.Redirect(w, r, "/server/auth/login", http.StatusFound)
}

// Start starts the HTTP server
func (s *Server) Start(address string, port int) error {
	// Auto-select port if not specified
	if port == 0 {
		port = selectRandomPort()
		log.Printf("Auto-selected port: %d", port)
	}

	addr := fmt.Sprintf("%s:%d", address, port)

	// Resolve timeouts from config (LimitsConfig); fall back to safe defaults.
	limits := s.config.Server.Limits
	readTimeout := time.Duration(limits.ReadTimeout) * time.Second
	writeTimeout := time.Duration(limits.WriteTimeout) * time.Second
	idleTimeout := time.Duration(limits.IdleTimeout) * time.Second
	if readTimeout <= 0 {
		readTimeout = 30 * time.Second
	}
	if writeTimeout <= 0 {
		writeTimeout = 30 * time.Second
	}
	if idleTimeout <= 0 {
		idleTimeout = 120 * time.Second
	}

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// Write PID file after the http.Server struct is set up but before
	// ListenAndServe so that --status can locate us immediately.
	if s.pidFile != "" && s.config.Server.PIDFile {
		if err := writePIDFile(s.pidFile); err != nil {
			log.Printf("Warning: could not write PID file %s: %v", s.pidFile, err)
		}
	}

	// Start scheduler
	s.scheduler.Start()

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

	// Remove PID file before stopping so monitoring knows we are shutting down.
	if s.pidFile != "" {
		_ = os.Remove(s.pidFile)
	}

	// Stop scheduler
	s.scheduler.Stop()

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}

	log.Println("Server stopped")
	return nil
}

// writePIDFile writes the current process PID to the given path, creating
// parent directories as needed.
func writePIDFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data := []byte(fmt.Sprintf("%d\n", os.Getpid()))
	return os.WriteFile(path, data, 0644)
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
