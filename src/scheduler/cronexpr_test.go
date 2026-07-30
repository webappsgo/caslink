package scheduler

import (
	"testing"
	"time"
)

func TestParseCronScheduleShorthands(t *testing.T) {
	cases := []string{"@hourly", "@daily", "@midnight", "@weekly", "@monthly", "@every 15m", "@every 1h", "0 3 * * *", "0 3 * * 0", "*/15 * * * *"}
	for _, expr := range cases {
		if _, err := parseCronSchedule(expr); err != nil {
			t.Errorf("parseCronSchedule(%q) returned error: %v", expr, err)
		}
	}
}

func TestParseCronScheduleInvalid(t *testing.T) {
	cases := []string{"", "not a cron", "60 * * * *", "* * * * * *", "@every -5m", "@every 0m"}
	for _, expr := range cases {
		if _, err := parseCronSchedule(expr); err == nil {
			t.Errorf("parseCronSchedule(%q) expected an error, got nil", expr)
		}
	}
}

func TestCronScheduleNextEvery(t *testing.T) {
	cs, err := parseCronSchedule("@every 15m")
	if err != nil {
		t.Fatalf("parseCronSchedule: %v", err)
	}
	from := time.Date(2026, 7, 30, 10, 3, 0, 0, time.UTC)
	next := cs.Next(from, time.UTC)
	want := from.Add(15 * time.Minute)
	if !next.Equal(want) {
		t.Errorf("Next() = %v, want %v", next, want)
	}
}

func TestCronScheduleNextDaily(t *testing.T) {
	cs, err := parseCronSchedule("0 3 * * *")
	if err != nil {
		t.Fatalf("parseCronSchedule: %v", err)
	}

	// Before 03:00 the same day → next run is 03:00 today.
	from := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	next := cs.Next(from, time.UTC)
	want := time.Date(2026, 7, 30, 3, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next() = %v, want %v", next, want)
	}

	// After 03:00 the same day → next run is 03:00 the following day.
	from2 := time.Date(2026, 7, 30, 4, 0, 0, 0, time.UTC)
	next2 := cs.Next(from2, time.UTC)
	want2 := time.Date(2026, 7, 31, 3, 0, 0, 0, time.UTC)
	if !next2.Equal(want2) {
		t.Errorf("Next() = %v, want %v", next2, want2)
	}
}

func TestCronScheduleNextWeekly(t *testing.T) {
	// Sunday 03:00 — 2026-07-30 is a Thursday.
	cs, err := parseCronSchedule("0 3 * * 0")
	if err != nil {
		t.Fatalf("parseCronSchedule: %v", err)
	}
	from := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	next := cs.Next(from, time.UTC)
	if next.Weekday() != time.Sunday {
		t.Errorf("Next() = %v, weekday %v, want Sunday", next, next.Weekday())
	}
	if next.Hour() != 3 || next.Minute() != 0 {
		t.Errorf("Next() = %v, want 03:00", next)
	}
	if !next.After(from) {
		t.Errorf("Next() = %v, want strictly after %v", next, from)
	}
}

func TestCronScheduleNextStepAndRange(t *testing.T) {
	cs, err := parseCronSchedule("*/15 * * * *")
	if err != nil {
		t.Fatalf("parseCronSchedule: %v", err)
	}
	from := time.Date(2026, 7, 30, 10, 3, 0, 0, time.UTC)
	next := cs.Next(from, time.UTC)
	want := time.Date(2026, 7, 30, 10, 15, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("Next() = %v, want %v", next, want)
	}
}
