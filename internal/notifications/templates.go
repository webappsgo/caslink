package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/db"
	"github.com/sirupsen/logrus"
)

// TemplateService manages notification templates
type TemplateService struct {
	db        *db.DB
	config    *config.Config
	logger    *logrus.Logger
	templates map[string]*NotificationTemplate
}

// NotificationTemplate represents a notification template
type NotificationTemplate struct {
	ID          string                 `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	Type        string                 `json:"type" db:"type"`
	Channel     string                 `json:"channel" db:"channel"`
	Language    string                 `json:"language" db:"language"`
	Subject     string                 `json:"subject" db:"subject"`
	TextBody    string                 `json:"text_body" db:"text_body"`
	HTMLBody    string                 `json:"html_body" db:"html_body"`
	Variables   []string               `json:"variables" db:"variables"`
	IsActive    bool                   `json:"is_active" db:"is_active"`
	IsDefault   bool                   `json:"is_default" db:"is_default"`
	Metadata    map[string]interface{} `json:"metadata" db:"metadata"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// TemplateData represents data for template rendering
type TemplateData struct {
	User         map[string]interface{} `json:"user"`
	URL          map[string]interface{} `json:"url"`
	Domain       map[string]interface{} `json:"domain"`
	System       map[string]interface{} `json:"system"`
	Custom       map[string]interface{} `json:"custom"`
	Timestamp    time.Time              `json:"timestamp"`
	BaseURL      string                 `json:"base_url"`
	BrandName    string                 `json:"brand_name"`
	SupportEmail string                 `json:"support_email"`
}

// NewTemplateService creates a new template service
func NewTemplateService(database *db.DB, cfg *config.Config, logger *logrus.Logger) (*TemplateService, error) {
	service := &TemplateService{
		db:        database,
		config:    cfg,
		logger:    logger,
		templates: make(map[string]*NotificationTemplate),
	}

	// Load default templates
	if err := service.loadDefaultTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load default templates: %w", err)
	}

	return service, nil
}

// GetTemplate retrieves a template by name
func (ts *TemplateService) GetTemplate(ctx context.Context, name string) (*NotificationTemplate, error) {
	// Check in-memory cache first
	if template, exists := ts.templates[name]; exists {
		return template, nil
	}

	// Query database
	query := `
		SELECT id, name, description, type, channel, language, subject, text_body, html_body,
		       variables, is_active, is_default, metadata, created_at, updated_at
		FROM notification_templates
		WHERE name = ? AND is_active = true`

	row := ts.db.QueryRowContext(ctx, query, name)

	template := &NotificationTemplate{}
	var variablesJSON, metadataJSON string

	err := row.Scan(
		&template.ID, &template.Name, &template.Description, &template.Type,
		&template.Channel, &template.Language, &template.Subject, &template.TextBody,
		&template.HTMLBody, &variablesJSON, &template.IsActive, &template.IsDefault,
		&metadataJSON, &template.CreatedAt, &template.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}

	// Parse JSON fields
	if variablesJSON != "" {
		if err := json.Unmarshal([]byte(variablesJSON), &template.Variables); err != nil {
			ts.logger.WithError(err).Warn("Failed to parse template variables")
		}
	}

	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &template.Metadata); err != nil {
			ts.logger.WithError(err).Warn("Failed to parse template metadata")
		}
	}

	// Cache the template
	ts.templates[name] = template

	return template, nil
}

// RenderTemplate renders a template with the given data
func (ts *TemplateService) RenderTemplate(tmpl *NotificationTemplate, data map[string]interface{}) (string, string, error) {
	templateData := ts.buildTemplateData(data)

	// Render subject
	subject, err := ts.renderString(tmpl.Subject, templateData)
	if err != nil {
		return "", "", fmt.Errorf("failed to render subject: %w", err)
	}

	// Render text body
	textBody, err := ts.renderString(tmpl.TextBody, templateData)
	if err != nil {
		return "", "", fmt.Errorf("failed to render text body: %w", err)
	}

	return subject, textBody, nil
}

// RenderHTMLTemplate renders the HTML version of a template
func (ts *TemplateService) RenderHTMLTemplate(tmpl *NotificationTemplate, data map[string]interface{}) (string, error) {
	if tmpl.HTMLBody == "" {
		return "", fmt.Errorf("template has no HTML body")
	}

	templateData := ts.buildTemplateData(data)

	return ts.renderString(tmpl.HTMLBody, templateData)
}

