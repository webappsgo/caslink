package theme

import "testing"

// TestGetThemePaletteRespectsNoColor verifies NO_COLOR wins over every other
// input, including an explicit "dark"/"light" request — this is a
// security/accessibility-adjacent contract from PART 7 ("NO_COLOR disables
// colors") that must not be silently broken.
func TestGetThemePaletteRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	for _, color := range []string{"dark", "light", "on", "auto", ""} {
		got := GetThemePalette(color)
		if got != &ThemePaletteNone {
			t.Errorf("GetThemePalette(%q) with NO_COLOR set = %p, want ThemePaletteNone (%p)", color, got, &ThemePaletteNone)
		}
	}
}

// TestGetThemePaletteExplicitOff verifies "off" always returns the no-color
// palette regardless of NO_COLOR being unset.
func TestGetThemePaletteExplicitOff(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if got := GetThemePalette("off"); got != &ThemePaletteNone {
		t.Fatalf("GetThemePalette(off) = %p, want ThemePaletteNone (%p)", got, &ThemePaletteNone)
	}
}

// TestGetThemePaletteExplicitDarkLight verifies explicit color selection
// bypasses system detection entirely.
func TestGetThemePaletteExplicitDarkLight(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	if got := GetThemePalette("dark"); got != &ThemePaletteDark {
		t.Fatalf("GetThemePalette(dark) = %p, want ThemePaletteDark (%p)", got, &ThemePaletteDark)
	}
	if got := GetThemePalette("light"); got != &ThemePaletteLight {
		t.Fatalf("GetThemePalette(light) = %p, want ThemePaletteLight (%p)", got, &ThemePaletteLight)
	}
}

// TestGetThemePaletteAutoDetectsFromSystem covers the "on"/"auto"/default
// path, which defers to IsSystemDarkTheme. Since the true system state is
// environment-dependent, we assert only that a real (non-nil, non-"none")
// palette matching the actual detection result is returned — this still
// fails if the auto branch is broken (e.g. always returns light or always
// returns none).
func TestGetThemePaletteAutoDetectsFromSystem(t *testing.T) {
	t.Setenv("NO_COLOR", "")

	want := &ThemePaletteLight
	if IsSystemDarkTheme() {
		want = &ThemePaletteDark
	}

	for _, color := range []string{"auto", "on", "", "unknown-value"} {
		got := GetThemePalette(color)
		if got != want {
			t.Errorf("GetThemePalette(%q) = %p, want %p (system dark=%v)", color, got, want, IsSystemDarkTheme())
		}
	}
}

// TestThemePaletteNoneHasNoColorCodes verifies the no-color palette truly
// carries no ANSI color escape sequences beyond a (harmless) reset, so
// NO_COLOR mode never leaks escape codes into piped/logged output.
func TestThemePaletteNoneHasNoColorCodes(t *testing.T) {
	p := ThemePaletteNone
	if p.Primary != "" || p.Secondary != "" || p.Accent != "" ||
		p.Success != "" || p.Warning != "" || p.Error != "" || p.Info != "" ||
		p.Text != "" || p.TextMuted != "" || p.TextBold != "" ||
		p.Background != "" || p.Surface != "" || p.Border != "" ||
		p.Highlight != "" || p.Selected != "" || p.Cursor != "" {
		t.Fatalf("ThemePaletteNone must have no color codes, got %+v", p)
	}
}

// TestThemePalettesAreDistinct is a regression guard: dark and light must not
// accidentally be defined as identical palettes, and every field that is
// documented as always-set must be non-empty (an accidental deletion during
// editing would otherwise pass silently).
func TestThemePalettesAreDistinct(t *testing.T) {
	if ThemePaletteDark == ThemePaletteLight {
		t.Fatal("ThemePaletteDark and ThemePaletteLight must not be identical")
	}

	fields := map[string]struct{ dark, light string }{
		"Primary":    {ThemePaletteDark.Primary, ThemePaletteLight.Primary},
		"Secondary":  {ThemePaletteDark.Secondary, ThemePaletteLight.Secondary},
		"Accent":     {ThemePaletteDark.Accent, ThemePaletteLight.Accent},
		"Success":    {ThemePaletteDark.Success, ThemePaletteLight.Success},
		"Warning":    {ThemePaletteDark.Warning, ThemePaletteLight.Warning},
		"Error":      {ThemePaletteDark.Error, ThemePaletteLight.Error},
		"Info":       {ThemePaletteDark.Info, ThemePaletteLight.Info},
		"Text":       {ThemePaletteDark.Text, ThemePaletteLight.Text},
		"Background": {ThemePaletteDark.Background, ThemePaletteLight.Background},
		"Reset":      {ThemePaletteDark.Reset, ThemePaletteLight.Reset},
	}
	for name, v := range fields {
		if v.dark == "" {
			t.Errorf("ThemePaletteDark.%s must not be empty", name)
		}
		if v.light == "" {
			t.Errorf("ThemePaletteLight.%s must not be empty", name)
		}
	}
}
