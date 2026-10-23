package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GitRunner executes Git without a shell.
type GitRunner interface {
	Run(ctx context.Context, directory string, environment []string, arguments ...string) (string, error)
}

// ExecGitRunner executes the system Git command with literal argument arrays.
type ExecGitRunner struct {
	Input io.Reader
}

// Run executes one Git process.
//
// Args:
//   - ctx: Cancellation context.
//   - directory: Process working directory.
//   - environment: Additional environment entries.
//   - arguments: Literal Git arguments.
//
// Returns:
//   - string: Combined standard output and standard error.
//   - error: Command lookup or execution failure.
func (runner ExecGitRunner) Run(ctx context.Context, directory string, environment []string, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	command.Stdin = runner.Input
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return string(output), fmt.Errorf("git %s: %s", strings.Join(arguments, " "), message)
	}
	return string(output), nil
}

// GitRepository describes the caller's worktree and configured remotes.
type GitRepository struct {
	Root           string
	OriginalRef    string
	OriginalCommit string
	Remotes        []GitRemote
}

// GitRemote describes one configured remote.
type GitRemote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// GitBranch describes one fetched remote branch.
type GitBranch struct {
	Name    string
	Commit  string
	Default bool
}

// GitSyncSelection contains the chosen synchronization endpoints.
type GitSyncSelection struct {
	SourceRemote      GitRemote
	SourceBranch      GitBranch
	DestinationRemote GitRemote
	DestinationBranch GitBranch
	CommitMessage     string
	Environment       []string
}

// GitSyncChanges counts tracked snapshot changes.
type GitSyncChanges struct {
	Added    int
	Modified int
	Deleted  int
}

// GitSyncPlan is a validated immutable synchronization operation.
type GitSyncPlan struct {
	Repository GitRepository
	Selection  GitSyncSelection
	Changes    GitSyncChanges
}

// GitSyncResult reports the created commit or a no-op.
type GitSyncResult struct {
	Commit    string
	Changes   GitSyncChanges
	NoChanges bool
}

