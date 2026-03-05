package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SupportedAgents lists agent config directories that can receive command symlinks.
var SupportedAgents = []string{"claude", "cursor", "codex"}

const commandPrefix = "praetor-"

// Command describes a generated agent command.
type Command struct {
	Name    string
	Content string
}

type commandDefinition struct {
	BaseName string
	Content  string
}

var defaultCommandDefinitions = []commandDefinition{
	{BaseName: "plan-create", Content: planCreateContent},
	{BaseName: "plan-run", Content: planRunContent},
	{BaseName: "review-task", Content: reviewTaskContent},
	{BaseName: "doctor", Content: doctorContent},
	{BaseName: "diagnose", Content: diagnoseContent},
}

// DefaultCommands returns the built-in command set.
func DefaultCommands() []Command {
	commands := make([]Command, 0, len(defaultCommandDefinitions))
	for _, def := range defaultCommandDefinitions {
		commands = append(commands, Command{
			Name:    commandPrefix + def.BaseName,
			Content: def.Content,
		})
	}
	return commands
}

// Sync writes commands to .agents/commands/ and creates symlinks for each agent.
func Sync(projectRoot string, agents []string) error {
	if len(agents) == 0 {
		agents = SupportedAgents
	}

	commandsDir := filepath.Join(projectRoot, ".agents", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		return fmt.Errorf("create commands directory: %w", err)
	}

	for _, cmd := range DefaultCommands() {
		path := filepath.Join(commandsDir, cmd.Name+".md")
		if err := os.WriteFile(path, []byte(cmd.Content), 0o644); err != nil {
			return fmt.Errorf("write command %s: %w", cmd.Name, err)
		}
	}

	for _, agent := range agents {
		agentDir := filepath.Join(projectRoot, "."+agent, "commands")
		if err := os.MkdirAll(filepath.Dir(agentDir), 0o755); err != nil {
			return fmt.Errorf("create agent directory for %s: %w", agent, err)
		}

		// Remove existing symlink or directory.
		if info, err := os.Lstat(agentDir); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				_ = os.Remove(agentDir)
			}
		}

		// Create relative symlink: .claude/commands -> ../.agents/commands
		relPath, err := filepath.Rel(filepath.Dir(agentDir), commandsDir)
		if err != nil {
			return fmt.Errorf("compute relative path for %s: %w", agent, err)
		}
		if err := os.Symlink(relPath, agentDir); err != nil {
			if !os.IsExist(err) {
				return fmt.Errorf("create symlink for %s: %w", agent, err)
			}
		}
	}

	return nil
}

// List returns all command names found in .agents/commands/.
func List(projectRoot string) ([]string, error) {
	commandsDir := filepath.Join(projectRoot, ".agents", "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".md") {
			names = append(names, strings.TrimSuffix(name, ".md"))
		}
	}
	return names, nil
}
