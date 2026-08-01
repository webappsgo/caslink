package service

import (
	"context"
	"testing"
	"time"

	"github.com/webappsgo/caslink/src/server/store"
)

// sqliteTime formats a time.Time the same way SQLite's own datetime()
// function does, so text comparisons against datetime('now', '-N days')
// behave correctly.
func sqliteTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04:05")
}

// insertTestURL creates a urls row and returns its short_code.
func insertTestURL(t *testing.T, st *store.Store, shortCode string, userID *int64) {
	t.Helper()

	if _, err := st.ServerDB.Exec(
		`INSERT INTO urls (short_code, long_url, user_id) VALUES (?, ?, ?)`,
		shortCode, "https://example.com/"+shortCode, userID,
	); err != nil {
		t.Fatalf("failed to insert url %q: %v", shortCode, err)
	}
}

// insertTestClick inserts one clicks row for the given short_code, at the
// given clicked_at time.
func insertTestClick(t *testing.T, st *store.Store, shortCode, ipHash, referrer string, clickedAt time.Time) {
	t.Helper()

	var urlID int64
	if err := st.ServerDB.QueryRow(`SELECT id FROM urls WHERE short_code = ?`, shortCode).Scan(&urlID); err != nil {
		t.Fatalf("failed to resolve url id for %q: %v", shortCode, err)
	}
	if _, err := st.ServerDB.Exec(
		`INSERT INTO clicks (url_id, ip_hash, referrer, clicked_at) VALUES (?, ?, ?, ?)`,
		urlID, ipHash, referrer, sqliteTime(clickedAt),
	); err != nil {
		t.Fatalf("failed to insert click: %v", err)
	}
}

// TestGetURLStatsNotFound covers the error path for an unknown short code.
func TestGetURLStatsNotFound(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAnalyticsService(st)
	ctx := context.Background()

	if _, err := svc.GetURLStats(ctx, "does-not-exist"); err == nil {
		t.Fatal("expected error for unknown short code, got nil")
	}
}

// TestGetURLStatsEmptyDataset covers a URL that exists but has zero clicks.
func TestGetURLStatsEmptyDataset(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAnalyticsService(st)
	ctx := context.Background()

	insertTestURL(t, st, "empty", nil)

	stats, err := svc.GetURLStats(ctx, "empty")
	if err != nil {
		t.Fatalf("GetURLStats failed: %v", err)
	}
	if stats.TotalClicks != 0 || stats.UniqueIPs != 0 {
		t.Errorf("expected zero clicks/IPs, got %+v", stats)
	}
	if stats.Last24h != 0 || stats.Last7d != 0 || stats.Last30d != 0 {
		t.Errorf("expected zero windowed counts, got %+v", stats)
	}
	if len(stats.TopReferers) != 0 {
		t.Errorf("expected no referrers, got %v", stats.TopReferers)
	}
	if len(stats.DailyClicks) != 0 {
		t.Errorf("expected no daily clicks, got %v", stats.DailyClicks)
	}
}

