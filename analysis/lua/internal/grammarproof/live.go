package grammarproof

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func liveFromGrammar(grammar []grammarProduction) []liveProduction {
	live := make([]liveProduction, 0, len(grammar))
	for _, production := range grammar {
		if recoveryOnly(production) {
			continue
		}
		live = append(live, liveProduction{key: grammarKey(production)})
	}
	sort.Slice(live, func(left, right int) bool { return live[left].key < live[right].key })
	return live
}

func allGrammarKeys(grammar []grammarProduction) map[string]bool {
	keys := make(map[string]bool, len(grammar))
	for _, production := range grammar {
		keys[grammarKey(production)] = true
	}
	return keys
}

// recoveryOnly excludes yacc's explicit error alternatives. They are parser
// recovery mechanics, not accepted Lua productions, so no accepted source can
// honestly witness them. The current grammar has none; retaining this filter
// makes that boundary explicit if one is added later.
func recoveryOnly(production grammarProduction) bool {
	for _, symbol := range production.rhs {
		if symbol == "error" {
			return true
		}
	}
	return false
}

func corpus(root string) ([]source, error) {
	all := append([]source(nil), grammarCorpus...)
	fixtureRoot := filepath.Join(root, "testdata", "fixtures")
	if err := filepath.WalkDir(fixtureRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		text, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		all = append(all, source{id: "fixture:" + filepath.ToSlash(relative), text: string(text), required: true})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("read fixture corpus: %w", err)
	}
	sort.Slice(all, func(left, right int) bool { return all[left].id < all[right].id })
	return all, nil
}

func evidenceDigest(grammar []grammarProduction, sources []source) string {
	hash := sha256.New()
	for _, production := range grammar {
		fmt.Fprintf(hash, "%s\x00%s\x00%s\n", grammarKey(production), strings.Join(production.rhs, "\x1f"), production.actionSignature)
	}
	for _, source := range sources {
		fmt.Fprintf(hash, "%s\x00%s\n", source.id, source.text)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
