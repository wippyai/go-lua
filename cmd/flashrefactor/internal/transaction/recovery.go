package transaction

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Recovery is the inspectable, non-secret portion of a durable crash record.
// It deliberately exposes no source bytes; clients obtain those only through
// a guarded Complete callback while holding the recovery lease.
type Recovery struct {
	State   RecoveryState
	Inputs  []RecoveryInput
	Outputs []ExpectedOutput
}

type RecoveryInput struct {
	Path   string
	Exists bool
	SHA256 string
}

// Inspect reads a durable recovery record without changing it. It is useful
// for a human or workbench to decide whether rollback or completion is the
// intended explicit action; Run never calls it automatically.
func Inspect(root string) (Recovery, error) {
	canonicalRoot, err := canonicalRoot(root)
	if err != nil {
		return Recovery{}, err
	}
	value, err := readJournal(canonicalRoot)
	if err != nil {
		return Recovery{}, err
	}
	result := Recovery{State: value.State, Inputs: make([]RecoveryInput, 0, len(value.Entries)), Outputs: append([]ExpectedOutput(nil), value.Outputs...)}
	for _, entry := range value.Entries {
		result.Inputs = append(result.Inputs, RecoveryInput{Path: entry.Path, Exists: entry.Exists, SHA256: entry.SHA256})
	}
	return result, nil
}

// Rollback restores the recorded preimage only after explicitly acquiring an
// unlocked crash lease. It never replays final output bytes, so a mixed crash
// state cannot become an accepted new revision by accident.
func Rollback(root string) error {
	t, err := openRecovery(root)
	if err != nil {
		return err
	}
	if t.journal.State == RecoveryRolledBack {
		return t.finish()
	}
	if err := t.setJournalState(RecoveryRecovering); err != nil {
		return t.abandon(err)
	}
	if err := t.restore(); err != nil {
		return t.abandon(err)
	}
	if err := t.setJournalState(RecoveryRolledBack); err != nil {
		return t.abandon(err)
	}
	return t.finish()
}

// Complete accepts a prior fully-applied durable transaction only after the
// caller explicitly reruns its in-lease postflight and the exact journaled
// output hashes still match. It rejects Prepared/Applying states rather than
// guessing that a partial crash happened to be complete.
func Complete(root string, postflight Guard) error {
	if postflight == nil {
		return fmt.Errorf("transaction recovery completion requires an in-lease postflight")
	}
	t, err := openRecovery(root)
	if err != nil {
		return err
	}
	if t.journal.State != RecoveryApplied && t.journal.State != RecoveryVerified {
		return t.abandon(fmt.Errorf("cannot complete transaction recovery in state %q; rollback is required", t.journal.State))
	}
	if err := t.verify(); err != nil {
		return t.abandon(err)
	}
	if err := postflight(t.preimage()); err != nil {
		return t.abandon(err)
	}
	return t.finish()
}

func openRecovery(root string) (*transaction, error) {
	canonicalRoot, err := canonicalRoot(root)
	if err != nil {
		return nil, err
	}
	lease, err := acquireRecoveryLease(canonicalRoot)
	if err != nil {
		return nil, err
	}
	value, err := readJournal(canonicalRoot)
	if err != nil {
		_ = closeRecoveryLease(lease)
		return nil, err
	}
	snapshots, err := journalSnapshots(canonicalRoot, value)
	if err != nil {
		_ = closeRecoveryLease(lease)
		return nil, err
	}
	paths := make([]string, 0, len(value.Outputs))
	changes := make(map[string]Change, len(value.Outputs))
	for _, output := range value.Outputs {
		paths = append(paths, output.Path)
		changes[output.Path] = Change{Path: output.Path, Delete: output.Deleted}
	}
	inputs := make([]string, 0, len(value.Entries))
	for _, entry := range value.Entries {
		inputs = append(inputs, entry.Path)
	}
	existingDirs := make(map[string]bool, len(value.ExistingDirs))
	for _, path := range value.ExistingDirs {
		existingDirs[path] = true
	}
	return &transaction{
		root: canonicalRoot, paths: paths, inputs: inputs, changes: changes,
		outputs: append([]ExpectedOutput(nil), value.Outputs...), snapshots: snapshots,
		existingDirs: existingDirs, lease: lease, journal: value,
	}, nil
}

func closeRecoveryLease(lease *workspaceLease) error {
	return errors.Join(unlock(lease.file), lease.file.Close())
}

func (t *transaction) finish() error {
	if err := t.clearJournal(); err != nil {
		return t.abandon(err)
	}
	return t.releaseLease()
}

func (t *transaction) abortBeforeMutation(cause error) error {
	if err := t.clearJournal(); err != nil {
		return t.abandon(errors.Join(cause, err))
	}
	return errors.Join(cause, t.releaseLease())
}

func (t *transaction) abortAfterMutation(cause error) error {
	if err := t.setJournalState(RecoveryRecovering); err != nil {
		return t.abandon(errors.Join(cause, err))
	}
	if err := t.restore(); err != nil {
		return t.abandon(errors.Join(cause, err))
	}
	if err := t.setJournalState(RecoveryRolledBack); err != nil {
		return t.abandon(errors.Join(cause, err))
	}
	if err := t.finish(); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (t *transaction) assertPreimage() error {
	return t.assertPaths(t.inputs, "in-lease guard")
}

func (t *transaction) assertObserved() error {
	return t.assertPaths(t.observed, "transaction")
}

func (t *transaction) assertPaths(paths []string, phase string) error {
	for _, path := range paths {
		full, err := t.securePath(path)
		if err != nil {
			return err
		}
		want := t.snapshots[path]
		info, err := os.Lstat(full)
		if !want.exists {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("transaction input %q appeared during %s", path, phase)
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != want.mode {
			return fmt.Errorf("transaction input %q changed during %s", path, phase)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if digest(data) != digest(want.data) {
			return fmt.Errorf("transaction input %q changed during %s", path, phase)
		}
	}
	return nil
}

func (t *transaction) restore() error {
	var result error
	for _, path := range t.paths {
		full, err := t.securePath(path)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		original := t.snapshots[path]
		if !original.exists {
			if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
				result = errors.Join(result, err)
			}
			if err := syncDirectory(filepath.Dir(full)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				result = errors.Join(result, err)
			}
			continue
		}
		if err := t.ensureParent(filepath.Dir(full)); err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := install(full, original.data, original.mode); err != nil {
			result = errors.Join(result, err)
		}
	}
	if err := t.removeCreatedParents(); err != nil {
		result = errors.Join(result, err)
	}
	return result
}

func (t *transaction) removeCreatedParents() error {
	candidates := make(map[string]bool)
	for _, path := range t.paths {
		parent := filepath.ToSlash(filepath.Dir(path))
		for parent != "." && parent != "/" && !t.existingDirs[parent] {
			candidates[parent] = true
			parent = filepath.ToSlash(filepath.Dir(parent))
		}
	}
	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		return strings.Count(paths[i], "/") > strings.Count(paths[j], "/") || strings.Count(paths[i], "/") == strings.Count(paths[j], "/") && paths[i] > paths[j]
	})
	var result error
	for _, path := range paths {
		full, err := t.securePath(path)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
			entries, readErr := os.ReadDir(full)
			if readErr == nil && len(entries) != 0 {
				continue
			}
			result = errors.Join(result, err)
		}
	}
	return result
}
