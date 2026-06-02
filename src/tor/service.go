package tor

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/cretz/bine/control"
	binetор "github.com/cretz/bine/tor"

	"github.com/casjaysdevdocker/caslink/src/config"
)

// TorService holds the running Tor instance and its metadata.
type TorService struct {
	instance    *binetор.Tor
	onionAddr   string
	outDialer   *binetор.Dialer
}

// TorManager manages the full lifecycle of the Tor hidden service.
type TorManager struct {
	mu         sync.Mutex
	service    *TorService
	cfg        *config.TorConfig
	configDir  string
	dataDir    string
	serverPort int
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewTorManager returns a TorManager. Start() must be called to launch Tor.
func NewTorManager(ctx context.Context, serverPort int, cfg *config.TorConfig, configDir, dataDir string) *TorManager {
	child, cancel := context.WithCancel(ctx)
	return &TorManager{
		cfg:        cfg,
		configDir:  configDir,
		dataDir:    dataDir,
		serverPort: serverPort,
		ctx:        child,
		cancel:     cancel,
	}
}

// Start finds the Tor binary, creates the hidden service, and optionally
// creates an outbound SOCKS5 dialer. A missing Tor binary is not an error —
// it is logged at INFO level and Start returns nil.
func (tm *TorManager) Start() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	torBin := findTorBinary(tm.cfg.Binary)
	if torBin == "" {
		log.Printf("[tor] INFO: tor binary not found, hidden service disabled")
		return nil
	}

	log.Printf("[tor] INFO: starting tor hidden service...")

	if err := ensureTorDirs(tm.configDir, tm.dataDir); err != nil {
		log.Printf("[tor] WARN: could not create tor directories: %v", err)
		return nil
	}

	torrcPath := filepath.Join(tm.configDir, "torrc")
	torDataDir := filepath.Join(tm.dataDir, "tor", "data")
	keyDir := filepath.Join(tm.dataDir, "tor", "site")
	keyFile := filepath.Join(keyDir, "hs_ed25519_secret_key")

	torrcContent := []byte(getTorConfig(tm.cfg))
	created, err := ensureTorrc(torrcPath, torrcContent)
	if err != nil {
		log.Printf("[tor] WARN: could not write torrc: %v", err)
		return nil
	}
	if !created {
		// Overwrite with current config on subsequent starts.
		if err := updateTorrc(torrcPath, torrcContent); err != nil {
			log.Printf("[tor] WARN: could not update torrc: %v", err)
		}
	}

	startConf := &binetор.StartConf{
		ExePath:         torBin,
		DataDir:         torDataDir,
		TorrcFile:       torrcPath,
		NoAutoSocksPort: !tm.cfg.UseNetwork,
	}

	t, err := binetор.Start(tm.ctx, startConf)
	if err != nil {
		log.Printf("[tor] WARN: failed to start tor process: %v", err)
		return nil
	}

	bootstrapTimeout := parseDuration(tm.cfg.BootstrapTimeout, 3*time.Minute)
	bootCtx, bootCancel := context.WithTimeout(tm.ctx, bootstrapTimeout)
	defer bootCancel()

	// Notify on slow bootstrap.
	slowTimer := time.AfterFunc(30*time.Second, func() {
		log.Printf("[tor] tor: connecting...")
	})

	if err := t.EnableNetwork(bootCtx, true); err != nil {
		slowTimer.Stop()
		t.Close()
		log.Printf("[tor] WARN: tor bootstrap timeout, hidden service unavailable")
		return nil
	}
	slowTimer.Stop()

	// Load or generate ED25519 key for a persistent .onion address.
	var onionKey control.Key
	if data, err := os.ReadFile(keyFile); err == nil && len(data) > 0 {
		existing, parseErr := control.ED25519KeyFromBlob(strings.TrimSpace(string(data)))
		if parseErr != nil {
			log.Printf("[tor] WARN: key file invalid, generating new key: %v", parseErr)
			onionKey = control.GenKey(control.KeyAlgoED25519V3)
		} else {
			onionKey = existing
		}
	} else {
		onionKey = control.GenKey(control.KeyAlgoED25519V3)
	}

	virtualPort := tm.cfg.VirtualPort
	if virtualPort <= 0 {
		virtualPort = 80
	}

	resp, err := t.Control.AddOnion(&control.AddOnionRequest{
		Key: onionKey,
		Ports: []*control.KeyVal{
			control.NewKeyVal(fmt.Sprintf("%d", virtualPort), fmt.Sprintf("127.0.0.1:%d", tm.serverPort)),
		},
	})
	if err != nil {
		t.Close()
		log.Printf("[tor] WARN: ADD_ONION failed: %v", err)
		return nil
	}

	// Persist key only when Tor generated a new one (resp.Key is non-nil).
	if resp.Key != nil {
		if err := os.WriteFile(keyFile, []byte(resp.Key.Blob()), 0600); err != nil {
			log.Printf("[tor] WARN: could not save onion key: %v", err)
		}
	}

	svc := &TorService{
		instance:  t,
		onionAddr: resp.ServiceID + ".onion",
	}

	// Create outbound SOCKS5 dialer when enabled.
	if tm.cfg.UseNetwork {
		d, dialErr := t.Dialer(tm.ctx, &binetор.DialConf{SkipEnableNetwork: true})
		if dialErr != nil {
			log.Printf("[tor] WARN: could not create outbound dialer: %v", dialErr)
		} else {
			svc.outDialer = d
		}
	}

	tm.service = svc
	log.Printf("[tor] tor: %s", svc.onionAddr)
	return nil
}

