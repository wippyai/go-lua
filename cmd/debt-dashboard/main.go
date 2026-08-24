// Command debt-dashboard reproduces the debt-dashboard measurement
// (journal finding cdea2897) as a repo command, so every wave boundary is
// measured the same way instead of by hand-run shell one-liners.
//
// It clones the repository --shared into a cache directory, resets that
// clone to each requested commit in turn, measures it, and prints the
// resulting table with deltas from the first commit to the last. It runs
// no build and no tests - counting only, bounded by clone size.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/wippyai/go-lua/internal/debtdash/render"
)

const cloneDirName = "debt-dashboard-clone"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "debt-dashboard:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("debt-dashboard", flag.ContinueOnError)
	commits := fs.String("commits", "", "comma-separated list of commit-ish revisions to measure, oldest first")
	gateOnly := fs.Bool("gate", false, "print only the gate verdict for the last commit")
	repoRoot := fs.String("repo", ".", "path to the git repository to measure")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*commits) == "" {
		return fmt.Errorf("-commits is required, e.g. -commits A,B,C")
	}
	revisions := strings.Split(*commits, ",")
	for i := range revisions {
		revisions[i] = strings.TrimSpace(revisions[i])
		if revisions[i] == "" {
			return fmt.Errorf("empty commit in -commits list")
		}
	}

	absRepoRoot, err := filepath.Abs(*repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	clonePath, err := ensureClone(scratchRoot(), absRepoRoot)
	if err != nil {
		return fmt.Errorf("prepare scratch clone: %w", err)
	}

	labels := make([]render.Labeled, 0, len(revisions))
	for _, revision := range revisions {
		sha, err := resolveCommit(clonePath, revision)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", revision, err)
		}
		if err := resetClone(clonePath, sha); err != nil {
			return fmt.Errorf("reset to %s: %w", sha, err)
		}
		report, err := render.Measure(clonePath)
		if err != nil {
			return fmt.Errorf("measure %s: %w", sha, err)
		}
		labels = append(labels, render.Labeled{Commit: shortSHA(sha), Report: report})
	}

	if *gateOnly {
		gate := render.Gate(labels[len(labels)-1].Report)
		fmt.Print(render.FormatGate(gate))
		if gate.Overall == render.GateFail {
			os.Exit(1)
		}
		return nil
	}

	fmt.Print(render.FormatTable(labels))
	fmt.Println()
	gate := render.Gate(labels[len(labels)-1].Report)
	fmt.Print(render.FormatGate(gate))
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

// scratchRoot resolves the directory the cached clone lives under: the
// DEBTDASH_SCRATCH environment variable when set, otherwise the OS temp
// directory.
func scratchRoot() string {
	if dir := os.Getenv("DEBTDASH_SCRATCH"); dir != "" {
		return dir
	}
	return os.TempDir()
}

// ensureClone returns the path to a shared clone of repoRoot under
// scratchRoot, cloning it once if it does not already exist. --shared keeps
// the clone's object database as an alternate onto repoRoot, so commits
// made in repoRoot after the clone was created are still reachable by SHA
// without a fetch.
func ensureClone(scratchDir, repoRoot string) (string, error) {
	clonePath := filepath.Join(scratchDir, cloneDirName)
	if info, err := os.Stat(filepath.Join(clonePath, ".git")); err == nil && info.IsDir() {
		return clonePath, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat %s: %w", clonePath, err)
	}

	if err := os.MkdirAll(scratchDir, 0o755); err != nil {
		return "", fmt.Errorf("create scratch root %s: %w", scratchDir, err)
	}
	cmd := exec.Command("git", "clone", "--shared", repoRoot, clonePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git clone --shared %s %s: %w\n%s", repoRoot, clonePath, err, out)
	}
	return clonePath, nil
}

// resetClone hard-resets the clone to commit and removes untracked files.
func resetClone(clonePath, commit string) error {
	cmd := exec.Command("git", "reset", "--hard", commit)
	cmd.Dir = clonePath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard %s in %s: %w\n%s", commit, clonePath, err, out)
	}
	cmd = exec.Command("git", "clean", "-fd")
	cmd.Dir = clonePath
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean -fd in %s: %w\n%s", clonePath, err, out)
	}
	return nil
}

// resolveCommit resolves a revision to its full commit SHA inside the
// clone, fetching from repoRoot first is unnecessary because the clone was
// made --shared against the live object database.
func resolveCommit(clonePath, revision string) (string, error) {
	cmd := exec.Command("git", "rev-parse", revision)
	cmd.Dir = clonePath
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s in %s: %w", revision, clonePath, err)
	}
	return strings.TrimSpace(string(out)), nil
}
