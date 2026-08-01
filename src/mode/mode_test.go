package mode

import "testing"

// TestModeStringAndPredicates verifies the small accessor methods on Mode.
func TestModeStringAndPredicates(t *testing.T) {
	if Production.String() != "production" {
		t.Errorf("Production.String() = %q, want %q", Production.String(), "production")
	}
	if Development.String() != "development" {
		t.Errorf("Development.String() = %q, want %q", Development.String(), "development")
	}
	if !Production.IsProduction() {
		t.Error("Production.IsProduction() = false, want true")
	}
	if Production.IsDevelopment() {
		t.Error("Production.IsDevelopment() = true, want false")
	}
	if !Development.IsDevelopment() {
		t.Error("Development.IsDevelopment() = false, want true")
	}
	if Development.IsProduction() {
		t.Error("Development.IsProduction() = true, want false")
	}
}

// clearModeEnv unsets every environment variable Detect consults so each
// test starts from a clean slate regardless of the host's own environment.
func clearModeEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{"MODE", "CASLINK_MODE", "APP_ENV", "ENV", "ENVIRONMENT", "DEBUG"} {
		t.Setenv(v, "")
	}
}

// TestDetectDefaultIsProduction verifies that with no CLI flag, no env vars,
// and no config value, Detect falls back to Production (safe default).
func TestDetectDefaultIsProduction(t *testing.T) {
	clearModeEnv(t)
	if got := Detect("", ""); got != Production {
		t.Errorf("Detect(\"\", \"\") = %q, want %q", got, Production)
	}
}

// TestDetectCLIFlagWinsOverEverything verifies priority 1: the CLI flag
// wins even when env vars and DEBUG disagree.
func TestDetectCLIFlagWinsOverEverything(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("MODE", "development")
	t.Setenv("DEBUG", "true")

	if got := Detect("production", "development"); got != Production {
		t.Errorf("Detect(\"production\", \"development\") = %q, want %q (CLI flag must win)", got, Production)
	}
}

// TestDetectEnvVarWinsOverConfigAndDebug verifies priority 2: an env var
// (with no CLI flag set) wins over both the config file value and DEBUG.
func TestDetectEnvVarWinsOverConfigAndDebug(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("MODE", "production")
	t.Setenv("DEBUG", "true")

	if got := Detect("", "development"); got != Production {
		t.Errorf("Detect(\"\", \"development\") with MODE=production, DEBUG=true = %q, want %q", got, Production)
	}
}

