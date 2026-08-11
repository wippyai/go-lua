package transaction

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrWorkspaceBusy means another flashrefactor transaction currently owns the
// root-local lease (or a prior crashed worker left one for recovery).
var ErrWorkspaceBusy = errors.New("flashrefactor workspace transaction is already active")

// Change is one exact final state for a declared regular file.  Delete and
// Data are mutually exclusive.  Changes are values, not callbacks, so the
// transaction can bind its expected output before the first mutation.
type Change struct {
	Path   string
	Data   []byte
	Delete bool
}

// Plan is the entire write authority for a transaction.  Declared and Changes
// must name the same paths exactly; a refactor cannot mutate an incidental
// file, nor leave a declared mutation unspecified.
type Plan struct {
	Declared []string
	Changes  []Change
	// Observed are read-only source inputs. They are captured under the same
	// lease as Declared files and must remain byte-identical until mutation
	// starts. They cannot overlap Declared.
	Observed []string
}

// ExpectedOutput records the final proof checked after a successful callback.
// Deleted files have an empty SHA256.
type ExpectedOutput struct {
	Path    string
	SHA256  string
	Deleted bool
}

// Run snapshots all declared paths, installs the exact changes, invokes
// verify, and verifies the final hashes/deletions.  Any failure restores the
// original bytes and modes and removes only directories created by this run
// when they are empty.
func Run(root string, plan Plan, verify func() error) ([]ExpectedOutput, error) {
	return RunWithGuard(root, plan, nil, verify)
}

// Guard receives the immutable transaction preimage while the root-local
// lease is held and before any source mutation. It may perform semantic
// validation against exactly the bytes later protected by the transaction.
// RunWithGuard rechecks every observed and declared input after Guard returns;
// a concurrent source mutation fails before the first rename.
type Guard func(Preimage) error

// RunWithGuard installs an exact Plan only after its in-lease Guard accepts
// the durable preimage. It never takes over a recovery lease left by a crash.
func RunWithGuard(root string, plan Plan, guard Guard, verify func() error) ([]ExpectedOutput, error) {
	t, err := begin(root, plan)
	if err != nil {
		return nil, err
	}
	if guard != nil {
		if err := guard(t.preimage()); err != nil {
			return nil, t.abortBeforeMutation(err)
		}
	}
	if err := t.assertPreimage(); err != nil {
		return nil, t.abortBeforeMutation(err)
	}
	if err := t.setJournalState(RecoveryApplying); err != nil {
		return nil, t.abandon(err)
	}
	if err := t.apply(); err != nil {
		return nil, t.abortAfterMutation(err)
	}
	if err := t.setJournalState(RecoveryApplied); err != nil {
		return nil, t.abandon(err)
	}
	if verify != nil {
		if err := verify(); err != nil {
			return nil, t.abortAfterMutation(err)
		}
	}
	if err := t.assertObserved(); err != nil {
		return nil, t.abortAfterMutation(err)
	}
	if err := t.verify(); err != nil {
		return nil, t.abortAfterMutation(err)
	}
	if err := t.setJournalState(RecoveryVerified); err != nil {
		return nil, t.abandon(err)
	}
	t.committed = true
	outputs := t.expected()
	if err := t.finish(); err != nil {
		return nil, err
	}
	return outputs, nil
}

type snapshot struct {
	exists bool
	data   []byte
	mode   fs.FileMode
}

type transaction struct {
	root         string
	changes      map[string]Change
	paths        []string
	outputs      []ExpectedOutput
	observed     []string
	inputs       []string
	snapshots    map[string]snapshot
	existingDirs map[string]bool
	committed    bool
	lease        *workspaceLease
	journal      journal
}

func begin(root string, plan Plan) (*transaction, error) {
	canonicalRoot, err := canonicalRoot(root)
	if err != nil {
		return nil, err
	}
	paths, observed, inputs, changes, err := validatePlan(plan)
	if err != nil {
		return nil, err
	}
	lease, err := acquireLease(canonicalRoot)
	if err != nil {
		return nil, err
	}
	t := &transaction{
		root: canonicalRoot, paths: paths, observed: observed, inputs: inputs,
		changes: changes, snapshots: make(map[string]snapshot, len(inputs)),
		existingDirs: make(map[string]bool), lease: lease,
	}
	for _, path := range inputs {
		full, err := t.securePath(path)
		if err != nil {
			_ = t.releaseLease()
			return nil, err
		}
		info, err := os.Lstat(full)
		if errors.Is(err, fs.ErrNotExist) {
			if containsPath(observed, path) {
				_ = t.releaseLease()
				return nil, fmt.Errorf("observed input %q does not exist", path)
			}
			t.snapshots[path] = snapshot{}
			t.recordExistingParents(path)
			continue
		}
		if err != nil {
			_ = t.releaseLease()
			return nil, err
		}
		if !info.Mode().IsRegular() {
			_ = t.releaseLease()
			return nil, fmt.Errorf("declared path %q is not a regular file", path)
		}
		if err := rejectHardLink(path, info); err != nil {
			_ = t.releaseLease()
			return nil, err
		}
		data, err := os.ReadFile(full)
		if err != nil {
			_ = t.releaseLease()
			return nil, err
		}
		t.snapshots[path] = snapshot{exists: true, data: data, mode: info.Mode().Perm()}
		t.recordExistingParents(path)
	}
	if err := t.prepareJournal(); err != nil {
		_ = t.clearJournal()
		_ = t.releaseLease()
		return nil, err
	}
	return t, nil
}

func canonicalRoot(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("transaction root is not a real directory")
	}
	return filepath.Clean(absolute), nil
}

