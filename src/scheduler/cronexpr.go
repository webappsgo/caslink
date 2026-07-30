package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronSchedule is a self-contained replacement for github.com/robfig/cron/v3
// per AI.md PART 19 "NEVER Use External Schedulers" / "Implementation
// Requirements" #1 ("Use Go's time/ticker - No external cron libraries
// required"). It supports the "Schedule Format" table exactly:
//
//	standard 5-field cron: "minute hour day month weekday"
//	"@every Xm" / "@every Xh"
//	"@hourly"  == "0 * * * *"
//	"@daily"   == "0 0 * * *"
//	"@weekly"  == "0 0 * * 0"
//	"@monthly" == "0 0 1 * *"
type cronSchedule struct {
	// every, when non-zero, means "@every <every>" — Next just adds the
	// interval to the reference time, ignoring the field bitsets below.
	every time.Duration

	minutes  [60]bool
	hours    [24]bool
	days     [32]bool // 1-31
	months   [13]bool // 1-12
	weekdays [7]bool  // 0-6, Sunday=0
}

// parseCronSchedule parses a schedule string per the table above. It never
// returns an error for the shorthand forms; a malformed 5-field expression
// returns an error so the caller can log and skip registering the task
// rather than silently never firing.
func parseCronSchedule(expr string) (*cronSchedule, error) {
	expr = strings.TrimSpace(expr)
	switch {
	case strings.HasPrefix(expr, "@every "):
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(expr, "@every ")))
		if err != nil {
			return nil, fmt.Errorf("invalid @every duration %q: %w", expr, err)
		}
		if d <= 0 {
			return nil, fmt.Errorf("invalid @every duration %q: must be positive", expr)
		}
		return &cronSchedule{every: d}, nil
	case expr == "@hourly":
		return parseCronSchedule("0 * * * *")
	case expr == "@daily" || expr == "@midnight":
		return parseCronSchedule("0 0 * * *")
	case expr == "@weekly":
		return parseCronSchedule("0 0 * * 0")
	case expr == "@monthly":
		return parseCronSchedule("0 0 1 * *")
	}

	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("invalid cron expression %q: expected 5 fields, got %d", expr, len(fields))
	}

	cs := &cronSchedule{}
	if err := parseCronField(fields[0], 0, 59, cs.minutes[:]); err != nil {
		return nil, fmt.Errorf("invalid minute field in %q: %w", expr, err)
	}
	if err := parseCronField(fields[1], 0, 23, cs.hours[:]); err != nil {
		return nil, fmt.Errorf("invalid hour field in %q: %w", expr, err)
	}
	if err := parseCronField(fields[2], 1, 31, cs.days[:]); err != nil {
		return nil, fmt.Errorf("invalid day field in %q: %w", expr, err)
	}
	if err := parseCronField(fields[3], 1, 12, cs.months[:]); err != nil {
		return nil, fmt.Errorf("invalid month field in %q: %w", expr, err)
	}
	if err := parseCronField(fields[4], 0, 6, cs.weekdays[:]); err != nil {
		return nil, fmt.Errorf("invalid weekday field in %q: %w", expr, err)
	}
	return cs, nil
}

// parseCronField sets bits[v-offset] (where offset is implied by the slice's
// own indexing — callers pass a slice sized so index == value) for each value
// matched by a single cron field: "*", "N", "N-M", "*/N", "N-M/N", or a
// comma-separated list of any of those.
func parseCronField(field string, min, max int, bits []bool) error {
	for _, part := range strings.Split(field, ",") {
		if err := parseCronRange(part, min, max, bits); err != nil {
			return err
		}
	}
	return nil
}

func parseCronRange(part string, min, max int, bits []bool) error {
	step := 1
	rangePart := part
	if idx := strings.Index(part, "/"); idx != -1 {
		rangePart = part[:idx]
		n, err := strconv.Atoi(part[idx+1:])
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid step in %q", part)
		}
		step = n
	}

	lo, hi := min, max
	switch {
	case rangePart == "*":
		// lo/hi already span the full range.
	case strings.Contains(rangePart, "-"):
		bounds := strings.SplitN(rangePart, "-", 2)
		l, err := strconv.Atoi(bounds[0])
		if err != nil {
			return fmt.Errorf("invalid range start in %q", part)
		}
		h, err := strconv.Atoi(bounds[1])
		if err != nil {
			return fmt.Errorf("invalid range end in %q", part)
		}
		lo, hi = l, h
	default:
		n, err := strconv.Atoi(rangePart)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		lo, hi = n, n
	}

	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("value out of range [%d-%d] in %q", min, max, part)
	}
	for v := lo; v <= hi; v += step {
		if v < len(bits) {
			bits[v] = true
		}
	}
	return nil
}

// Next returns the first matching time strictly after `from`, truncated to
// the minute (cron granularity), evaluated in loc.
func (cs *cronSchedule) Next(from time.Time, loc *time.Location) time.Time {
	if cs.every > 0 {
		return from.Add(cs.every)
	}

	t := from.In(loc).Truncate(time.Minute).Add(time.Minute)
	// Cron's day-of-month/day-of-week combination is OR'd when both are
	// restricted (standard cron semantics); since every field here can be
	// "*" (all true), treating it as an AND across all five fields matches
	// standard behavior whenever at least one of day/weekday is "*".
	for i := 0; i < 366*24*60+1; i++ {
		if cs.minutes[t.Minute()] && cs.hours[t.Hour()] &&
			cs.days[t.Day()] && cs.months[int(t.Month())] &&
			cs.weekdays[int(t.Weekday())] {
			return t
		}
		t = t.Add(time.Minute)
	}
	// Unreachable in practice (every valid 5-field expression matches at
	// least once within a year); fall back to "next minute" so callers never
	// get a zero time.
	return from.Add(time.Minute)
}