// Stop shuts down the Tor process and clears the service state.
func (tm *TorManager) Stop() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.stopLocked()
}

// stopLocked shuts down without acquiring the lock — caller must hold mu.
func (tm *TorManager) stopLocked() error {
	if tm.service == nil {
		return nil
	}
	err := tm.service.instance.Close()
	tm.service = nil
	return err
}

// Restart stops and restarts the Tor hidden service.
func (tm *TorManager) Restart() error {
	tm.mu.Lock()
	if err := tm.stopLocked(); err != nil {
		tm.mu.Unlock()
		return err
	}
	tm.mu.Unlock()
	return tm.Start()
}

// RegenerateAddress deletes the stored key and restarts Tor so a new .onion
// address is issued. The new address is returned.
func (tm *TorManager) RegenerateAddress() (string, error) {
	keyFile := filepath.Join(tm.dataDir, "tor", "site", "hs_ed25519_secret_key")
	if err := os.Remove(keyFile); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove key: %w", err)
	}
	if err := tm.Restart(); err != nil {
		return "", err
	}
	return tm.OnionAddress(), nil
}

// ApplyKeys writes the given private key blob to disk and restarts Tor so the
// provided .onion address is used. The resulting address is returned.
func (tm *TorManager) ApplyKeys(privateKeyBlob []byte) (string, error) {
	keyDir := filepath.Join(tm.dataDir, "tor", "site")
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", fmt.Errorf("create key dir: %w", err)
	}
	keyFile := filepath.Join(keyDir, "hs_ed25519_secret_key")
	if err := os.WriteFile(keyFile, privateKeyBlob, 0600); err != nil {
		return "", fmt.Errorf("write key: %w", err)
	}
	if err := tm.Restart(); err != nil {
		return "", err
	}
	return tm.OnionAddress(), nil
}

// OnionAddress returns the active .onion address, or an empty string when the
// hidden service is not running.
func (tm *TorManager) OnionAddress() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.service == nil {
		return ""
	}
	return tm.service.onionAddr
}

// IsRunning reports whether the hidden service is currently active.
func (tm *TorManager) IsRunning() bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.service != nil
}