// DiscoverGitRepository resolves and validates a Git worktree.
//
// Args:
//   - ctx: Cancellation context.
//   - runner: Git process boundary.
//   - directory: Current directory or repository subdirectory.
//
// Returns:
//   - GitRepository: Clean repository and sorted remotes.
//   - error: Discovery, state, or remote validation failure.
func DiscoverGitRepository(ctx context.Context, runner GitRunner, directory string) (GitRepository, error) {
	rootOutput, err := runner.Run(ctx, directory, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return GitRepository{}, fmt.Errorf("no Git working tree was found: %w", err)
	}
	root := strings.TrimSpace(rootOutput)
	status, err := runner.Run(ctx, root, nil, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return GitRepository{}, fmt.Errorf("inspect working tree: %w", err)
	}
	if status != "" {
		return GitRepository{}, fmt.Errorf("working tree is not clean; commit, stash, or remove local changes before synchronizing")
	}
	commit, err := runner.Run(ctx, root, nil, "rev-parse", "HEAD")
	if err != nil {
		return GitRepository{}, fmt.Errorf("resolve current commit: %w", err)
	}
	ref, _ := runner.Run(ctx, root, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	remoteOutput, err := runner.Run(ctx, root, nil, "remote")
	if err != nil {
		return GitRepository{}, fmt.Errorf("list remotes: %w", err)
	}
	names := nonemptyLines(remoteOutput)
	if len(names) < 2 {
		return GitRepository{}, fmt.Errorf("at least two Git remotes are required; found %d", len(names))
	}
	sort.Strings(names)
	remotes := make([]GitRemote, 0, len(names))
	for _, name := range names {
		fetchURL, fetchErr := runner.Run(ctx, root, nil, "remote", "get-url", name)
		if fetchErr != nil {
			return GitRepository{}, fmt.Errorf("read fetch URL for remote %s: %w", name, fetchErr)
		}
		pushURL, pushErr := runner.Run(ctx, root, nil, "remote", "get-url", "--push", name)
		if pushErr != nil {
			return GitRepository{}, fmt.Errorf("read push URL for remote %s: %w", name, pushErr)
		}
		remotes = append(remotes, GitRemote{Name: name, FetchURL: sanitizedRemoteURL(fetchURL), PushURL: sanitizedRemoteURL(pushURL)})
	}
	return GitRepository{Root: root, OriginalRef: strings.TrimSpace(ref), OriginalCommit: strings.TrimSpace(commit), Remotes: remotes}, nil
}

// FetchRemoteBranches fetches and enumerates one remote's branches.
//
// Args:
//   - ctx: Cancellation context.
//   - runner: Git process boundary.
//   - repository: Validated repository.
//   - remote: Remote to fetch.
//
// Returns:
//   - []GitBranch: Sorted remote branches with an optional default marker.
//   - error: Fetch or branch discovery failure.
func FetchRemoteBranches(ctx context.Context, runner GitRunner, repository GitRepository, remote GitRemote) ([]GitBranch, error) {
	return fetchRemoteBranches(ctx, runner, repository, remote, nil)
}

func fetchRemoteBranches(ctx context.Context, runner GitRunner, repository GitRepository, remote GitRemote, environment []string) ([]GitBranch, error) {
	refspec := "+refs/heads/*:refs/remotes/" + remote.Name + "/*"
	if _, err := runner.Run(ctx, repository.Root, environment, "fetch", "--prune", remote.Name, refspec); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", remote.Name, err)
	}
	prefix := "refs/remotes/" + remote.Name + "/"
	output, err := runner.Run(ctx, repository.Root, nil, "for-each-ref", "--format=%(refname) %(objectname)", prefix)
	if err != nil {
		return nil, fmt.Errorf("list branches for %s: %w", remote.Name, err)
	}
	defaultName := ""
	if symbolic, symbolicErr := runner.Run(ctx, repository.Root, nil, "symbolic-ref", "--quiet", prefix+"HEAD"); symbolicErr == nil {
		defaultName = strings.TrimPrefix(strings.TrimSpace(symbolic), prefix)
	}
	branches := []GitBranch{}
	for _, line := range nonemptyLines(output) {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		name := strings.TrimPrefix(fields[0], prefix)
		if name == "HEAD" {
			continue
		}
		branches = append(branches, GitBranch{Name: name, Commit: fields[1], Default: name == defaultName})
	}
	sort.Slice(branches, func(left, right int) bool {
		if branches[left].Default != branches[right].Default {
			return branches[left].Default
		}
		return branches[left].Name < branches[right].Name
	})
	if len(branches) == 0 {
		return nil, fmt.Errorf("remote %s has no branches", remote.Name)
	}
	return branches, nil
}

// PlanGitSync compares the selected remote snapshots.
//
// Args:
//   - ctx: Cancellation context.
//   - runner: Git process boundary.
//   - repository: Validated repository.
//   - selection: Source, destination, and commit message.
//
// Returns:
//   - GitSyncPlan: Validated plan and change counts.
//   - error: Invalid selection or comparison failure.
func PlanGitSync(ctx context.Context, runner GitRunner, repository GitRepository, selection GitSyncSelection) (GitSyncPlan, error) {
	if selection.SourceRemote.Name == selection.DestinationRemote.Name && selection.SourceBranch.Name == selection.DestinationBranch.Name {
		return GitSyncPlan{}, fmt.Errorf("source and destination must differ")
	}
	if strings.TrimSpace(selection.CommitMessage) == "" {
		return GitSyncPlan{}, fmt.Errorf("commit message cannot be empty")
	}
	output, err := runner.Run(ctx, repository.Root, nil, "diff", "--name-status", "-z", selection.DestinationBranch.Commit, selection.SourceBranch.Commit)
	if err != nil {
		return GitSyncPlan{}, fmt.Errorf("compare remote snapshots: %w", err)
	}
	changes := parseGitChanges(output)
	return GitSyncPlan{Repository: repository, Selection: selection, Changes: changes}, nil
}

