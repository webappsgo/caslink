package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
	"golang.org/x/crypto/acme/autocert"

	"net/http/pprof"

	appcrypto "github.com/webappsgo/caslink/src/common/crypto"
	"github.com/webappsgo/caslink/src/common/i18n"
	"github.com/webappsgo/caslink/src/config"
	"github.com/webappsgo/caslink/src/geoip"
	"github.com/webappsgo/caslink/src/graphql"
	"github.com/webappsgo/caslink/src/logger"
	appmetrics "github.com/webappsgo/caslink/src/metrics"
	"github.com/webappsgo/caslink/src/mode"
	"github.com/webappsgo/caslink/src/paths"
	"github.com/webappsgo/caslink/src/scheduler"
	"github.com/webappsgo/caslink/src/server/handler"
	"github.com/webappsgo/caslink/src/server/service"
	"github.com/webappsgo/caslink/src/server/store"
	"github.com/webappsgo/caslink/src/server/tmpl"
	"github.com/webappsgo/caslink/src/swagger"
	apktor "github.com/webappsgo/caslink/src/tor"
)

// Server represents the HTTP server
type Server struct {
	router         *chi.Mux
	server         *http.Server
	config         *config.Config
	mode           mode.Mode
	store          *store.Store
	scheduler      *scheduler.Scheduler
	renderer       *tmpl.Renderer
	metrics        *appmetrics.Metrics
	metricsHandler http.Handler
	log            *logger.Logger
	pidFile        string             // path to PID file; empty = no PID file
	acmeManager    *autocert.Manager  // non-nil when LE HTTP-01 is active
	geoip          *geoip.Service     // non-nil when GeoIP is enabled
	torManager     *apktor.TorManager // non-nil when Tor binary was found at startup
	configDir      string             // kept for TorManager (port not known until Start)
	dataDir        string             // kept for TorManager

	// Request counters for health endpoint (AI.md PART 13 stats fields).
	// reqTotal is incremented on each request; activeConn is a live gauge.
	// reqWindow is a 1440-bucket ring (one per minute) for the 24h window.
	reqTotal   int64
	activeConn int64
	reqWindow  [1440]int64 // ring buffer: one slot per minute, 24 × 60 = 1440
	reqWinSlot int64       // minute-index of the last written slot (Unix minutes)

	// Version information
	Version   string
	CommitID  string
	BuildDate string
}