// TestDetectEnvVarPriorityOrder verifies that among the recognized env
// vars, MODE is consulted before CASLINK_MODE, APP_ENV, ENV, and
// ENVIRONMENT, in that documented order.
func TestDetectEnvVarPriorityOrder(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
		want Mode
	}{
		{
			name: "MODE beats all others",
			set: map[string]string{
				"MODE":         "production",
				"CASLINK_MODE": "development",
				"APP_ENV":      "development",
				"ENV":          "development",
				"ENVIRONMENT":  "development",
			},
			want: Production,
		},
		{
			name: "CASLINK_MODE beats APP_ENV/ENV/ENVIRONMENT when MODE unset",
			set: map[string]string{
				"CASLINK_MODE": "production",
				"APP_ENV":      "development",
				"ENV":          "development",
				"ENVIRONMENT":  "development",
			},
			want: Production,
		},
		{
			name: "APP_ENV beats ENV/ENVIRONMENT when MODE/CASLINK_MODE unset",
			set: map[string]string{
				"APP_ENV":     "production",
				"ENV":         "development",
				"ENVIRONMENT": "development",
			},
			want: Production,
		},
		{
			name: "ENV beats ENVIRONMENT when higher-priority vars unset",
			set: map[string]string{
				"ENV":         "production",
				"ENVIRONMENT": "development",
			},
			want: Production,
		},
		{
			name: "ENVIRONMENT used as last resort",
			set: map[string]string{
				"ENVIRONMENT": "development",
			},
			want: Development,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearModeEnv(t)
			for k, v := range tt.set {
				t.Setenv(k, v)
			}
			if got := Detect("", ""); got != tt.want {
				t.Errorf("Detect(\"\", \"\") = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDetectDebugEnvTriggersDevelopment verifies that when no CLI flag and
// no MODE-family env var is set, a truthy DEBUG env var alone is enough to
// select Development, even overriding an explicit "production" config value.
func TestDetectDebugEnvTriggersDevelopment(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("DEBUG", "true")

	if got := Detect("", "production"); got != Development {
		t.Errorf("Detect(\"\", \"production\") with DEBUG=true = %q, want %q", got, Development)
	}
}

// TestDetectConfigModeUsedWhenNoFlagOrEnv verifies priority 3: the config
// file's mode value is used only once CLI flag, all MODE-family env vars,
// and DEBUG are all absent/falsy.
func TestDetectConfigModeUsedWhenNoFlagOrEnv(t *testing.T) {
	clearModeEnv(t)
	if got := Detect("", "development"); got != Development {
		t.Errorf("Detect(\"\", \"development\") = %q, want %q", got, Development)
	}
}

// TestDetectModeDebugDebugFalseEdgeCase documents the specific documented
// edge case: MODE=debug combined with DEBUG=false still resolves to
// Development, because the MODE env var is consulted (and "debug" parses
// as Development) before DEBUG is ever checked.
func TestDetectModeDebugDebugFalseEdgeCase(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("MODE", "debug")
	t.Setenv("DEBUG", "false")

	got := Detect("", "")
	if got != Development {
		t.Errorf("Detect with MODE=debug DEBUG=false = %q, want %q", got, Development)
	}
	if !got.IsDevelopment() {
		t.Error("resolved mode should report IsDevelopment() = true")
	}
}

// TestDetectFalsyDebugDoesNotTriggerDevelopment verifies a falsy DEBUG
// value does not flip the mode to Development when nothing else is set.
func TestDetectFalsyDebugDoesNotTriggerDevelopment(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("DEBUG", "false")

	if got := Detect("", ""); got != Production {
		t.Errorf("Detect(\"\", \"\") with DEBUG=false = %q, want %q", got, Production)
	}
}

// TestParseModeNormalization exercises parseMode's case-insensitivity,
// whitespace trimming, and safe fallback to Production for unknown input,
// via the exported Detect(cliMode, "") entry point.
func TestParseModeNormalization(t *testing.T) {
	clearModeEnv(t)

	tests := []struct {
		input string
		want  Mode
	}{
		{"development", Development},
		{"dev", Development},
		{"devel", Development},
		{"debug", Development},
		{"DEVELOPMENT", Development},
		{"  development  ", Development},
		{"Dev", Development},
		{"production", Production},
		{"prod", Production},
		{"release", Production},
		{"PRODUCTION", Production},
		{"", Production},
		{"nonsense", Production},
		{"  ", Production},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Detect(tt.input, ""); got != tt.want {
				t.Errorf("Detect(%q, \"\") = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestIsContainerEnvVarDetection verifies that any of the recognized
// container-indicator environment variables cause IsContainer to report
// true. It does not assert the false case, since the host running this
// test suite may itself be a container (e.g. the CI Docker toolchain
// image), which would make a "not a container" assertion environment
// dependent and flaky.
func TestIsContainerEnvVarDetection(t *testing.T) {
	for _, envVar := range []string{"KUBERNETES_SERVICE_HOST", "CONTAINER", "DOCKER_CONTAINER"} {
		t.Run(envVar, func(t *testing.T) {
			t.Setenv(envVar, "1")
			if !IsContainer() {
				t.Errorf("IsContainer() = false with %s set, want true", envVar)
			}
		})
	}
}

// TestGetModeInfoFields verifies GetModeInfo reports mode-derived fields
// consistently with the Mode's own predicate methods, for both modes.
func TestGetModeInfoFields(t *testing.T) {
	for _, m := range []Mode{Production, Development} {
		t.Run(m.String(), func(t *testing.T) {
			info := GetModeInfo(m)

			if got := info["mode"]; got != m.String() {
				t.Errorf("info[mode] = %v, want %v", got, m.String())
			}
			if got := info["production"]; got != m.IsProduction() {
				t.Errorf("info[production] = %v, want %v", got, m.IsProduction())
			}
			if got := info["development"]; got != m.IsDevelopment() {
				t.Errorf("info[development] = %v, want %v", got, m.IsDevelopment())
			}
			if got := info["debug_mode"]; got != m.IsDevelopment() {
				t.Errorf("info[debug_mode] = %v, want %v", got, m.IsDevelopment())
			}
			if _, ok := info["in_container"].(bool); !ok {
				t.Error("info[in_container] should be a bool")
			}
		})
	}
}