// ExecuteGitSync creates and pushes the synchronization commit.
//
// Args:
//   - ctx: Cancellation context.
//   - runner: Git process boundary.
//   - plan: Validated synchronization plan.
//
// Returns:
//   - GitSyncResult: Created commit and changes, or a no-op result.
//   - error: Worktree, commit, push, or cleanup failure.
func ExecuteGitSync(ctx context.Context, runner GitRunner, plan GitSyncPlan) (result GitSyncResult, returnErr error) {
	if plan.Changes == (GitSyncChanges{}) {
		return GitSyncResult{Changes: plan.Changes, NoChanges: true}, nil
	}
	temporary, err := os.MkdirTemp("", "tb-git-sync-*")
	if err != nil {
		return GitSyncResult{}, fmt.Errorf("create temporary worktree directory: %w", err)
	}
	_ = os.Remove(temporary)
	added := false
	defer func() {
		if added {
			_, cleanupErr := runner.Run(context.Background(), plan.Repository.Root, nil, "worktree", "remove", temporary)
			if cleanupErr != nil {
				if removeErr := os.RemoveAll(temporary); removeErr != nil {
					returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary worktree: %w", cleanupErr), fmt.Errorf("remove temporary directory: %w", removeErr))
					return
				}
				if _, pruneErr := runner.Run(context.Background(), plan.Repository.Root, nil, "worktree", "prune"); pruneErr != nil {
					returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary worktree: %w", cleanupErr), fmt.Errorf("prune temporary worktree registration: %w", pruneErr))
				}
			}
		}
		if cleanupErr := os.RemoveAll(temporary); cleanupErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary directory: %w", cleanupErr))
		}
	}()
	if _, err := runner.Run(ctx, plan.Repository.Root, nil, "worktree", "add", "--detach", temporary, plan.Selection.DestinationBranch.Commit); err != nil {
		return GitSyncResult{}, fmt.Errorf("create temporary worktree: %w", err)
	}
	added = true
	if _, err := runner.Run(ctx, temporary, nil, "rm", "-r", "--ignore-unmatch", "."); err != nil {
		return GitSyncResult{}, fmt.Errorf("clear destination snapshot: %w", err)
	}
	if _, err := runner.Run(ctx, temporary, nil, "restore", "--source="+plan.Selection.SourceBranch.Commit, "--staged", "--worktree", "--", ":/"); err != nil {
		return GitSyncResult{}, fmt.Errorf("restore source snapshot: %w", err)
	}
	if _, err := runner.Run(ctx, temporary, nil, "commit", "-m", plan.Selection.CommitMessage); err != nil {
		return GitSyncResult{}, fmt.Errorf("create synchronization commit: %w", err)
	}
	commitOutput, err := runner.Run(ctx, temporary, nil, "rev-parse", "HEAD")
	if err != nil {
		return GitSyncResult{}, fmt.Errorf("resolve synchronization commit: %w", err)
	}
	pushArguments := []string{"push", plan.Selection.DestinationRemote.Name, "HEAD:refs/heads/" + plan.Selection.DestinationBranch.Name}
	if _, err := runner.Run(ctx, temporary, plan.Selection.Environment, pushArguments...); err != nil {
		return GitSyncResult{}, fmt.Errorf("push destination failed; no force push was attempted: %w", err)
	}
	return GitSyncResult{Commit: strings.TrimSpace(commitOutput), Changes: plan.Changes}, nil
}

