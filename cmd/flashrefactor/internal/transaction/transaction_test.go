package transaction

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
)

func TestRunRollsBackBytesModesDeletesAndCreatedDirectories(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "old.go"), []byte("old"), 0600)
	write(t, filepath.Join(root, "delete.go"), []byte("delete"), 0640)
	plan := Plan{
		Declared: []string{"old.go", "delete.go", "new/deep/new.go"},
		Changes: []Change{
			{Path: "old.go", Data: []byte("new")},
			{Path: "delete.go", Delete: true},
			{Path: "new/deep/new.go", Data: []byte("created")},
		},
	}
	_, err := Run(root, plan, func() error { return errors.New("gate failed") })
	if err == nil || !strings.Contains(err.Error(), "gate failed") {
		t.Fatalf("Run error = %v, want gate error", err)
	}
	assertFile(t, filepath.Join(root, "old.go"), "old", 0600)
	assertFile(t, filepath.Join(root, "delete.go"), "delete", 0640)
	if _, err := os.Stat(filepath.Join(root, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created directory remained after rollback: %v", err)
	}
}

func TestRunVerifiesExpectedHashAndDeletion(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "retire.go"), []byte("retire"), 0644)
	outputs, err := Run(root, Plan{
		Declared: []string{"retire.go", "component/flow.go"},
		Changes:  []Change{{Path: "retire.go", Delete: true}, {Path: "component/flow.go", Data: []byte("package component\n")}},
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(outputs) != 2 || outputs[0].Path != "component/flow.go" || outputs[0].SHA256 != digest([]byte("package component\n")) || outputs[1].Path != "retire.go" || !outputs[1].Deleted {
		t.Fatalf("unexpected output proof: %#v", outputs)
	}
	if _, err := os.Stat(filepath.Join(root, "retire.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired file: %v", err)
	}
	assertFile(t, filepath.Join(root, "component/flow.go"), "package component\n", 0644)
}

func TestRunPreservesExistingFileMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owned.go")
	write(t, path, []byte("old"), 0600)
	_, err := Run(root, Plan{Declared: []string{"owned.go"}, Changes: []Change{{Path: "owned.go", Data: []byte("new")}}}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertFile(t, path, "new", 0600)
}

func TestRunRollsBackWhenOutputIsTampered(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "flow.go"), []byte("old"), 0644)
	_, err := Run(root, Plan{Declared: []string{"flow.go"}, Changes: []Change{{Path: "flow.go", Data: []byte("expected")}}}, func() error {
		return os.WriteFile(filepath.Join(root, "flow.go"), []byte("tampered"), 0644)
	})
	if err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("Run error = %v, want hash mismatch", err)
	}
	assertFile(t, filepath.Join(root, "flow.go"), "old", 0644)
}

func TestRunRejectsSymlinkAndPathEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := Run(root, Plan{Declared: []string{"escape/owned.go"}, Changes: []Change{{Path: "escape/owned.go", Data: []byte("x")}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink error = %v", err)
	}
	_, err = Run(root, Plan{Declared: []string{"../escape.go"}, Changes: []Change{{Path: "../escape.go", Data: []byte("x")}}}, nil)
	if err == nil {
		t.Fatal("path escape was accepted")
	}
}

func TestRunRejectsDuplicateAndMissingPlanRows(t *testing.T) {
	root := t.TempDir()
	for _, plan := range []Plan{
		{
			Declared: []string{"one.go", "one.go"},
			Changes:  []Change{{Path: "one.go", Data: []byte("one")}},
		},
		{
			Declared: []string{"one.go", "two.go"},
			Changes:  []Change{{Path: "one.go", Data: []byte("one")}},
		},
		{
			Declared: []string{"one.go"},
			Changes: []Change{
				{Path: "one.go", Data: []byte("one")},
				{Path: "one.go", Data: []byte("two")},
			},
		},
	} {
		if _, err := Run(root, plan, nil); err == nil {
			t.Fatal("invalid plan was accepted")
		}
	}
}

func TestRunRejectsHardLinkedInput(t *testing.T) {
	root := t.TempDir()
	owner := filepath.Join(root, "owner.go")
	write(t, owner, []byte("old"), 0644)
	if err := os.Link(owner, filepath.Join(root, "alias.go")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	_, err := Run(root, Plan{Declared: []string{"owner.go"}, Changes: []Change{{Path: "owner.go", Data: []byte("new")}}}, nil)
	if err == nil || !strings.Contains(err.Error(), "hard-linked") {
		t.Fatalf("hard-link error = %v", err)
	}
	assertFile(t, owner, "old", 0644)
	assertFile(t, filepath.Join(root, "alias.go"), "old", 0644)
}

func TestRunHasExclusiveWorkspaceLease(t *testing.T) {
	root := t.TempDir()
	plan := Plan{Declared: []string{"flow.go"}, Changes: []Change{{Path: "flow.go", Data: []byte("one")}}}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Run(root, plan, func() error {
			close(entered)
			<-release
			return nil
		})
		done <- err
	}()
	<-entered
	if _, err := Run(root, plan, nil); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("concurrent Run error = %v, want ErrWorkspaceBusy", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("lease owner Run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, leaseFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lease remained after completed transaction: %v", err)
	}
	if _, err := Run(root, plan, nil); err != nil {
		t.Fatalf("Run after lease release: %v", err)
	}
}

func TestRunWithGuardObservesReadOnlyInputsUnderSameLease(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "owned.go"), []byte("old"), 0644)
	write(t, filepath.Join(root, "config.go"), []byte("contract"), 0644)
	plan := Plan{
		Declared: []string{"owned.go"},
		Observed: []string{"config.go"},
		Changes:  []Change{{Path: "owned.go", Data: []byte("new")}},
	}
	called := false
	_, err := RunWithGuard(root, plan, func(image Preimage) error {
		called = true
		if got, exists, err := image.Read("config.go"); err != nil || !exists || string(got) != "contract" {
			t.Fatalf("guard config preimage = %q exists=%v err=%v", got, exists, err)
		}
		if got := image.Paths(); strings.Join(got, ",") != "config.go,owned.go" {
			t.Fatalf("guard paths = %v", got)
		}
		if _, err := Run(root, Plan{Declared: []string{"other.go"}, Changes: []Change{{Path: "other.go", Data: []byte("x")}}}, nil); !errors.Is(err, ErrWorkspaceBusy) {
			t.Fatalf("nested run = %v, want ErrWorkspaceBusy", err)
		}
		return nil
	}, nil)
	if err != nil || !called {
		t.Fatalf("RunWithGuard called=%v err=%v", called, err)
	}
	assertFile(t, filepath.Join(root, "owned.go"), "new", 0644)
	assertFile(t, filepath.Join(root, "config.go"), "contract", 0644)
}

func TestRunWithGuardRejectsChangedObservedInputBeforeRename(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "owned.go"), []byte("old"), 0644)
	write(t, filepath.Join(root, "config.go"), []byte("contract"), 0644)
	_, err := RunWithGuard(root, Plan{
		Declared: []string{"owned.go"}, Observed: []string{"config.go"},
		Changes: []Change{{Path: "owned.go", Data: []byte("new")}},
	}, func(Preimage) error {
		return os.WriteFile(filepath.Join(root, "config.go"), []byte("changed"), 0644)
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "changed during in-lease guard") {
		t.Fatalf("RunWithGuard error = %v", err)
	}
	assertFile(t, filepath.Join(root, "owned.go"), "old", 0644)
	assertFile(t, filepath.Join(root, "config.go"), "changed", 0644)
	if _, err := Inspect(root); !errors.Is(err, ErrNoRecovery) {
		t.Fatalf("failed preflight left recovery state: %v", err)
	}
}

