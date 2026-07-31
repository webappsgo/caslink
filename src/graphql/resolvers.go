package graphql

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/webappsgo/caslink/src/server/model"
	"github.com/webappsgo/caslink/src/server/service"
)

// startTime marks when this process (and therefore the GraphQL resolver
// layer) started, used for the health.uptime field — mirrors handler/
// health.go's own startTime, which this package cannot import directly.
var startTime = time.Now()

// Info carries the build/runtime metadata the health/version resolvers
// expose, mirroring handler.HealthHandler/VersionHandler's fields.
type Info struct {
	Version   string
	CommitID  string
	BuildDate string
	Mode      string
}

// Resolver backs the GraphQL schema in schema.go against the real
// URL/token services, per AI.md PART 14 ("the REQUIRED GraphQL endpoint
// actually works"). Auth is optional-per-request (unlike the REST API's
// BearerAuthMiddleware, which 401s unconditionally): a single GraphQL
// endpoint serves both anonymous (public URL lookups) and authenticated
// (owned-link mutations) operations, so the Bearer token is validated
// directly from the request inside the resolver instead of via router
// middleware.
type Resolver struct {
	urlService   *service.URLService
	tokenService *service.TokenService
	info         Info
}

// NewResolver constructs a Resolver.
func NewResolver(urlService *service.URLService, tokenService *service.TokenService, info Info) *Resolver {
	return &Resolver{urlService: urlService, tokenService: tokenService, info: info}
}

// Execute parses and runs a single GraphQL query/mutation document and
// returns the canonical GraphQL response envelope:
// {"data": {...}, "errors": [...]} — errors is present only when at least
// one top-level field failed to resolve (partial-success semantics).
func (res *Resolver) Execute(r *http.Request, query string, variables map[string]interface{}) map[string]interface{} {
	doc, err := parseDocument(query)
	if err != nil {
		return map[string]interface{}{
			"data": nil,
			"errors": []map[string]interface{}{
				{"message": "Invalid GraphQL query: " + err.Error()},
			},
		}
	}
	if len(doc.selection) == 0 {
		return map[string]interface{}{
			"data": nil,
			"errors": []map[string]interface{}{
				{"message": "GraphQL document has no selected fields"},
			},
		}
	}

	ctx := r.Context()
	data := map[string]interface{}{}
	var errs []map[string]interface{}

	for _, f := range doc.selection {
		key := f.Name
		if f.Alias != "" {
			key = f.Alias
		}

		var val interface{}
		var ferr error
		switch doc.operation {
		case "mutation":
			val, ferr = res.resolveMutationField(ctx, r, f, variables)
		default:
			val, ferr = res.resolveQueryField(ctx, r, f, variables)
		}

		if ferr != nil {
			data[key] = nil
			errs = append(errs, map[string]interface{}{
				"message": ferr.Error(),
				"path":    []string{key},
			})
			continue
		}
		data[key] = val
	}

	result := map[string]interface{}{"data": data}
	if len(errs) > 0 {
		result["errors"] = errs
	}
	return result
}

func (res *Resolver) resolveQueryField(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	switch f.Name {
	case "health":
		return res.resolveHealth(f), nil
	case "version":
		return res.resolveVersion(f), nil
	case "url":
		return res.resolveURL(ctx, r, f, variables)
	case "urls":
		return res.resolveURLs(ctx, r, f, variables)
	default:
		return nil, fmt.Errorf("unknown query field %q", f.Name)
	}
}

func (res *Resolver) resolveMutationField(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	switch f.Name {
	case "createURL":
		return res.resolveCreateURL(ctx, r, f, variables)
	case "updateURL":
		return res.resolveUpdateURL(ctx, r, f, variables)
	case "deleteURL":
		return res.resolveDeleteURL(ctx, r, f, variables)
	default:
		return nil, fmt.Errorf("unknown mutation field %q", f.Name)
	}
}

// ---- health / version ----------------------------------------------------

