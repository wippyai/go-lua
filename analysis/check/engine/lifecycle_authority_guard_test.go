package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestLifecycleProtocolsAreRegistryOwned is the displacement guard. Lifecycle
// publications are declared fact families with typestate payloads; engine code
// may neither spell their wire prefixes nor rebuild per-kind state readers and
// escape publishers.
func TestLifecycleProtocolsAreRegistryOwned(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate engine source")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(filename), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	forbiddenPrefixes := map[string]bool{
		"effect.lifecycle.channel/":         true,
		"effect.lifecycle.channel.display/": true,
		"effect.lifecycle.resource/":        true,
	}
	forbiddenFunctions := map[string]bool{
		"channelLifecycleStateFact":  true,
		"channelLifecycleState":      true,
		"channelLifecycleEscape":     true,
		"resourceLifecycleStateFact": true,
		"resourceLifecycleState":     true,
		"resourceLifecycleEscape":    true,
	}
	forbiddenStates := map[string]bool{
		"open": true, "closed": true, "active": true, "committed": true, "escaped": true,
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Errorf("parse %s: %v", path, parseErr)
			continue
		}
		prefixes := make([]string, 0, len(forbiddenPrefixes))
		for prefix := range forbiddenPrefixes {
			prefixes = append(prefixes, prefix)
		}
		if prefix := fenceFoldedStringContains(file, prefixes); prefix != "" {
			t.Errorf("%s restates lifecycle family prefix %q; use factkey", path, prefix)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch typed := node.(type) {
			case *ast.FuncDecl:
				if forbiddenFunctions[typed.Name.Name] {
					t.Errorf("%s retains displaced lifecycle helper %s", path, typed.Name.Name)
				}
				if strings.Contains(strings.ToLower(typed.Name.Name), "lifecycle") ||
					typed.Name.Name == "resourceUnreleasedDiagnostics" {
					ast.Inspect(typed.Body, func(bodyNode ast.Node) bool {
						switch bodyTyped := bodyNode.(type) {
						case *ast.BasicLit:
							if bodyTyped.Kind != token.STRING {
								return true
							}
							value, unquoteErr := strconv.Unquote(bodyTyped.Value)
							if unquoteErr == nil && forbiddenStates[value] {
								t.Errorf("%s restates lifecycle state %q; use typestate's declaration", path, value)
							}
						case *ast.SelectorExpr:
							if bodyTyped.Sel.Name == "LatestValuePrefix" {
								t.Errorf("%s performs a raw latest-prefix lifecycle read; use FamilyValues", path)
							}
						}
						return true
					})
				}
			}
			return true
		})
	}
}

func TestLifecycleFenceRejectsSplitLiteralReconstruction(t *testing.T) {
	file, err := fenceParseSource(`package engine
func resourceUnreleasedDiagnostics() bool {
	return strings.HasPrefix("", "effect.lifecycle." + "resource/")
}`)
	if err != nil {
		t.Fatal(err)
	}
	if got := fenceFoldedStringContains(file, []string{"effect.lifecycle.resource/"}); got == "" {
		t.Fatal("lifecycle ownership predicate accepted split-literal prefix reconstruction")
	}
}
