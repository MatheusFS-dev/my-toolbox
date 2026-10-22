package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type credentialGitRunner struct {
	helper string
}

func (runner credentialGitRunner) Run(_ context.Context, _ string, _ []string, arguments ...string) (string, error) {
	if strings.Join(arguments, " ") == "config --get credential.helper" {
		return runner.helper + "\n", nil
	}
	return "", nil
}

type recordingGitRunner struct {
	calls [][]string
}

func (runner *recordingGitRunner) Run(_ context.Context, _ string, _ []string, arguments ...string) (string, error) {
	runner.calls = append(runner.calls, append([]string(nil), arguments...))
	if len(arguments) > 0 && arguments[0] == "rm" {
		return "", errors.New("planned snapshot failure")
	}
	return "", nil
}

func TestDiscoverGitRepositoryFromSubdirectory(t *testing.T) {
	repository := newGitSyncRepository(t)
	nested := filepath.Join(repository, "chapters", "one")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	discovered, err := DiscoverGitRepository(context.Background(), ExecGitRunner{}, nested)
	if err != nil {
		t.Fatal(err)
	}
	if discovered.Root != repository {
		t.Fatalf("root = %q, want %q", discovered.Root, repository)
	}
	if len(discovered.Remotes) != 2 || discovered.Remotes[0].Name != "backup" || discovered.Remotes[1].Name != "origin" {
		t.Fatalf("remotes = %#v", discovered.Remotes)
	}
}

