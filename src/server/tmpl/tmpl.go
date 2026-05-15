// Package tmpl provides the embedded static assets and HTML template renderer.
package tmpl

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

//go:embed static
var staticFS embed.FS

//go:embed template
var templateFS embed.FS

// Renderer holds the compiled template set and inline CSS/JS.
type Renderer struct {
	templates *template.Template
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

	t := template.New("")

	// Walk and parse all *.html files.
	if err := fs.WalkDir(templateFS, "template", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		data, readErr := templateFS.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("failed to read template %s: %w", path, readErr)
		}
		_, parseErr := t.New(path).Parse(string(data))
		if parseErr != nil {
			return fmt.Errorf("failed to parse template %s: %w", path, parseErr)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	// Inject inline CSS and JS into base layout helper blocks.
	if _, err := t.New("inline-css").Parse(string(css)); err != nil {
		return nil, fmt.Errorf("failed to parse inline-css: %w", err)
	}
	if _, err := t.New("inline-js").Parse(string(js)); err != nil {
		return nil, fmt.Errorf("failed to parse inline-js: %w", err)
	}

	return &Renderer{templates: t}, nil
}

// Data is the base template context.
type Data struct {
	AppName   string
	AppDesc   string
	Version   string
	Title     string
	CSRFToken string
	Theme     string // dark|light|auto
	User      interface{}
	Flash     *Flash
}

// Flash is a one-shot UI notification.
type Flash struct {
	Type    string // success|danger|warn
	Message string
}

// Render executes the named template and writes to w. On render error, writes 500.
func (r *Renderer) Render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template render error [%s]: %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// StaticHandler returns an http.Handler that serves embedded static files.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("failed to create static sub-FS: %v", err)
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(sub)))
}

var startTime = time.Now()

// Uptime returns the number of seconds since process start.
func Uptime() int64 { return int64(time.Since(startTime).Seconds()) }