func (res *Resolver) resolveHealth(f astField) map[string]interface{} {
	h := map[string]interface{}{
		"status":    "healthy",
		"version":   res.info.Version,
		"mode":      res.info.Mode,
		"uptime":    formatUptime(time.Since(startTime)),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return project(h, f.Selection)
}

func (res *Resolver) resolveVersion(f astField) map[string]interface{} {
	v := map[string]interface{}{
		"version":    res.info.Version,
		"commit":     res.info.CommitID,
		"built":      res.info.BuildDate,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}
	return project(v, f.Selection)
}

// formatUptime renders a duration as "Xd Xh Xm Xs", matching
// handler/health.go's uptime formatting.
func formatUptime(d time.Duration) string {
	d = d.Round(time.Second)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	minutes := d / time.Minute
	d -= minutes * time.Minute
	seconds := d / time.Second
	return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
}

// project narrows a flat scalar-field map down to only the requested
// (possibly aliased) sub-fields. Every GraphQL object type in this schema
// (Health, Version, URL) has scalar-only fields, so no recursive
// projection is needed.
func project(obj map[string]interface{}, sel []astField) map[string]interface{} {
	if len(sel) == 0 {
		return obj
	}
	out := make(map[string]interface{}, len(sel))
	for _, f := range sel {
		alias := f.Name
		if f.Alias != "" {
			alias = f.Alias
		}
		out[alias] = obj[f.Name]
	}
	return out
}

// ---- url / urls -----------------------------------------------------------

func (res *Resolver) resolveURL(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	code, err := stringArg(f, "code", variables, true)
	if err != nil {
		return nil, err
	}

	u, err := res.urlService.GetURLByCode(ctx, code)
	if err != nil {
		if err == model.ErrURLNotFound || err == model.ErrURLExpired {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load URL")
	}
	if !res.canView(r, u) {
		return nil, nil
	}
	return project(urlToMap(u), f.Selection), nil
}

// resolveURLs lists the Bearer-authenticated caller's own links, mirroring
// handler.URLHandler.ListURLs (a user or org token is required; admin
// tokens are rejected the same way REST rejects them there).
func (res *Resolver) resolveURLs(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	rec, ok := res.bearerFromRequest(r)
	if !ok || !(strings.EqualFold(rec.OwnerType, "user") || strings.EqualFold(rec.OwnerType, "org")) {
		return nil, fmt.Errorf("bearer user or org token required")
	}

	limit := 10
	if n, err := intArg(f, "limit", variables); err == nil && n > 0 {
		limit = n
	}
	if limit > 200 {
		limit = 200
	}
	offset := 0
	if n, err := intArg(f, "offset", variables); err == nil && n >= 0 {
		offset = n
	}

	var urls []*model.URL
	var err error
	if strings.EqualFold(rec.OwnerType, "org") {
		urls, err = res.urlService.ListByOrgPage(ctx, rec.OwnerID, limit, offset)
	} else {
		urls, err = res.urlService.ListByUserPage(ctx, rec.OwnerID, limit, offset)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list URLs")
	}

	out := make([]map[string]interface{}, len(urls))
	for i, u := range urls {
		out[i] = project(urlToMap(u), f.Selection)
	}
	return out, nil
}

// canView mirrors handler.URLHandler.canViewURL.
func (res *Resolver) canView(r *http.Request, u *model.URL) bool {
	rec, ok := res.bearerFromRequest(r)
	if !ok {
		return service.CanView(u, false, "", 0)
	}
	return service.CanView(u, strings.EqualFold(rec.OwnerType, "admin"), rec.OwnerType, rec.OwnerID)
}

// urlToMap converts a model.URL into the scalar-only shape the URL GraphQL
// type exposes (id, short_code, long_url, title, description, custom_code,
// expires_at, created_at, updated_at).
func urlToMap(u *model.URL) map[string]interface{} {
	m := map[string]interface{}{
		"id":          u.ID,
		"short_code":  u.ShortCode,
		"long_url":    u.LongURL,
		"custom_code": u.CustomCode,
		"created_at":  u.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  u.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if u.Title != nil {
		m["title"] = *u.Title
	}
	if u.Description != nil {
		m["description"] = *u.Description
	}
	if u.ExpiresAt != nil {
		m["expires_at"] = u.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return m
}

// ---- mutations --------------------------------------------------------

// resolveCreateURL mirrors handler.URLHandler.CreateURL's bearer-ownership
// dispatch: user/org tokens own the created link, admin (or anonymous)
// tokens create an unowned link.
func (res *Resolver) resolveCreateURL(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	input, err := objectArg(f, "input", variables, true)
	if err != nil {
		return nil, err
	}
	req := &model.CreateURLRequest{
		LongURL:     toString(input["url"]),
		CustomCode:  toString(input["custom_code"]),
		Title:       toStringPtr(input["title"]),
		Description: toStringPtr(input["description"]),
		Password:    toString(input["password"]),
		ExpireAfter: toString(input["expire_after"]),
	}
	if req.LongURL == "" {
		return nil, fmt.Errorf("input.url is required")
	}

	var u *model.URL
	switch rec, ok := res.bearerFromRequest(r); {
	case ok && strings.EqualFold(rec.OwnerType, "user"):
		u, err = res.urlService.CreateURLForUser(ctx, rec.OwnerID, req)
	case ok && strings.EqualFold(rec.OwnerType, "org"):
		u, err = res.urlService.CreateURLForOrg(ctx, rec.OwnerID, req)
	default:
		u, err = res.urlService.CreateURL(ctx, req)
	}
	if err != nil {
		return nil, createURLError(err)
	}
	return project(urlToMap(u), f.Selection), nil
}

func createURLError(err error) error {
	switch err {
	case model.ErrCodeAlreadyExists:
		return fmt.Errorf("short code already exists")
	case model.ErrReservedWord:
		return fmt.Errorf("short code is a reserved word")
	case model.ErrInvalidCustomCode:
		return fmt.Errorf("invalid custom code")
	default:
		return fmt.Errorf("failed to create URL")
	}
}

// resolveUpdateURL mirrors handler.URLHandler.UpdateURL / checkURLOwnership.
func (res *Resolver) resolveUpdateURL(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	code, err := stringArg(f, "code", variables, true)
	if err != nil {
		return nil, err
	}
	if err := res.checkOwnership(ctx, r, code); err != nil {
		return nil, err
	}

	input, err := objectArg(f, "input", variables, true)
	if err != nil {
		return nil, err
	}
	req := &model.UpdateURLRequest{
		LongURL:     toStringPtr(input["url"]),
		Title:       toStringPtr(input["title"]),
		Description: toStringPtr(input["description"]),
		Password:    toStringPtr(input["password"]),
	}

	u, err := res.urlService.UpdateURL(ctx, code, req)
	if err != nil {
		if err == model.ErrURLNotFound {
			return nil, fmt.Errorf("URL not found")
		}
		return nil, fmt.Errorf("failed to update URL")
	}
	return project(urlToMap(u), f.Selection), nil
}

// resolveDeleteURL mirrors handler.URLHandler.DeleteURL / checkURLOwnership.
func (res *Resolver) resolveDeleteURL(ctx context.Context, r *http.Request, f astField, variables map[string]interface{}) (interface{}, error) {
	code, err := stringArg(f, "code", variables, true)
	if err != nil {
		return nil, err
	}
	if err := res.checkOwnership(ctx, r, code); err != nil {
		return nil, err
	}
	if err := res.urlService.DeleteURL(ctx, code); err != nil {
		if err == model.ErrURLNotFound {
			return nil, fmt.Errorf("URL not found")
		}
		return nil, fmt.Errorf("failed to delete URL")
	}
	return true, nil
}

// checkOwnership mirrors handler.URLHandler.checkURLOwnership: admin tokens
// always pass; user/org tokens must own the link; a missing/invalid Bearer
// token, or a link owned by someone else, is reported as "not found" (never
// "forbidden") to avoid leaking existence, matching the REST behavior.
func (res *Resolver) checkOwnership(ctx context.Context, r *http.Request, code string) error {
	rec, ok := res.bearerFromRequest(r)
	if !ok {
		return fmt.Errorf("bearer token required")
	}
	if strings.EqualFold(rec.OwnerType, "admin") {
		return nil
	}

	existing, err := res.urlService.GetURLByCodeAny(ctx, code)
	if err != nil {
		if err == model.ErrURLNotFound {
			return fmt.Errorf("URL not found")
		}
		return fmt.Errorf("failed to load URL")
	}
	if strings.EqualFold(rec.OwnerType, "org") {
		if existing.OrgID == nil || *existing.OrgID != rec.OwnerID {
			return fmt.Errorf("URL not found")
		}
		return nil
	}
	if existing.UserID == nil || *existing.UserID != rec.OwnerID {
		return fmt.Errorf("URL not found")
	}
	return nil
}

// bearerFromRequest validates the Authorization: Bearer header directly
// against tokenService, mirroring server.BearerAuthMiddleware's extraction
// logic exactly — but, unlike that middleware, treats a missing header as
// "anonymous" rather than an immediate 401, since the single GraphQL
// endpoint must serve both authenticated and anonymous operations.
func (res *Resolver) bearerFromRequest(r *http.Request) (*service.TokenRecord, bool) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return nil, false
	}
	plaintext := strings.TrimPrefix(auth, "Bearer ")
	rec, err := res.tokenService.ValidateToken(r.Context(), plaintext)
	if err != nil {
		return nil, false
	}
	return rec, true
}

// ---- argument helpers ------------------------------------------------

func argValue(f astField, name string, variables map[string]interface{}) (interface{}, bool) {
	v, ok := f.Arguments[name]
	if !ok {
		return nil, false
	}
	return v.resolve(variables), true
}

func stringArg(f astField, name string, variables map[string]interface{}, required bool) (string, error) {
	v, ok := argValue(f, name, variables)
	s := toString(v)
	if !ok || s == "" {
		if required {
			return "", fmt.Errorf("argument %q is required", name)
		}
		return "", nil
	}
	return s, nil
}

func intArg(f astField, name string, variables map[string]interface{}) (int, error) {
	v, ok := argValue(f, name, variables)
	if !ok || v == nil {
		return 0, fmt.Errorf("argument %q not set", name)
	}
	switch n := v.(type) {
	case int64:
		return int(n), nil
	case int:
		return n, nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("argument %q is not a number", name)
	}
}

func objectArg(f astField, name string, variables map[string]interface{}, required bool) (map[string]interface{}, error) {
	v, ok := argValue(f, name, variables)
	if !ok || v == nil {
		if required {
			return nil, fmt.Errorf("argument %q is required", name)
		}
		return map[string]interface{}{}, nil
	}
	obj, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("argument %q must be an object", name)
	}
	return obj, nil
}

func toString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toStringPtr(v interface{}) *string {
	if v == nil {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return nil
	}
	return &s
}
