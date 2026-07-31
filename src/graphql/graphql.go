package graphql

import (
	"encoding/json"
	"html/template"
	"net/http"
)

// Handler serves the GraphQL query console. It is a plain server-rendered
// HTML form (GET renders an empty console, POST executes the submitted
// query and re-renders the page with the result) — no client-side
// rendering framework, per AI.md PART 16's "NEVER do client-side
// rendering (React/Vue)" rule. It works fully without JavaScript.
func Handler(version string, resolver *Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Detect theme from query param or default to dark
		theme := r.URL.Query().Get("theme")
		if theme == "" {
			theme = "dark"
		}

		var query, resultJSON string
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err == nil {
				query = r.FormValue("query")
			}
			if query != "" {
				result := resolver.Execute(r, query, nil)
				if encoded, err := json.MarshalIndent(result, "", "  "); err == nil {
					resultJSON = string(encoded)
				}
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		tmpl := template.Must(template.New("graphiql").Parse(graphiQLTemplate))
		data := map[string]interface{}{
			"Version": version,
			"Theme":   theme,
			"Query":   query,
			"Result":  resultJSON,
		}
		_ = tmpl.Execute(w, data)
	}
}

// QueryHandler handles GraphQL queries
func QueryHandler(resolver *Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse GraphQL query from request body
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, "Invalid request body")
			return
		}

		result := resolver.Execute(r, req.Query, req.Variables)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// respondError sends an error response
func respondError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"errors": []map[string]interface{}{
			{"message": message},
		},
	})
}

const graphiQLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Caslink GraphQL API</title>
    <style>
        body { margin: 0; padding: 1.5rem; font-family: system-ui, sans-serif; word-break: break-word; }
        .console { max-width: 900px; margin: 0 auto; }
        textarea { width: 100%; min-height: 12rem; font-family: monospace; font-size: 0.9rem;
                   box-sizing: border-box; padding: 0.75rem; border-radius: 6px; }
        pre { white-space: pre-wrap; word-break: break-word; padding: 0.75rem; border-radius: 6px; }
        button { padding: 0.5rem 1.25rem; border-radius: 6px; border: none; cursor: pointer; font-size: 1rem; }
        a { color: inherit; }
        {{if eq .Theme "dark"}}
        {{template "darkTheme"}}
        {{else}}
        {{template "lightTheme"}}
        {{end}}
    </style>
</head>
<body>
    <div class="console">
        <h1>Caslink GraphQL Console</h1>
        <p>Submit a query below (works without JavaScript). Programmatic clients should
        POST JSON to <code>/api/graphql</code> directly.</p>
        <form method="post" action="/server/docs/graphql">
            <textarea name="query" placeholder="query { health { status version uptime } }">{{.Query}}</textarea>
            <p><button type="submit">Run query</button></p>
        </form>
        {{if .Result}}
        <h2>Result</h2>
        <pre>{{.Result}}</pre>
        {{end}}
    </div>
</body>
</html>

{{define "darkTheme"}}
body { background: #282a36; color: #f8f8f2; }
textarea, pre { background: #1e1f29; color: #f8f8f2; border: 1px solid #44475a; }
button { background: #50fa7b; color: #282a36; }
{{end}}

{{define "lightTheme"}}
body { background: #ffffff; color: #1a1a1a; }
textarea, pre { background: #f5f5f5; color: #1a1a1a; border: 1px solid #e0e0e0; }
button { background: #e0e0e0; color: #1a1a1a; }
{{end}}`
