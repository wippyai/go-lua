package target

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTargetCountsDoNotFilterTheGlobalCatalog(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location")
	}
	path := filepath.Join(filepath.Dir(current), "counts.go")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, source, 0); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "GeneratedRelationEntries") {
		t.Fatal("target counts filter the global denominator catalog")
	}
}
