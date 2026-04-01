package cmd

import (
	"fmt"

	"github.com/aescoubas/tasc/skills"
	"github.com/spf13/cobra"
)

var (
	skillClient string
	skillDest   string
	skillForce  bool
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage bundled agent skills",
}

var skillInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the bundled tasc agent skill",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		rootDir, err := resolveSkillInstallRoot(skillClient, skillDest)
		if err != nil {
			return err
		}

		installedDir, err := skills.InstallTascTaskManager(rootDir, skillForce)
		if err != nil {
			return err
		}

		fmt.Printf("Installed bundled skill to %s\n", installedDir)
		fmt.Println("Configure your client to launch the MCP server with: tasc mcp")
		return nil
	},
}

func resolveSkillInstallRoot(client, dest string) (string, error) {
	if dest != "" {
		return dest, nil
	}

	switch client {
	case "", "codex":
		return skills.DefaultCodexRoot()
	case "claude":
		return skills.DefaultClaudeRoot()
	case "gemini":
		return skills.DefaultGeminiRoot()
	default:
		return "", fmt.Errorf("unsupported client %q; use codex, claude, gemini, or --dest to install into a compatible skills directory", client)
	}
}

func init() {
	skillInstallCmd.Flags().StringVar(&skillClient, "client", "codex", "Predefined install target. Supported: codex, claude, gemini")
	skillInstallCmd.Flags().StringVar(&skillDest, "dest", "", "Install into this skills root directory instead of a predefined client path")
	skillInstallCmd.Flags().BoolVar(&skillForce, "force", false, "Replace an existing installed copy of the bundled skill")

	skillCmd.AddCommand(skillInstallCmd)
	rootCmd.AddCommand(skillCmd)
}
