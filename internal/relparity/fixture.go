package relparity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FixtureRelativeRoot is the checked-in corpus directory both sides read.
const FixtureRelativeRoot = "testdata/fixtures"

// ListFixtures enumerates the fixture names under a checkout, in canonical
// order: every directory below testdata/fixtures that holds at least one .lua
// file, named by its slash-separated path relative to that root.
//
// Enumeration is a directory listing rather than a call into the analyzer's
// corpus loader, because the harness holds the no-runtime fence: it produces
// argv for two foreign binaries and never loads a fixture itself.
func ListFixtures(checkout string) ([]string, error) {
	root := filepath.Join(checkout, FixtureRelativeRoot)
	seen := make(map[string]struct{})
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		relative, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		seen[filepath.ToSlash(relative)] = struct{}{}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("relparity: walk fixture corpus %s: %w", root, err)
	}
	fixtures := make([]string, 0, len(seen))
	for name := range seen {
		fixtures = append(fixtures, name)
	}
	sort.Strings(fixtures)
	return fixtures, nil
}

// ReadFixtureList reads a newline-separated fixture list from a file, ignoring
// blank lines and # comments, and returns it in canonical order.
func ReadFixtureList(path string) ([]string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("relparity: read fixture list %s: %w", path, err)
	}
	var fixtures []string
	for _, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fixtures = append(fixtures, line)
	}
	sort.Strings(fixtures)
	return fixtures, nil
}

// Shard selects one deterministic slice of a canonically ordered fixture list.
// Shards partition the list: every fixture belongs to exactly one shard, and
// the assignment depends only on a fixture's position in the sorted list, so
// two hosts running shard i of n compare the same fixtures.
func Shard(fixtures []string, index, count int) ([]string, error) {
	if count < 1 {
		return nil, fmt.Errorf("relparity: shard count %d is not positive", count)
	}
	if index < 0 || index >= count {
		return nil, fmt.Errorf("relparity: shard index %d outside [0,%d)", index, count)
	}
	var selected []string
	for position, fixture := range fixtures {
		if position%count == index {
			selected = append(selected, fixture)
		}
	}
	return selected, nil
}

// FixtureListDigest is the identity of the compared corpus. Both sides read
// one fixture tree, so a report carries this digest to state which corpus the
// comparison ranged over.
func FixtureListDigest(fixtures []string) string {
	hash := sha256.New()
	for _, fixture := range fixtures {
		hash.Write([]byte(fixture))
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
