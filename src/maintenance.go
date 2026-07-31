// Offline maintenance helpers for `caslink --maintenance backup` and
// `caslink --maintenance restore` (AI.md PART 22). These run against the
// filesystem only — for external databases the operator must still capture
// a DB dump separately. SQLite databases are part of the data directory and
// are included automatically.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/webappsgo/caslink/src/backup"
	"golang.org/x/term"
)

// runOfflineBackup delegates to backup.RunBackup.
func runOfflineBackup(configDir, dataDir, backupDir, explicitDst string, opts backup.Options) error {
	return backup.RunBackup(configDir, dataDir, backupDir, explicitDst, opts)
}

// runOfflineRestore delegates to backup.RunRestore.
func runOfflineRestore(src, configDir, dataDir, password string) error {
	return backup.RunRestore(src, configDir, dataDir, password)
}

// extractPasswordFlag scans args for "--password <value>" (or
// "--password=<value>") per AI.md PART 22's CLI usage, returning the
// password and the remaining positional args in order.
func extractPasswordFlag(args []string) (password string, rest []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--password":
			if i+1 < len(args) {
				password = args[i+1]
				i++
			}
		case len(arg) > len("--password=") && arg[:len("--password=")] == "--password=":
			password = arg[len("--password="):]
		default:
			rest = append(rest, arg)
		}
	}
	return password, rest
}

// promptBackupPassword prints prompt to stderr and reads a password from the
// terminal without echoing it, per AI.md PART 22 ("Prompts for password").
// Falls back to a plain (echoed) line read when stdin is not a terminal, so
// scripted/non-interactive callers do not hang.
func promptBackupPassword(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	if term.IsTerminal(int(os.Stdin.Fd())) {
		pw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return ""
		}
		return string(pw)
	}
	// Non-terminal stdin: read a full line so passwords containing spaces are
	// preserved (Fscanln would truncate at the first whitespace).
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimRight(line, "\r\n")
}
