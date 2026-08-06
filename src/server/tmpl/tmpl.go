// Package tmpl provides the embedded static assets and HTML template renderer.
package tmpl

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed static
var staticFS embed.FS

//go:embed template
var templateFS embed.FS

// Renderer holds one compiled template set per page, keyed by embedded path.
//
// Every page file defines a block named "content" (see layout/base.html's
// {{template "content" .}}). html/template associates all named blocks
// globally within a single *template.Template, so parsing every page into
// one shared tree makes each page's "content" definition silently overwrite
// the last one — only the alphabetically-last-parsed page renders correctly
// and every other page renders that page's content instead of its own. To
// avoid this collision, each page gets its own clone of the shared base set
// (layout, nav, inline CSS/JS, funcs), and only that page's file is parsed
// into the clone.
type Renderer struct {
	base  *template.Template
	pages map[string]*template.Template
}

// New parses all templates and loads static assets.
func New() (*Renderer, error) {
	css, err := staticFS.ReadFile("static/css/app.css")
	if err != nil {
		return nil, fmt.Errorf("failed to read app.css: %w", err)
	}
	js, err := staticFS.ReadFile("static/js/app.js")
	if err != nil {
		return nil, fmt.Errorf("failed to read app.js: %w", err)
	}

	base := template.New("").Funcs(template.FuncMap{
		// inc returns n+1; used in range loops for 1-based display indices.
		"inc": func(n int) int { return n + 1 },
		// join renders a []string (e.g. URL.Tags, URL.GeoCountries) as a
		// comma-separated string for pre-filling an editable text input.
		"join": func(vals []string) string { return strings.Join(vals, ", ") },
	})

	// Parse the shared layout (base, nav) once.
	layoutData, err := templateFS.ReadFile("template/layout/base.html")
	if err != nil {
		return nil, fmt.Errorf("failed to read layout/base.html: %w", err)
	}
	if _, err := base.New("template/layout/base.html").Parse(string(layoutData)); err != nil {
		return nil, fmt.Errorf("failed to parse layout/base.html: %w", err)
	}

	// Inject inline CSS and JS into base layout helper blocks.
	if _, err := base.New("inline-css").Parse(string(css)); err != nil {
		return nil, fmt.Errorf("failed to parse inline-css: %w", err)
	}
	if _, err := base.New("inline-js").Parse(string(js)); err != nil {
		return nil, fmt.Errorf("failed to parse inline-js: %w", err)
	}

	// Walk and parse every page (*.html outside layout/) into its own clone
	// of base, so each page's "content" block is isolated from the others.
	pages := make(map[string]*template.Template)
	if err := fs.WalkDir(templateFS, "template", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		if strings.HasPrefix(path, "template/layout/") {
			return nil
		}
		data, readErr := templateFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("failed to read template %s: %w", path, readErr)
		}
		clone, cloneErr := base.Clone()
		if cloneErr != nil {
			return fmt.Errorf("failed to clone base template for %s: %w", path, cloneErr)
		}
		if _, parseErr := clone.New(path).Parse(string(data)); parseErr != nil {
			return fmt.Errorf("failed to parse template %s: %w", path, parseErr)
		}
		pages[path] = clone
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return &Renderer{base: base, pages: pages}, nil
}

// Data is the base template context.
type Data struct {
	AppName            string
	AppDesc            string
	Version            string
	Title              string
	CSRFToken          string
	Theme              string // dark|light|auto
	Lang               string // active language code, e.g. "en"
	AvailableLanguages []LangOption
	User               interface{}
	Flash              *Flash
	// HasConsentCookie is true when the request already carries a
	// cookie_consent cookie; the consent banner is rendered only when false
	// (server decides, so it works without JS) per AI.md PART 12/16.
	HasConsentCookie bool
	// Consent carries the banner text/links/labels; always non-nil so the
	// layout can render the banner when HasConsentCookie is false.
	Consent *ConsentView
	// CurrentPath is the request path+query, posted back with the consent
	// choice so a no-JS visitor is redirected to the page they were on.
	CurrentPath string
}

// ConsentView is the rendered-ready cookie-consent banner context, derived
// from server.privacy.consent (PART 12). Message is pre-selected via
// PrivacyConfig.GetConsentMessage (data-sold aware).
type ConsentView struct {
	Message         string
	PolicyText      string
	PolicyURL       string
	AcceptText      string
	DeclineText     string
	Position        string
	ShowPreferences bool
	PreferencesText string
	DataSold        bool
}

// LangOption is a single entry in the language selector dropdown.
type LangOption struct {
	Code       string
	NativeName string
	Active     bool
}

// Flash is a one-shot UI notification.
type Flash struct {
	Type    string // success|danger|warn
	Message string
}

// Render executes the named page template and writes to w. On render error
// (including an unknown page name), writes 500.
func (r *Renderer) Render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	page, ok := r.pages[name]
	if !ok {
		log.Printf("template render error [%s]: unknown template", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if err := page.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template render error [%s]: %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// StaticHandler returns an http.Handler that serves embedded static files.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// Exit code 1 (general error) per AI.md's CLI Exit Codes table.
		log.Printf("failed to create static sub-FS: %v", err)
		os.Exit(1)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

var startTime = time.Now()

// Uptime returns the number of seconds since process start.
func Uptime() int64 { return int64(time.Since(startTime).Seconds()) }