// GetHTTPClient returns an *http.Client. When useTor is true and an outbound
// dialer is available, connections are routed through the Tor SOCKS5 proxy.
// Otherwise a standard client is returned.
func (tm *TorManager) GetHTTPClient(useTor bool) *http.Client {
	tm.mu.Lock()
	d := tm.service
	tm.mu.Unlock()

	if useTor && d != nil && d.outDialer != nil {
		transport := &http.Transport{
			DialContext: d.outDialer.DialContext,
		}
		return &http.Client{Transport: transport}
	}
	return &http.Client{}
}

// findTorBinary returns the path to the tor executable. It checks the
// explicitly configured path first, then PATH, then common system locations.
func findTorBinary(configured string) string {
	if configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured
		}
	}

	if path, err := exec.LookPath("tor"); err == nil {
		return path
	}

	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{`C:\Program Files\Tor\tor.exe`}
	case "darwin":
		candidates = []string{"/usr/local/bin/tor", "/opt/homebrew/bin/tor"}
	case "freebsd":
		candidates = []string{"/usr/local/bin/tor"}
	default:
		// Linux and other Unix-like systems.
		candidates = []string{"/usr/bin/tor", "/usr/local/bin/tor"}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// ensureTorDirs creates all required Tor subdirectories with mode 0700.
func ensureTorDirs(configDir, dataDir string) error {
	dirs := []string{
		configDir,
		filepath.Join(dataDir, "tor", "data"),
		filepath.Join(dataDir, "tor", "site"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0700); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

// ensureTorrc writes content to path only when the file does not already
// exist. It returns (true, nil) when the file was created, (false, nil) when
// it already existed.
func ensureTorrc(path string, content []byte) (created bool, err error) {
	if _, statErr := os.Stat(path); statErr == nil {
		return false, nil
	}
	if err := os.WriteFile(path, content, 0600); err != nil {
		return false, err
	}
	return true, nil
}

// updateTorrc always overwrites path with content.
func updateTorrc(path string, content []byte) error {
	return os.WriteFile(path, content, 0600)
}

// getTorConfig generates torrc content from TorConfig.
func getTorConfig(cfg *config.TorConfig) string {
	var sb strings.Builder

	if cfg.UseNetwork {
		sb.WriteString("SocksPort auto\n")
	} else {
		sb.WriteString("SocksPort 0\n")
	}

	sb.WriteString("ControlPort 127.0.0.1:auto\n")

	if cfg.SafeLogging {
		sb.WriteString("SafeLogging 1\n")
	} else {
		sb.WriteString("SafeLogging 0\n")
	}

	sb.WriteString("MaxCircuitDirtiness 600\n")

	if cfg.BandwidthRate != "" {
		fmt.Fprintf(&sb, "BandwidthRate %s\n", cfg.BandwidthRate)
	}
	if cfg.BandwidthBurst != "" {
		fmt.Fprintf(&sb, "BandwidthBurst %s\n", cfg.BandwidthBurst)
	}

	if cfg.MaxMonthlyBandwidth != "" && cfg.MaxMonthlyBandwidth != "unlimited" {
		sb.WriteString("AccountingStart month 1 00:00\n")
		fmt.Fprintf(&sb, "AccountingMax %s\n", cfg.MaxMonthlyBandwidth)
	}

	sb.WriteString("ExitRelay 0\n")
	sb.WriteString("ExitPolicy reject *:*\n")
	sb.WriteString("ORPort 0\n")
	sb.WriteString("DirPort 0\n")
	sb.WriteString("HiddenServiceSingleHopMode 0\n")
	sb.WriteString("FetchDirInfoEarly 1\n")
	sb.WriteString("FetchDirInfoExtraEarly 1\n")
	sb.WriteString("DisableDebuggerAttachment 1\n")

	return sb.String()
}

// parseDuration parses a duration string (e.g. "3m", "60s"). Falls back to
// fallback when the string is empty or unparseable.
func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

// dialContextAdapter bridges binetор.Dialer.DialContext for http.Transport.
func dialContextAdapter(d *binetор.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return d.DialContext(ctx, network, addr)
	}
}