// CreateTemplate creates a new notification template
func (ts *TemplateService) CreateTemplate(ctx context.Context, template *NotificationTemplate) error {
	if template.ID == "" {
		template.ID = ts.generateTemplateID()
	}

	template.CreatedAt = time.Now()
	template.UpdatedAt = time.Now()

	// Validate template
	if err := ts.validateTemplate(template); err != nil {
		return fmt.Errorf("template validation failed: %w", err)
	}

	// Save to database
	if err := ts.saveTemplate(ctx, template); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	// Update cache
	ts.templates[template.Name] = template

	ts.logger.WithField("template_name", template.Name).Info("Template created")
	return nil
}

// UpdateTemplate updates an existing template
func (ts *TemplateService) UpdateTemplate(ctx context.Context, template *NotificationTemplate) error {
	template.UpdatedAt = time.Now()

	// Validate template
	if err := ts.validateTemplate(template); err != nil {
		return fmt.Errorf("template validation failed: %w", err)
	}

	// Save to database
	if err := ts.saveTemplate(ctx, template); err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	// Update cache
	ts.templates[template.Name] = template

	ts.logger.WithField("template_name", template.Name).Info("Template updated")
	return nil
}

// DeleteTemplate deletes a template
func (ts *TemplateService) DeleteTemplate(ctx context.Context, templateName string) error {
	// Check if it's a default template
	template, err := ts.GetTemplate(ctx, templateName)
	if err != nil {
		return err
	}

	if template.IsDefault {
		return fmt.Errorf("cannot delete default template")
	}

	// Delete from database
	_, err = ts.db.ExecContext(ctx, "DELETE FROM notification_templates WHERE name = ?", templateName)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}

	// Remove from cache
	delete(ts.templates, templateName)

	ts.logger.WithField("template_name", templateName).Info("Template deleted")
	return nil
}

// ListTemplates lists all available templates
func (ts *TemplateService) ListTemplates(ctx context.Context, channel string) ([]*NotificationTemplate, error) {
	query := `
		SELECT id, name, description, type, channel, language, subject, text_body, html_body,
		       variables, is_active, is_default, metadata, created_at, updated_at
		FROM notification_templates
		WHERE is_active = true`

	args := []interface{}{}
	if channel != "" {
		query += " AND channel = ?"
		args = append(args, channel)
	}

	query += " ORDER BY is_default DESC, name ASC"

	rows, err := ts.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query templates: %w", err)
	}
	defer rows.Close()

	var templates []*NotificationTemplate
	for rows.Next() {
		template := &NotificationTemplate{}
		var variablesJSON, metadataJSON string

		err := rows.Scan(
			&template.ID, &template.Name, &template.Description, &template.Type,
			&template.Channel, &template.Language, &template.Subject, &template.TextBody,
			&template.HTMLBody, &variablesJSON, &template.IsActive, &template.IsDefault,
			&metadataJSON, &template.CreatedAt, &template.UpdatedAt,
		)

		if err != nil {
			ts.logger.WithError(err).Warn("Failed to scan template")
			continue
		}

		// Parse JSON fields
		if variablesJSON != "" {
			if err := json.Unmarshal([]byte(variablesJSON), &template.Variables); err != nil {
				ts.logger.WithError(err).Warn("Failed to parse template variables")
			}
		}

		if metadataJSON != "" {
			if err := json.Unmarshal([]byte(metadataJSON), &template.Metadata); err != nil {
				ts.logger.WithError(err).Warn("Failed to parse template metadata")
			}
		}

		templates = append(templates, template)
	}

	return templates, nil
}

