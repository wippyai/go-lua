package program

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProgramDenominatorDoesNotFilterTheGlobalCatalog(t *testing.T) {
	assertNoGeneratedRelationEntriesCall(t, "denominator.go")
}

func assertNoGeneratedRelationEntriesCall(t *testing.T, name string) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("source location")
	}
	path := filepath.Join(filepath.Dir(current), name)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(source), "GeneratedRelationEntries") {
		t.Fatalf("%s filters the global denominator catalog", file.Name.Name)
	}
}
