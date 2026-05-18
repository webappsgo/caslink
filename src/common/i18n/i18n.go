// Package i18n provides internationalization support for all caslink binaries.
// Translation files are embedded at build time — no filesystem dependency at runtime.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Embeds src/common/i18n/locales/*.json (path relative to this package).
//
//go:embed locales/*.json
var localeFS embed.FS

// contextKey is the unexported key type for storing the active language in a context.
type contextKey struct{}

var langKey = contextKey{}

// supportedLanguages lists every language code embedded in the binary.
var supportedLanguages = []string{"en", "es", "fr", "de", "zh", "ar", "ja"}

// defaultLanguage is the fallback language used when no Accept-Language
// header, cookie, or query parameter selects one. Set via SetDefaultLanguage.
var defaultLanguage = "en"

// SetDefaultLanguage sets the process-wide fallback language. Invalid codes
// are silently ignored so a bogus --lang flag never breaks startup.
func SetDefaultLanguage(lang string) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if isSupported(lang) {
		mu.Lock()
		defaultLanguage = lang
		mu.Unlock()
	}
}

// DefaultLanguage returns the currently-configured fallback language.
func DefaultLanguage() string {
	mu.RLock()
	defer mu.RUnlock()
	return defaultLanguage
}

// translations holds the parsed locale data, keyed by language code.
var (
	mu           sync.RWMutex
	translations = map[string]map[string]any{}
)

func init() {
	for _, lang := range supportedLanguages {
		data, err := localeFS.ReadFile(fmt.Sprintf("locales/%s.json", lang))
		if err != nil {
			panic(fmt.Sprintf("i18n: missing locale file for %q: %v", lang, err))
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			panic(fmt.Sprintf("i18n: invalid locale file for %q: %v", lang, err))
		}
		mu.Lock()
		translations[lang] = m
		mu.Unlock()
	}
}

// isSupported returns true if lang is a supported language code.
func isSupported(lang string) bool {
	lang = strings.ToLower(strings.TrimSpace(lang))
	for _, s := range supportedLanguages {
		if s == lang {
			return true
		}
	}
	return false
}

// parseAcceptLanguage selects the best supported language from an Accept-Language header value.
// Falls back to "en" when no match is found.
func parseAcceptLanguage(header string) string {
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		// Try exact match first (e.g. "zh-Hans" → "zh").
		base := strings.ToLower(strings.SplitN(tag, "-", 2)[0])
		if isSupported(base) {
			return base
		}
	}
	return DefaultLanguage()
}

// LanguageMiddleware selects the active language for each request using:
//  1. ?lang= query parameter (highest priority; sets a persistent cookie)
//  2. lang cookie
//  3. Accept-Language header
//  4. Default: "en"
func LanguageMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := ""

		// 1. Query parameter — highest priority; also sets a persistent cookie.
		if q := r.URL.Query().Get("lang"); q != "" && isSupported(q) {
			lang = q
			http.SetCookie(w, &http.Cookie{
				Name:     "lang",
				Value:    lang,
				Path:     "/",
				MaxAge:   365 * 24 * 60 * 60, // 1 year
				SameSite: http.SameSiteLaxMode,
				Secure:   r.TLS != nil,
				HttpOnly: true,
			})
		}

		// 2. Persistent cookie.
		if lang == "" {
			if c, err := r.Cookie("lang"); err == nil && isSupported(c.Value) {
				lang = c.Value
			}
		}

		// 3. Accept-Language header.
		if lang == "" {
			lang = parseAcceptLanguage(r.Header.Get("Accept-Language"))
		}

		// 4. Fallback default.
		if lang == "" {
			lang = DefaultLanguage()
		}

		ctx := context.WithValue(r.Context(), langKey, lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LangFromContext returns the active language stored in ctx, defaulting to "en".
func LangFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(langKey).(string); ok && v != "" {
		return v
	}
	return DefaultLanguage()
}

// T returns the translated string for key in lang. Keys use dot notation (e.g. "errors.not_found").
// Falls back to English when the key is missing in lang. Returns the raw key when missing everywhere.
func T(lang, key string) string {
	mu.RLock()
	defer mu.RUnlock()

	if v := lookup(translations[lang], key); v != "" {
		return v
	}
	if lang != "en" {
		if v := lookup(translations["en"], key); v != "" {
			return v
		}
	}
	return key
}

// Tf returns the translated string for key with named placeholder substitution.
// Placeholders use the form {name}. Example: Tf("en", "errors.too_short", "min", "8").
// Args are alternating key-value pairs: Tf(lang, key, "k1", "v1", "k2", "v2", ...).
func Tf(lang, key string, args ...string) string {
	s := T(lang, key)
	for i := 0; i+1 < len(args); i += 2 {
		s = strings.ReplaceAll(s, "{"+args[i]+"}", args[i+1])
	}
	return s
}

// Tr translates a key from the language stored in the request context.
func Tr(r *http.Request, key string) string {
	return T(LangFromContext(r.Context()), key)
}

// Trf translates with placeholders from the request context language.
func Trf(r *http.Request, key string, args ...string) string {
	return Tf(LangFromContext(r.Context()), key, args...)
}

// SupportedLanguages returns a copy of the supported language codes.
func SupportedLanguages() []string {
	out := make([]string, len(supportedLanguages))
	copy(out, supportedLanguages)
	return out
}

// LanguageInfo holds display metadata for a supported language.
type LanguageInfo struct {
	Code       string
	Name       string
	NativeName string
	Direction  string
}

// Languages returns display metadata for all supported languages.
func Languages() []LanguageInfo {
	mu.RLock()
	defer mu.RUnlock()

	var out []LanguageInfo
	for _, code := range supportedLanguages {
		m := translations[code]
		info := LanguageInfo{Code: code, Direction: "ltr"}
		if meta, ok := m["meta"].(map[string]any); ok {
			if v, ok := meta["name"].(string); ok {
				info.Name = v
			}
			if v, ok := meta["native_name"].(string); ok {
				info.NativeName = v
			}
			if v, ok := meta["direction"].(string); ok {
				info.Direction = v
			}
		}
		out = append(out, info)
	}
	return out
}

// lookup resolves a dot-separated key path inside a nested map.
func lookup(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	parts := strings.SplitN(key, ".", 2)
	v, ok := m[parts[0]]
	if !ok {
		return ""
	}
	if len(parts) == 1 {
		if s, ok := v.(string); ok {
			return s
		}
		return ""
	}
	if sub, ok := v.(map[string]any); ok {
		return lookup(sub, parts[1])
	}
	return ""
}
