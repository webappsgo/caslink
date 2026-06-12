package store

import (
	"context"
	"database/sql"
	"time"
)

// GetConfigValue retrieves a value from the config key-value store in server.db.
// Returns ("", false, nil) when the key does not exist.
func (s *Store) GetConfigValue(key string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var value string
	err := s.ServerDB.QueryRowContext(ctx,
		`SELECT value FROM config WHERE key = ?`, key,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetConfigValue inserts or updates a key in the config table in server.db.
// updatedBy is the admin username performing the change (used for auditing).
func (s *Store) SetConfigValue(key, value, updatedBy string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	now := time.Now().Unix()
	_, err := s.ServerDB.ExecContext(ctx,
		`INSERT INTO config (key, value, updated_at, updated_by)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET
		   value      = excluded.value,
		   updated_at = excluded.updated_at,
		   updated_by = excluded.updated_by`,
		key, value, now, updatedBy,
	)
	return err
}

// GetConfigValues retrieves multiple keys in one query from server.db.
// Returns a map of key → value for keys that exist.
func (s *Store) GetConfigValues(keys ...string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return result, nil
	}

	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}

	placeholders := "?"
	for i := 1; i < len(keys); i++ {
		placeholders += ",?"
	}

	rows, err := s.ServerDB.QueryContext(ctx,
		`SELECT key, value FROM config WHERE key IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		result[k] = v
	}
	return result, rows.Err()
}
