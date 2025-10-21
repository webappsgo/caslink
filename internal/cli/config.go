package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
	Long:  `Manage Caslink configuration settings.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Current configuration:")
		for _, key := range viper.AllKeys() {
			fmt.Printf("  %s: %v\n", key, viper.Get(key))
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := args[0], args[1]
		viper.Set(key, value)

		if err := viper.WriteConfig(); err != nil {
			// Try to write to default location if no config file exists
			viper.SetConfigFile("./config.toml")
			if err := viper.WriteConfigAs("./config.toml"); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile := "./config.toml"
		if _, err := os.Stat(configFile); err == nil {
			return fmt.Errorf("configuration file already exists: %s", configFile)
		}

		// Set some default values
		viper.SetDefault("server.host", "0.0.0.0")
		viper.SetDefault("server.port", "auto")
		viper.SetDefault("database.type", "sqlite")
		viper.SetDefault("database.url", "sqlite:///var/lib/caslink/caslink.db")

		if err := viper.WriteConfigAs(configFile); err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}

		fmt.Printf("Created configuration file: %s\n", configFile)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configInitCmd)
}