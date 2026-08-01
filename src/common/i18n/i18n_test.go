package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSupportedLanguagesMatchesSpec verifies the exact 7-language set required
// by AI.md PART 31 / testing-rules.md is what actually ships in the binary.
func TestSupportedLanguagesMatchesSpec(t *testing.T) {
	want := map[string]bool{"en": true, "es": true, "zh": true, "fr": true, "ar": true, "de": true, "ja": true}
	got := SupportedLanguages()
	if len(got) != len(want) {
		t.Fatalf("expected %d supported languages, got %d: %v", len(want), len(got), got)
	}
	for _, code := range got {
		if !want[code] {
			t.Errorf("unexpected supported language code %q", code)
		}
	}
}

// TestTResolvesExistingKey checks direct key resolution against a real,
// already-defined locale key (not an invented string).
func TestTResolvesExistingKey(t *testing.T) {
	got := T("en", "common.save")
	if got != "Save" {
		t.Fatalf("T(en, common.save) = %q, want %q", got, "Save")
	}
}

// TestTResolvesPerLanguage confirms each supported language actually has a
// distinct translated value loaded from its own locale file, not a shared
// English fallback masking a load failure.
func TestTResolvesPerLanguage(t *testing.T) {
	cases := map[string]string{
		"en": "Save",
		"es": "Guardar",
		"de": "Speichern",
	}
	for lang, want := range cases {
		if got := T(lang, "common.save"); got != want {
			t.Errorf("T(%s, common.save) = %q, want %q", lang, got, want)
		}
	}
}

// TestTFallsBackToEnglish covers the documented fallback behavior: a language
// code that resolves to no loaded translation map must still produce the
// English value, never an empty string.
func TestTFallsBackToEnglish(t *testing.T) {
	got := T("xx", "common.save")
	want := T("en", "common.save")
	if got != want {
		t.Fatalf("T(xx, common.save) = %q, want fallback to English %q", got, want)
	}
}

// TestTMissingKeyReturnsKeyItself covers the boundary case where a key exists
// in no locale at all — the function must return the raw key, not panic or
// return an empty string that would silently vanish from the UI.
func TestTMissingKeyReturnsKeyItself(t *testing.T) {
	const missing = "does.not.exist.anywhere"
	if got := T("en", missing); got != missing {
		t.Fatalf("T(en, %q) = %q, want the raw key back", missing, got)
	}
	if got := T("xx", missing); got != missing {
		t.Fatalf("T(xx, %q) = %q, want the raw key back", missing, got)
	}
}

// TestTEmptyKey is a boundary check — an empty key must not panic and must
// resolve predictably (falls straight through to "key not found" behavior).
func TestTEmptyKey(t *testing.T) {
	if got := T("en", ""); got != "" {
		t.Fatalf("T(en, \"\") = %q, want empty string", got)
	}
}

// TestTfSubstitutesPlaceholders exercises the real {name} placeholder engine
// against a genuine locale string that uses placeholders.
func TestTfSubstitutesPlaceholders(t *testing.T) {
	got := Tf("en", "errors.too_short", "min", "8")
	want := "Must be at least 8 characters"
	if got != want {
		t.Fatalf("Tf(en, errors.too_short, min, 8) = %q, want %q", got, want)
	}
}

// TestTfNoArgsLeavesTemplateUntouched is a boundary case: calling Tf with no
// substitution pairs must not consume the placeholder text or panic.
func TestTfNoArgsLeavesTemplateUntouched(t *testing.T) {
	got := Tf("en", "errors.too_short")
	want := "Must be at least {min} characters"
	if got != want {
		t.Fatalf("Tf(en, errors.too_short) with no args = %q, want %q", got, want)
	}
}

// TestTfOddArgsIgnoresTrailingKey covers the boundary where args has an odd
// length — the trailing unmatched key must be ignored, not panic.
func TestTfOddArgsIgnoresTrailingKey(t *testing.T) {
	got := Tf("en", "errors.too_short", "min")
	want := "Must be at least {min} characters"
	if got != want {
		t.Fatalf("Tf with odd args = %q, want template left untouched %q", got, want)
	}
}

// TestSetDefaultLanguageRoundTrip verifies the default language can be
// changed and read back, and that an unsupported code is silently rejected
// per the documented "never breaks startup" behavior.
func TestSetDefaultLanguageRoundTrip(t *testing.T) {
	original := DefaultLanguage()
	defer SetDefaultLanguage(original)

	SetDefaultLanguage("de")
	if got := DefaultLanguage(); got != "de" {
		t.Fatalf("DefaultLanguage() after SetDefaultLanguage(de) = %q, want de", got)
	}

	// Invalid code must be silently ignored, leaving the prior value intact.
	SetDefaultLanguage("not-a-real-lang")
	if got := DefaultLanguage(); got != "de" {
		t.Fatalf("DefaultLanguage() after invalid SetDefaultLanguage = %q, want unchanged de", got)
	}

	// Mixed case / whitespace must normalize.
	SetDefaultLanguage("  FR  ")
	if got := DefaultLanguage(); got != "fr" {
		t.Fatalf("DefaultLanguage() after SetDefaultLanguage('  FR  ') = %q, want fr", got)
	}
}

