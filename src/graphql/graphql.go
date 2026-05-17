package graphql

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFiles embed.FS

// StaticHandler serves embedded GraphiQL vendor assets at /server/docs/graphql/static/*.
func StaticHandler() http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("graphql: failed to sub static FS: " + err.Error())
	}
	return http.StripPrefix("/server/docs/graphql/static/", http.FileServer(http.FS(sub)))
}

// Handler serves the GraphiQL UI
func Handler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Detect theme from query param or default to dark
		theme := r.URL.Query().Get("theme")
		if theme == "" {
			theme = "dark"
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		tmpl := template.Must(template.New("graphiql").Parse(graphiQLTemplate))
		data := map[string]interface{}{
			"Version": version,
			"Theme":   theme,
		}
		_ = tmpl.Execute(w, data)
	}
}

// QueryHandler handles GraphQL queries
func QueryHandler() http.HandlerFunc {
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

		// Execute query (simplified for now)
		result := executeQuery(req.Query, req.Variables)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// SchemaHandler serves the GraphQL schema
func SchemaHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := GetSchema()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(schema))
	}
}

// executeQuery executes a GraphQL query
func executeQuery(query string, variables map[string]interface{}) map[string]interface{} {
	_ = query
	_ = variables
	return map[string]interface{}{
		"data": map[string]interface{}{
			"health": map[string]interface{}{
				"status":  "healthy",
				"message": "GraphQL endpoint functional",
			},
		},
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
    <link rel="stylesheet" href="/server/docs/graphql/static/graphiql.min.css">
    <style>
        html, body, #graphiql { height: 100%; margin: 0; padding: 0; }
        {{if eq .Theme "dark"}}
        {{template "darkTheme"}}
        {{else}}
        {{template "lightTheme"}}
        {{end}}
    </style>
</head>
<body>
    <div id="graphiql"></div>
    <script src="/server/docs/graphql/static/react.production.min.js"></script>
    <script src="/server/docs/graphql/static/react-dom.production.min.js"></script>
    <script src="/server/docs/graphql/static/graphiql.min.js"></script>
    <script>
        const fetcher = GraphiQL.createFetcher({ url: '/graphql' });
        ReactDOM.render(
            React.createElement(GraphiQL, { fetcher: fetcher }),
            document.getElementById('graphiql')
        );
    </script>
</body>
</html>

{{define "darkTheme"}}
.graphiql-container { background: #282a36; color: #f8f8f2; }
.CodeMirror { background: #282a36; color: #f8f8f2; }
.CodeMirror-gutters { background: #1e1f29; border-right: 1px solid #44475a; }
.result-window { background: #282a36; }
.execute-button { background: #50fa7b; color: #282a36; }
.toolbar-button { background: #44475a; color: #f8f8f2; }
{{end}}

{{define "lightTheme"}}
.graphiql-container { background: #ffffff; color: #1a1a1a; }
.CodeMirror { background: #ffffff; color: #1a1a1a; }
{{end}}`
