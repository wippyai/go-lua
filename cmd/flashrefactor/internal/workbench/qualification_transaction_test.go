package workbench

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/transaction"
)

func TestSyntheticCrossPackageStaleLockIsReadOnly(t *testing.T) {
	root, intent := qualificationFixture(t)
	bench := qualificationBench(t, root)
	prepared, err := bench.Prepare(context.Background(), intent)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	path := filepath.Join(root, "core", "link.go")
	if err := os.WriteFile(path, append(mustReadQualification(t, path), []byte("\n// stale lock mutation\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	before := qualificationTransactionTree(t, root)
	if _, err := bench.Replay(context.Background(), prepared.Lock); err == nil || !strings.Contains(err.Error(), "locked input changed") {
		t.Fatalf("stale replay error = %v", err)
	}
	qualificationTransactionTreeEqual(t, before, qualificationTransactionTree(t, root), "stale replay")
	if err := bench.Apply(context.Background(), prepared.Lock); err == nil || !strings.Contains(err.Error(), "locked input changed") {
		t.Fatalf("stale apply error = %v", err)
	}
	qualificationTransactionTreeEqual(t, before, qualificationTransactionTree(t, root), "stale apply")
	if _, err := os.Stat(filepath.Join(root, ".flashrefactor")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale lock created transaction metadata: %v", err)
	}
}

func TestSyntheticCrossPackageGateRollback(t *testing.T) {
	root, intent := qualificationFixture(t)
	path := filepath.Join(root, "core", "link_test.go")
	source := string(mustReadQualification(t, path))
	broken := strings.Replace(source, "trace = nil", "t.Fatal(\"ordinary gate failure\")\n\ttrace = nil", 1)
	if broken == source {
		t.Fatal("qualification fixture no longer contains TestLinkOrder setup")
	}
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	bench := qualificationBench(t, root)
	prepared, err := bench.Prepare(context.Background(), intent)
	if err != nil {
		t.Fatalf("prepare failing fixture: %v", err)
	}
	before := qualificationTransactionTree(t, root)
	err = bench.Apply(context.Background(), prepared.Lock)
	if err == nil || errors.Is(err, transaction.ErrSafetyFailure) {
		t.Fatalf("ordinary gate error = %v", err)
	}
	qualificationTransactionTreeEqual(t, before, qualificationTransactionTree(t, root), "ordinary gate rollback")
	qualificationTransactionOutputsAbsent(t, root, prepared.Lock)
	qualificationTransactionNoRecovery(t, root)
}

func TestSyntheticCrossPackageSafetyFailureRollback(t *testing.T) {
	root, intent := qualificationFixture(t)
	bench := qualificationBench(t, root)
	prepared, err := bench.Prepare(context.Background(), intent)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	runner := filepath.Join(root, "scripts", "bounded_test.sh")
	const script = "#!/bin/sh\ncalls=.flashrefactor/gate.calls\nif [ -e \"$calls\" ]; then\n  printf second > \"$calls\"\nelse\n  printf first > \"$calls\"\nfi\nexit 125\n"
	if err := os.WriteFile(runner, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	before := qualificationTransactionTree(t, root)
	err = bench.Apply(context.Background(), prepared.Lock)
	if !errors.Is(err, transaction.ErrSafetyFailure) {
		t.Fatalf("safety gate error = %v, want transaction.ErrSafetyFailure", err)
	}
	qualificationTransactionTreeEqual(t, before, qualificationTransactionTree(t, root), "safety gate rollback")
	qualificationTransactionOutputsAbsent(t, root, prepared.Lock)
	qualificationTransactionNoRecovery(t, root)
	calls, readErr := os.ReadFile(filepath.Join(root, ".flashrefactor", "gate.calls"))
	if readErr != nil || string(calls) != "first" {
		t.Fatalf("safety gate invocation trace = %q err=%v", calls, readErr)
	}
}

type qualificationTransactionFile struct {
	data []byte
	mode fs.FileMode
}

func qualificationTransactionTree(t *testing.T, root string) map[string]qualificationTransactionFile {
	t.Helper()
	result := map[string]qualificationTransactionFile{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == ".flashrefactor" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		result[filepath.ToSlash(relative)] = qualificationTransactionFile{data: data, mode: info.Mode().Perm()}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func qualificationTransactionTreeEqual(t *testing.T, want, got map[string]qualificationTransactionFile, stage string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s changed file denominator: want=%v got=%v", stage, qualificationTransactionPaths(want), qualificationTransactionPaths(got))
	}
	for path, expected := range want {
		actual, exists := got[path]
		if !exists || string(actual.data) != string(expected.data) || actual.mode != expected.mode {
			t.Fatalf("%s changed %s: exists=%v mode=%#o/%#o bytes=%q/%q", stage, path, exists, actual.mode, expected.mode, actual.data, expected.data)
		}
	}
}

func qualificationTransactionPaths(values map[string]qualificationTransactionFile) []string {
	result := make([]string, 0, len(values))
	for path := range values {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func qualificationTransactionOutputsAbsent(t *testing.T, root string, lock cutplan.Lock) {
	t.Helper()
	for _, path := range lock.Evidence.Inputs.Absent {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rollback left initially absent output %s: %v", path, err)
		}
	}
}

func qualificationTransactionNoRecovery(t *testing.T, root string) {
	t.Helper()
	if _, err := transaction.Inspect(root); !errors.Is(err, transaction.ErrNoRecovery) {
		t.Fatalf("rollback left recovery journal: %v", err)
	}
}

func mustReadQualification(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
