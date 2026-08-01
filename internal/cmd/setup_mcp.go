package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type SetupMCPConfig struct {
	Client string
	Scale  string
	Scope  string // deprecated
	DryRun bool
}

type ClientDefinition struct {
	ID                   string
	Name                 string
	GlobalConfigRelPath  func() (string, error)
	WorkspaceConfigRel   string
	UseZedFormat         bool
	UseCrushFormat       bool
	TopLevelKey          string
	IncludeToolsWildcard bool
}

func getHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine user home directory: %w", err)
	}
	return home, nil
}

func getAppDataDir() (string, error) {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return appData, nil
	}
	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "AppData", "Roaming"), nil
}

func isMacOS() bool {
	return strings.HasPrefix(os.Getenv("HOME"), "/Users")
}

// RowliteServerConfig represents the internal configuration for the Rowlite MCP server.
type RowliteServerConfig struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"` // "stdio", "http", or "sse"
}

// ZedServerConfig represents Zed configuration format.
type ZedServerConfig struct {
	Command struct {
		Path string   `json:"path"`
		Args []string `json:"args"`
	} `json:"command"`
}

// CrushServerConfig represents Crush configuration format.
type CrushServerConfig struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// StandardServerConfig represents the standard MCP server configuration block.
type StandardServerConfig struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Tools   []string `json:"tools,omitempty"`
}

var knownClients = []ClientDefinition{
	{
		ID:   "antigravity",
		Name: "Antigravity",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".gemini", "config", "mcp_config.json"), nil
		},
		WorkspaceConfigRel: filepath.Join(".agents", "mcp_config.json"),
	},
	{
		ID:   "claude-desktop",
		Name: "Claude Desktop",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			if os.Getenv("OS") == "Windows_NT" || strings.HasSuffix(os.Getenv("OS"), "Windows") {
				appData, err := getAppDataDir()
				if err != nil {
					return "", err
				}
				return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
			} else if isMacOS() {
				return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
			}
			return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
		},
	},
	{
		ID:   "cursor",
		Name: "Cursor",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".cursor", "mcp.json"), nil
		},
		WorkspaceConfigRel: filepath.Join(".cursor", "mcp.json"),
	},
	{
		ID:   "claude-code",
		Name: "Claude Code",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".claude.json"), nil
		},
		WorkspaceConfigRel: ".mcp.json",
	},
	{
		ID:   "windsurf",
		Name: "Windsurf",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), nil
		},
	},
	{
		ID:                 "vscode",
		Name:               "VS Code",
		WorkspaceConfigRel: filepath.Join(".vscode", "mcp.json"),
		TopLevelKey:        "servers",
	},
	{
		ID:   "copilot-cli",
		Name: "GitHub Copilot CLI",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".copilot", "mcp-config.json"), nil
		},
		WorkspaceConfigRel:   filepath.Join(".copilot", "mcp-config.json"),
		IncludeToolsWildcard: true,
	},
	{
		ID:   "zed",
		Name: "Zed",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".config", "zed", "settings.json"), nil
		},
		UseZedFormat: true,
	},
	{
		ID:   "cline",
		Name: "Cline",
		GlobalConfigRelPath: func() (string, error) {
			if os.Getenv("OS") == "Windows_NT" || strings.HasSuffix(os.Getenv("OS"), "Windows") {
				appData, err := getAppDataDir()
				if err != nil {
					return "", err
				}
				return filepath.Join(appData, "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json"), nil
			} else if isMacOS() {
				home, err := getHomeDir()
				if err != nil {
					return "", err
				}
				return filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json"), nil
			}
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".config", "Code", "User", "globalStorage", "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json"), nil
		},
	},
	{
		ID:   "crush",
		Name: "Crush",
		GlobalConfigRelPath: func() (string, error) {
			home, err := getHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, ".config", "crush", "crush.json"), nil
		},
		WorkspaceConfigRel: ".crush.json",
		UseCrushFormat:     true,
	},
}

