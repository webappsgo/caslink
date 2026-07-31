package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/webappsgo/caslink/src/server/store"
)

// AuditService records and retrieves admin/security audit events. The audit
// log is append-only (AI.md PART 17 "Audit Log" — see also IDEA.md line 51:
// "Audit log append-only; never contains raw credentials") and stored in
// UsersDB per Store's own field documentation ("Users, sessions, tokens,
// domains, config, audit logs").
type AuditService struct {
	store *store.Store
}

// NewAuditService creates a new audit service.
func NewAuditService(st *store.Store) *AuditService {
	return &AuditService{store: st}
}

// AuditEntry is a single audit_log row.
type AuditEntry struct {
	ID        int64
	UserID    *int64
	UserType  string
	Action    string
	Resource  string
	Details   string
	IPAddress string
	UserAgent string
	CreatedAt time.Time
}

// RecordEvent appends one entry to the audit log. userID may be nil for
// anonymous/system events (e.g. a failed login with an unknown username).
// Never pass raw credentials in details.
func (a *AuditService) RecordEvent(ctx context.Context, userID *int64, userType, action, resource, details, ipAddress, userAgent string) error {
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := a.store.UsersDB.ExecContext(qctx,
		`INSERT INTO audit_log (user_id, user_type, action, resource, details, ip_address, user_agent, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, userType, action, resource, details, ipAddress, userAgent, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("failed to record audit event: %w", err)
	}
	return nil
}

// ListEvents returns a paginated, most-recent-first slice of audit log
// entries along with the total row count.
func (a *AuditService) ListEvents(ctx context.Context, page, limit int) ([]AuditEntry, int, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 250 {
		limit = 50
	}
	offset := (page - 1) * limit

	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var total int
	if err := a.store.UsersDB.QueryRowContext(qctx, `SELECT COUNT(*) FROM audit_log`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count audit log: %w", err)
	}

	rows, err := a.store.UsersDB.QueryContext(qctx,
		`SELECT id, user_id, user_type, action, resource, details, ip_address, user_agent, created_at
		 FROM audit_log ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var userID sql.NullInt64
		var userType, resource, details, ipAddress, userAgent sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&e.ID, &userID, &userType, &e.Action, &resource, &details, &ipAddress, &userAgent, &createdAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan audit entry: %w", err)
		}
		if userID.Valid {
			id := userID.Int64
			e.UserID = &id
		}
		e.UserType = userType.String
		e.Resource = resource.String
		e.Details = details.String
		e.IPAddress = ipAddress.String
		e.UserAgent = userAgent.String
		e.CreatedAt = createdAt
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate audit log: %w", err)
	}

	return entries, total, nil
}
