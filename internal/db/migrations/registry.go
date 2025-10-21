package migrations

import (
	"sort"
	"sync"
)

var (
	// Global migration registry
	migrationRegistry = &Registry{
		migrations: make(map[string]*Migration),
		order:      make([]string, 0),
		mutex:      sync.RWMutex{},
	}
)

// Registry holds all registered migrations
type Registry struct {
	migrations map[string]*Migration
	order      []string
	mutex      sync.RWMutex
}

// RegisterMigration registers a new migration
func RegisterMigration(migration *Migration) {
	migrationRegistry.mutex.Lock()
	defer migrationRegistry.mutex.Unlock()

	migrationRegistry.migrations[migration.ID] = migration
	migrationRegistry.order = append(migrationRegistry.order, migration.ID)

	// Keep order sorted
	sort.Strings(migrationRegistry.order)
}

// GetAllMigrations returns all registered migrations in order
func GetAllMigrations() []*Migration {
	migrationRegistry.mutex.RLock()
	defer migrationRegistry.mutex.RUnlock()

	migrations := make([]*Migration, len(migrationRegistry.order))
	for i, id := range migrationRegistry.order {
		migrations[i] = migrationRegistry.migrations[id]
	}

	return migrations
}

// GetMigrationByID returns a migration by its ID
func GetMigrationByID(id string) *Migration {
	migrationRegistry.mutex.RLock()
	defer migrationRegistry.mutex.RUnlock()

	return migrationRegistry.migrations[id]
}

// GetMigrationIDs returns all migration IDs in order
func GetMigrationIDs() []string {
	migrationRegistry.mutex.RLock()
	defer migrationRegistry.mutex.RUnlock()

	ids := make([]string, len(migrationRegistry.order))
	copy(ids, migrationRegistry.order)
	return ids
}

// GetMigrationCount returns the total number of registered migrations
func GetMigrationCount() int {
	migrationRegistry.mutex.RLock()
	defer migrationRegistry.mutex.RUnlock()

	return len(migrationRegistry.migrations)
}