// loadDefaultTemplates loads default notification templates
func (ts *TemplateService) loadDefaultTemplates() error {
	defaultTemplates := []*NotificationTemplate{
		{
			ID:          "welcome_email",
			Name:        "welcome_email",
			Description: "Welcome email for new users",
			Type:        "user_welcome",
			Channel:     "email",
			Language:    "en",
			Subject:     "Welcome to {{.BrandName}}!",
			TextBody: `Hello {{.User.username}},

Welcome to {{.BrandName}}! Your account has been successfully created.

You can now start creating short URLs and tracking analytics.

Best regards,
The {{.BrandName}} Team`,
			HTMLBody: `<h1>Welcome to {{.BrandName}}!</h1>
<p>Hello {{.User.username}},</p>
<p>Welcome to {{.BrandName}}! Your account has been successfully created.</p>
<p>You can now start creating short URLs and tracking analytics.</p>
<p>Best regards,<br>The {{.BrandName}} Team</p>`,
			Variables: []string{"User.username", "BrandName"},
			IsActive:  true,
			IsDefault: true,
		},
		{
			ID:          "password_reset",
			Name:        "password_reset",
			Description: "Password reset email",
			Type:        "password_reset",
			Channel:     "email",
			Language:    "en",
			Subject:     "Password Reset - {{.BrandName}}",
			TextBody: `Hello {{.User.username}},

You requested a password reset for your {{.BrandName}} account.

Click the link below to reset your password:
{{.Custom.reset_url}}

This link will expire in 24 hours.

If you didn't request this reset, please ignore this email.

Best regards,
The {{.BrandName}} Team`,
			HTMLBody: `<h1>Password Reset</h1>
<p>Hello {{.User.username}},</p>
<p>You requested a password reset for your {{.BrandName}} account.</p>
<p><a href="{{.Custom.reset_url}}" style="background: #007cba; color: white; padding: 10px 20px; text-decoration: none; border-radius: 4px;">Reset Password</a></p>
<p>This link will expire in 24 hours.</p>
<p>If you didn't request this reset, please ignore this email.</p>
<p>Best regards,<br>The {{.BrandName}} Team</p>`,
			Variables: []string{"User.username", "BrandName", "Custom.reset_url"},
			IsActive:  true,
			IsDefault: true,
		},
		{
			ID:          "url_expired",
			Name:        "url_expired",
			Description: "URL expiration notification",
			Type:        "url_expired",
			Channel:     "email",
			Language:    "en",
			Subject:     "URL Expired - {{.URL.short_code}}",
			TextBody: `Hello {{.User.username}},

Your URL has expired:
Short URL: {{.BaseURL}}/{{.URL.short_code}}
Original URL: {{.URL.original_url}}
Expired on: {{.URL.expired_at}}

You can create a new short URL by visiting {{.BaseURL}}.

Best regards,
The {{.BrandName}} Team`,
			HTMLBody: `<h1>URL Expired</h1>
<p>Hello {{.User.username}},</p>
<p>Your URL has expired:</p>
<ul>
<li><strong>Short URL:</strong> {{.BaseURL}}/{{.URL.short_code}}</li>
<li><strong>Original URL:</strong> {{.URL.original_url}}</li>
<li><strong>Expired on:</strong> {{.URL.expired_at}}</li>
</ul>
<p><a href="{{.BaseURL}}">Create a new short URL</a></p>
<p>Best regards,<br>The {{.BrandName}} Team</p>`,
			Variables: []string{"User.username", "URL.short_code", "URL.original_url", "URL.expired_at", "BaseURL", "BrandName"},
			IsActive:  true,
			IsDefault: true,
		},
		{
			ID:          "ssl_expiring",
			Name:        "ssl_expiring",
			Description: "SSL certificate expiring notification",
			Type:        "ssl_expiring",
			Channel:     "email",
			Language:    "en",
			Subject:     "SSL Certificate Expiring - {{.Domain.name}}",
			TextBody: `Hello {{.User.username}},

Your SSL certificate for domain {{.Domain.name}} is expiring soon.

Expiration Date: {{.Domain.ssl_expires_at}}
Days Remaining: {{.Domain.days_remaining}}

Please renew your SSL certificate to avoid service interruption.

Best regards,
The {{.BrandName}} Team`,
			HTMLBody: `<h1>SSL Certificate Expiring</h1>
<p>Hello {{.User.username}},</p>
<p>Your SSL certificate for domain <strong>{{.Domain.name}}</strong> is expiring soon.</p>
<ul>
<li><strong>Expiration Date:</strong> {{.Domain.ssl_expires_at}}</li>
<li><strong>Days Remaining:</strong> {{.Domain.days_remaining}}</li>
</ul>
<p>Please renew your SSL certificate to avoid service interruption.</p>
<p>Best regards,<br>The {{.BrandName}} Team</p>`,
			Variables: []string{"User.username", "Domain.name", "Domain.ssl_expires_at", "Domain.days_remaining", "BrandName"},
			IsActive:  true,
			IsDefault: true,
		},
	}

	for _, template := range defaultTemplates {
		ctx := context.Background()

		// Check if template already exists
		_, err := ts.GetTemplate(ctx, template.Name)
		if err == nil {
			// Template exists, skip
			continue
		}

		if err := ts.CreateTemplate(ctx, template); err != nil {
			ts.logger.WithError(err).WithField("template", template.Name).Error("Failed to create default template")
		}
	}

	return nil
}

