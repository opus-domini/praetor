package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractFirstUserText(t *testing.T) {
	t.Parallel()

	got := extractFirstUserText([]any{
		map[string]any{"type": "tool_result", "content": "ignored"},
		map[string]any{"type": "text", "text": "  hello world  "},
	})
	if got != "hello world" {
		t.Fatalf("unexpected text: %q", got)
	}
}

func TestListSessionsReadsAndSortsMetadata(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	project := filepath.Join(projectsDir, "-tmp-repo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	sessionA := filepath.Join(project, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa.jsonl")
	contentA := "" +
		`{"type":"user","cwd":"/tmp/repo","gitBranch":"main","message":{"role":"user","content":[{"type":"text","text":"Very long first prompt used to validate fallback summary when no explicit summary line exists"}]}}` + "\n"
	if err := os.WriteFile(sessionA, []byte(contentA), 0o644); err != nil {
		t.Fatalf("write session A: %v", err)
	}

	sessionB := filepath.Join(project, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb.jsonl")
	contentB := "" +
		`{"type":"user","cwd":"/tmp/repo","gitBranch":"feature/x","message":{"role":"user","content":"Initial message"}}` + "\n" +
		`{"type":"summary","summary":"Custom Summary From Session"}` + "\n"
	if err := os.WriteFile(sessionB, []byte(contentB), 0o644); err != nil {
		t.Fatalf("write session B: %v", err)
	}

	now := time.Now()
	if err := os.Chtimes(sessionA, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("chtimes session A: %v", err)
	}
	if err := os.Chtimes(sessionB, now.Add(-1*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("chtimes session B: %v", err)
	}

	sessions, err := ListSessions(&ListSessionsOptions{ProjectsDir: projectsDir})
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	if sessions[0].SessionID != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Fatalf("expected newest session first, got %s", sessions[0].SessionID)
	}
	if sessions[0].Summary != "Custom Summary From Session" {
		t.Fatalf("expected summary from summary line, got %q", sessions[0].Summary)
	}

	if sessions[1].Summary == "" {
		t.Fatalf("expected fallback summary from first prompt")
	}
	if sessions[1].FirstPrompt == "" {
		t.Fatalf("expected first prompt to be extracted")
	}
	if sessions[1].GitBranch != "main" {
		t.Fatalf("unexpected git branch: %q", sessions[1].GitBranch)
	}
	if sessions[1].CWD != "/tmp/repo" {
		t.Fatalf("unexpected cwd: %q", sessions[1].CWD)
	}
}

func TestListSessionsLimit(t *testing.T) {
	t.Parallel()

	projectsDir := t.TempDir()
	project := filepath.Join(projectsDir, "-tmp-repo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}

	for i := 0; i < 3; i++ {
		file := filepath.Join(project, fmt.Sprintf("00000000-0000-0000-0000-00000000000%d.jsonl", i))
		if err := os.WriteFile(file, []byte(`{"type":"summary","summary":"s"}`+"\n"), 0o644); err != nil {
			t.Fatalf("write session %d: %v", i, err)
		}
	}

	sessions, err := ListSessions(&ListSessionsOptions{
		ProjectsDir: projectsDir,
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("ListSessions returned error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions with limit, got %d", len(sessions))
	}
}