func TestPreparedJournalCapturesInputsBeforeAnyRename(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "owned.go"), []byte("old"), 0600)
	write(t, filepath.Join(root, "read.go"), []byte("read"), 0644)
	tx, err := begin(root, Plan{
		Declared: []string{"owned.go", "new.go"}, Observed: []string{"read.go"},
		Changes: []Change{{Path: "owned.go", Data: []byte("new")}, {Path: "new.go", Data: []byte("created")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.State != RecoveryPrepared || len(recovery.Inputs) != 3 {
		t.Fatalf("recovery = %#v", recovery)
	}
	if _, err := os.Stat(filepath.Join(root, metadataDirectory, preimageDirectory, "000000")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent output had a preimage blob: %v", err)
	}
	assertFile(t, filepath.Join(root, metadataDirectory, preimageDirectory, "000001"), "old", 0600)
	assertFile(t, filepath.Join(root, metadataDirectory, preimageDirectory, "000002"), "read", 0600)
	if err := tx.abandon(nil); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(root); err != nil {
		t.Fatalf("rollback prepared: %v", err)
	}
}

func TestExplicitRecoveryRollsBackMixedCrashState(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "old.go"), []byte("old"), 0640)
	tx, err := begin(root, Plan{
		Declared: []string{"old.go", "new.go"},
		Changes:  []Change{{Path: "old.go", Data: []byte("new")}, {Path: "new.go", Data: []byte("created")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.setJournalState(RecoveryApplying); err != nil {
		t.Fatal(err)
	}
	if err := tx.apply(); err != nil {
		t.Fatal(err)
	}
	if err := tx.abandon(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root, Plan{Declared: []string{"other.go"}, Changes: []Change{{Path: "other.go", Data: []byte("x")}}}, nil); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("Run stole crash marker: %v", err)
	}
	if err := Complete(root, func(Preimage) error { return nil }); err == nil || !strings.Contains(err.Error(), "rollback is required") {
		t.Fatalf("Complete applying state = %v", err)
	}
	if err := Rollback(root); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	assertFile(t, filepath.Join(root, "old.go"), "old", 0640)
	if _, err := os.Stat(filepath.Join(root, "new.go")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new output remained after recovery rollback: %v", err)
	}
	if _, err := Inspect(root); !errors.Is(err, ErrNoRecovery) {
		t.Fatalf("recovery remained after rollback: %v", err)
	}
}

func TestExplicitRecoveryDoesNotTakeLiveLease(t *testing.T) {
	root := t.TempDir()
	tx, err := begin(root, Plan{Declared: []string{"owned.go"}, Changes: []Change{{Path: "owned.go", Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := Rollback(root); !errors.Is(err, ErrWorkspaceBusy) {
		t.Fatalf("Rollback took live lease: %v", err)
	}
	if err := tx.abortBeforeMutation(errors.New("stop")); err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("abort = %v", err)
	}
}

func TestExplicitRecoveryCompletesOnlyExactAppliedStateWithPostflight(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "old.go"), []byte("old"), 0644)
	tx, err := begin(root, Plan{Declared: []string{"old.go"}, Changes: []Change{{Path: "old.go", Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.setJournalState(RecoveryApplying); err != nil {
		t.Fatal(err)
	}
	if err := tx.apply(); err != nil {
		t.Fatal(err)
	}
	if err := tx.setJournalState(RecoveryApplied); err != nil {
		t.Fatal(err)
	}
	if err := tx.abandon(nil); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := Complete(root, func(image Preimage) error {
		called = true
		got, exists, readErr := image.Read("old.go")
		if readErr != nil || !exists || string(got) != "old" {
			t.Fatalf("recovery preimage = %q exists=%v err=%v", got, exists, readErr)
		}
		return nil
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !called {
		t.Fatal("recovery postflight was not called")
	}
	assertFile(t, filepath.Join(root, "old.go"), "new", 0644)
	if _, err := Inspect(root); !errors.Is(err, ErrNoRecovery) {
		t.Fatalf("recovery remained after completion: %v", err)
	}
}

func TestExplicitRecoveryCompletionRejectsTamperedOutput(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "owned.go"), []byte("old"), 0644)
	tx, err := begin(root, Plan{Declared: []string{"owned.go"}, Changes: []Change{{Path: "owned.go", Data: []byte("new")}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.setJournalState(RecoveryApplying); err != nil {
		t.Fatal(err)
	}
	if err := tx.apply(); err != nil {
		t.Fatal(err)
	}
	if err := tx.setJournalState(RecoveryApplied); err != nil {
		t.Fatal(err)
	}
	if err := tx.abandon(nil); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "owned.go"), []byte("tampered"), 0644)
	if err := Complete(root, func(Preimage) error { return nil }); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("Complete tampered output = %v", err)
	}
	if state, err := Inspect(root); err != nil || state.State != RecoveryApplied {
		t.Fatalf("tampered completion destroyed recovery: state=%#v err=%v", state, err)
	}
	if err := Rollback(root); err != nil {
		t.Fatal(err)
	}
	assertFile(t, filepath.Join(root, "owned.go"), "old", 0644)
}

func TestPlanReservesDurableTransactionMetadata(t *testing.T) {
	root := t.TempDir()
	for _, plan := range []Plan{
		{Declared: []string{leaseFile}, Changes: []Change{{Path: leaseFile, Data: []byte("x")}}},
		{Declared: []string{"owned.go"}, Observed: []string{metadataDirectory + "/state.json"}, Changes: []Change{{Path: "owned.go", Data: []byte("x")}}},
	} {
		if _, err := Run(root, plan, nil); err == nil {
			t.Fatalf("metadata plan was accepted: %#v", plan)
		}
	}
}

func TestRunGatesUsesOnlyBoundedRunnerArguments(t *testing.T) {
	root := t.TempDir()
	runner := filepath.Join(root, "scripts", "bounded_test.sh")
	if err := os.MkdirAll(filepath.Dir(runner), 0755); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(root, "arguments")
	write(t, runner, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > arguments\nprintf '%s\\n' '{\"Action\":\"run\",\"Package\":\"./program/flow\",\"Test\":\"TestExactLaw\"}' '{\"Action\":\"pass\",\"Package\":\"./program/flow\",\"Test\":\"TestExactLaw\"}' '{\"Action\":\"pass\",\"Package\":\"./program/flow\"}'\n"), 0755)
	spec := cutplan.Law{ID: "exact", Package: "./program/flow", Test: "TestExactLaw"}
	if err := RunGates(root, []cutplan.Law{spec}); err != nil {
		t.Fatalf("runGates: %v", err)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "go\ntest\n-json\n./program/flow\n-run\n^TestExactLaw$\n-count=1\n"
	if string(got) != want {
		t.Fatalf("runner argv = %q, want %q", got, want)
	}
}

// This uses the actual Go JSON emitter. Its Package field is the canonical
// module import path, not the reviewed relative ./law argument.
func TestRunGatesAcceptsCanonicalGoJSONPackageIdentity(t *testing.T) {
	root := t.TempDir()
	copyRepositoryBoundedRunner(t, root)
	write(t, filepath.Join(root, "go.mod"), []byte("module example.com/lawfixture\n\ngo 1.23.0\n"), 0644)
	write(t, filepath.Join(root, "law", "law_test.go"), []byte("package law\n\nimport \"testing\"\n\nfunc TestExactLaw(t *testing.T) {}\n"), 0644)
	if err := RunGates(root, []cutplan.Law{{ID: "exact", Package: "./law", Test: "TestExactLaw"}}); err != nil {
		t.Fatalf("real canonical Go JSON law: %v", err)
	}
}

func TestRunGatesDoesNotFindSameNamedLawInAnotherPackage(t *testing.T) {
	root := t.TempDir()
	copyRepositoryBoundedRunner(t, root)
	write(t, filepath.Join(root, "go.mod"), []byte("module example.com/lawfixture\n\ngo 1.23.0\n"), 0644)
	write(t, filepath.Join(root, "law", "law_test.go"), []byte("package law\n\nimport \"testing\"\n\nfunc TestDifferent(t *testing.T) {}\n"), 0644)
	write(t, filepath.Join(root, "other", "other_test.go"), []byte("package other\n\nimport \"testing\"\n\nfunc TestExactLaw(t *testing.T) {}\n"), 0644)
	err := RunGates(root, []cutplan.Law{{ID: "exact", Package: "./law", Test: "TestExactLaw"}})
	if err == nil || !strings.Contains(err.Error(), "run=0") {
		t.Fatalf("same-name law in another package satisfied ./law: %v", err)
	}
}

func TestRunGatesStopsOnSafetyFailure(t *testing.T) {
	root := t.TempDir()
	runner := filepath.Join(root, "scripts", "bounded_test.sh")
	if err := os.MkdirAll(filepath.Dir(runner), 0755); err != nil {
		t.Fatal(err)
	}
	write(t, runner, []byte("#!/bin/sh\nprintf x >> calls\nexit 125\n"), 0755)
	err := RunGates(root, []cutplan.Law{
		{ID: "one", Package: "./one", Test: "TestOne"},
		{ID: "two", Package: "./two", Test: "TestTwo"},
	})
	if !errors.Is(err, ErrSafetyFailure) {
		t.Fatalf("runGates error = %v, want safety failure", err)
	}
	calls, readErr := os.ReadFile(filepath.Join(root, "calls"))
	if readErr != nil || string(calls) != "x" {
		t.Fatalf("safety failure ran more than one gate: calls=%q err=%v", calls, readErr)
	}
}

func TestRunGatesUsesRealBoundedWrapperForDirectGate(t *testing.T) {
	root := t.TempDir()
	copyRepositoryBoundedRunner(t, root)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(root, "gate-session")
	write(t, filepath.Join(bin, "go"), []byte("#!/usr/bin/env bash\nps -o sid= -p \"$$\" | awk '{$1=$1; print}' > \"$BOUNDARY_SESSION_FILE\"\nprintf '%s\\n' '{\"Action\":\"run\",\"Package\":\"./program/flow\",\"Test\":\"TestExactLaw\"}' '{\"Action\":\"pass\",\"Package\":\"./program/flow\",\"Test\":\"TestExactLaw\"}' '{\"Action\":\"pass\",\"Package\":\"./program/flow\"}'\n"), 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BOUNDARY_SESSION_FILE", sessionFile)
	// A bare user-set marker is not a bypass: the real inherited descriptor is
	// deliberately closed before the wrapper starts, so it must create its own
	// session.
	t.Setenv("WIPPY_BOUNDED_CAPABILITY_FD", "9")
	runner := filepath.Join(root, "scripts", "bounded_test.sh")
	command := exec.Command("bash", "-c", `exec 9<&-; exec "$@"`, "bash", runner,
		"go", "test", "./program/flow", "-run", "^TestExactLaw$", "-count=1")
	command.Env = boundedRunnerEnvironment("WIPPY_BOUNDED_CAPABILITY_FD=9")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("direct bounded gate: %v\n%s", err, output)
	}
	gateSession := readSession(t, sessionFile)
	if gateSession == processSession(t, os.Getpid()) {
		t.Fatalf("direct gate stayed in caller session %s; bounded wrapper was bypassed", gateSession)
	}
}

func TestRunGatesKeepsActualOuterBoundedSession(t *testing.T) {
	if os.Getenv("WIPPY_BOUNDED_ACTIVE") != "wippy-bounded-v1" {
		t.Fatal("this law must run through scripts/bounded_test.sh")
	}
	root := t.TempDir()
	copyRepositoryBoundedRunner(t, root)
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(root, "gate-session")
	write(t, filepath.Join(bin, "go"), []byte("#!/usr/bin/env bash\nps -o sid= -p \"$$\" | awk '{$1=$1; print}' > \"$BOUNDARY_SESSION_FILE\"\nprintf '%s\\n' '{\"Action\":\"run\",\"Package\":\"./program/flow\",\"Test\":\"TestExactLaw\"}' '{\"Action\":\"pass\",\"Package\":\"./program/flow\",\"Test\":\"TestExactLaw\"}' '{\"Action\":\"pass\",\"Package\":\"./program/flow\"}'\n"), 0755)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BOUNDARY_SESSION_FILE", sessionFile)
	if err := RunGates(root, []cutplan.Law{{ID: "exact", Package: "./program/flow", Test: "TestExactLaw"}}); err != nil {
		t.Fatalf("RunGates: %v", err)
	}
	if got, want := readSession(t, sessionFile), processSession(t, os.Getpid()); got != want {
		t.Fatalf("nested transaction gate session = %s, want outer bounded session %s", got, want)
	}
}

func TestBoundedRunnerKeepsNestedGateInParentSession(t *testing.T) {
	runner := repositoryBoundedRunner(t)
	root := t.TempDir()
	outerSession := filepath.Join(root, "outer-session")
	innerSessions := []string{filepath.Join(root, "inner-session-one"), filepath.Join(root, "inner-session-two")}
	command := exec.Command(runner, "bash", "-c", `
outer=$(ps -o sid= -p "$$" | awk '{$1=$1; print}')
printf '%s\n' "$outer" > "$1"
"$2" bash -c 'ps -o sid= -p "$$" | awk '\''{$1=$1; print}'\'' > "$1"' bash "$3"
"$2" bash -c 'ps -o sid= -p "$$" | awk '\''{$1=$1; print}'\'' > "$1"' bash "$4"
`, "bash", outerSession, runner, innerSessions[0], innerSessions[1])
	command.Env = boundedRunnerEnvironment("WIPPY_BOUNDED_CAPABILITY_FD=9")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("nested bounded runner: %v\n%s", err, output)
	}
	want := readSession(t, outerSession)
	for _, path := range innerSessions {
		if got := readSession(t, path); got != want {
			t.Fatalf("nested gate session = %s, want parent bounded session %s", got, want)
		}
	}
}

func TestBoundedRunnerFailsClosedForInvalidNestedCapability(t *testing.T) {
	runner := repositoryBoundedRunner(t)
	for _, marker := range []string{"broken", "wippy-bounded-v1:1"} {
		t.Run(marker, func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "child-ran")
			status := filepath.Join(root, "status")
			command := exec.Command(runner, "bash", "-c", `
marker=$(mktemp)
printf '%s\n' "$1" > "$marker"
exec 9<"$marker"
rm -f -- "$marker"
export WIPPY_BOUNDED_ACTIVE=wippy-bounded-v1 WIPPY_BOUNDED_CAPABILITY_FD=9
"$2" bash -c 'printf child > "$1"' bash "$3"
code=$?
printf '%s\n' "$code" > "$4"
test "$code" -eq 125
test ! -e "$3"
`, "bash", marker, runner, child, status)
			command.Env = boundedRunnerEnvironment()
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("invalid nested capability: %v\n%s", err, output)
			}
			if got := strings.TrimSpace(string(readFile(t, status))); got != "125" {
				t.Fatalf("invalid nested capability exit = %s, want 125", got)
			}
			if _, err := os.Stat(child); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid nested capability started child: %v", err)
			}
		})
	}
}

func TestBoundedRunnerFailsClosedWhenNestedDescriptorIsClosed(t *testing.T) {
	runner := repositoryBoundedRunner(t)
	root := t.TempDir()
	child := filepath.Join(root, "child-ran")
	status := filepath.Join(root, "status")
	command := exec.Command(runner, "bash", "-c", `
exec 9<&-
"$1" bash -c 'printf child > "$1"' bash "$2"
code=$?
printf '%s\n' "$code" > "$3"
test "$code" -eq 125
test ! -e "$2"
`, "bash", runner, child, status)
	command.Env = boundedRunnerEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("closed nested descriptor: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(readFile(t, status))); got != "125" {
		t.Fatalf("closed nested descriptor exit = %s, want 125", got)
	}
	if _, err := os.Stat(child); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("closed nested descriptor started child: %v", err)
	}
}

func copyRepositoryBoundedRunner(t *testing.T, root string) {
	t.Helper()
	source := repositoryBoundedRunner(t)
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(source)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "scripts", "bounded_test.sh"), data, info.Mode().Perm())
}

func repositoryBoundedRunner(t *testing.T) string {
	t.Helper()
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate transaction test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "..", "..", "scripts", "bounded_test.sh"))
}

func boundedRunnerEnvironment(extra ...string) []string {
	result := make([]string, 0, len(os.Environ())+len(extra))
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "WIPPY_BOUNDED_CAPABILITY_") || strings.HasPrefix(value, "WIPPY_BOUNDED_ACTIVE=") {
			continue
		}
		result = append(result, value)
	}
	return append(result, extra...)
}

func readSession(t *testing.T, path string) string {
	t.Helper()
	data := readFile(t, path)
	value := strings.TrimSpace(string(data))
	if _, err := strconv.ParseInt(value, 10, 64); err != nil || value == "" {
		t.Fatalf("session at %s = %q, want positive decimal: %v", path, value, err)
	}
	return value
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func processSession(t *testing.T, pid int) string {
	t.Helper()
	command := exec.Command("ps", "-o", "sid=", "-p", strconv.Itoa(pid))
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	value := strings.TrimSpace(string(output))
	if _, err := strconv.ParseInt(value, 10, 64); err != nil || value == "" {
		t.Fatalf("session for pid %d = %q, want positive decimal: %v", pid, value, err)
	}
	return value
}

func write(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string, mode os.FileMode) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %s = %q, want %q", path, got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != mode {
		t.Fatalf("mode %s = %04o, want %04o", path, info.Mode().Perm(), mode)
	}
}
