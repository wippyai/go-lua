package semantic

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// VirtualFile states one exact post-render source change. Delete is explicit;
// nil Content never means deletion because the semantic loader always receives
// a complete materialized shadow, not a partial overlay.
type VirtualFile struct {
	Path    string
	Content []byte
	Delete  bool
}

func (session *Session) virtualWorkspace(files []VirtualFile) (string, map[string][]byte, func(), error) {
	if len(files) == 0 {
		return session.root, nil, func() {}, nil
	}
	seen := map[string]bool{}
	for _, file := range files {
		full, relative, err := session.overlayPath(file.Path)
		if err != nil {
			return "", nil, nil, fmt.Errorf("virtual file %q: %w", file.Path, err)
		}
		if seen[relative] {
			return "", nil, nil, fmt.Errorf("duplicate virtual file: %s", relative)
		}
		if excludedShadowPath(relative, false) {
			return "", nil, nil, fmt.Errorf("virtual file is outside semantic shadow: %s", relative)
		}
		seen[relative] = true
		if file.Delete {
			if len(file.Content) != 0 {
				return "", nil, nil, fmt.Errorf("deleted virtual file has content: %s", relative)
			}
			if _, err := os.Stat(full); err != nil {
				return "", nil, nil, fmt.Errorf("deleted virtual file must exist: %s", relative)
			}
		}
	}
	shadow, err := os.MkdirTemp(session.scratch, "shadow-")
	if err != nil {
		return "", nil, nil, fmt.Errorf("create post-state shadow: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(shadow) }
	if err := mirrorWorkspace(session.root, shadow, session.scratchParent, session.scratch); err != nil {
		cleanup()
		return "", nil, nil, err
	}
	for _, file := range files {
		_, relative, err := session.overlayPath(file.Path)
		if err != nil {
			cleanup()
			return "", nil, nil, err
		}
		target := filepath.Join(shadow, filepath.FromSlash(relative))
		if file.Delete {
			if err := os.Remove(target); err != nil {
				cleanup()
				return "", nil, nil, fmt.Errorf("delete post-state source %s: %w", relative, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			cleanup()
			return "", nil, nil, fmt.Errorf("create post-state directory %s: %w", relative, err)
		}
		// mirrorWorkspace hard-links immutable preimage files. Break that link
		// before rendering a changed file or the write would truncate source.
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			cleanup()
			return "", nil, nil, fmt.Errorf("detach post-state source %s: %w", relative, err)
		}
		if err := os.WriteFile(target, file.Content, 0o600); err != nil {
			cleanup()
			return "", nil, nil, fmt.Errorf("write post-state source %s: %w", relative, err)
		}
	}
	return shadow, nil, cleanup, nil
}

func mirrorWorkspace(source, destination, scratchParent, scratch string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if path == scratch || path == destination {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, resolveErr := filepath.EvalSymlinks(path)
			if resolveErr != nil {
				return fmt.Errorf("resolve shadow symlink %s: %w", relative, resolveErr)
			}
			if scratchAliasTarget(source, scratchParent, scratch, resolved) {
				// A configured scratch parent may be symlinked below the source
				// root. It is session-owned and must be omitted, never followed
				// or admitted as workspace input.
				return nil
			}
			return fmt.Errorf("shadow workspace rejects symlink: %s", relative)
		}
		if excludedShadowPath(relative, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
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
		return copyRegular(path, target)
	})
}

func scratchAliasTarget(source, parent, scratch, target string) bool {
	// A source-root alias is never session-owned, including when the caller
	// selected the source root itself as scratch parent. Do not turn that alias
	// into an omission just because scratch happens to live below it.
	if target == source {
		return false
	}
	if target == parent || target == scratch {
		return true
	}
	relative, err := filepath.Rel(scratch, target)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func excludedShadowPath(relative string, directory bool) bool {
	if relative == "." {
		return false
	}
	first := relative
	for index, rune := range first {
		if rune == filepath.Separator {
			first = first[:index]
			break
		}
	}
	if first == ".git" || first == ".cache" || first == ".gocache" || first == ".wippy" || first == ".codegraph" || first == ".flashrefactor" || first == "__pycache__" || first == ".pytest_cache" || first == ".idea" || first == ".vscode" || len(first) >= len(".promptmap") && first[:len(".promptmap")] == ".promptmap" {
		return true
	}
	return !directory && relative == first && len(first) > len(".test") && first[len(first)-len(".test"):] == ".test"
}

func copyRegular(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
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
