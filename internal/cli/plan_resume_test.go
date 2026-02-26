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
	t.Parallel()

	repo := t.TempDir()
	gitCmd(t, repo, "init")

	planPath := filepath.Join(repo, "plan.json")
	plan := domain.Plan{Tasks: []domain.Task{{ID: "TASK-001", Title: "Task 1", Executor: domain.AgentCodex, Reviewer: domain.AgentNone}}}
	writePlanJSON(t, planPath, plan)

	checksum, err := domain.PlanChecksum(planPath)
	if err != nil {
		t.Fatalf("plan checksum: %v", err)
	}
	state := domain.State{
		PlanFile:     planPath,
		PlanChecksum: checksum,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339),
		Tasks:        domain.StateTasksFromPlan(plan),
	}
	state.Tasks[0].Status = domain.TaskDone

	snapshots := localstate.NewLocalSnapshotStore(repo, "run-1")
	if err := snapshots.Init(planPath, checksum); err != nil {
		t.Fatalf("init snapshot store: %v", err)
	}
	if err := snapshots.Save(localstate.LocalSnapshot{
		RunID:        "run-1",
		PlanFile:     planPath,
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

	stateRoot := filepath.Join(repo, "state")
	root := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "resume", planPath, "--state-root", stateRoot})

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

	store := localstate.NewStore(stateRoot, "")
	restored, err := store.ReadState(planPath)
	if err != nil {
		t.Fatalf("read restored state: %v", err)
	}
	if restored.DoneCount() != 1 {
		t.Fatalf("expected restored done count 1, got %d", restored.DoneCount())
	}
}

func writePlanJSON(t *testing.T, path string, plan domain.Plan) {
	t.Helper()
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
