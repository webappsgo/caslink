package banner

import (
	"strings"
	"testing"
)

// testCfg builds a representative BannerConfig for the render tests below.
func testCfg() BannerConfig {
	return BannerConfig{
		AppName:    "caslink",
		Version:    "1.2.3",
		CommitID:   "abc1234",
		BuildDate:  "2026-07-30T00:00:00Z",
		Mode:       "production",
		ServerURL:  "https://example.test",
		AdminURL:   "https://example.test/server/admin",
		SetupToken: "setup_tok_xyz",
		Debug:      false,
	}
}

// TestPrintFullContainsAllFields verifies the full (>=80 col) banner includes
// every populated config field and the ASCII logo — this is the richest
// variant and must not silently drop fields.
func TestPrintFullContainsAllFields(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	printFull(&buf, cfg)
	out := buf.String()

	for _, want := range []string{
		asciiLogo,
		cfg.AppName,
		cfg.Version,
		cfg.CommitID,
		cfg.BuildDate,
		cfg.Mode,
		cfg.ServerURL,
		cfg.AdminURL,
		cfg.SetupToken,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printFull output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestPrintFullOmitsUnknownCommitAndBuildDate verifies the documented
// suppression of placeholder "unknown" values so a dev build doesn't print
// a misleading literal "unknown" line.
func TestPrintFullOmitsUnknownCommitAndBuildDate(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	cfg.CommitID = "unknown"
	cfg.BuildDate = "unknown"
	printFull(&buf, cfg)
	out := buf.String()

	if strings.Contains(out, "Commit:") {
		t.Error("printFull should omit the Commit line when CommitID is \"unknown\"")
	}
	if strings.Contains(out, "Built:") {
		t.Error("printFull should omit the Built line when BuildDate is \"unknown\"")
	}
}

// TestPrintFullOmitsEmptyOptionalFields verifies empty optional fields
// (AdminURL, SetupToken, ServerURL) don't leave stray labels in the output —
// relevant for the CLI client banner, which has no AdminURL.
func TestPrintFullOmitsEmptyOptionalFields(t *testing.T) {
	var buf strings.Builder
	cfg := BannerConfig{AppName: "caslink-cli", Version: "1.0.0", Mode: "production"}
	printFull(&buf, cfg)
	out := buf.String()

	for _, unwanted := range []string{"Admin URL:", "Server URL:", "SETUP TOKEN", "Debug:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("printFull output unexpectedly contains %q for empty-field config:\n%s", unwanted, out)
		}
	}
}

// TestPrintFullShowsDebugFlag verifies Debug:true renders the debug line,
// and that it's absent when false.
func TestPrintFullShowsDebugFlag(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	cfg.Debug = true
	printFull(&buf, cfg)
	if !strings.Contains(buf.String(), "Debug:") {
		t.Error("printFull with Debug=true must include a Debug: line")
	}

	buf.Reset()
	cfg.Debug = false
	printFull(&buf, cfg)
	if strings.Contains(buf.String(), "Debug:") {
		t.Error("printFull with Debug=false must not include a Debug: line")
	}
}

// TestPrintCompactContainsCoreFields verifies the compact (60-79 col)
// variant keeps version, mode, URL, and setup token but drops the ASCII art.
func TestPrintCompactContainsCoreFields(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	printCompact(&buf, cfg)
	out := buf.String()

	if strings.Contains(out, asciiLogo) {
		t.Error("printCompact must not include the full ASCII logo")
	}
	for _, want := range []string{cfg.Version, cfg.Mode, cfg.ServerURL, cfg.SetupToken} {
		if !strings.Contains(out, want) {
			t.Errorf("printCompact output missing %q\noutput:\n%s", want, out)
		}
	}
}

// TestPrintMinimalContainsVersionAndMode verifies the minimal (40-59 col)
// variant is a single summary line plus optional token line.
func TestPrintMinimalContainsVersionAndMode(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	printMinimal(&buf, cfg)
	out := buf.String()

	if !strings.Contains(out, cfg.Version) || !strings.Contains(out, cfg.Mode) {
		t.Errorf("printMinimal output missing version/mode: %q", out)
	}
	if !strings.Contains(out, cfg.SetupToken) {
		t.Errorf("printMinimal output missing setup token: %q", out)
	}
	if strings.Contains(out, cfg.ServerURL) {
		t.Error("printMinimal must not include the full server URL (too verbose for 40-59 cols)")
	}
}

// TestPrintMinimalOmitsTokenWhenEmpty verifies no stray "token:" line appears
// once setup is complete (SetupToken == "").
func TestPrintMinimalOmitsTokenWhenEmpty(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	cfg.SetupToken = ""
	printMinimal(&buf, cfg)
	if strings.Contains(buf.String(), "token:") {
		t.Error("printMinimal must omit the token line when SetupToken is empty")
	}
}

// TestPrintMicroContainsOnlyVersion verifies the micro (<40 col) variant is
// reduced to the bare minimum — no ASCII, no mode, no token, no URLs — since
// anything more would overflow the terminal width.
func TestPrintMicroContainsOnlyVersion(t *testing.T) {
	var buf strings.Builder
	cfg := testCfg()
	printMicro(&buf, cfg)
	out := buf.String()

	if !strings.Contains(out, cfg.Version) {
		t.Errorf("printMicro output missing version: %q", out)
	}
	for _, unwanted := range []string{cfg.Mode, cfg.SetupToken, cfg.ServerURL, cfg.AdminURL, asciiLogo} {
		if unwanted != "" && strings.Contains(out, unwanted) {
			t.Errorf("printMicro output unexpectedly contains %q: %q", unwanted, out)
		}
	}
}

// TestPrintStartupBannerNilWriterDefaultsToStdout verifies passing a nil
// io.Writer doesn't panic (it falls back to os.Stdout per the doc comment).
// We can't easily capture os.Stdout output here without redirecting the
// process-wide fd, so this test only asserts the call completes safely.
func TestPrintStartupBannerNilWriterDefaultsToStdout(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PrintStartupBanner(nil, cfg) panicked: %v", r)
		}
	}()
	PrintStartupBanner(nil, testCfg())
}

// TestPrintStartupBannerEmptyConfig is a boundary test: an entirely zero
// BannerConfig must render without panicking and must still show something
// for Version (even if empty) rather than crash on nil map/slice access.
func TestPrintStartupBannerEmptyConfig(t *testing.T) {
	var buf strings.Builder
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("PrintStartupBanner with zero-value config panicked: %v", r)
		}
	}()
	PrintStartupBanner(&buf, BannerConfig{})
	if buf.Len() == 0 {
		t.Fatal("PrintStartupBanner with zero-value config produced no output at all")
	}
}