// New creates a new server instance
func New(cfg *config.Config, appMode mode.Mode, dataDir, logDir, pidFile string, appLogger *logger.Logger, version, commitID, buildDate string, configDir, backupDir string) (*Server, error) {
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

	// Initialise the GeoIP service when enabled. Failures are non-fatal —
	// country blocking and click enrichment gracefully degrade.
	var geoSvc *geoip.Service
	if cfg.Server.GeoIP.Enabled {
		gs, gerr := geoip.New(cfg.Server.GeoIP, dataDir)
		if gerr != nil {
			log.Printf("[server] geoip init: %v (disabled)", gerr)
		} else {
			geoSvc = gs
		}
	}

	sched := scheduler.New(db, logDir, configDir, dataDir, backupDir, version, geoSvc, cfg.Server.Security, cfg.Server.Compliance.Enabled, cfg.Server.Backup.Retention, cfg.Server.Scheduler)

	renderer, err := tmpl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize template renderer: %w", err)
	}

	// Initialize Prometheus metrics (always; gated by config at route level).
	// A single registry/handler pair is created here and reused for both the
	// Middleware (recording metrics) and the /metrics route (serving them) —
	// two separate appmetrics.New() calls would create two disjoint registries,
	// leaving /metrics permanently empty of everything Middleware records.
	m, metricsHandler := appmetrics.New(
		version, commitID, buildDate,
		cfg.Server.Metrics.IncludeRuntime, cfg.Server.Metrics.IncludeSystem, dataDir,
	)

	// Initialise Let's Encrypt autocert.Manager.
	// HTTP-01 (default) and TLS-ALPN-01 are both handled by autocert;
	// DNS-01 is optional per AI.md PART 15 and is not implemented.
	// autocert.Manager.TLSConfig() includes the "acme-tls/1" ALPN protocol
	// required for TLS-ALPN-01 challenges per RFC 8737.
	var acmeMgr *autocert.Manager
	if cfg.Server.SSL.LetsEncrypt.Enabled &&
		(cfg.Server.SSL.LetsEncrypt.Challenge == "http-01" ||
			cfg.Server.SSL.LetsEncrypt.Challenge == "tls-alpn-01" ||
			cfg.Server.SSL.LetsEncrypt.Challenge == "") {
		cacheDir := filepath.Join(dataDir, "ssl", "acme-cache")
		if err := os.MkdirAll(cacheDir, 0700); err != nil {
			log.Printf("acme: could not create cache dir %s: %v — Let's Encrypt disabled", cacheDir, err)
		} else {
			leEmail := cfg.Server.SSL.LetsEncrypt.Email
			if leEmail == "" {
				leEmail = cfg.Server.Admin.Email
			}
			hosts := []string{cfg.Server.FQDN}
			if len(cfg.Server.SSL.LetsEncrypt.Domains) > 0 {
				hosts = cfg.Server.SSL.LetsEncrypt.Domains
			}
			// HostPolicy must stay dynamic, not the static
			// autocert.HostWhitelist(hosts...): custom domains (PART 36) are
			// added and DNS-verified by users at runtime, long after this
			// autocert.Manager is constructed, so a whitelist frozen at
			// startup could never authorize ACME issuance for them. Every
			// TLS handshake re-checks the configured hosts first (no DB hit
			// for the common case), then falls back to the custom-domains
			// table for verified, active, non-wildcard domains.
			domainSvc := service.NewDomainService(db, cfg.Server.Features.CustomDomains)
			staticHosts := make(map[string]bool, len(hosts))
			for _, h := range hosts {
				staticHosts[h] = true
			}
			acmeMgr = &autocert.Manager{
				Prompt: autocert.AcceptTOS,
				HostPolicy: func(ctx context.Context, host string) error {
					if staticHosts[host] {
						return nil
					}
					ok, err := domainSvc.IsDomainVerifiedActive(ctx, host)
					if err != nil {
						return fmt.Errorf("acme/autocert: host policy check failed for %q: %w", host, err)
					}
					if !ok {
						return fmt.Errorf("acme/autocert: host %q not permitted", host)
					}
					return nil
				},
				Cache: autocert.DirCache(cacheDir),
				Email: leEmail,
			}
			if cfg.Server.SSL.LetsEncrypt.Staging {
				// Staging CA — certificates will not be trusted by browsers.
				acmeMgr.Client = nil
				log.Printf("acme: Let's Encrypt staging mode")
			}
			log.Printf("acme: autocert manager initialised (challenge=%s, cache=%s, hosts=%v)",
				cfg.Server.SSL.LetsEncrypt.Challenge, cacheDir, hosts)
		}
	}

	s := &Server{
		router:         chi.NewRouter(),
		config:         cfg,
		mode:           appMode,
		store:          db,
		scheduler:      sched,
		renderer:       renderer,
		metrics:        m,
		metricsHandler: metricsHandler,
		log:            appLogger,
		pidFile:        pidFile,
		acmeManager:    acmeMgr,
		geoip:          geoSvc,
		configDir:      configDir,
		dataDir:        dataDir,
		Version:        version,
		CommitID:       commitID,
		BuildDate:      buildDate,
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

	// Real IP middleware — only trusts X-Forwarded-* from configured proxies
	s.router.Use(realIPMiddleware(s.config.Server.TrustedProxies.Additional))

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

	// Request counter middleware — increments reqTotal, activeConn, and the
	// rolling 24h ring buffer used by /server/healthz stats per AI.md PART 13.
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&s.reqTotal, 1)
			atomic.AddInt64(&s.activeConn, 1)
			defer atomic.AddInt64(&s.activeConn, -1)

			// Write into the current minute slot of the ring buffer.
			nowMin := time.Now().Unix() / 60
			slot := int(nowMin % 1440)
			prevSlot := atomic.LoadInt64(&s.reqWinSlot)
			if prevSlot != nowMin {
				// New minute — zero the slot before writing to it.
				if atomic.CompareAndSwapInt64(&s.reqWinSlot, prevSlot, nowMin) {
					atomic.StoreInt64(&s.reqWindow[slot], 0)
				}
			}
			atomic.AddInt64(&s.reqWindow[slot], 1)

			next.ServeHTTP(w, r)
		})
	})

	// Timeout middleware (30 second timeout)
	// Language selection per AI.md PART 31: ?lang= query param > lang cookie > Accept-Language > default (en)
	s.router.Use(i18n.LanguageMiddleware)

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
	rateLimiter.SetMetrics(s.metrics)

	// Create services
	urlService := service.NewURLService(s.store)
	urlService.SetGeoIP(s.geoip)
	authService := service.NewAuthService(s.store)
	var encryptionKey []byte
	if k, err := appcrypto.DecodeKey(s.config.Server.Security.EncryptionKey); err == nil {
		encryptionKey = k
	} else {
		log.Printf("Warning: server.security.encryption_key is not configured/valid — TOTP secrets will be stored in plaintext: %v", err)
	}
	totpService := service.NewTOTPService(s.store, encryptionKey, s.config.Server.Security.EncryptionKeyVersion)
	emailService := service.NewEmailService(s.config)
	qrService := service.NewQRService(s.store)
	qrService.SetMetrics(s.metrics)
	orgService := service.NewOrgService(s.store)
	inviteService := service.NewInviteService(s.store)
	domainService := service.NewDomainService(s.store, s.config.Server.Features.CustomDomains)
	analyticsService := service.NewAnalyticsService(s.store)
	bulkService := service.NewBulkService(s.store, urlService)
	userAdminService := service.NewUserAdminService(s.store)
	auditService := service.NewAuditService(s.store)
	s.scheduler.SetAuditService(auditService)
	s.scheduler.SetMetrics(s.metrics)
	s.scheduler.SetDomainService(domainService)

	// Token service (needed by user handler and bearer middleware)
	tokenService := service.NewTokenService(s.store)

	// WebAuthn service — RPID and origin derived from configured FQDN.
	// Failures are non-fatal; passkey endpoints will return 503 when nil.
	var webauthnSvc *service.WebAuthnService
	if fqdn := s.config.Server.FQDN; fqdn != "" {
		scheme := "http"
		if s.config.Server.SSL.Enabled {
			scheme = "https"
		}
		origin := scheme + "://" + fqdn
		var webauthnErr error
		webauthnSvc, webauthnErr = service.NewWebAuthnService(s.store, fqdn, origin)
		if webauthnErr != nil {
			log.Printf("[webauthn] service init failed (passkeys disabled): %v", webauthnErr)
			webauthnSvc = nil
		}
	}

	// Create handlers
	urlHandler := handler.NewURLHandler(urlService, analyticsService, s.renderer, s.config)
	graphqlResolver := graphql.NewResolver(urlService, tokenService, graphql.Info{
		Version:   s.Version,
		CommitID:  s.CommitID,
		BuildDate: s.BuildDate,
		Mode:      s.mode.String(),
	})
	qrHandler := handler.NewQRHandler(qrService, urlService)
	bulkHandler := handler.NewBulkHandler(bulkService)
	adminHandler := handler.NewAdminHandler(authService, userAdminService, auditService, domainService, s.Version, s.mode.String(), adminPath, s.config, s.store, func() *apktor.TorManager { return s.torManager })
	setupHandler := handler.NewSetupHandler(authService, s.config, s.Version)
	authUserHandler := handler.NewAuthUserHandler(authService, inviteService, s.renderer, s.config)
	twoFactorHandler := handler.NewTwoFactorHandler(authService, totpService)
	passwordHandler := handler.NewPasswordHandler(authService, emailService, s.renderer, s.config)
	userHandler := handler.NewUserHandler(authService, tokenService, urlService, s.renderer, s.config)
	orgHandler := handler.NewOrgHandler(orgService, authService, inviteService, s.renderer, s.config)
	domainHandler := handler.NewDomainHandler(domainService, authService, orgService, auditService)
	pagesHandler := handler.NewPagesHandler(s.config, s.renderer, s.Version, s.BuildDate, func() *apktor.TorManager { return s.torManager })

	// Custom domain resolver (PART 36). Registered before any route is mounted
	// on the router (chi requires all Use() calls to precede routing). It
	// attaches the resolved custom domain to the request context when the Host
	// is a verified, active custom domain, and passes every other Host through
	// untouched.
	s.router.Use(CustomDomainMiddleware(domainService))

	// Static assets (CSS, JS, PWA manifest, service worker)
	s.router.Handle("/static/*", tmpl.StaticHandler())

	// Serve embedded locale JSON files for frontend i18n (AI.md PART 31).
	// GET /locales/{lang}.json → returns the embedded translation file.
	s.router.Get("/locales/{lang}.json", func(w http.ResponseWriter, r *http.Request) {
		lang := chi.URLParam(r, "lang")
		data, err := i18n.LocaleJSON(lang)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	})

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
	// /server/healthz is always registered (PART 13). The bare-root /healthz
	// alias is opt-in via server.healthz.root.enabled; when enabled it mounts
	// the same handler directly (never a redirect to /server/healthz).
	s.router.Get("/server/healthz", handler.HealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String()))
	if s.config.Server.Healthz.Root.Enabled {
		s.router.Get("/healthz", handler.HealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String()))
	}
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
		metricsHandler := s.metricsHandler
		token := s.config.Server.Metrics.Token
		s.router.Get(endpoint, func(w http.ResponseWriter, r *http.Request) {
			if token != "" {
				auth := r.Header.Get("Authorization")
				expected := "Bearer " + token
				if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
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

	// GraphQL API per AI.md PART 14.
	// Console UI (GET renders the form, POST executes and re-renders) — no
	// client-side rendering framework, per AI.md PART 16.
	s.router.Get("/server/docs/graphql", graphql.Handler(s.Version, graphqlResolver))
	s.router.Post("/server/docs/graphql", graphql.Handler(s.Version, graphqlResolver))
	// Unversioned alias — mounts the SAME handler as the versioned canonical
	// route below, never a redirect (per PART 14 Route Migration Rule).
	s.router.Post("/api/graphql", graphql.QueryHandler(graphqlResolver))

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

		// Per-link management (stats, QR display, edit, delete) and bulk
		// import/export forms — PART 16 "link-management frontend beyond
		// create". Works fully without JavaScript.
		r.Get("/urls/{code}", urlHandler.WebURLManage)
		r.Post("/urls/{code}", urlHandler.WebURLManage)
		r.Get("/urls/bulk", userHandler.Bulk)
		r.Get("/urls/export", bulkHandler.Export)
		r.Post("/urls/import", bulkHandler.Import)

		r.Get("/profile", userHandler.Profile)
		r.Get("/settings", userHandler.Settings)
		r.Get("/tokens", userHandler.Tokens)
		r.Post("/tokens", userHandler.Tokens)
		r.Get("/security", userHandler.Security)

		// Security sub-routes per spec PART 23
		userSecurityHandler := handler.NewUserSecurityHandler(authService, totpService, qrService, emailService, webauthnSvc, s.renderer, s.config)
		r.HandleFunc("/security/password", userSecurityHandler.Password)
		r.HandleFunc("/security/sessions", userSecurityHandler.Sessions)
		r.HandleFunc("/security/2fa", userSecurityHandler.TwoFactor)
		r.HandleFunc("/security/passkeys", userSecurityHandler.Passkeys)
		r.HandleFunc("/security/recovery", userSecurityHandler.Recovery)

		// WebAuthn ceremony API endpoints (passkey registration/login per PART 34).
		// These are intentionally under /users/* (requires user session).
		r.Post("/passkeys/begin-register", userSecurityHandler.PasskeyBeginRegister)
		r.Post("/passkeys/finish-register", userSecurityHandler.PasskeyFinishRegister)
		r.Post("/passkeys/begin-login", userSecurityHandler.PasskeyBeginLogin)
		r.Post("/passkeys/finish-login", userSecurityHandler.PasskeyFinishLogin)

		// Custom domain management per PART 35
		r.Get("/domains", domainHandler.ListUserDomains)
		r.Post("/domains", domainHandler.AddUserDomain)
		r.Get("/domains/{domain}", domainHandler.APIGetUserDomain)
		r.Get("/domains/{domain}/dns", domainHandler.APIUserDomainDNS)
		r.Get("/domains/{domain}/ssl", domainHandler.APIUserDomainSSL)
		r.Post("/domains/{domain}/verify", domainHandler.VerifyUserDomain)
		r.Delete("/domains/{domain}", domainHandler.APIDeleteUserDomain)
	})

	// Organization routes — /orgs/* per spec PART 17 (requires auth)
	s.router.Route("/orgs", func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))
		r.Use(UserAuthMiddleware(authService))
		r.Use(CSRFMiddleware())

		r.Get("/", orgHandler.ListOrgs)
		r.Get("/new", orgHandler.CreateOrgPage)
		r.Post("/", orgHandler.CreateOrg)

		// Invite acceptance — authenticated but NOT membership-gated (the
		// acceptor is not yet a member). Static path wins over /{slug}.
		r.Get("/invite/accept", orgHandler.OrgAcceptInvite)

		// Organization-specific routes (requires org membership)
		r.Route("/{slug}", func(sr chi.Router) {
			sr.Use(OrgMemberMiddleware(orgService))

			sr.Get("/", orgHandler.OrgDashboard)
			sr.Get("/settings", orgHandler.OrgSettings)
			sr.Post("/settings", orgHandler.OrgSettingsSave)
			sr.Get("/members", orgHandler.OrgMembers)
			sr.Post("/members", orgHandler.OrgMembersAction)

			// Custom domain management per PART 35
			sr.Get("/domains", domainHandler.ListOrgDomains)
			sr.Post("/domains", domainHandler.AddOrgDomain)
			sr.Get("/domains/{domain}", domainHandler.APIGetOrgDomain)
			sr.Get("/domains/{domain}/dns", domainHandler.APIOrgDomainDNS)
			sr.Get("/domains/{domain}/ssl", domainHandler.APIOrgDomainSSL)
			sr.Post("/domains/{domain}/verify", domainHandler.VerifyOrgDomain)
			sr.Delete("/domains/{domain}", domainHandler.APIDeleteOrgDomain)
		})
	})

	// Public server information pages — /server/* per PART 16.
	// These routes are unauthenticated and serve About, Help, Privacy, Contact, Terms.
	s.router.Get("/server", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/server/about", http.StatusMovedPermanently)
	})
	s.router.Get("/server/about", pagesHandler.About)
	s.router.Get("/server/help", pagesHandler.Help)
	s.router.Get("/server/privacy", pagesHandler.Privacy)
	s.router.Get("/server/terms", pagesHandler.Terms)
	s.router.Route("/server/contact", func(r chi.Router) {
		r.Use(CSRFMiddleware())
		r.Get("/", pagesHandler.Contact)
		r.Post("/", pagesHandler.ContactSubmit)
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

			// Server settings
			ar.Get("/config/settings", adminHandler.ConfigSettings)
			ar.Post("/config/settings", adminHandler.ConfigSettingsSave)

			// Branding
			ar.Get("/config/branding", adminHandler.ConfigBranding)
			ar.Post("/config/branding", adminHandler.ConfigBrandingSave)

			// SSL/TLS
			ar.Get("/config/ssl", adminHandler.ConfigSSL)
			ar.Post("/config/ssl", adminHandler.ConfigSSLSave)

			// Scheduler
			ar.Get("/config/scheduler", adminHandler.ConfigScheduler)

			// Email
			ar.Get("/config/email", adminHandler.ConfigEmail)
			ar.Post("/config/email", adminHandler.ConfigEmailSave)
			ar.Post("/config/email/test", adminHandler.ConfigEmailTest)

			// Logs
			ar.Get("/config/logs", adminHandler.ConfigLogs)
			ar.Get("/config/logs/audit", adminHandler.ConfigLogsAudit)

			// Backup/restore
			ar.Get("/config/backup", adminHandler.ConfigBackup)
			ar.Post("/config/backup", adminHandler.ConfigBackupAction)

			// Maintenance
			ar.Get("/config/maintenance", adminHandler.ConfigMaintenance)
			ar.Post("/config/maintenance", adminHandler.ConfigMaintenanceSave)

			// Updates
			ar.Get("/config/updates", adminHandler.ConfigUpdates)
			ar.Post("/config/updates", adminHandler.ConfigUpdatesAction)

			// Server info
			ar.Get("/config/info", adminHandler.ConfigInfo)

			// Security — auth
			ar.Get("/config/security/auth", adminHandler.ConfigSecurityAuth)
			ar.Post("/config/security/auth", adminHandler.ConfigSecurityAuthSave)

			// Security — API tokens
			ar.Get("/config/security/tokens", adminHandler.ConfigSecurityTokens)
			ar.Post("/config/security/tokens", adminHandler.ConfigSecurityTokensAction)

			// Security — rate limiting
			ar.Get("/config/security/ratelimit", adminHandler.ConfigSecurityRateLimit)
			ar.Post("/config/security/ratelimit", adminHandler.ConfigSecurityRateLimitSave)

			// Security — firewall
			ar.Get("/config/security/firewall", adminHandler.ConfigSecurityFirewall)
			ar.Post("/config/security/firewall", adminHandler.ConfigSecurityFirewallSave)

			// Security — allowlist
			ar.Get("/config/security/allowlist", adminHandler.ConfigSecurityAllowlist)
			ar.Post("/config/security/allowlist", adminHandler.ConfigSecurityAllowlistSave)

			// Network — Tor
			ar.Get("/config/network/tor", adminHandler.ConfigNetworkTor)

			// Network — GeoIP
			ar.Get("/config/network/geoip", adminHandler.ConfigNetworkGeoIP)
			ar.Post("/config/network/geoip", adminHandler.ConfigNetworkGeoIPSave)

			// Network — blocklists
			ar.Get("/config/network/blocklists", adminHandler.ConfigNetworkBlocklists)
			ar.Post("/config/network/blocklists", adminHandler.ConfigNetworkBlocklistsSave)

			// Custom domain management (PART 36) — server-rendered admin pages.
			ar.Get("/config/domains", adminHandler.ConfigDomains)
			ar.Get("/config/domains/{domain}", adminHandler.ConfigDomainDetail)
			ar.Post("/config/domains/{domain}/suspend", adminHandler.ConfigDomainSuspend)
			ar.Post("/config/domains/{domain}/unsuspend", adminHandler.ConfigDomainUnsuspend)
			ar.Post("/config/domains/{domain}/delete", adminHandler.ConfigDomainDelete)

			// User moderation
			ar.Get("/config/users", adminHandler.UserList)
			ar.Get("/config/users/{id}", adminHandler.UserDetail)
			ar.Post("/config/users/{id}/suspend", adminHandler.SuspendUser)
			ar.Post("/config/users/{id}/activate", adminHandler.ActivateUser)
			// Admin force-regenerate recovery keys (PART 17/34)
			ar.Post("/config/users/{id}/recovery-keys", adminHandler.RegenerateRecoveryKeys)

			// User invites
			ar.Get("/config/users/invites", adminHandler.ConfigUsersInvites)
			ar.Post("/config/users/invites", adminHandler.ConfigUsersInvitesAction)
			ar.Post("/config/users/invites/{id}/revoke", adminHandler.ConfigUsersInvitesRevoke)

			// Moderation queue
			ar.Get("/config/moderation/users", adminHandler.ConfigModerationUsers)

			// Cluster
			ar.Get("/config/cluster/nodes", adminHandler.ConfigClusterNodes)
			ar.Get("/config/cluster/add", adminHandler.ConfigClusterAdd)
			ar.Post("/config/cluster/add", adminHandler.ConfigClusterAddAction)

			// Help — under config/ per PART 17 (only {admin_username} and
			// config may be direct children of /server/{admin_path}/).
			ar.Get("/config/help", adminHandler.AdminHelp)
		})
	})

	// Bearer token middleware factory for API routes
	bearerMiddleware := BearerAuthMiddleware(tokenService)

	// /api/autodiscover — NOT versioned; clients call this before knowing the API version (AI.md PART 14).
	s.router.Get("/api/autodiscover", handler.AutodiscoverHandler(s.Version, s.config, func() *apktor.TorManager { return s.torManager }))

	// /api/healthz — unversioned direct alias (AI.md PART 14). Serves the same
	// JSON handler as /api/v1/server/healthz, mounted directly (no redirect).
	s.router.Get("/api/healthz", handler.APIHealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String(), s.store, func() *apktor.TorManager { return s.torManager }, func() (reqTotal, reqs24h, activeConn int64) {
		reqTotal = atomic.LoadInt64(&s.reqTotal)
		activeConn = atomic.LoadInt64(&s.activeConn)
		for i := range s.reqWindow {
			reqs24h += atomic.LoadInt64(&s.reqWindow[i])
		}
		return
	}))

	// API v1
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Use(SecurityHeadersMiddleware(s.config.Server.SSL.Enabled, s.mode.IsDevelopment()))

		// Public endpoints (no auth)
		r.Get("/server/healthz", handler.APIHealthHandler(s.Version, s.CommitID, s.BuildDate, s.mode.String(), s.store, func() *apktor.TorManager { return s.torManager }, func() (reqTotal, reqs24h, activeConn int64) {
			reqTotal = atomic.LoadInt64(&s.reqTotal)
			activeConn = atomic.LoadInt64(&s.activeConn)
			for i := range s.reqWindow {
				reqs24h += atomic.LoadInt64(&s.reqWindow[i])
			}
			return
		}))
		r.Get("/version", handler.VersionHandler(s.Version, s.CommitID, s.BuildDate))
		// OpenAPI JSON spec — canonical per spec PART 14 + IDEA.md
		r.Get("/server/swagger", swagger.SpecHandler(s.Version))
		// GraphQL — canonical versioned route per spec PART 14; same handler
		// as the unversioned /api/graphql alias above.
		r.Post("/server/graphql", graphql.QueryHandler(graphqlResolver))

		// CSP violation report endpoint (AI.md PART 11 report-uri).
		// Accepts browser-POSTed CSP reports and returns 204; the report body is discarded
		// since CSP violations are already visible in browser devtools and the TLS audit log.
		r.Post("/server/reports/csp", func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})

		// Public server information API endpoints per PART 16.
		r.Get("/server/about", pagesHandler.APIAbout)
		r.Get("/server/help", pagesHandler.APIHelp)
		r.Get("/server/privacy", pagesHandler.APIPrivacy)
		r.Get("/server/terms", pagesHandler.APITerms)
		r.Post("/server/contact", pagesHandler.APIContact)

		// Auth API — /api/v1/server/auth/*
		r.Route("/server/auth", func(ar chi.Router) {
			ar.Use(RateLimitMiddleware(rateLimiter))
			ar.Post("/login", authUserHandler.Login)
			ar.Post("/register", authUserHandler.Register)
		})

		// Admin API — /api/v1/server/{adminPath}/*
		r.Route("/server/"+adminPath, func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			// Admin API endpoints accept ONLY admin-scoped (adm_) tokens; a
			// usr_/org_ token must never reach server management (PART 24).
			ar.Use(RequireBearerAdmin)
			ar.Get("/config/users", adminHandler.APIUserList)
			ar.Get("/config/users/{id}", adminHandler.APIUserDetail)
			ar.Post("/config/users/{id}/suspend", adminHandler.APISuspendUser)
			ar.Post("/config/users/{id}/activate", adminHandler.APIActivateUser)
			// Admin force-regenerate recovery keys (PART 17/34)
			ar.Post("/config/users/{id}/recovery-keys", adminHandler.APIRegenerateRecoveryKeys)
			// Server settings API
			ar.Get("/config/settings", adminHandler.APIConfigSettings)
			ar.Patch("/config/settings", adminHandler.APIConfigSettingsSave)
			// Branding API
			ar.Get("/config/branding", adminHandler.APIConfigBranding)
			ar.Patch("/config/branding", adminHandler.APIConfigBrandingSave)
			// Info API
			ar.Get("/config/info", adminHandler.APIConfigInfo)
			// Scheduler API
			ar.Get("/config/scheduler", adminHandler.APIConfigScheduler)
			// Maintenance API
			ar.Get("/config/maintenance", adminHandler.APIConfigMaintenance)
			ar.Patch("/config/maintenance", adminHandler.APIConfigMaintenanceSave)
			// Network/Tor API
			ar.Get("/config/network/tor", adminHandler.APIConfigNetworkTor)
			// Custom-domain administration API (PART 36). Force-delete,
			// suspend, and unsuspend act on any owner's domain; ssl/renew is
			// deferred to the ACME issuance unit.
			ar.Get("/config/domains", domainHandler.APIAdminListDomains)
			ar.Get("/config/domains/{domain}", domainHandler.APIAdminGetDomain)
			ar.Delete("/config/domains/{domain}", domainHandler.APIAdminDeleteDomain)
			ar.Post("/config/domains/{domain}/suspend", domainHandler.APIAdminSuspendDomain)
			ar.Post("/config/domains/{domain}/unsuspend", domainHandler.APIAdminUnsuspendDomain)
		})

		// URL management endpoints (require Bearer auth per spec)
		r.Group(func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			ar.Get("/urls", urlHandler.ListURLs)
			ar.Post("/urls", urlHandler.CreateURL)
			ar.Patch("/urls/{code}", urlHandler.UpdateURL)
			ar.Delete("/urls/{code}", urlHandler.DeleteURL)
		})

		// Public read endpoints
		r.Get("/urls/{code}", urlHandler.GetURL)
		r.Get("/urls/{code}/stats", urlHandler.Stats)

		// QR code endpoints
		r.Get("/qr/{code}", qrHandler.GenerateQR)

		// Users API — /api/v1/users/*
		r.Route("/users", func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			// Current user profile, tokens, settings, and security per PART 14.
			ar.Get("/", userHandler.APIProfile)
			ar.Patch("/", userHandler.APIUpdateProfile)
			ar.Get("/tokens", userHandler.APITokens)
			ar.Post("/tokens", userHandler.APICreateToken)
			ar.Delete("/tokens/{id}", userHandler.APIRevokeToken)
			ar.Get("/settings", userHandler.APISettings)
			ar.Get("/security", userHandler.APISecurity)
			// Bulk URL operations
			ar.Get("/urls/export", bulkHandler.Export)
			ar.Post("/urls/import", bulkHandler.Import)
			// Custom domains (PART 36) — CRUD parity with /users/domains web routes.
			ar.Get("/domains", domainHandler.APIListUserDomains)
			ar.Post("/domains", domainHandler.APIAddUserDomain)
			ar.Get("/domains/{domain}", domainHandler.APIGetUserDomain)
			ar.Get("/domains/{domain}/dns", domainHandler.APIUserDomainDNS)
			ar.Get("/domains/{domain}/ssl", domainHandler.APIUserDomainSSL)
			ar.Delete("/domains/{domain}", domainHandler.APIDeleteUserDomain)
			ar.Post("/domains/{domain}/verify", domainHandler.APIVerifyUserDomain)
		})

		// Orgs API — /api/v1/orgs/*
		r.Route("/orgs", func(ar chi.Router) {
			ar.Use(bearerMiddleware)
			ar.Get("/", orgHandler.APIListOrgs)
			ar.Post("/", orgHandler.APICreateOrg)
			ar.Get("/{slug}", orgHandler.APIGetOrg)
			ar.Patch("/{slug}", orgHandler.APIUpdateOrg)
			ar.Delete("/{slug}", orgHandler.APIDeleteOrg)
			ar.Get("/{slug}/members", orgHandler.APIGetMembers)

			// Org-membership invites (owner/admin only) per PART 35.
			ar.Get("/{slug}/invites", orgHandler.APIListOrgInvites)
			ar.Post("/{slug}/invites", orgHandler.APICreateOrgInvite)
			ar.Delete("/{slug}/invites/{inviteID}", orgHandler.APIRevokeOrgInvite)
			// Org-scoped API tokens (PART 35)
			ar.Get("/{slug}/tokens", orgHandler.APIListOrgTokens)
			ar.Post("/{slug}/tokens", orgHandler.APICreateOrgToken)
			ar.Delete("/{slug}/tokens/{tokenID}", orgHandler.APIRevokeOrgToken)
			// Org ownership transfer (PART 35)
			ar.Post("/{slug}/transfer", orgHandler.APITransferOrgOwnership)
			// Org custom domains (PART 36) — CRUD parity with /orgs/{slug}/domains.
			ar.Get("/{slug}/domains", domainHandler.APIListOrgDomains)
			ar.Post("/{slug}/domains", domainHandler.APIAddOrgDomain)
			ar.Get("/{slug}/domains/{domain}", domainHandler.APIGetOrgDomain)
			ar.Get("/{slug}/domains/{domain}/dns", domainHandler.APIOrgDomainDNS)
			ar.Get("/{slug}/domains/{domain}/ssl", domainHandler.APIOrgDomainSSL)
			ar.Delete("/{slug}/domains/{domain}", domainHandler.APIDeleteOrgDomain)
			ar.Post("/{slug}/domains/{domain}/verify", domainHandler.APIVerifyOrgDomain)
		})
	})

	// Root handler
	s.router.Get("/", s.handleRoot)

	// Web URL creation — PRG (Post/Redirect/Get) for progressive enhancement
	// without JavaScript per AI.md PART 16. The form on the dashboard POSTs here
	// and is redirected to /users/dashboard on success.
	s.router.With(UserAuthMiddleware(authService)).Post("/urls", urlHandler.WebCreateURL)

	// SEO endpoints — registered before the short-URL catch-all so they are
	// served directly and never treated as short codes.
	s.router.Get("/robots.txt", s.robotsTxt)
	s.router.Get("/sitemap.xml", s.sitemapXML)

	// Short URL redirect (must be last to not catch other routes). POST is
	// accepted for the same code so a password-protected link's unlock form
	// (progressive enhancement / no-JS, PART 16) submits back to the same URL.
	s.router.Get("/{code}", urlHandler.RedirectURL)
	s.router.Post("/{code}", urlHandler.RedirectURL)
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

