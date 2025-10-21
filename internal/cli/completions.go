package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `Generate shell completion scripts for caslink.

To load completions:

Bash:
  $ source <(caslink completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ caslink completion bash > /etc/bash_completion.d/caslink
  # macOS:
  $ caslink completion bash > $(brew --prefix)/etc/bash_completion.d/caslink

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ caslink completion zsh > "${fpath[1]}/_caslink"

  # You will need to start a new shell for this setup to take effect.

Fish:
  $ caslink completion fish | source

  # To load completions for each session, execute once:
  $ caslink completion fish > ~/.config/fish/completions/caslink.fish

PowerShell:
  PS> caslink completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> caslink completion powershell > caslink.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}