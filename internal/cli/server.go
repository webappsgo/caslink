package cli

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/casjaysdevdocker/caslink/internal/config"
	"github.com/casjaysdevdocker/caslink/internal/server"
)

var serverCmd = &cobra.Command{
	Use:     "webui",
	Aliases: []string{"server", "start", "run"},
	Short:   "Start the web server",
	Long:    `Start the Caslink web server with the web UI interface.`,
	RunE:    runServer,
}

func init() {
	serverCmd.Flags().String("host", "", "server bind address (default: 0.0.0.0)")
	serverCmd.Flags().String("port", "", "server port (default: auto-select)")
	serverCmd.Flags().Bool("setup", false, "force setup mode")

	viper.BindPFlag("server.host", serverCmd.Flags().Lookup("host"))
	viper.BindPFlag("server.port", serverCmd.Flags().Lookup("port"))
	viper.BindPFlag("setup", serverCmd.Flags().Lookup("setup"))
}

func runServer(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Auto-detect external IP for display
	externalIP := detectExternalIP()

	// Display startup information
	displayStartupInfo(cfg, externalIP)

	// Create and start server
	srv, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	return srv.Start(ctx)
}

func detectExternalIP() string {
	// Try to detect external IP through various methods

	// Method 1: Check network interfaces
	addrs, err := net.InterfaceAddrs()
	if err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					return ipNet.IP.String()
				}
			}
		}
	}

	// Method 2: Check common environment variables
	if ip := os.Getenv("SERVER_IP"); ip != "" {
		return ip
	}
	if ip := os.Getenv("HOST_IP"); ip != "" {
		return ip
	}

	// Fallback to localhost
	return "127.0.0.1"
}

func displayStartupInfo(cfg *config.Config, externalIP string) {
	host := cfg.Server.Host
	if host == "0.0.0.0" || host == "" {
		host = "localhost"
	}

	port := cfg.Server.Port
	if port == 0 {
		port = 64000 // Default port range start
	}

	// ASCII box for visual appeal
	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────┐")
	fmt.Println("│                     Caslink URL Shortener                  │")
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Version: %-50s │\n", getVersionString())
	fmt.Printf("│ Local:   http://%-42s │\n", fmt.Sprintf("%s:%d", host, port))
	if externalIP != "127.0.0.1" && externalIP != host {
		fmt.Printf("│ Network: http://%-42s │\n", fmt.Sprintf("%s:%d", externalIP, port))
	}
	fmt.Println("├─────────────────────────────────────────────────────────────┤")
	fmt.Println("│ First-time setup:                                          │")
	fmt.Println("│ 1. Open the URL above in your browser                     │")
	fmt.Println("│ 2. Create your admin account                              │")
	fmt.Println("│ 3. Create your first short URL                           │")
	fmt.Println("│                                                            │")
	fmt.Println("│ Press Ctrl+C to stop the server                          │")
	fmt.Println("└─────────────────────────────────────────────────────────────┘")
	fmt.Println()

	log.Printf("Starting Caslink server on %s:%d", cfg.Server.Host, port)
}