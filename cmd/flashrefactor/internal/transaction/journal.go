package transaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

const (
	journalFile       = "state.json"
	preimageDirectory = "preimage"
	journalVersion    = 1
)

// RecoveryState says exactly what the durable transaction observed last. A
// state is never inferred from partially installed source files.
type RecoveryState string

const (
	RecoveryPrepared   RecoveryState = "prepared"
	RecoveryApplying   RecoveryState = "applying"
	RecoveryApplied    RecoveryState = "applied"
	RecoveryVerified   RecoveryState = "verified"
	RecoveryRecovering RecoveryState = "recovering"
	RecoveryRolledBack RecoveryState = "rolled_back"
)

// ErrNoRecovery means no durable crash record exists at the supplied root.
var ErrNoRecovery = errors.New("no flashrefactor transaction recovery record")

// Preimage is an immutable copy of every writable and read-only transaction
// input. Its methods return copies, so an in-lease guard cannot change what
// the transaction will later check or restore.
type Preimage struct {
	entries map[string]snapshot
	paths   []string
}

// Paths returns lexically ordered repository-relative input paths.
func (p Preimage) Paths() []string { return append([]string(nil), p.paths...) }

// Read returns a copy of an input's bytes and whether the input existed at
// transaction start. An unknown path is an error; guards must not quietly
// treat omitted authority as an empty input.
func (p Preimage) Read(path string) ([]byte, bool, error) {
	entry, ok := p.entries[path]
	if !ok {
		return nil, false, fmt.Errorf("path %q is not in the transaction preimage", path)
	}
	return append([]byte(nil), entry.data...), entry.exists, nil
}

// SHA256 returns the exact starting digest for an existing input.
func (p Preimage) SHA256(path string) (string, bool, error) {
	entry, ok := p.entries[path]
	if !ok {
		return "", false, fmt.Errorf("path %q is not in the transaction preimage", path)
	}
	if !entry.exists {
		return "", false, nil
	}
	return digest(entry.data), true, nil
}

type journal struct {
	Version      int              `json:"version"`
	State        RecoveryState    `json:"state"`
	Entries      []journalEntry   `json:"entries"`
	Outputs      []ExpectedOutput `json:"outputs"`
	ExistingDirs []string         `json:"existing_dirs"`
}

type journalEntry struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
	Mode   uint32 `json:"mode,omitempty"`
	Blob   string `json:"blob,omitempty"`
}

func (t *transaction) preimage() Preimage {
	entries := make(map[string]snapshot, len(t.snapshots))
	for path, entry := range t.snapshots {
		entries[path] = snapshot{exists: entry.exists, data: append([]byte(nil), entry.data...), mode: entry.mode}
	}
	return Preimage{entries: entries, paths: append([]string(nil), t.inputs...)}
}

func (t *transaction) prepareJournal() error {
	metadata := filepath.Join(t.root, metadataDirectory)
	preimages := filepath.Join(metadata, preimageDirectory)
	if err := os.Mkdir(preimages, 0700); err != nil {
		return err
	}
	if err := syncDirectory(metadata); err != nil {
		return err
	}
	entries := make([]journalEntry, 0, len(t.inputs))
	for index, path := range t.inputs {
		source := t.snapshots[path]
		entry := journalEntry{Path: path, Exists: source.exists}
		if source.exists {
			entry.SHA256 = digest(source.data)
			entry.Mode = uint32(source.mode)
			entry.Blob = fmt.Sprintf("%s/%06d", preimageDirectory, index)
			if err := install(filepath.Join(metadata, filepath.FromSlash(entry.Blob)), source.data, 0600); err != nil {
				return err
			}
		}
		entries = append(entries, entry)
	}
	if err := syncDirectory(preimages); err != nil {
		return err
	}
	existing := make([]string, 0, len(t.existingDirs))
	for path := range t.existingDirs {
		existing = append(existing, path)
	}
	sort.Strings(existing)
	t.journal = journal{
		Version: journalVersion, State: RecoveryPrepared, Entries: entries,
		Outputs: t.expected(), ExistingDirs: existing,
	}
	return t.writeJournal()
}

func (t *transaction) setJournalState(state RecoveryState) error {
	t.journal.State = state
	return t.writeJournal()
}

func (t *transaction) writeJournal() error {
	if err := validateJournal(t.journal); err != nil {
		return err
	}
	data, err := json.Marshal(t.journal)
	if err != nil {
		return err
	}
	return install(filepath.Join(t.root, metadataDirectory, journalFile), append(data, '\n'), 0600)
}

