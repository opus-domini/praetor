package loop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opus-domini/praetor/internal/providers/claude"
	"github.com/opus-domini/praetor/internal/providers/codex"
)

// SDKAgentRuntime executes prompts using the in-repo Claude and Codex SDK ports.
type SDKAgentRuntime struct{}

// Run executes one role prompt on one agent runtime.
func (r SDKAgentRuntime) Run(ctx context.Context, req AgentRequest) (AgentResult, error) {
	start := time.Now()
	switch normalizeAgent(req.Agent) {
	case AgentCodex:
		output, err := runCodex(ctx, req)
		return AgentResult{
			Output:    output,
			DurationS: time.Since(start).Seconds(),
		}, err
	case AgentClaude:
		output, costUSD, err := runClaude(ctx, req)
		return AgentResult{
			Output:    output,
			CostUSD:   costUSD,
			DurationS: time.Since(start).Seconds(),
		}, err
	default:
		return AgentResult{}, fmt.Errorf("unsupported agent %q", req.Agent)
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

func runClaude(ctx context.Context, req AgentRequest) (string, float64, error) {
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
		return "", 0, err
	}

	decoded, costUSD, err := decodeClaudeResponse(message)
	if err != nil {
		return "", 0, err
	}
	return decoded, costUSD, nil
}

func decodeClaudeResponse(message claude.SDKMessage) (string, float64, error) {
	result, err := claude.ParseResultMessage(message)
	if err != nil {
		trimmed := strings.TrimSpace(string(message.Raw))
		if trimmed == "" {
			return "", 0, nil
		}
		return trimmed, 0, nil
	}

	if result.IsError {
		reason := strings.TrimSpace(result.Result)
		if reason == "" {
			reason = "claude returned an error result"
		}
		return "", 0, errors.New(reason)
	}

	if trimmed := strings.TrimSpace(result.Result); trimmed != "" {
		return trimmed, result.TotalCostUSD, nil
	}
	return strings.TrimSpace(string(message.Raw)), result.TotalCostUSD, nil
}