func TestDiscoverGitRepositoryRejectsDirtyTree(t *testing.T) {
	repository := newGitSyncRepository(t)
	if err := os.WriteFile(filepath.Join(repository, "untracked.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := DiscoverGitRepository(context.Background(), ExecGitRunner{}, repository)
	if err == nil || !strings.Contains(err.Error(), "working tree is not clean") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiscoverGitRepositoryRedactsEmbeddedHTTPSCredentials(t *testing.T) {
	repository := newGitSyncRepository(t)
	runGit(t, repository, "remote", "set-url", "origin", "https://user:secret@example.com/team/project.git")

	discovered, err := DiscoverGitRepository(context.Background(), ExecGitRunner{}, repository)
	if err != nil {
		t.Fatal(err)
	}
	for _, remote := range discovered.Remotes {
		if strings.Contains(remote.FetchURL, "secret") || strings.Contains(remote.PushURL, "secret") {
			t.Fatalf("remote exposes embedded credential: %#v", remote)
		}
	}
}

func TestExecuteGitSyncPreservesDestinationHistory(t *testing.T) {
	repository := newGitSyncRepository(t)
	runGit(t, repository, "checkout", "-b", "source")
	if err := os.WriteFile(filepath.Join(repository, "paper.tex"), []byte("source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "paper.tex")
	runGit(t, repository, "commit", "-m", "source content")
	runGit(t, repository, "push", "origin", "HEAD:source")
	runGit(t, repository, "checkout", "main")
	runGit(t, repository, "push", "backup", "HEAD:main")

	discovered, err := DiscoverGitRepository(context.Background(), ExecGitRunner{}, repository)
	if err != nil {
		t.Fatal(err)
	}
	branchesOrigin, err := FetchRemoteBranches(context.Background(), ExecGitRunner{}, discovered, discovered.Remotes[1])
	if err != nil {
		t.Fatal(err)
	}
	branchesBackup, err := FetchRemoteBranches(context.Background(), ExecGitRunner{}, discovered, discovered.Remotes[0])
	if err != nil {
		t.Fatal(err)
	}
	selection := GitSyncSelection{
		SourceRemote:      discovered.Remotes[1],
		SourceBranch:      findGitBranch(t, branchesOrigin, "source"),
		DestinationRemote: discovered.Remotes[0],
		DestinationBranch: findGitBranch(t, branchesBackup, "main"),
		CommitMessage:     "Sync origin/source to backup/main",
	}
	plan, err := PlanGitSync(context.Background(), ExecGitRunner{}, discovered, selection)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changes.Modified != 1 || plan.Changes.Added != 0 || plan.Changes.Deleted != 0 {
		t.Fatalf("changes = %#v", plan.Changes)
	}
	result, err := ExecuteGitSync(context.Background(), ExecGitRunner{}, plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.NoChanges || result.Commit == "" {
		t.Fatalf("result = %#v", result)
	}
	parent := strings.TrimSpace(runGit(t, repository, "--git-dir="+discovered.Remotes[0].FetchURL, "rev-parse", "main^"))
	if parent != selection.DestinationBranch.Commit {
		t.Fatalf("destination parent = %s, want %s", parent, selection.DestinationBranch.Commit)
	}
	content := runGit(t, repository, "--git-dir="+discovered.Remotes[0].FetchURL, "show", "main:paper.tex")
	if content != "source\n" {
		t.Fatalf("destination content = %q", content)
	}
}

func TestRunGitSyncCancelsBeforeMutation(t *testing.T) {
	repository := newGitSyncRepository(t)
	runGit(t, repository, "push", "backup", "HEAD:main")
	if err := os.WriteFile(filepath.Join(repository, "paper.tex"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "paper.tex")
	runGit(t, repository, "commit", "-m", "change source")
	runGit(t, repository, "push", "origin", "HEAD:main")
	input := strings.NewReader("2\n1\n1\n1\n\nNo\n")
	output := &bytes.Buffer{}
	err := RunGitSync(context.Background(), ExecGitRunner{}, repository, "linux-amd64", input, output, output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Synchronization cancelled") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestSecureCredentialModeRejectsNonSecureCacheHelper(t *testing.T) {
	repository := GitRepository{Root: t.TempDir()}
	remote := GitRemote{Name: "origin", PushURL: "https://example.com/team/project.git"}
	reader := bufio.NewReader(strings.NewReader("1\n"))
	output := &bytes.Buffer{}

	_, err := chooseCredentialMode(context.Background(), credentialGitRunner{helper: "cache"}, repository, remote, reader, output)
	if err == nil || !strings.Contains(err.Error(), "no secure Git credential manager") {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteGitSyncCleanupNeverUsesForceOption(t *testing.T) {
	runner := &recordingGitRunner{}
	plan := GitSyncPlan{
		Repository: GitRepository{Root: t.TempDir()},
		Selection: GitSyncSelection{
			SourceBranch:      GitBranch{Commit: "source"},
			DestinationBranch: GitBranch{Commit: "destination"},
		},
		Changes: GitSyncChanges{Modified: 1},
	}

	_, err := ExecuteGitSync(context.Background(), runner, plan)
	if err == nil {
		t.Fatal("ExecuteGitSync() succeeded, want planned failure")
	}
	for _, call := range runner.calls {
		for _, argument := range call {
			if argument == "--force" || argument == "--force-with-lease" {
				t.Fatalf("cleanup used prohibited argument in git %v", call)
			}
		}
	}
}

func newGitSyncRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "work")
	origin := filepath.Join(root, "origin.git")
	backup := filepath.Join(root, "backup.git")
	runGit(t, root, "init", "--bare", "--initial-branch=main", origin)
	runGit(t, root, "init", "--bare", "--initial-branch=main", backup)
	runGit(t, root, "init", "--initial-branch=main", repository)
	runGit(t, repository, "config", "user.name", "Git Sync Test")
	runGit(t, repository, "config", "user.email", "git-sync@example.invalid")
	if err := os.WriteFile(filepath.Join(repository, "paper.tex"), []byte("destination\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repository, "add", "paper.tex")
	runGit(t, repository, "commit", "-m", "initial")
	runGit(t, repository, "remote", "add", "origin", origin)
	runGit(t, repository, "remote", "add", "backup", backup)
	return repository
}

func runGit(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}

func findGitBranch(t *testing.T, branches []GitBranch, name string) GitBranch {
	t.Helper()
	for _, branch := range branches {
		if branch.Name == name {
			return branch
		}
	}
	t.Fatalf("branch %q not found in %#v", name, branches)
	return GitBranch{}
}