// baseURL returns the scheme + host base URL for the current request, honouring
// X-Forwarded-Proto and X-Forwarded-Host headers set by trusted reverse proxies.
func (s *Server) baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host
}

// robotsTxt generates and serves /robots.txt dynamically.
// The admin path is kept private; the API and all other public pages are allowed.
func (s *Server) robotsTxt(w http.ResponseWriter, r *http.Request) {
	adminPath := s.config.Server.Admin.Path
	if adminPath == "" {
		adminPath = "admin"
	}
	baseURL := s.baseURL(r)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nAllow: /api\nDisallow: /server/%s\nSitemap: %s/sitemap.xml\n", adminPath, baseURL)
}

// sitemapXML generates and serves /sitemap.xml with the static public pages.
func (s *Server) sitemapXML(w http.ResponseWriter, r *http.Request) {
	baseURL := s.baseURL(r)
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	now := time.Now().UTC().Format("2006-01-02")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/</loc><lastmod>%s</lastmod><changefreq>daily</changefreq><priority>1.0</priority></url>
  <url><loc>%s/server/about</loc><lastmod>%s</lastmod><changefreq>weekly</changefreq><priority>0.8</priority></url>
  <url><loc>%s/server/help</loc><lastmod>%s</lastmod><changefreq>weekly</changefreq><priority>0.8</priority></url>
  <url><loc>%s/server/privacy</loc><lastmod>%s</lastmod><changefreq>monthly</changefreq><priority>0.5</priority></url>
  <url><loc>%s/server/terms</loc><lastmod>%s</lastmod><changefreq>monthly</changefreq><priority>0.5</priority></url>
  <url><loc>%s/server/docs/swagger</loc><lastmod>%s</lastmod><changefreq>weekly</changefreq><priority>0.7</priority></url>
</urlset>`, baseURL, now, baseURL, now, baseURL, now, baseURL, now, baseURL, now, baseURL, now)
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

// Start starts the HTTP server. afterBind, when non-nil, is invoked after all
// privileged ports are bound but before any traffic is served — this is where
// the caller drops root privileges (PART 8 step 6: bind privileged ports while
// still root, THEN drop). A nil afterBind skips the drop (used in tests).
func (s *Server) Start(address string, port int, afterBind func() error) error {
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

	// Bind the main listener while still root so a privileged port (80/443)
	// binds successfully; privileges are dropped in afterBind immediately after
	// (PART 8 step 6). Binding here rather than inside ListenAndServe is what
	// lets the bind run as root and the serve run as the dropped user.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind %s: %w", addr, err)
	}

	// Bind the TLS-ALPN-01 :443 listener (also privileged) before dropping.
	// autocert.Manager.TLSConfig() includes the "acme-tls/1" ALPN protocol
	// required by RFC 8737; the challenge server must be reachable on port 443.
	var tlsSrv *http.Server
	var tlsLn net.Listener
	if s.acmeManager != nil &&
		(s.config.Server.SSL.LetsEncrypt.Challenge == "tls-alpn-01" ||
			s.config.Server.SSL.LetsEncrypt.Challenge == "") {
		tlsSrv = &http.Server{
			Addr:         ":443",
			Handler:      s.router,
			TLSConfig:    s.acmeManager.TLSConfig(),
			ReadTimeout:  s.server.ReadTimeout,
			WriteTimeout: s.server.WriteTimeout,
			IdleTimeout:  s.server.IdleTimeout,
		}
		tlsLn, err = net.Listen("tcp", ":443")
		if err != nil {
			log.Printf("HTTPS (TLS-ALPN-01) bind failed (non-fatal — HTTP still running): %v", err)
			tlsSrv = nil
		}
	}

	// All privileged ports are now bound — drop root before serving any traffic.
	if afterBind != nil {
		if err := afterBind(); err != nil {
			log.Printf("Warning: could not drop privileges: %v", err)
		}
	}

	// Write PID file after the drop (PART 8 step 12) so the file is owned by the
	// dropped user. Skip inside containers (PART 8): the runtime supervises the
	// process and a namespace-local PID is meaningless read across namespaces.
	if s.pidFile != "" && s.config.Server.PIDFile && !mode.IsContainer() {
		if err := paths.WritePIDFile(s.pidFile); err != nil {
			log.Printf("Warning: could not write PID file %s: %v", s.pidFile, err)
		}
	}

	// Initialise Tor hidden service (PART 32). TorManager.Start() is a no-op
	// when no Tor binary is found; it never returns an error in that case.
	torMgr := apktor.NewTorManager(context.Background(), port, &s.config.Server.Tor, s.configDir, s.dataDir)
	s.torManager = torMgr
	s.scheduler.SetTorChecker(torMgr)

	// Start scheduler
	s.scheduler.Start()

	// Serve on the already-bound listener (bound as root above, served as the
	// dropped user).
	go func() {
		log.Printf("Server starting on %s (mode: %s)", addr, s.mode)
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			// Exit code 1 (general error) per AI.md's CLI Exit Codes table.
			log.Printf("Server failed: %v", err)
			os.Exit(1)
		}
	}()

	// Serve TLS-ALPN-01 on the pre-bound :443 listener (AI.md PART 15). It also
	// issues and renews certificates via autocert.
	if tlsSrv != nil {
		go func() {
			log.Printf("HTTPS (TLS-ALPN-01) listener starting on :443")
			if err := tlsSrv.ServeTLS(tlsLn, "", ""); err != nil && err != http.ErrServerClosed {
				log.Printf("HTTPS listener error (non-fatal — HTTP still running): %v", err)
			}
		}()
	}

	// Start Tor hidden service after the HTTP listener is up.
	if err := s.torManager.Start(); err != nil {
		log.Printf("[tor] startup error: %v", err)
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit

	log.Println("Shutting down server...")

	// Remove PID file before stopping so monitoring knows we are shutting down.
	if s.pidFile != "" && !mode.IsContainer() {
		paths.RemovePIDFile(s.pidFile)
	}

	// Stop Tor hidden service before scheduler/HTTP so health checks stop first.
	if s.torManager != nil {
		if err := s.torManager.Stop(); err != nil {
			log.Printf("[tor] stop error: %v", err)
		}
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