func NewSetupMCPCmd() *cobra.Command {
	var cfg SetupMCPConfig

	cmd := &cobra.Command{
		Use:     "mcp-setup",
		Aliases: []string{"setup-mcp"},
		Short:   "Register rowlite MCP server in AI client configuration files",
		Long: `Automatically discovers installed AI clients (Antigravity, Cursor, Claude Desktop, Windsurf, Zed, VS Code, Claude Code, Copilot CLI, Cline)
and registers the rowlite MCP server with an absolute binary path.

Use --client to target a specific client, or --scale workspace to update local project configs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetupMCP(cmd, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Client, "client", "", "target a specific client (antigravity, claude, cursor, claude-code, windsurf, vscode, copilot, zed, cline, all)")
	cmd.Flags().StringVarP(&cfg.Scale, "scale", "s", "global", "configuration scale: global or workspace")
	cmd.Flags().StringVar(&cfg.Scope, "scope", "", "deprecated: use --scale")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "display projected JSON changes without modifying files on disk")

	return cmd
}

func runSetupMCP(cmd *cobra.Command, cfg SetupMCPConfig) error {
	if cfg.Scope != "" {
		cfg.Scale = cfg.Scope
	}

	if cfg.Scale != "global" && cfg.Scale != "workspace" {
		return fmt.Errorf("invalid scale %q: must be 'global' or 'workspace'", cfg.Scale)
	}

	execPath, err := os.Executable()
	if err != nil {
		execPath = "rowlite"
	} else {
		execPath = filepath.Clean(execPath)
	}

	var targets []ClientDefinition

	if cfg.Client == "" {
		// 1. Scan system for installed configurations
		var detected []ClientDefinition
		for _, client := range knownClients {
			var pathToCheck string
			if cfg.Scale == "workspace" {
				if client.WorkspaceConfigRel != "" {
					pathToCheck = client.WorkspaceConfigRel
				}
			} else {
				if client.GlobalConfigRelPath != nil {
					p, err := client.GlobalConfigRelPath()
					if err == nil {
						pathToCheck = p
					}
				}
			}

			if pathToCheck != "" {
				dir := filepath.Dir(pathToCheck)
				if _, err := os.Stat(dir); err == nil {
					detected = append(detected, client)
				}
			}
		}

		if len(detected) == 0 {
			cmd.Println("[rowlite] No installed AI client configurations detected on this machine.")
			cmd.Println("Use --client <name> to explicitly configure a client.")
			return nil
		}

		// Show interactive menu
		cmd.Println("[rowlite] Interactive Setup - Detected AI Client Configurations:")
		for i, client := range detected {
			var configPath string
			if cfg.Scale == "workspace" {
				configPath = client.WorkspaceConfigRel
			} else {
				configPath, _ = client.GlobalConfigRelPath()
			}
			cmd.Printf("  %d) %s (%s) - Config Path: %s\n", i+1, client.Name, strings.Title(cfg.Scale), configPath)
		}
		cmd.Printf("  A) All detected clients\n")
		cmd.Printf("  Q) Quit\n\n")

		fmt.Fprint(cmd.OutOrStdout(), "Select client(s) to configure (enter number, comma-separated list, 'A' for all, or 'Q' to quit) [default: Q]: ")
		
		var input string
		var inputBuf [512]byte
		n, err := cmd.InOrStdin().Read(inputBuf[:])
		if err == nil && n > 0 {
			input = strings.TrimSpace(string(inputBuf[:n]))
		}

		if input == "" || strings.ToUpper(input) == "Q" {
			cmd.Println("\nSetup cancelled. Use --client <name> to configure explicitly.")
			return nil
		}

		if strings.ToUpper(input) == "A" {
			targets = detected
		} else {
			parts := strings.Split(input, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				var idx int
				_, err := fmt.Sscanf(p, "%d", &idx)
				if err == nil && idx >= 1 && idx <= len(detected) {
					targets = append(targets, detected[idx-1])
				}
			}
			if len(targets) == 0 {
				cmd.Println("\nNo valid selections made. Setup cancelled.")
				return nil
			}
		}
	} else if strings.ToLower(cfg.Client) == "all" {
		for _, client := range knownClients {
			if cfg.Scale == "workspace" && client.WorkspaceConfigRel != "" {
				targets = append(targets, client)
			} else if cfg.Scale == "global" && client.GlobalConfigRelPath != nil {
				targets = append(targets, client)
			}
		}
	} else {
		clientLower := strings.ToLower(cfg.Client)
		var found bool
		for _, c := range knownClients {
			if c.ID == clientLower || (c.ID == "claude-desktop" && clientLower == "claude") || (c.ID == "copilot-cli" && clientLower == "copilot") {
				targets = append(targets, c)
				found = true
				break
			}
		}
		if !found {
			var valid []string
			for _, c := range knownClients {
				valid = append(valid, c.ID)
			}
			return fmt.Errorf("unknown client %q (valid clients: %s)", cfg.Client, strings.Join(valid, ", "))
		}
	}

	// Auto-resolve scale if client is specified and scale/scope wasn't explicitly changed.
	if cfg.Client != "" && !cmd.Flags().Changed("scale") && !cmd.Flags().Changed("scope") {
		client := targets[0]
		if client.GlobalConfigRelPath == nil && client.WorkspaceConfigRel != "" {
			cfg.Scale = "workspace"
		} else if client.GlobalConfigRelPath != nil && client.WorkspaceConfigRel == "" {
			cfg.Scale = "global"
		}
	}

	var registered int
	for _, client := range targets {
		var configPath string

		if cfg.Scale == "workspace" {
			if client.WorkspaceConfigRel == "" {
				if cfg.Client != "" {
					return fmt.Errorf("client %s does not support workspace-level configuration", client.Name)
				}
				continue
			}
			configPath = client.WorkspaceConfigRel
		} else { // global
			if client.GlobalConfigRelPath == nil {
				if cfg.Client != "" {
					return fmt.Errorf("client %s does not support global-level configuration", client.Name)
				}
				continue
			}
			p, err := client.GlobalConfigRelPath()
			if err != nil {
				continue
			}
			configPath = p
		}

		if cfg.DryRun {
			cmd.Printf("[rowlite] Dry run for %s (%s):\n", client.Name, configPath)
			var node interface{}
			if client.UseZedFormat {
				zc := ZedServerConfig{}
				zc.Command.Path = execPath
				zc.Command.Args = []string{"mcp"}
				node = zc
			} else if client.UseCrushFormat {
				cc := CrushServerConfig{
					Type:    "stdio",
					Command: execPath,
					Args:    []string{"mcp"},
				}
				node = cc
			} else {
				sc := StandardServerConfig{
					Command: execPath,
					Args:    []string{"mcp"},
				}
				if client.IncludeToolsWildcard {
					sc.Tools = []string{"*"}
				}
				node = sc
			}
			nodeBytes, _ := json.MarshalIndent(node, "  ", "  ")
			cmd.Printf("Proposed configuration addition under key %q:\n%s\n\n", "rowlite", string(nodeBytes))
			registered++
			continue
		}

		err := upsertMCPConfig(configPath, execPath, client.UseZedFormat, client.UseCrushFormat, client.TopLevelKey, client.IncludeToolsWildcard)
		if err != nil {
			cmd.PrintErrf("[rowlite] Error configuring %s (%s): %v\n", client.Name, configPath, err)
			continue
		}

		cmd.Printf("[rowlite] Successfully registered MCP server!\n")
		cmd.Printf("Client:     %s\n", client.Name)
		cmd.Printf("Config File: %s\n", configPath)
		cmd.Printf("Binary Path: %s\n\n", execPath)
		registered++
	}

	if registered == 0 && cfg.Client == "" {
		cmd.Println("[rowlite] No installed AI client configurations found to update.")
	}

	return nil
}

func upsertMCPConfig(configPath, execPath string, useZedFormat, useCrushFormat bool, topLevelKey string, includeToolsWildcard bool) error {
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	var root map[string]interface{}
	content, err := os.ReadFile(configPath)
	if err == nil && len(content) > 0 {
		if err := json.Unmarshal(content, &root); err != nil {
			return fmt.Errorf("failed to parse existing config JSON: %w", err)
		}
	}
	if root == nil {
		root = make(map[string]interface{})
	}

	key := "mcpServers"
	if topLevelKey != "" {
		key = topLevelKey
	} else if useCrushFormat {
		key = "mcp"
	}

	servers, ok := root[key].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
		root[key] = servers
	}

	var node interface{}
	if useZedFormat {
		zc := ZedServerConfig{}
		zc.Command.Path = execPath
		zc.Command.Args = []string{"mcp"}
		node = zc
	} else if useCrushFormat {
		cc := CrushServerConfig{
			Type:    "stdio",
			Command: execPath,
			Args:    []string{"mcp"},
		}
		node = cc
	} else {
		sc := StandardServerConfig{
			Command: execPath,
			Args:    []string{"mcp"},
		}
		if includeToolsWildcard {
			sc.Tools = []string{"*"}
		}
		node = sc
	}

	nodeBytes, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal server node: %w", err)
	}
	var nodeMap interface{}
	if err := json.Unmarshal(nodeBytes, &nodeMap); err != nil {
		return fmt.Errorf("failed to unmarshal server node: %w", err)
	}

	servers["rowlite"] = nodeMap

	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to format updated JSON: %w", err)
	}
	output = append(output, '\n')

	tmpFile := configPath + ".tmp"
	if err := os.WriteFile(tmpFile, output, 0600); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	return os.Rename(tmpFile, configPath)
}
