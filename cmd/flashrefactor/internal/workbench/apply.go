package workbench

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/transaction"
)

// Apply is the only mutating workbench operation. It first obtains a full
// replay, then replays again while transaction owns the workspace lease. The
// latter is the acceptance point: no pre-lock observation can race mutation.
func (bench Bench) Apply(ctx context.Context, lock cutplan.Lock) error {
	prepared, err := bench.Replay(ctx, lock)
	if err != nil {
		return err
	}
	plan, err := transactionPlan(lock, prepared.rendered.files)
	if err != nil {
		return err
	}
	var guarded Prepared
	_, err = transaction.RunWithGuard(bench.config.Root, plan, func(transaction.Preimage) error {
		guardedReplay, replayErr := bench.Replay(ctx, lock)
		if replayErr != nil {
			return replayErr
		}
		guarded = guardedReplay
		if err := exactRendered(prepared.rendered, guarded.rendered); err != nil {
			return err
		}
		return cutplan.VerifyDiff(guarded.rendered.diff, lock)
	}, func() error {
		if err := bench.verifyApplied(ctx, lock, guarded.rendered.source); err != nil {
			return err
		}
		return transaction.RunGates(bench.config.Root, allTests(lock.Intent))
	})
	return err
}

func transactionPlan(lock cutplan.Lock, files []semantic.VirtualFile) (transaction.Plan, error) {
	changes := make([]transaction.Change, 0, len(files))
	for _, file := range files {
		changes = append(changes, transaction.Change{Path: file.Path, Data: append([]byte(nil), file.Content...), Delete: file.Delete})
	}
	declared := cutplan.WritePaths(lock.Intent)
	if len(changes) != len(declared) {
		return transaction.Plan{}, fmt.Errorf("rendered changes do not cover lock write footprint")
	}
	writes := map[string]bool{}
	for _, path := range declared {
		writes[path] = true
	}
	observed := make([]string, 0)
	for _, path := range cutplan.ReadPaths(lock.Intent) {
		if !writes[path] {
			observed = append(observed, path)
		}
	}
	return transaction.Plan{Declared: declared, Changes: changes, Observed: observed}, nil
}

func exactRendered(left, right rendered) error {
	if string(left.diff) != string(right.diff) || len(left.files) != len(right.files) {
		return fmt.Errorf("in-lease render changed")
	}
	for index := range left.files {
		before, after := left.files[index], right.files[index]
		if before.Path != after.Path || before.Delete != after.Delete || string(before.Content) != string(after.Content) {
			return fmt.Errorf("in-lease render changed at %s", before.Path)
		}
	}
	return nil
}

func (bench Bench) verifyApplied(ctx context.Context, lock cutplan.Lock, source semantic.Snapshot) error {
	if err := cutplan.VerifyOutputs(bench.config.Root, lock); err != nil {
		return err
	}
	session, err := semantic.NewSession(bench.config.Semantic)
	if err != nil {
		return err
	}
	defer session.Close()
	post, err := session.CollectVirtual(ctx, lock.Intent, nil, nil)
	if err != nil {
		return err
	}
	if err := semantic.VerifyExpected(targetEvidence(lock.Evidence.Resolution.Objects), post.Objects); err != nil {
		return fmt.Errorf("postflight target evidence: %w", err)
	}
	toolchain, err := bench.toolchainAt(post)
	if err != nil {
		return err
	}
	if toolchain != lock.Toolchain {
		return fmt.Errorf("postflight toolchain evidence changed")
	}
	gates, err := verifyGates(lock.Intent, source, post)
	if err != nil {
		return fmt.Errorf("postflight gates: %w", err)
	}
	if err := compareGates(lock.Evidence.Gates, gates); err != nil {
		return fmt.Errorf("postflight gate evidence: %w", err)
	}
	return nil
}

func targetEvidence(values []cutplan.ObjectEvidence) []cutplan.ObjectEvidence {
	result := make([]cutplan.ObjectEvidence, 0, len(values))
	for _, value := range values {
		if value.Role == cutplan.ObjectTarget {
			result = append(result, value)
		}
	}
	return result
}

func allTests(intent cutplan.Intent) []cutplan.Law {
	result := make([]cutplan.Law, 0)
	for _, operation := range intent.Operations {
		result = append(result, operation.Verify.Laws...)
	}
	return result
}

func compareGates(expected, actual []cutplan.GateEvidence) error {
	if len(expected) != len(actual) {
		return fmt.Errorf("gate denominator changed")
	}
	for index := range expected {
		if expected[index] != actual[index] {
			return fmt.Errorf("gate evidence changed: %s", expected[index].Gate)
		}
	}
	return nil
}
