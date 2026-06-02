// Offline maintenance helpers for `caslink --maintenance backup` and
// `caslink --maintenance restore` (AI.md PART 22). These run against the
// filesystem only — for external databases the operator must still capture
// a DB dump separately. SQLite databases are part of the data directory and
// are included automatically.
package main

import "github.com/casjaysdevdocker/caslink/src/backup"

// runOfflineBackup delegates to backup.RunBackup.
func runOfflineBackup(configDir, dataDir, backupDir, explicitDst string) error {
	return backup.RunBackup(configDir, dataDir, backupDir, explicitDst)
}

// runOfflineRestore delegates to backup.RunRestore.
func runOfflineRestore(src, configDir, dataDir string) error {
	return backup.RunRestore(src, configDir, dataDir)
}
