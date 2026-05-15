package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/casjaysdevdocker/caslink/src/server/store"
)

// AnalyticsService handles URL click analytics queries.
type AnalyticsService struct {
	store *store.Store
}

// NewAnalyticsService creates a new AnalyticsService.
func NewAnalyticsService(st *store.Store) *AnalyticsService {
	return &AnalyticsService{store: st}
}

// ClickSummary holds the click count for a single day.
type ClickSummary struct {
	Date   string `json:"date"`   // YYYY-MM-DD
	Clicks int    `json:"clicks"`
}

// RefererSummary holds the click count for a single referrer.
type RefererSummary struct {
	Referer string `json:"referer"`
	Clicks  int    `json:"clicks"`
}

// URLStats is the aggregate statistics for one short code (or a whole user).
type URLStats struct {
	ShortCode   string           `json:"short_code"`
	TotalClicks int              `json:"total_clicks"`
	UniqueIPs   int              `json:"unique_ips"`
	Last24h     int              `json:"last_24h"`
	Last7d      int              `json:"last_7d"`
	Last30d     int              `json:"last_30d"`
	TopReferers []RefererSummary `json:"top_referers"`
	DailyClicks []ClickSummary   `json:"daily_clicks"`
}

// GetURLStats returns click statistics for a single short code.
func (s *AnalyticsService) GetURLStats(ctx context.Context, shortCode string) (*URLStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Resolve short_code → url_id
	var urlID int64
	err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT id FROM urls WHERE short_code = ?`, shortCode,
	).Scan(&urlID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("URL not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to look up URL: %w", err)
	}

	stats := &URLStats{ShortCode: shortCode}

	// Total clicks and unique IPs
	err = s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT ip_hash) FROM clicks WHERE url_id = ?`, urlID,
	).Scan(&stats.TotalClicks, &stats.UniqueIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to query total clicks: %w", err)
	}

	// Last 24 h
	if err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks WHERE url_id = ? AND clicked_at >= datetime('now', '-1 day')`, urlID,
	).Scan(&stats.Last24h); err != nil {
		return nil, fmt.Errorf("failed to query last-24h clicks: %w", err)
	}

	// Last 7 d
	if err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks WHERE url_id = ? AND clicked_at >= datetime('now', '-7 days')`, urlID,
	).Scan(&stats.Last7d); err != nil {
		return nil, fmt.Errorf("failed to query last-7d clicks: %w", err)
	}

	// Last 30 d
	if err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks WHERE url_id = ? AND clicked_at >= datetime('now', '-30 days')`, urlID,
	).Scan(&stats.Last30d); err != nil {
		return nil, fmt.Errorf("failed to query last-30d clicks: %w", err)
	}

	// Top referrers (top 10)
	refRows, err := s.store.ServerDB.QueryContext(ctx,
		`SELECT COALESCE(referrer, ''), COUNT(*) AS cnt
		 FROM clicks WHERE url_id = ?
		 GROUP BY referrer ORDER BY cnt DESC LIMIT 10`, urlID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query top referrers: %w", err)
	}
	defer refRows.Close()
	for refRows.Next() {
		var rs RefererSummary
		if err := refRows.Scan(&rs.Referer, &rs.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan referrer row: %w", err)
		}
		stats.TopReferers = append(stats.TopReferers, rs)
	}
	if err := refRows.Err(); err != nil {
		return nil, fmt.Errorf("referrer iteration error: %w", err)
	}

	// Daily clicks — last 30 days
	dailyRows, err := s.store.ServerDB.QueryContext(ctx,
		`SELECT strftime('%Y-%m-%d', clicked_at) AS day, COUNT(*) AS cnt
		 FROM clicks
		 WHERE url_id = ? AND clicked_at >= datetime('now', '-30 days')
		 GROUP BY day ORDER BY day ASC`, urlID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily clicks: %w", err)
	}
	defer dailyRows.Close()
	for dailyRows.Next() {
		var cs ClickSummary
		if err := dailyRows.Scan(&cs.Date, &cs.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan daily row: %w", err)
		}
		stats.DailyClicks = append(stats.DailyClicks, cs)
	}
	if err := dailyRows.Err(); err != nil {
		return nil, fmt.Errorf("daily-clicks iteration error: %w", err)
	}

	return stats, nil
}

// GetUserStats returns aggregate click statistics across all URLs owned by userID.
func (s *AnalyticsService) GetUserStats(ctx context.Context, userID int64) (*URLStats, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stats := &URLStats{ShortCode: ""}

	// Total clicks and unique IPs across all URLs for this user
	err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(DISTINCT c.ip_hash)
		 FROM clicks c
		 JOIN urls u ON u.id = c.url_id
		 WHERE u.user_id = ?`, userID,
	).Scan(&stats.TotalClicks, &stats.UniqueIPs)
	if err != nil {
		return nil, fmt.Errorf("failed to query total clicks: %w", err)
	}

	// Last 24 h
	if err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks c
		 JOIN urls u ON u.id = c.url_id
		 WHERE u.user_id = ? AND c.clicked_at >= datetime('now', '-1 day')`, userID,
	).Scan(&stats.Last24h); err != nil {
		return nil, fmt.Errorf("failed to query last-24h clicks: %w", err)
	}

	// Last 7 d
	if err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks c
		 JOIN urls u ON u.id = c.url_id
		 WHERE u.user_id = ? AND c.clicked_at >= datetime('now', '-7 days')`, userID,
	).Scan(&stats.Last7d); err != nil {
		return nil, fmt.Errorf("failed to query last-7d clicks: %w", err)
	}

	// Last 30 d
	if err := s.store.ServerDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM clicks c
		 JOIN urls u ON u.id = c.url_id
		 WHERE u.user_id = ? AND c.clicked_at >= datetime('now', '-30 days')`, userID,
	).Scan(&stats.Last30d); err != nil {
		return nil, fmt.Errorf("failed to query last-30d clicks: %w", err)
	}

	// Top referrers (top 10)
	refRows, err := s.store.ServerDB.QueryContext(ctx,
		`SELECT COALESCE(c.referrer, ''), COUNT(*) AS cnt
		 FROM clicks c
		 JOIN urls u ON u.id = c.url_id
		 WHERE u.user_id = ?
		 GROUP BY c.referrer ORDER BY cnt DESC LIMIT 10`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query top referrers: %w", err)
	}
	defer refRows.Close()
	for refRows.Next() {
		var rs RefererSummary
		if err := refRows.Scan(&rs.Referer, &rs.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan referrer row: %w", err)
		}
		stats.TopReferers = append(stats.TopReferers, rs)
	}
	if err := refRows.Err(); err != nil {
		return nil, fmt.Errorf("referrer iteration error: %w", err)
	}

	// Daily clicks — last 30 days
	dailyRows, err := s.store.ServerDB.QueryContext(ctx,
		`SELECT strftime('%Y-%m-%d', c.clicked_at) AS day, COUNT(*) AS cnt
		 FROM clicks c
		 JOIN urls u ON u.id = c.url_id
		 WHERE u.user_id = ? AND c.clicked_at >= datetime('now', '-30 days')
		 GROUP BY day ORDER BY day ASC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily clicks: %w", err)
	}
	defer dailyRows.Close()
	for dailyRows.Next() {
		var cs ClickSummary
		if err := dailyRows.Scan(&cs.Date, &cs.Clicks); err != nil {
			return nil, fmt.Errorf("failed to scan daily row: %w", err)
		}
		stats.DailyClicks = append(stats.DailyClicks, cs)
	}
	if err := dailyRows.Err(); err != nil {
		return nil, fmt.Errorf("daily-clicks iteration error: %w", err)
	}

	return stats, nil
}