// RunGitSync runs the interactive synchronization workflow.
//
// Args:
//   - ctx: Cancellation context.
//   - runner: Git process boundary.
//   - directory: Invocation directory.
//   - platform: Toolbox platform identifier.
//   - input: Terminal input.
//   - output: Status and prompt output.
//   - errorOutput: Git diagnostic output destination.
//
// Returns:
//   - error: Discovery, input, authentication, or synchronization failure.
func RunGitSync(ctx context.Context, runner GitRunner, directory, platform string, input io.Reader, output, errorOutput io.Writer) error {
	_ = platform
	_ = errorOutput
	messages := newToolOutput(output, "git-sync", time.Now, detectToolColor(output))
	if err := messages.Info("Checking the repository and configured remotes..."); err != nil {
		return err
	}
	repository, err := DiscoverGitRepository(ctx, runner, directory)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(input)
	if err := messages.Info(fmt.Sprintf("Repository: %s", repository.Root)); err != nil {
		return err
	}
	source, err := selectRemote(reader, messages, "Select the source remote", repository.Remotes)
	if err != nil {
		return err
	}
	sourceEnvironment, err := chooseCredentialMode(ctx, runner, repository, source, reader, messages)
	if err != nil {
		return err
	}
	if err := messages.Info(fmt.Sprintf("Fetching branches from %s...", source.Name)); err != nil {
		return err
	}
	sourceBranches, err := fetchRemoteBranches(ctx, runner, repository, source, sourceEnvironment)
	if err != nil {
		return err
	}
	sourceBranch, err := selectBranch(reader, messages, "Select the source branch", sourceBranches)
	if err != nil {
		return err
	}
	destinations := make([]GitRemote, 0, len(repository.Remotes)-1)
	for _, remote := range repository.Remotes {
		if remote.Name != source.Name {
			destinations = append(destinations, remote)
		}
	}
	destination, err := selectRemote(reader, messages, "Select the destination remote", destinations)
	if err != nil {
		return err
	}
	environment, err := chooseCredentialMode(ctx, runner, repository, destination, reader, messages)
	if err != nil {
		return err
	}
	if err := messages.Info(fmt.Sprintf("Fetching branches from %s...", destination.Name)); err != nil {
		return err
	}
	destinationBranches, err := fetchRemoteBranches(ctx, runner, repository, destination, environment)
	if err != nil {
		return err
	}
	destinationBranch, err := selectBranch(reader, messages, "Select the destination branch", destinationBranches)
	if err != nil {
		return err
	}
	defaultMessage := fmt.Sprintf("Sync %s/%s to %s/%s", source.Name, sourceBranch.Name, destination.Name, destinationBranch.Name)
	if err := messages.Prompt(fmt.Sprintf("Commit message [default: %s]", defaultMessage)); err != nil {
		return err
	}
	message, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read commit message: %w", err)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = defaultMessage
	}
	selection := GitSyncSelection{SourceRemote: source, SourceBranch: sourceBranch, DestinationRemote: destination, DestinationBranch: destinationBranch, CommitMessage: message, Environment: environment}
	if err := messages.Info("Checking the selected branch snapshots..."); err != nil {
		return err
	}
	plan, err := PlanGitSync(ctx, runner, repository, selection)
	if err != nil {
		return err
	}
	if plan.Changes == (GitSyncChanges{}) {
		return messages.Warn("Skipping synchronization because the tracked snapshots are identical.")
	}
	overview := fmt.Sprintf("Source: %s/%s (%s)\nDestination: %s/%s (%s)\nChanges: %d added, %d modified, %d deleted\nThe destination history will be preserved. No force push will be used.", source.Name, sourceBranch.Name, source.FetchURL, destination.Name, destinationBranch.Name, destination.PushURL, plan.Changes.Added, plan.Changes.Modified, plan.Changes.Deleted)
	if err := messages.Info(overview); err != nil {
		return err
	}
	if err := messages.Prompt("Continue? [y/N] (default: n):"); err != nil {
		return err
	}
	confirmation, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read confirmation: %w", err)
	}
	if !isYesConfirmation(confirmation) {
		return messages.Info("Synchronization cancelled; no synchronization commit was created.")
	}
	if err := messages.Info("Synchronizing tracked files and pushing the destination branch..."); err != nil {
		return err
	}
	result, err := ExecuteGitSync(ctx, runner, plan)
	if err != nil {
		return err
	}
	return messages.Success(fmt.Sprintf("Synchronized tracked files in commit %s.", result.Commit))
}

