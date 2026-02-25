package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opus-domini/praetor/internal/providers/claude"
	"github.com/opus-domini/praetor/internal/providers/codex"
)

// SDKAgentRuntime executes prompts using the in-repo Claude and Codex SDK ports.
type SDKAgentRuntime struct{}

// Run executes one role prompt on one agent runtime.
func (r SDKAgentRuntime) Run(ctx context.Context, req AgentRequest) (string, error) {
	switch normalizeAgent(req.Agent) {
	case AgentCodex:
		return runCodex(ctx, req)
	case AgentClaude:
		return runClaude(ctx, req)
	default:
		return "", fmt.Errorf("unsupported agent %q", req.Agent)
	}
}

func runCodex(ctx context.Context, req AgentRequest) (string, error) {
	options := codex.CodexOptions{}
	if strings.TrimSpace(req.CodexBin) != "" {
		options.CodexPathOverride = strings.TrimSpace(req.CodexBin)
	}

	client, err := codex.New(options)
	if err != nil {
		return "", err
	}

	threadOptions := &codex.ThreadOptions{
		WorkingDirectory: strings.TrimSpace(req.Workdir),
		SandboxMode:      codex.SandboxModeWorkspaceWrite,
		SkipGitRepoCheck: true,
		ApprovalPolicy:   codex.ApprovalModeNever,
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		threadOptions.Model = model
	}

	prompt := strings.TrimSpace(req.Prompt)
	if systemPrompt := strings.TrimSpace(req.SystemPrompt); systemPrompt != "" {
		prompt = systemPrompt + "\n\n" + prompt
	}

	turn, err := client.StartThread(threadOptions).Run(ctx, prompt, nil)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(turn.FinalResponse), nil
}

func runClaude(ctx context.Context, req AgentRequest) (string, error) {
	persistSession := false
	options := claude.Options{
		Command:                         strings.TrimSpace(req.ClaudeBin),
		CWD:                             strings.TrimSpace(req.Workdir),
		Model:                           strings.TrimSpace(req.Model),
		AllowDangerouslySkipPermissions: true,
		PersistSession:                  &persistSession,
		AppendSystemPrompt:              strings.TrimSpace(req.SystemPrompt),
		PermissionMode:                  claude.PermissionModeBypassPermissions,
	}

	maxTurns := 25
	if req.Role == "review" {
		maxTurns = 10
	}
	options.MaxTurns = &maxTurns

	message, err := claude.Prompt(ctx, strings.TrimSpace(req.Prompt), options)
	if err != nil {
		return "", err
	}

	decoded, err := decodeClaudeResponse(message)
	if err != nil {
		return "", err
	}
	return decoded, nil
}

func decodeClaudeResponse(message claude.SDKMessage) (string, error) {
	result, err := claude.ParseResultMessage(message)
	if err != nil {
		trimmed := strings.TrimSpace(string(message.Raw))
		if trimmed == "" {
			return "", nil
		}
		return trimmed, nil
	}

	if result.IsError {
		reason := strings.TrimSpace(result.Result)
		if reason == "" {
			reason = "claude returned an error result"
		}
		return "", errors.New(reason)
	}

	if trimmed := strings.TrimSpace(result.Result); trimmed != "" {
		return trimmed, nil
	}
	return strings.TrimSpace(string(message.Raw)), nil
}
