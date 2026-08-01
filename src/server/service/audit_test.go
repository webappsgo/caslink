package service

import (
	"context"
	"testing"
	"time"
)

// TestRecordEventAndListEvents covers the happy path: recording several
// events (with and without a user ID) and reading them back most-recent
// first, along with the correct total count.
func TestRecordEventAndListEvents(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuditService(st)
	ctx := context.Background()

	uid := int64(42)
	events := []struct {
		userID   *int64
		userType string
		action   string
	}{
		{nil, "system", "startup"},
		{&uid, "admin", "login"},
		{&uid, "admin", "logout"},
	}

	for _, e := range events {
		if err := svc.RecordEvent(ctx, e.userID, e.userType, e.action, "resource", "details", "127.0.0.1", "test-agent"); err != nil {
			t.Fatalf("RecordEvent(%q) failed: %v", e.action, err)
		}
		// Ensure created_at ordering is distinguishable across inserts.
		time.Sleep(time.Millisecond)
	}

	entries, total, err := svc.ListEvents(ctx, 1, 50)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if total != len(events) {
		t.Fatalf("total = %d, want %d", total, len(events))
	}
	if len(entries) != len(events) {
		t.Fatalf("len(entries) = %d, want %d", len(entries), len(events))
	}

	// Most-recent-first: last recorded event ("logout") must come first.
	if entries[0].Action != "logout" {
		t.Errorf("entries[0].Action = %q, want %q", entries[0].Action, "logout")
	}
	if entries[len(entries)-1].Action != "startup" {
		t.Errorf("entries[last].Action = %q, want %q", entries[len(entries)-1].Action, "startup")
	}

	// Nil vs non-nil UserID must round-trip correctly.
	if entries[len(entries)-1].UserID != nil {
		t.Errorf("expected nil UserID for system event, got %v", *entries[len(entries)-1].UserID)
	}
	if entries[0].UserID == nil || *entries[0].UserID != uid {
		t.Errorf("expected UserID %d for admin event, got %v", uid, entries[0].UserID)
	}
}

// TestListEventsEmpty covers the zero-state: no rows recorded yet.
func TestListEventsEmpty(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuditService(st)
	ctx := context.Background()

	entries, total, err := svc.ListEvents(ctx, 1, 50)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

// TestListEventsPaginationBoundaries covers page<=0, limit<=0, and
// limit>250 normalization, plus actual pagination across pages.
func TestListEventsPaginationBoundaries(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuditService(st)
	ctx := context.Background()

	// Record 3 events, using the action field to track insertion order.
	for i := 0; i < 3; i++ {
		if err := svc.RecordEvent(ctx, nil, "system", "event", "resource", "details", "127.0.0.1", "test-agent"); err != nil {
			t.Fatalf("RecordEvent failed: %v", err)
		}
		time.Sleep(time.Millisecond)
	}

	cases := []struct {
		name  string
		page  int
		limit int
	}{
		{"zero page", 0, 50},
		{"negative page", -1, 50},
		{"zero limit", 1, 0},
		{"negative limit", 1, -5},
		{"limit over max", 1, 251},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries, total, err := svc.ListEvents(ctx, tc.page, tc.limit)
			if err != nil {
				t.Fatalf("ListEvents(%d, %d) failed: %v", tc.page, tc.limit, err)
			}
			if total != 3 {
				t.Errorf("total = %d, want 3", total)
			}
			// All boundary cases normalize to a limit that comfortably
			// covers the 3 inserted rows.
			if len(entries) != 3 {
				t.Errorf("len(entries) = %d, want 3", len(entries))
			}
		})
	}

	// Real pagination: limit 1 must split the 3 rows across 3 pages, in
	// most-recent-first order, with no overlap.
	var seen []int64
	for page := 1; page <= 3; page++ {
		entries, total, err := svc.ListEvents(ctx, page, 1)
		if err != nil {
			t.Fatalf("ListEvents(page=%d, limit=1) failed: %v", page, err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(entries) != 1 {
			t.Fatalf("page %d: len(entries) = %d, want 1", page, len(entries))
		}
		seen = append(seen, entries[0].ID)
	}
	if seen[0] == seen[1] || seen[1] == seen[2] || seen[0] == seen[2] {
		t.Errorf("expected 3 distinct IDs across pages, got %v", seen)
	}
	// Page 1 (most recent) must have the highest ID; page 3 the lowest.
	if !(seen[0] > seen[1] && seen[1] > seen[2]) {
		t.Errorf("expected descending IDs across pages, got %v", seen)
	}

	// Page far beyond available data returns an empty slice, not an error.
	entries, total, err := svc.ListEvents(ctx, 100, 50)
	if err != nil {
		t.Fatalf("ListEvents(page=100) failed: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0 for out-of-range page", len(entries))
	}
}

// TestRecordEventOptionalFields verifies empty-string optional fields
// (resource, details, ip_address, user_agent) round-trip as empty strings,
// not as errors or NULLs breaking the scan.
func TestRecordEventOptionalFields(t *testing.T) {
	st := newFullSchemaStore(t)
	svc := NewAuditService(st)
	ctx := context.Background()

	if err := svc.RecordEvent(ctx, nil, "", "action-only", "", "", "", ""); err != nil {
		t.Fatalf("RecordEvent with empty optional fields failed: %v", err)
	}

	entries, _, err := svc.ListEvents(ctx, 1, 50)
	if err != nil {
		t.Fatalf("ListEvents failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Action != "action-only" {
		t.Errorf("Action = %q, want %q", e.Action, "action-only")
	}
	if e.UserID != nil {
		t.Errorf("expected nil UserID, got %v", *e.UserID)
	}
	if e.UserType != "" || e.Resource != "" || e.Details != "" || e.IPAddress != "" || e.UserAgent != "" {
		t.Errorf("expected all optional fields empty, got %+v", e)
	}
}
