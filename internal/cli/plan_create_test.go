package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opus-domini/praetor/internal/domain"
	localstate "github.com/opus-domini/praetor/internal/state"
)

func TestPlanCreateNoAgentDryRunDoesNotWriteFile(t *testing.T) {
	repo, store := setupPlanCreateTestRepo(t)
	t.Chdir(repo)

	root := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "create", "Implement auth flow", "--no-agent", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr=%s)", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), `"name":`) {
		t.Fatalf("expected valid JSON in dry-run output, got: %s", stdout.String())
	}
	entries, err := os.ReadDir(store.PlansDir())
	if err != nil {
		t.Fatalf("read plans dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no persisted plans on dry-run, found %d", len(entries))
	}
}

func TestPlanCreateNoAgentFromFileAndStdin(t *testing.T) {
	repo, store := setupPlanCreateTestRepo(t)
	t.Chdir(repo)

	briefPath := filepath.Join(repo, "brief.md")
	if err := os.WriteFile(briefPath, []byte("# Implementar login seguro\n\nDetalhes..."), 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"plan", "create", "--from-file", briefPath, "--no-agent", "--slug", "login-seguro"})
	if err := root.Execute(); err != nil {
		t.Fatalf("create from file: %v", err)
	}

	plan, err := domain.LoadPlan(store.PlanFile("login-seguro"))
	if err != nil {
		t.Fatalf("load created plan: %v", err)
	}
	if plan.Name == "" {
		t.Fatal("expected plan name to be populated")
	}

	root2 := NewRootCmd()
	root2.SetIn(strings.NewReader("Implementar endpoint de refresh token"))
	root2.SetOut(&bytes.Buffer{})
	root2.SetErr(&bytes.Buffer{})
	root2.SetArgs([]string{"plan", "create", "--stdin", "--no-agent", "--slug", "refresh-token"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("create from stdin: %v", err)
	}

	if _, err := os.Stat(store.PlanFile("refresh-token")); err != nil {
		t.Fatalf("expected plan file from stdin, err=%v", err)
	}
}

func TestPlanCreateInteractiveBriefWithoutArgs(t *testing.T) {
	repo, store := setupPlanCreateTestRepo(t)
	t.Chdir(repo)

	root := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	root.SetIn(strings.NewReader("Implementar fluxo de autenticação com testes\n"))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "create", "--no-agent", "--slug", "interactive-brief"})

	if err := root.Execute(); err != nil {
		t.Fatalf("interactive create: %v (stderr=%s)", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Type or paste the plan brief below.") {
		t.Fatalf("expected interactive prompt in stdout, got: %s", stdout.String())
	}
	if _, err := os.Stat(store.PlanFile("interactive-brief")); err != nil {
		t.Fatalf("expected plan file from interactive brief, err=%v", err)
	}
}

func TestResolvePlanCreateBriefInteractiveStopsOnDoubleBlankLine(t *testing.T) {
	var prompt bytes.Buffer
	brief, err := resolvePlanCreateBrief(
		nil,
		"",
		false,
		strings.NewReader("Linha 1\n\nLinha 2\n\n\nConteudo ignorado\n"),
		&prompt,
	)
	if err != nil {
		t.Fatalf("resolve interactive brief: %v", err)
	}
	if brief != "Linha 1\n\nLinha 2" {
		t.Fatalf("unexpected interactive brief: %q", brief)
	}
	if !strings.Contains(prompt.String(), "Type or paste the plan brief below.") {
		t.Fatalf("expected interactive prompt, got: %s", prompt.String())
	}
}

func TestPlanCreateRejectsMultipleSources(t *testing.T) {
	repo, _ := setupPlanCreateTestRepo(t)
	t.Chdir(repo)

	briefPath := filepath.Join(repo, "brief.md")
	if err := os.WriteFile(briefPath, []byte("brief"), 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	root := NewRootCmd()
	var stderr bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	root.SetArgs([]string{"plan", "create", "arg brief", "--from-file", briefPath, "--no-agent"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected multiple-source error")
	}
}

func TestPlanCreateAutoSlugCollisionAddsSuffix(t *testing.T) {
	repo, store := setupPlanCreateTestRepo(t)
	t.Chdir(repo)

	cmd1 := NewRootCmd()
	cmd1.SetOut(&bytes.Buffer{})
	cmd1.SetErr(&bytes.Buffer{})
	cmd1.SetArgs([]string{"plan", "create", "Implementar autenticação", "--no-agent"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("first create: %v", err)
	}

	cmd2 := NewRootCmd()
	cmd2.SetOut(&bytes.Buffer{})
	cmd2.SetErr(&bytes.Buffer{})
	cmd2.SetArgs([]string{"plan", "create", "Implementar autenticação", "--no-agent"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second create: %v", err)
	}

	if _, err := os.Stat(store.PlanFile("implementar-autenticacao")); err != nil {
		t.Fatalf("expected first slug file, err=%v", err)
	}
	if _, err := os.Stat(store.PlanFile("implementar-autenticacao-2")); err != nil {
		t.Fatalf("expected collision suffix file, err=%v", err)
	}
}

func setupPlanCreateTestRepo(t *testing.T) (string, *localstate.Store) {
	t.Helper()

	praetorHome := t.TempDir()
	t.Setenv("PRAETOR_HOME", praetorHome)

	repo := t.TempDir()
	gitCmd := exec.Command("git", "-C", repo, "init")
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	projectHome, err := localstate.ResolveProjectHome("", repo)
	if err != nil {
		t.Fatalf("resolve project home: %v", err)
	}
	store := localstate.NewStore(projectHome)
	if err := store.Init(); err != nil {
		t.Fatalf("init store: %v", err)
	}
	return repo, store
}