// Helper methods

func (ts *TemplateService) buildTemplateData(data map[string]interface{}) *TemplateData {
	templateData := &TemplateData{
		Timestamp:    time.Now(),
		BaseURL:      ts.config.Server.BaseURL,
		BrandName:    ts.config.Application.BrandName,
		SupportEmail: ts.config.Email.FromAddress,
		User:         make(map[string]interface{}),
		URL:          make(map[string]interface{}),
		Domain:       make(map[string]interface{}),
		System:       make(map[string]interface{}),
		Custom:       make(map[string]interface{}),
	}

	// Populate data from input
	if data != nil {
		if user, ok := data["user"].(map[string]interface{}); ok {
			templateData.User = user
		}
		if url, ok := data["url"].(map[string]interface{}); ok {
			templateData.URL = url
		}
		if domain, ok := data["domain"].(map[string]interface{}); ok {
			templateData.Domain = domain
		}
		if system, ok := data["system"].(map[string]interface{}); ok {
			templateData.System = system
		}
		if custom, ok := data["custom"].(map[string]interface{}); ok {
			templateData.Custom = custom
		}

		// Add any additional fields directly
		for key, value := range data {
			if key != "user" && key != "url" && key != "domain" && key != "system" && key != "custom" {
				templateData.Custom[key] = value
			}
		}
	}

	return templateData
}

func (ts *TemplateService) renderString(templateStr string, data *TemplateData) (string, error) {
	tmpl, err := template.New("notification").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func (ts *TemplateService) validateTemplate(template *NotificationTemplate) error {
	if template.Name == "" {
		return fmt.Errorf("template name is required")
	}

	if template.Type == "" {
		return fmt.Errorf("template type is required")
	}

	if template.Channel == "" {
		return fmt.Errorf("template channel is required")
	}

	if template.Subject == "" {
		return fmt.Errorf("template subject is required")
	}

	if template.TextBody == "" && template.HTMLBody == "" {
		return fmt.Errorf("template must have either text body or HTML body")
	}

	// Validate template syntax
	if _, err := template.New("test").Parse(template.Subject); err != nil {
		return fmt.Errorf("invalid subject template syntax: %w", err)
	}

	if template.TextBody != "" {
		if _, err := template.New("test").Parse(template.TextBody); err != nil {
			return fmt.Errorf("invalid text body template syntax: %w", err)
		}
	}

	if template.HTMLBody != "" {
		if _, err := template.New("test").Parse(template.HTMLBody); err != nil {
			return fmt.Errorf("invalid HTML body template syntax: %w", err)
		}
	}

	return nil
}

func (ts *TemplateService) saveTemplate(ctx context.Context, template *NotificationTemplate) error {
	variablesJSON, err := json.Marshal(template.Variables)
	if err != nil {
		return fmt.Errorf("failed to marshal variables: %w", err)
	}

	metadataJSON, err := json.Marshal(template.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT OR REPLACE INTO notification_templates
		(id, name, description, type, channel, language, subject, text_body, html_body,
		 variables, is_active, is_default, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = ts.db.ExecContext(ctx, query,
		template.ID, template.Name, template.Description, template.Type,
		template.Channel, template.Language, template.Subject, template.TextBody,
		template.HTMLBody, string(variablesJSON), template.IsActive, template.IsDefault,
		string(metadataJSON), template.CreatedAt, template.UpdatedAt,
	)

	return err
}

func (ts *TemplateService) generateTemplateID() string {
	return fmt.Sprintf("tmpl_%d", time.Now().UnixNano())
}

// ExtractVariables extracts template variables from template content
func (ts *TemplateService) ExtractVariables(templateContent string) []string {
	var variables []string

	// Simple regex-based extraction of {{.Variable}} patterns
	// This is a basic implementation - could be enhanced with proper template parsing
	parts := strings.Split(templateContent, "{{")
	for _, part := range parts[1:] {
		if endIdx := strings.Index(part, "}}"); endIdx != -1 {
			variable := strings.TrimSpace(part[:endIdx])
			if strings.HasPrefix(variable, ".") {
				variable = variable[1:] // Remove leading dot
			}
			variables = append(variables, variable)
		}
	}

	return variables
}