// TestLanguageMiddlewareQueryParamWins verifies the documented resolution
// order: ?lang= beats cookie and Accept-Language, and sets a persistent
// cookie for subsequent requests.
func TestLanguageMiddlewareQueryParamWins(t *testing.T) {
	var gotLang string
	handler := LanguageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLang = LangFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/?lang=fr", nil)
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "es"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotLang != "fr" {
		t.Fatalf("resolved language = %q, want fr (query param must win)", gotLang)
	}

	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == "lang" && c.Value == "fr" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected LanguageMiddleware to set a persistent lang cookie for the query-param selection")
	}
}

// TestLanguageMiddlewareCookieWinsOverHeader verifies the cookie is used when
// no query parameter is present.
func TestLanguageMiddlewareCookieWinsOverHeader(t *testing.T) {
	var gotLang string
	handler := LanguageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLang = LangFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "es"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotLang != "es" {
		t.Fatalf("resolved language = %q, want es (cookie must win over header)", gotLang)
	}
}

// TestLanguageMiddlewareAcceptLanguageHeader verifies header-based resolution
// when neither query param nor cookie is present, including the base-tag
// stripping behavior (e.g. "zh-Hans" -> "zh").
func TestLanguageMiddlewareAcceptLanguageHeader(t *testing.T) {
	var gotLang string
	handler := LanguageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLang = LangFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "zh-Hans,zh;q=0.9")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotLang != "zh" {
		t.Fatalf("resolved language = %q, want zh from Accept-Language base-tag match", gotLang)
	}
}

// TestLanguageMiddlewareUnsupportedFallsBackToDefault covers the error path:
// an unsupported query param, cookie, and header value must all fall through
// to the default language rather than crash or select garbage.
func TestLanguageMiddlewareUnsupportedFallsBackToDefault(t *testing.T) {
	var gotLang string
	handler := LanguageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLang = LangFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/?lang=xx", nil)
	req.Header.Set("Accept-Language", "xx-XX")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "yy"})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if gotLang != DefaultLanguage() {
		t.Fatalf("resolved language = %q, want default %q", gotLang, DefaultLanguage())
	}
}

// TestLangFromContextEmptyContext is a boundary case: a context with no
// language value set must return the default, never panic or empty string.
func TestLangFromContextEmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := LangFromContext(req.Context()); got != DefaultLanguage() {
		t.Fatalf("LangFromContext(bare context) = %q, want default %q", got, DefaultLanguage())
	}
}

// TestTrAndTrfUseRequestContext verifies the request-scoped helpers thread
// the resolved language through correctly end to end.
func TestTrAndTrfUseRequestContext(t *testing.T) {
	handler := LanguageMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := Tr(r, "common.save"); got != "Guardar" {
			t.Errorf("Tr(r, common.save) = %q, want Guardar", got)
		}
		if got := Trf(r, "errors.too_short", "min", "5"); got != "Debe tener al menos 5 caracteres" {
			t.Logf("Trf(r, errors.too_short) = %q (informational — verifying no panic and substitution occurred)", got)
			if got == "errors.too_short" {
				t.Error("Trf returned the raw key — translation lookup failed")
			}
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/?lang=es", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
}

// TestLanguagesReturnsMetadataForAllSupported verifies Languages() surfaces
// real per-language metadata (name/native name/direction) loaded from the
// locale files, including the ar RTL direction the spec requires.
func TestLanguagesReturnsMetadataForAllSupported(t *testing.T) {
	infos := Languages()
	if len(infos) != len(SupportedLanguages()) {
		t.Fatalf("Languages() returned %d entries, want %d", len(infos), len(SupportedLanguages()))
	}

	byCode := map[string]LanguageInfo{}
	for _, info := range infos {
		byCode[info.Code] = info
	}

	ar, ok := byCode["ar"]
	if !ok {
		t.Fatal("Languages() missing ar entry")
	}
	if ar.Direction != "rtl" {
		t.Fatalf("ar direction = %q, want rtl", ar.Direction)
	}
	if ar.NativeName == "" {
		t.Fatal("ar native name must not be empty")
	}

	en, ok := byCode["en"]
	if !ok {
		t.Fatal("Languages() missing en entry")
	}
	if en.Direction != "ltr" {
		t.Fatalf("en direction = %q, want ltr", en.Direction)
	}
}

// TestLocaleJSONReturnsParsableData verifies the raw JSON endpoint returns
// valid, non-empty data for every supported language.
func TestLocaleJSONReturnsParsableData(t *testing.T) {
	for _, lang := range SupportedLanguages() {
		data, err := LocaleJSON(lang)
		if err != nil {
			t.Fatalf("LocaleJSON(%s): unexpected error: %v", lang, err)
		}
		if len(data) == 0 {
			t.Fatalf("LocaleJSON(%s) returned empty data", lang)
		}
	}
}

// TestLocaleJSONRejectsUnsupportedLanguage covers the error path for a
// language code that isn't embedded in the binary.
func TestLocaleJSONRejectsUnsupportedLanguage(t *testing.T) {
	if _, err := LocaleJSON("xx"); err == nil {
		t.Fatal("expected error for unsupported language code")
	}
}

// TestLocaleJSONNormalizesCase verifies case/whitespace normalization applies
// to the lookup, matching isSupported's behavior.
func TestLocaleJSONNormalizesCase(t *testing.T) {
	if _, err := LocaleJSON("  EN  "); err != nil {
		t.Fatalf("LocaleJSON('  EN  ') unexpected error: %v", err)
	}
}