// TestGetURLStatsAggregation covers the happy path: multiple clicks across
// different times, referrers, and IPs, verifying window counts, unique IP
// dedup, referrer ranking, and daily aggregation.
func TestGetURLStatsAggregation(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAnalyticsService(st)
	ctx := context.Background()

	insertTestURL(t, st, "abc123", nil)

	now := time.Now().UTC()
	// Two clicks within the last day (same IP, different referrers).
	insertTestClick(t, st, "abc123", "ip1", "https://google.com", now.Add(-1*time.Hour))
	insertTestClick(t, st, "abc123", "ip1", "https://bing.com", now.Add(-2*time.Hour))
	// One click within the last week but outside the last day.
	insertTestClick(t, st, "abc123", "ip2", "https://google.com", now.Add(-3*24*time.Hour))
	// One click within the last month but outside the last week.
	insertTestClick(t, st, "abc123", "ip3", "https://google.com", now.Add(-15*24*time.Hour))
	// One click outside the 30-day window entirely — must not be counted
	// in any windowed total, but does count toward TotalClicks/UniqueIPs.
	insertTestClick(t, st, "abc123", "ip4", "https://old-referrer.example", now.Add(-45*24*time.Hour))

	stats, err := svc.GetURLStats(ctx, "abc123")
	if err != nil {
		t.Fatalf("GetURLStats failed: %v", err)
	}

	if stats.TotalClicks != 5 {
		t.Errorf("TotalClicks = %d, want 5", stats.TotalClicks)
	}
	if stats.UniqueIPs != 4 {
		t.Errorf("UniqueIPs = %d, want 4", stats.UniqueIPs)
	}
	if stats.Last24h != 2 {
		t.Errorf("Last24h = %d, want 2", stats.Last24h)
	}
	if stats.Last7d != 3 {
		t.Errorf("Last7d = %d, want 3", stats.Last7d)
	}
	if stats.Last30d != 4 {
		t.Errorf("Last30d = %d, want 4", stats.Last30d)
	}

	// google.com appears 3 times, bing.com once, old-referrer once — google
	// must rank first.
	if len(stats.TopReferers) == 0 || stats.TopReferers[0].Referer != "https://google.com" {
		t.Fatalf("expected google.com to be top referrer, got %v", stats.TopReferers)
	}
	if stats.TopReferers[0].Clicks != 3 {
		t.Errorf("top referrer clicks = %d, want 3", stats.TopReferers[0].Clicks)
	}

	// Daily clicks only cover the last 30 days, so the 45-day-old click is
	// excluded from the daily breakdown even though it's in TotalClicks.
	var dailySum int
	for _, d := range stats.DailyClicks {
		dailySum += d.Clicks
	}
	if dailySum != 4 {
		t.Errorf("sum of DailyClicks = %d, want 4 (excludes the 45-day-old click)", dailySum)
	}
}

// TestGetUserStatsAggregatesAcrossURLs verifies GetUserStats sums clicks
// across all URLs owned by a user, and excludes URLs owned by other users.
func TestGetUserStatsAggregatesAcrossURLs(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAnalyticsService(st)
	ctx := context.Background()

	owner := int64(1)
	other := int64(2)
	insertTestURL(t, st, "u1", &owner)
	insertTestURL(t, st, "u2", &owner)
	insertTestURL(t, st, "u3", &other)

	now := time.Now().UTC()
	insertTestClick(t, st, "u1", "ip1", "https://a.example", now.Add(-1*time.Hour))
	insertTestClick(t, st, "u2", "ip2", "https://b.example", now.Add(-1*time.Hour))
	// Belongs to a different user — must not be counted.
	insertTestClick(t, st, "u3", "ip3", "https://c.example", now.Add(-1*time.Hour))

	stats, err := svc.GetUserStats(ctx, owner)
	if err != nil {
		t.Fatalf("GetUserStats failed: %v", err)
	}
	if stats.TotalClicks != 2 {
		t.Errorf("TotalClicks = %d, want 2", stats.TotalClicks)
	}
	if stats.UniqueIPs != 2 {
		t.Errorf("UniqueIPs = %d, want 2", stats.UniqueIPs)
	}
	if stats.Last24h != 2 {
		t.Errorf("Last24h = %d, want 2", stats.Last24h)
	}
	if len(stats.TopReferers) != 2 {
		t.Errorf("expected 2 distinct referrers, got %v", stats.TopReferers)
	}
}

// TestGetUserStatsNoURLs covers a user with no URLs at all — every field
// must be zero/empty, and no error.
func TestGetUserStatsNoURLs(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAnalyticsService(st)
	ctx := context.Background()

	stats, err := svc.GetUserStats(ctx, 999)
	if err != nil {
		t.Fatalf("GetUserStats failed: %v", err)
	}
	if stats.TotalClicks != 0 || stats.UniqueIPs != 0 {
		t.Errorf("expected zero stats for user with no URLs, got %+v", stats)
	}
	if len(stats.TopReferers) != 0 || len(stats.DailyClicks) != 0 {
		t.Errorf("expected no referrers/daily clicks, got %+v", stats)
	}
}