func (t *transaction) clearJournal() error { return clearJournal(t.root) }

func clearJournal(root string) error {
	metadata := filepath.Join(root, metadataDirectory)
	state := filepath.Join(metadata, journalFile)
	preimages := filepath.Join(metadata, preimageDirectory)
	if info, err := os.Lstat(metadata); err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("transaction metadata is not a real directory")
	}
	if err := removePreimageFiles(preimages); err != nil {
		return err
	}
	if info, err := os.Lstat(state); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("transaction journal is not a regular file")
		}
		if err := os.Remove(state); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDirectory(metadata)
}

func removePreimageFiles(directory string) error {
	info, err := os.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("transaction preimage directory is not a real directory")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !safeBlobName(entry.Name()) {
			return fmt.Errorf("invalid transaction preimage entry %q", entry.Name())
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	if err := os.Remove(directory); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(directory))
}

func readJournal(root string) (journal, error) {
	data, err := os.ReadFile(filepath.Join(root, metadataDirectory, journalFile))
	if errors.Is(err, fs.ErrNotExist) {
		return journal{}, ErrNoRecovery
	}
	if err != nil {
		return journal{}, err
	}
	var value journal
	if err := json.Unmarshal(data, &value); err != nil {
		return journal{}, fmt.Errorf("decode transaction journal: %w", err)
	}
	if err := validateJournal(value); err != nil {
		return journal{}, err
	}
	return value, nil
}

func validateJournal(value journal) error {
	if value.Version != journalVersion || !validRecoveryState(value.State) || len(value.Entries) == 0 || len(value.Outputs) == 0 {
		return fmt.Errorf("invalid transaction journal")
	}
	previous := ""
	entries := make(map[string]bool, len(value.Entries))
	for index, entry := range value.Entries {
		if !safeUserPath(entry.Path) || entry.Path <= previous || entries[entry.Path] {
			return fmt.Errorf("invalid transaction journal input %q", entry.Path)
		}
		previous = entry.Path
		entries[entry.Path] = true
		if entry.Exists {
			if len(entry.SHA256) != 64 || entry.Blob != fmt.Sprintf("%s/%06d", preimageDirectory, index) {
				return fmt.Errorf("invalid transaction journal preimage %q", entry.Path)
			}
		} else if entry.SHA256 != "" || entry.Mode != 0 || entry.Blob != "" {
			return fmt.Errorf("invalid absent transaction journal input %q", entry.Path)
		}
	}
	previous = ""
	for _, output := range value.Outputs {
		if !safeUserPath(output.Path) || output.Path <= previous || !entries[output.Path] {
			return fmt.Errorf("invalid transaction journal output %q", output.Path)
		}
		previous = output.Path
		if output.Deleted {
			if output.SHA256 != "" {
				return fmt.Errorf("invalid deleted transaction journal output %q", output.Path)
			}
		} else if len(output.SHA256) != 64 {
			return fmt.Errorf("invalid transaction journal output digest %q", output.Path)
		}
	}
	previous = ""
	for _, path := range value.ExistingDirs {
		if !safePath(path) || metadataPath(path) || path <= previous {
			return fmt.Errorf("invalid transaction journal directory %q", path)
		}
		previous = path
	}
	return nil
}

func validRecoveryState(value RecoveryState) bool {
	switch value {
	case RecoveryPrepared, RecoveryApplying, RecoveryApplied, RecoveryVerified, RecoveryRecovering, RecoveryRolledBack:
		return true
	default:
		return false
	}
}

func safeBlobName(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func journalSnapshots(root string, value journal) (map[string]snapshot, error) {
	result := make(map[string]snapshot, len(value.Entries))
	for _, entry := range value.Entries {
		if !entry.Exists {
			result[entry.Path] = snapshot{}
			continue
		}
		full := filepath.Join(root, metadataDirectory, filepath.FromSlash(entry.Blob))
		info, err := os.Lstat(full)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("transaction preimage %q is not a regular file", entry.Path)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return nil, err
		}
		if digest(data) != entry.SHA256 {
			return nil, fmt.Errorf("transaction preimage digest mismatch for %q", entry.Path)
		}
		result[entry.Path] = snapshot{exists: true, data: data, mode: fs.FileMode(entry.Mode)}
	}
	return result, nil
}
