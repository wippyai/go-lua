package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

// TestEngineFactKeysStayBehindFactkey is deliberately a crude source fence.
// A migrated key literal or resurrected family-prefix identifier in production
// engine code means a caller has started assembling or parsing the wire form
// beside factkey again.
func TestEngineFactKeysStayBehindFactkey(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate engine source")
	}
	directory := filepath.Dir(source)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []*regexp.Regexp{
		regexp.MustCompile("[\"`]heap/"),
		regexp.MustCompile("[\"`]value/"),
		regexp.MustCompile("[\"`](call-result|call-argument|local-call-result)/"),
		regexp.MustCompile("[\"`](type|declared-type|summary-type|method-return-summary)/"),
		regexp.MustCompile("[\"`](branch-proof|front/branch)/"),
		regexp.MustCompile("[\"`](iterator-element|iterator-key|iterator-key-source)/"),
		regexp.MustCompile(`equation\.Guard\{`),
		regexp.MustCompile(`\bheap[A-Za-z0-9_]*Prefix\b`),
		regexp.MustCompile(`factkey\.[A-Za-z0-9_]+\.Prefix`),
		regexp.MustCompile(`strings\.(HasPrefix|CutPrefix|TrimPrefix|Split|SplitN|Cut)\([^\n]*heap`),
	}
	for _, family := range factkey.Families() {
		forbidden = append(forbidden, regexp.MustCompile("[\"`]"+regexp.QuoteMeta(family.Prefix)))
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("read %s: %v", entry.Name(), readErr)
			continue
		}
		for _, pattern := range forbidden {
			if match := pattern.Find(content); match != nil {
				t.Errorf("%s contains forbidden fact-key handling %q", entry.Name(), match)
			}
		}
		if position := directKeyParser(path, content); position != "" {
			t.Errorf("%s contains direct key parser at %s", entry.Name(), position)
		}
	}
	frontPath := filepath.Join(directory, "..", "fixpoint", "front", "front.go")
	front, err := os.ReadFile(frontPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile("[\"`]front/branch/"),
		regexp.MustCompile(`equation\.Guard\{`),
	} {
		if match := pattern.Find(front); match != nil {
			t.Errorf("front.go contains forbidden branch guard handling %q", match)
		}
	}
	exporterPath := filepath.Join(directory, "..", "exporter", "exporter.go")
	exporter, err := os.ReadFile(exporterPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile("[\"`]value/"),
		regexp.MustCompile(`factkey\.Value\.Prefix`),
	} {
		if match := pattern.Find(exporter); match != nil {
			t.Errorf("exporter.go contains forbidden value-key handling %q", match)
		}
	}
}

func directKeyParser(path string, source []byte) string {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, source, 0)
	if err != nil {
		return err.Error()
	}
	parsers := map[string]bool{"HasPrefix": true, "CutPrefix": true, "TrimPrefix": true, "Split": true, "SplitN": true, "Cut": true}
	var found token.Pos
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || found.IsValid() {
			return !found.IsValid()
		}
		function, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		packageName, packageOK := function.X.(*ast.Ident)
		if !packageOK || packageName.Name != "strings" || !parsers[function.Sel.Name] {
			return true
		}
		ast.Inspect(call, func(child ast.Node) bool {
			if selector, ok := child.(*ast.SelectorExpr); ok && selector.Sel.Name == "Key" {
				found = selector.Pos()
				return false
			}
			return true
		})
		return !found.IsValid()
	})
	if !found.IsValid() {
		return ""
	}
	return fileSet.Position(found).String()
}