func chooseCredentialMode(ctx context.Context, runner GitRunner, repository GitRepository, remote GitRemote, reader *bufio.Reader, output *toolOutput) ([]string, error) {
	parsed, err := url.Parse(remote.PushURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, nil
	}
	if err := output.Menu("How should Git remember credentials?", []string{"1. Secure credential manager", "2. Plaintext Git storage", "3. Do not save"}); err != nil {
		return nil, err
	}
	choice, err := readChoice(reader, output, 3)
	if err != nil {
		return nil, err
	}
	scope := "credential." + remote.PushURL + ".helper"
	switch choice {
	case 1:
		helper, helperErr := runner.Run(ctx, repository.Root, nil, "config", "--get", "credential.helper")
		if helperErr != nil || !secureCredentialHelper(helper) {
			return nil, fmt.Errorf("no secure Git credential manager is configured; configure Git Credential Manager or libsecret, then rerun")
		}
		return nil, nil
	case 2:
		if err := output.Warn("Git will store credentials unencrypted in the user credentials file."); err != nil {
			return nil, err
		}
		if err := output.Prompt("Continue with plaintext credential storage? [y/N] (default: n):"); err != nil {
			return nil, err
		}
		confirmation, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, readErr
		}
		if !isYesConfirmation(confirmation) {
			return nil, fmt.Errorf("plaintext credential storage was not confirmed")
		}
		if _, configErr := runner.Run(ctx, repository.Root, nil, "config", "--global", scope, "store"); configErr != nil {
			return nil, fmt.Errorf("configure plaintext credential storage: %w", configErr)
		}
		return nil, nil
	default:
		return []string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=credential.helper", "GIT_CONFIG_VALUE_0="}, nil
	}
}

func isYesConfirmation(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "y" || normalized == "yes"
}

func secureCredentialHelper(helper string) bool {
	normalized := strings.ToLower(strings.TrimSpace(helper))
	for _, marker := range []string{"manager", "manager-core", "libsecret", "osxkeychain", "wincred"} {
		if normalized == marker || strings.Contains(normalized, "git-credential-"+marker) {
			return true
		}
	}
	return false
}

func selectRemote(reader *bufio.Reader, output *toolOutput, title string, remotes []GitRemote) (GitRemote, error) {
	lines := []string{}
	for index, remote := range remotes {
		lines = append(lines, fmt.Sprintf("%d. %s (%s)", index+1, remote.Name, remote.FetchURL))
	}
	if err := output.Menu(title, lines); err != nil {
		return GitRemote{}, err
	}
	choice, err := readChoice(reader, output, len(remotes))
	if err != nil {
		return GitRemote{}, err
	}
	return remotes[choice-1], nil
}

func selectBranch(reader *bufio.Reader, output *toolOutput, title string, branches []GitBranch) (GitBranch, error) {
	lines := []string{}
	for index, branch := range branches {
		label := ""
		if branch.Default {
			label = " (default)"
		}
		lines = append(lines, fmt.Sprintf("%d. %s%s", index+1, branch.Name, label))
	}
	if err := output.Menu(title, lines); err != nil {
		return GitBranch{}, err
	}
	choice, err := readChoice(reader, output, len(branches))
	if err != nil {
		return GitBranch{}, err
	}
	return branches[choice-1], nil
}

func readChoice(reader *bufio.Reader, output *toolOutput, maximum int) (int, error) {
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return 0, err
		}
		choice, conversionErr := strconv.Atoi(strings.TrimSpace(line))
		if conversionErr == nil && choice >= 1 && choice <= maximum {
			return choice, nil
		}
		if errors.Is(err, io.EOF) {
			return 0, fmt.Errorf("input ended before a valid selection")
		}
		if err := output.Warn(fmt.Sprintf("A selection is required; enter a number from 1 to %d.", maximum)); err != nil {
			return 0, err
		}
	}
}

func parseGitChanges(output string) GitSyncChanges {
	fields := strings.Split(output, "\x00")
	changes := GitSyncChanges{}
	for index := 0; index < len(fields); index++ {
		status := fields[index]
		if status == "" {
			continue
		}
		switch status[0] {
		case 'A':
			changes.Added++
		case 'D':
			changes.Deleted++
		default:
			changes.Modified++
		}
		// Every status is followed by at least one path. Rename and copy
		// records carry a second path.
		index++
		if status[0] == 'R' || status[0] == 'C' {
			index++
		}
	}
	return changes
}

func nonemptyLines(output string) []string {
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func sanitizedRemoteURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.User == nil {
		return trimmed
	}
	parsed.User = nil
	return parsed.String()
}

func currentDirectory() string {
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return filepath.Clean(directory)
}