func validatePlan(plan Plan) ([]string, []string, []string, map[string]Change, error) {
	if len(plan.Declared) == 0 {
		return nil, nil, nil, nil, fmt.Errorf("transaction declares no paths")
	}
	declared := make(map[string]bool, len(plan.Declared))
	for _, path := range plan.Declared {
		if !safeUserPath(path) || declared[path] {
			return nil, nil, nil, nil, fmt.Errorf("invalid or duplicate declared path %q", path)
		}
		declared[path] = true
	}
	observedSet := make(map[string]bool, len(plan.Observed))
	for _, path := range plan.Observed {
		if !safeUserPath(path) || declared[path] || observedSet[path] {
			return nil, nil, nil, nil, fmt.Errorf("invalid, duplicate, or writable observed path %q", path)
		}
		observedSet[path] = true
	}
	changes := make(map[string]Change, len(plan.Changes))
	for _, change := range plan.Changes {
		if !declared[change.Path] || changes[change.Path].Path != "" {
			return nil, nil, nil, nil, fmt.Errorf("change path %q is not declared exactly once", change.Path)
		}
		if change.Delete && change.Data != nil {
			return nil, nil, nil, nil, fmt.Errorf("delete %q has data", change.Path)
		}
		changes[change.Path] = Change{Path: change.Path, Data: append([]byte(nil), change.Data...), Delete: change.Delete}
	}
	if len(changes) != len(declared) {
		return nil, nil, nil, nil, fmt.Errorf("every declared path needs exactly one final state")
	}
	paths := make([]string, 0, len(declared))
	for path := range declared {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	observed := make([]string, 0, len(observedSet))
	for path := range observedSet {
		observed = append(observed, path)
	}
	sort.Strings(observed)
	inputs := append(append([]string(nil), paths...), observed...)
	sort.Strings(inputs)
	return paths, observed, inputs, changes, nil
}

func safePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func safeUserPath(path string) bool {
	return safePath(path) && !metadataPath(path)
}

func containsPath(paths []string, want string) bool {
	index := sort.SearchStrings(paths, want)
	return index < len(paths) && paths[index] == want
}

func (t *transaction) recordExistingParents(path string) {
	parent := filepath.ToSlash(filepath.Dir(path))
	if parent == "." || parent == "/" {
		return
	}
	current := ""
	for _, part := range strings.Split(parent, "/") {
		if current == "" {
			current = part
		} else {
			current += "/" + part
		}
		info, err := os.Lstat(filepath.Join(t.root, filepath.FromSlash(current)))
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			break
		}
		t.existingDirs[current] = true
	}
}

// securePath rejects every existing symlink from root through the target.  It
// is called before each filesystem effect rather than trusting a preflight
// check, so a concurrent symlink swap is detected at the boundary.
func (t *transaction) securePath(path string) (string, error) {
	if !safePath(path) {
		return "", fmt.Errorf("unsafe transaction path %q", path)
	}
	full := filepath.Join(t.root, filepath.FromSlash(path))
	current := t.root
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("transaction path %q crosses symlink", path)
		}
	}
	rel, err := filepath.Rel(t.root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("transaction path %q escapes root", path)
	}
	return full, nil
}

func (t *transaction) apply() error {
	for _, path := range t.paths {
		change := t.changes[path]
		full, err := t.securePath(path)
		if err != nil {
			return err
		}
		if change.Delete {
			if err := os.Remove(full); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := syncDirectory(filepath.Dir(full)); err != nil {
				return err
			}
			continue
		}
		if err := t.ensureParent(filepath.Dir(full)); err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if old := t.snapshots[path]; old.exists {
			mode = old.mode
		}
		if err := install(full, change.Data, mode); err != nil {
			return err
		}
	}
	return nil
}

func (t *transaction) ensureParent(parent string) error {
	rel, err := filepath.Rel(t.root, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("parent escapes transaction root")
	}
	current := t.root
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(current, 0755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent %q is not a real directory", current)
		}
	}
	return nil
}

// install replaces one file through a staged, synced, same-directory rename.
// The caller has already checked path authority and parent safety.
func install(path string, data []byte, mode fs.FileMode) (err error) {
	directory := filepath.Dir(path)
	staged, err := os.CreateTemp(directory, ".flashrefactor-stage-*")
	if err != nil {
		return err
	}
	stage := staged.Name()
	defer func() {
		if staged != nil {
			_ = staged.Close()
		}
		if err != nil {
			_ = os.Remove(stage)
		}
	}()
	if err = staged.Chmod(mode); err != nil {
		return err
	}
	if _, err = staged.Write(data); err != nil {
		return err
	}
	if err = staged.Sync(); err != nil {
		return err
	}
	if err = staged.Close(); err != nil {
		return err
	}
	staged = nil
	if err = os.Rename(stage, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

// syncDirectory asks the filesystem to persist a directory entry.
func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

func (t *transaction) expected() []ExpectedOutput {
	if t.outputs != nil {
		return append([]ExpectedOutput(nil), t.outputs...)
	}
	result := make([]ExpectedOutput, 0, len(t.paths))
	for _, path := range t.paths {
		change := t.changes[path]
		value := ExpectedOutput{Path: path, Deleted: change.Delete}
		if !change.Delete {
			value.SHA256 = digest(change.Data)
		}
		result = append(result, value)
	}
	return result
}

func (t *transaction) verify() error {
	for _, want := range t.expected() {
		full, err := t.securePath(want.Path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(full)
		if want.Deleted {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return err
			}
			return fmt.Errorf("expected deletion of %q, found %s", want.Path, info.Mode())
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("expected regular output %q", want.Path)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if digest(data) != want.SHA256 {
			return fmt.Errorf("output hash mismatch for %q", want.Path)
		}
	}
	return nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
