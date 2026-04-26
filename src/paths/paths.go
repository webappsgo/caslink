package paths

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

// Paths holds all application directory paths
type Paths struct {
	Config string // Configuration directory
	Data   string // Data directory
	Log    string // Log directory
	PID    string // PID file path
}

// GetDefaultPaths returns the default paths based on OS and user privileges.
//
// For root/Administrator:
//   - Linux:   /etc/casapps/caslink, /var/lib/casapps/caslink, /var/log/casapps/caslink
//   - Windows: C:\ProgramData\casapps\caslink
//   - macOS:   /Library/Application Support/casapps/caslink
//
// For regular users:
//   - Linux:   ~/.config/casapps/caslink, ~/.local/share/casapps/caslink
//   - Windows: %APPDATA%\casapps\caslink
//   - macOS:   ~/Library/Application Support/casapps/caslink
func GetDefaultPaths(projectOrg, projectName string) *Paths {
	isRoot := isRunningAsRoot()

	switch runtime.GOOS {
	case "windows":
		return getWindowsPaths(projectOrg, projectName, isRoot)
	case "darwin":
		return getDarwinPaths(projectOrg, projectName, isRoot)
	default:
		// Linux, FreeBSD, and other Unix-like systems
		return getUnixPaths(projectOrg, projectName, isRoot)
	}
}

// getUnixPaths returns paths for Linux, FreeBSD, and other Unix-like systems
func getUnixPaths(projectOrg, projectName string, isRoot bool) *Paths {
	if isRoot {
		// Root paths (system-wide)
		base := filepath.Join("/var/lib", projectOrg, projectName)
		return &Paths{
			Config: filepath.Join("/etc", projectOrg, projectName),
			Data:   base,
			Log:    filepath.Join("/var/log", projectOrg, projectName),
			PID:    filepath.Join("/var/run", projectOrg, projectName+".pid"),
		}
	}

	// User paths (XDG Base Directory Specification)
	homeDir := getHomeDir()
	configDir := getEnvOrDefault("XDG_CONFIG_HOME", filepath.Join(homeDir, ".config"))
	dataDir := getEnvOrDefault("XDG_DATA_HOME", filepath.Join(homeDir, ".local", "share"))

	base := filepath.Join(dataDir, projectOrg, projectName)

	return &Paths{
		Config: filepath.Join(configDir, projectOrg, projectName),
		Data:   base,
		Log:    filepath.Join(base, "logs"),
		PID:    filepath.Join(base, projectName+".pid"),
	}
}

// getDarwinPaths returns paths for macOS
func getDarwinPaths(projectOrg, projectName string, isRoot bool) *Paths {
	if isRoot {
		// Root paths (system-wide)
		base := filepath.Join("/Library/Application Support", projectOrg, projectName)
		return &Paths{
			Config: base,
			Data:   base,
			Log:    filepath.Join("/Library/Logs", projectOrg, projectName),
			PID:    filepath.Join("/var/run", projectOrg, projectName+".pid"),
		}
	}

	// User paths
	homeDir := getHomeDir()
	base := filepath.Join(homeDir, "Library/Application Support", projectOrg, projectName)

	return &Paths{
		Config: base,
		Data:   base,
		Log:    filepath.Join(homeDir, "Library/Logs", projectOrg, projectName),
		PID:    filepath.Join(base, projectName+".pid"),
	}
}

// getWindowsPaths returns paths for Windows
func getWindowsPaths(projectOrg, projectName string, isRoot bool) *Paths {
	if isRoot {
		// Administrator paths (system-wide)
		programData := getEnvOrDefault("ProgramData", "C:\\ProgramData")
		base := filepath.Join(programData, projectOrg, projectName)

		return &Paths{
			Config: base,
			Data:   base,
			Log:    filepath.Join(base, "logs"),
			PID:    filepath.Join(base, projectName+".pid"),
		}
	}

	// User paths
	appData := getEnvOrDefault("APPDATA", filepath.Join(getHomeDir(), "AppData", "Roaming"))
	base := filepath.Join(appData, projectOrg, projectName)

	return &Paths{
		Config: base,
		Data:   base,
		Log:    filepath.Join(base, "logs"),
		PID:    filepath.Join(base, projectName+".pid"),
	}
}

// isRunningAsRoot returns true if running as root/Administrator
func isRunningAsRoot() bool {
	if runtime.GOOS == "windows" {
		// On Windows, check if running as Administrator
		// This is a simplified check - proper Windows admin detection requires syscalls
		return os.Getenv("USERNAME") == "Administrator" || os.Getenv("USERDOMAIN") == "BUILTIN"
	}

	// On Unix-like systems, check if UID is 0
	return os.Geteuid() == 0
}

// getHomeDir returns the user's home directory
func getHomeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}

	if runtime.GOOS == "windows" {
		if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
			return userProfile
		}
		if homeDrive := os.Getenv("HOMEDRIVE"); homeDrive != "" {
			if homePath := os.Getenv("HOMEPATH"); homePath != "" {
				return filepath.Join(homeDrive, homePath)
			}
		}
	}

	// Fallback: try to get home from user package
	if currentUser, err := user.Current(); err == nil {
		return currentUser.HomeDir
	}

	// Last resort: use /tmp on Unix, C:\Windows\Temp on Windows
	if runtime.GOOS == "windows" {
		return "C:\\Windows\\Temp"
	}
	return "/tmp"
}

// getEnvOrDefault returns the value of an environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// EnsureDir creates a directory with appropriate permissions if it doesn't exist
func EnsureDir(path string) error {
	isRoot := isRunningAsRoot()

	perm := os.FileMode(0700) // User-only by default
	if isRoot {
		perm = 0755 // World-readable for root
	}

	if err := os.MkdirAll(path, perm); err != nil {
		return err
	}

	// Verify directory is writable
	testFile := filepath.Join(path, ".write-test")
	if err := os.WriteFile(testFile, []byte{}, 0600); err != nil {
		return err
	}
	os.Remove(testFile)

	return nil
}

// EnsurePIDFile creates the directory for a PID file if it doesn't exist
func EnsurePIDFile(pidPath string) error {
	dir := filepath.Dir(pidPath)
	return EnsureDir(dir)
}

// ExpandPath expands ~ and environment variables in a path
func ExpandPath(path string) string {
	// Expand ~
	if len(path) > 0 && path[0] == '~' {
		homeDir := getHomeDir()
		if len(path) == 1 {
			return homeDir
		}
		return filepath.Join(homeDir, path[1:])
	}

	// Expand environment variables
	return os.ExpandEnv(path)
}

// ResolvePath resolves a path, expanding ~ and environment variables
// and converting to absolute path
func ResolvePath(path string) (string, error) {
	expanded := ExpandPath(path)
	return filepath.Abs(expanded)
}
