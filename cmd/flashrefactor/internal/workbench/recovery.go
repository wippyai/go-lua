package workbench

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/transaction"
)

// InspectRecovery reports the durable recovery state without mutating it.
func (bench Bench) InspectRecovery() (transaction.Recovery, error) {
	return transaction.Inspect(bench.config.Root)
}

// RollbackRecovery is explicit: it restores the durable preimage and never
// guesses that a partially applied cut should be completed.
func (bench Bench) RollbackRecovery() error { return transaction.Rollback(bench.config.Root) }

// CompleteRecovery accepts only transaction's Applied/Verified states. Its
// mandatory postflight reconstructs the original semantic world from the
// durable preimage, then verifies the current physical target exactly as an
// ordinary apply would before releasing the recovery journal.
func (bench Bench) CompleteRecovery(ctx context.Context, lock cutplan.Lock) error {
	if err := cutplan.ValidateLock(lock); err != nil {
		return err
	}
	return transaction.Complete(bench.config.Root, func(preimage transaction.Preimage) error {
		source, cleanup, err := bench.recoverySource(ctx, lock, preimage)
		if err != nil {
			return err
		}
		defer cleanup()
		if err := bench.verifyApplied(ctx, lock, source); err != nil {
			return err
		}
		return transaction.RunGates(bench.config.Root, allTests(lock.Intent))
	})
}

func (bench Bench) recoverySource(ctx context.Context, lock cutplan.Lock, preimage transaction.Preimage) (semantic.Snapshot, func(), error) {
	if err := verifyRecoveryDenominator(lock, preimage); err != nil {
		return semantic.Snapshot{}, nil, err
	}
	shadow, err := os.MkdirTemp(bench.config.Semantic.CacheParent, "flashrefactor-recovery-")
	if err != nil {
		return semantic.Snapshot{}, nil, fmt.Errorf("create recovery shadow: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(shadow) }
	if err := mirrorRecoveryRoot(bench.config.Root, shadow); err != nil {
		cleanup()
		return semantic.Snapshot{}, nil, err
	}
	if err := overlayPreimage(shadow, preimage); err != nil {
		cleanup()
		return semantic.Snapshot{}, nil, err
	}
	config := bench.config.Semantic
	config.Root = shadow
	session, err := semantic.NewSession(config)
	if err != nil {
		cleanup()
		return semantic.Snapshot{}, nil, err
	}
	defer session.Close()
	source, err := session.Collect(ctx, lock.Intent, nil)
	if err != nil {
		cleanup()
		return semantic.Snapshot{}, nil, err
	}
	if err := semantic.VerifyExpected(sourceEvidence(lock.Evidence.Resolution.Objects), source.Objects); err != nil {
		cleanup()
		return semantic.Snapshot{}, nil, fmt.Errorf("recovery source evidence: %w", err)
	}
	toolchain, err := bench.toolchainAt(source)
	if err != nil {
		cleanup()
		return semantic.Snapshot{}, nil, err
	}
	if toolchain != lock.Toolchain {
		cleanup()
		return semantic.Snapshot{}, nil, fmt.Errorf("recovery source toolchain evidence changed")
	}
	return source, cleanup, nil
}

func verifyRecoveryDenominator(lock cutplan.Lock, preimage transaction.Preimage) error {
	allowed := map[string]bool{}
	for _, path := range cutplan.ReadPaths(lock.Intent) {
		allowed[path] = true
	}
	for _, path := range cutplan.WritePaths(lock.Intent) {
		allowed[path] = true
	}
	paths := preimage.Paths()
	if len(paths) != len(allowed) {
		return fmt.Errorf("recovery preimage denominator is incomplete or has undeclared paths")
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if !allowed[path] || seen[path] {
			return fmt.Errorf("recovery preimage has undeclared path: %s", path)
		}
		seen[path] = true
	}
	for path := range allowed {
		if !seen[path] {
			return fmt.Errorf("recovery preimage omits required path: %s", path)
		}
	}
	for _, input := range lock.Evidence.Inputs.Files {
		digest, exists, err := preimage.SHA256(input.Path)
		if err != nil {
			return err
		}
		if !exists || digest != input.SHA256 {
			return fmt.Errorf("recovery preimage does not match locked input: %s", input.Path)
		}
	}
	for _, absent := range lock.Evidence.Inputs.Absent {
		_, exists, err := preimage.Read(absent)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("recovery preimage expected absent path exists: %s", absent)
		}
	}
	return nil
}

func sourceEvidence(values []cutplan.ObjectEvidence) []cutplan.ObjectEvidence {
	result := make([]cutplan.ObjectEvidence, 0, len(values))
	for _, value := range values {
		if value.Role == cutplan.ObjectSource {
			result = append(result, value)
		}
	}
	return result
}

func mirrorRecoveryRoot(root, shadow string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excludedRecoveryPath(relative, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(shadow, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("recovery shadow rejects symlink: %s", relative)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Link(path, target); err == nil {
			return nil
		}
		return copyRecoveryFile(path, target)
	})
}

func excludedRecoveryPath(relative string, directory bool) bool {
	if relative == "." {
		return false
	}
	first := strings.Split(filepath.ToSlash(relative), "/")[0]
	switch first {
	case ".git", ".cache", ".gocache", ".wippy", ".codegraph", ".flashrefactor", "__pycache__", ".pytest_cache", ".idea", ".vscode":
		return true
	}
	return !directory && strings.HasSuffix(first, ".test") && relative == first
}

func copyRecoveryFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func overlayPreimage(shadow string, preimage transaction.Preimage) error {
	paths := preimage.Paths()
	sort.Strings(paths)
	for _, path := range paths {
		if !safeRecoveryPath(path) {
			return fmt.Errorf("recovery preimage path is unsafe: %s", path)
		}
		data, exists, err := preimage.Read(path)
		if err != nil {
			return err
		}
		target := filepath.Join(shadow, filepath.FromSlash(path))
		if !exists {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove shadow preimage %s: %w", path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func safeRecoveryPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.ToSlash(filepath.Clean(path)) != path {
		return false
	}
	return path != "." && !strings.HasPrefix(path, "../") && !strings.Contains(path, "/../")
}
