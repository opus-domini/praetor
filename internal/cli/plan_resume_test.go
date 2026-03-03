package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

func TestPlanResumeRestoresLatestSnapshot(t *testing.T) {
	// Cannot use t.Parallel because we set PRAETOR_HOME and chdir.
	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitCmd(t, repo, "init")

	// Resolve project home the same way the CLI does.
	projectHome, err := localstate.ResolveProjectHome("", repo)
	if err != nil {
		t.Fatalf("resolve project home: %v", err)
	}
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}

	slug := "test-plan"
	plan := domain.Plan{
		Name: "test plan",
		Settings: domain.PlanSettings{
			Agents: domain.PlanAgents{
				Executor: domain.PlanAgentConfig{Agent: domain.AgentCodex},
				Reviewer: domain.PlanAgentConfig{Agent: domain.AgentNone},
			},
		},
		Tasks: []domain.Task{{ID: "TASK-001", Title: "Task 1", Acceptance: []string{"task completes"}}},
	}
	writePlanJSON(t, store.PlanFile(slug), plan)

	checksum, err := domain.PlanChecksum(store.PlanFile(slug))
	if err != nil {
		t.Fatalf("plan checksum: %v", err)
	}
	state := domain.State{
		PlanSlug:     slug,
		PlanChecksum: checksum,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		Tasks:        domain.StateTasksFromPlan(plan),
	}
	state.Tasks[0].Status = domain.TaskDone

	snapshots := localstate.NewLocalSnapshotStore(store.RuntimeDir(), "run-1")
	if err := snapshots.Init(slug, checksum); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := snapshots.Save(localstate.LocalSnapshot{
		RunID:        "run-1",
		PlanSlug:     slug,
		PlanChecksum: checksum,
		ProjectRoot:  repo,
		Phase:        "finalize",
		Message:      "completed",
		Iteration:    1,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		State:        state,
	}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	// The resume command calls resolveStore("."), which resolves from the CWD.
	// Chdir into the repo so the project home matches.
	t.Chdir(repo)

	root := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "resume", slug})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute resume command: %v (stderr=%s)", err, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "Resumed from:") {
		t.Fatalf("expected resume output, got: %s", out)
	}
	if !strings.Contains(out, "Progress: 1/1 done") {
		t.Fatalf("expected progress output, got: %s", out)
	}

	restored, err := store.ReadState(slug)
	if err != nil {
		t.Fatalf("read restored state: %v", err)
	}
	if restored.DoneCount() != 1 {
		t.Fatalf("expected restored done count 1, got %d", restored.DoneCount())
	}
}

func writePlanJSON(t *testing.T, path string, plan domain.Plan) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create plan dir: %v", err)
	}
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatalf("marshal plan: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
}
