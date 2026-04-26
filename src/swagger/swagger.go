package swagger

import (
	"encoding/json"
	"html/template"
	"net/http"
)

// Handler serves the Swagger UI
func Handler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Detect theme from query param or default to dark
		theme := r.URL.Query().Get("theme")
		if theme == "" {
			theme = "dark"
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		tmpl := template.Must(template.New("swagger").Parse(swaggerUITemplate))
		data := map[string]interface{}{
			"Version": version,
			"Theme":   theme,
		}
		tmpl.Execute(w, data)
	}
}

// SpecHandler serves the OpenAPI specification JSON
func SpecHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spec := generateOpenAPISpec(version)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(spec)
	}
}

// generateOpenAPISpec generates the OpenAPI 3.0 specification
func generateOpenAPISpec(version string) map[string]interface{} {
	return map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "Caslink API",
			"description": "Self-Hosted URL Shortener API",
			"version":     version,
			"contact": map[string]interface{}{
				"name": "casapps",
				"url":  "https://github.com/casapps/caslink",
			},
			"license": map[string]interface{}{
				"name": "MIT",
				"url":  "https://github.com/casapps/caslink/blob/main/LICENSE.md",
			},
		},
		"servers": []map[string]interface{}{
			{
				"url":         "/api/v1",
				"description": "API v1",
			},
		},
		"paths": map[string]interface{}{
			"/healthz": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Health check",
					"description": "Returns the health status of the server",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Server is healthy",
						},
					},
				},
			},
			"/version": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Version information",
					"description": "Returns server version and build information",
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "Version information",
						},
					},
				},
			},
			"/urls": map[string]interface{}{
				"post": map[string]interface{}{
					"summary":     "Create short URL",
					"description": "Creates a new shortened URL",
					"requestBody": map[string]interface{}{
						"required": true,
						"content": map[string]interface{}{
							"application/json": map[string]interface{}{
								"schema": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"url": map[string]interface{}{
											"type":        "string",
											"description": "Long URL to shorten",
											"example":     "https://example.com",
										},
										"custom_code": map[string]interface{}{
											"type":        "string",
											"description": "Custom short code (optional, 3-50 chars)",
											"example":     "mylink",
										},
									},
									"required": []string{"url"},
								},
							},
						},
					},
					"responses": map[string]interface{}{
						"201": map[string]interface{}{
							"description": "URL created successfully",
						},
						"400": map[string]interface{}{
							"description": "Invalid request",
						},
						"409": map[string]interface{}{
							"description": "Short code already exists",
						},
					},
				},
			},
			"/urls/{code}": map[string]interface{}{
				"get": map[string]interface{}{
					"summary":     "Get URL details",
					"description": "Retrieves details for a shortened URL",
					"parameters": []map[string]interface{}{
						{
							"name":        "code",
							"in":          "path",
							"required":    true,
							"description": "Short code",
							"schema": map[string]interface{}{
								"type": "string",
							},
						},
					},
					"responses": map[string]interface{}{
						"200": map[string]interface{}{
							"description": "URL details",
						},
						"404": map[string]interface{}{
							"description": "URL not found",
						},
						"410": map[string]interface{}{
							"description": "URL has expired",
						},
					},
				},
			},
		},
	}
}

const swaggerUITemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Caslink API Documentation</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
    <style>
        html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
        *, *:before, *:after { box-sizing: inherit; }
        body { margin: 0; padding: 0; }
        {{if eq .Theme "dark"}}
        {{template "darkTheme"}}
        {{else}}
        {{template "lightTheme"}}
        {{end}}
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
    <script>
        window.onload = function() {
            SwaggerUIBundle({
                url: "/swagger/spec.json",
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [
                    SwaggerUIBundle.presets.apis,
                    SwaggerUIBundle.SwaggerUIStandalonePreset
                ],
                layout: "BaseLayout"
            });
        };
    </script>
</body>
</html>

{{define "darkTheme"}}
.swagger-ui { background: #282a36; }
.swagger-ui .topbar { background: #1e1f29; }
.swagger-ui .info .title { color: #f8f8f2; }
.swagger-ui .opblock-tag { color: #f8f8f2; }
.swagger-ui .opblock.opblock-get { background: rgba(139, 233, 253, 0.1); border-color: #8be9fd; }
.swagger-ui .opblock.opblock-post { background: rgba(80, 250, 123, 0.1); border-color: #50fa7b; }
.swagger-ui .opblock.opblock-put { background: rgba(255, 184, 108, 0.1); border-color: #ffb86c; }
.swagger-ui .opblock.opblock-delete { background: rgba(255, 85, 85, 0.1); border-color: #ff5555; }
{{end}}

{{define "lightTheme"}}
.swagger-ui { background: #ffffff; }
.swagger-ui .topbar { background: #f5f5f5; border-bottom: 1px solid #e0e0e0; }
{{end}}`
