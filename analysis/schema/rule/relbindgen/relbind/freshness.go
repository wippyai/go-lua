package relbind

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Prefix is the file-name prefix every emitted artifact carries. A file in an
// axis package that starts with it and that the corpus does not state is
// stale, and staleness is a refusal rather than a leftover.
const Prefix = "zz_"

// Drift is one artifact whose bytes on disk are not the corpus emission.
type Drift struct {
	Path   string
	Reason string
}

func (drift Drift) String() string { return drift.Path + ": " + drift.Reason }

// Check reports every emitted artifact under root that the corpus would not
// write exactly as it stands. An empty report is the freshness proof.
func Check(root string) ([]Drift, error) {
	artifacts, err := Emit(Declared())
	if err != nil {
		return nil, err
	}
	present, err := emitted(root, Declared().Axes)
	if err != nil {
		return nil, err
	}
	drifts := make([]Drift, 0, len(artifacts))
	for _, artifact := range artifacts {
		on, found := present[artifact.Path()]
		delete(present, artifact.Path())
		if !found {
			drifts = append(drifts, Drift{Path: artifact.Path(), Reason: "the corpus states this artifact and the axis does not carry it"})
			continue
		}
		if !bytes.Equal(on, artifact.Bytes) {
			drifts = append(drifts, Drift{Path: artifact.Path(), Reason: "the checked-in bytes are not the bytes the corpus emits"})
		}
	}
	for path := range present {
		drifts = append(drifts, Drift{Path: path, Reason: "the axis carries this artifact and the corpus does not state it"})
	}
	sort.Slice(drifts, func(left, right int) bool { return drifts[left].Path < drifts[right].Path })
	return drifts, nil
}

// Write renders the corpus under root and removes the artifacts it no longer
// states, so an axis package is the emission and never the emission plus
// history.
func Write(root string) error {
	artifacts, err := Emit(Declared())
	if err != nil {
		return err
	}
	present, err := emitted(root, Declared().Axes)
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		delete(present, artifact.Path())
		directory := filepath.Join(root, filepath.FromSlash(artifact.Dir))
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", artifact.Dir, err)
		}
		if err := os.WriteFile(filepath.Join(directory, artifact.Name), artifact.Bytes, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", artifact.Path(), err)
		}
	}
	stale := make([]string, 0, len(present))
	for path := range present {
		stale = append(stale, path)
	}
	sort.Strings(stale)
	for _, path := range stale {
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			return fmt.Errorf("remove stale %s: %w", path, err)
		}
	}
	return nil
}

func emitted(root string, axes []Axis) (map[string][]byte, error) {
	present := map[string][]byte{}
	for _, axis := range axes {
		directory := filepath.Join(root, filepath.FromSlash(axis.Dir))
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", axis.Dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasPrefix(name, Prefix) || !strings.HasSuffix(name, ".go") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(directory, name))
			if err != nil {
				return nil, fmt.Errorf("read %s/%s: %w", axis.Dir, name, err)
			}
			present[axis.Dir+"/"+name] = content
		}
	}
	return present, nil
}
