package architecture

import (
	"encoding/csv"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The external failure census matrix maps every engine fail-closed diagnostic
// family observed on the external Wippy corpus to a named census test.
// scripts/external_failure_census_check.sh is the forward gate: it normalizes
// harness diagnostics and reports families with no family_pattern row here.

var externalFailureCensusColumns = []string{
	"family_pattern",
	"census_test",
	"census_package",
	"source_count",
	"representative_entry",
}

func TestExternalFailureCensusMatrixIsPresentAndWellFormed(t *testing.T) {
	rows := externalFailureCensusRows(t)
	if len(rows) == 0 {
		t.Fatal("external failure census matrix has no rows")
	}
	patternRow := map[string]int{}
	for i, row := range rows {
		line := i + 2
		for _, name := range externalFailureCensusColumns {
			if row[name] == "" {
				t.Fatalf("row %d column %q is empty", line, name)
			}
		}
		pattern := row["family_pattern"]
		if prev, dup := patternRow[pattern]; dup {
			t.Fatalf("row %d duplicates family_pattern %q from row %d", line, pattern, prev)
		}
		patternRow[pattern] = line
		if strings.ContainsAny(pattern, "0123456789") {
			t.Fatalf("row %d family_pattern %q contains digits; normalization replaces digit runs with N and body hashes with <hash>, so a digit-bearing pattern can never match", line, pattern)
		}
		if strings.ContainsAny(pattern, `",`) {
			t.Fatalf("row %d family_pattern %q contains a comma or quote; the shell gate extracts column 1 with cut -d,", line, pattern)
		}
		if !strings.HasPrefix(row["census_test"], "Test") {
			t.Fatalf("row %d census_test %q is not a Go test function name", line, row["census_test"])
		}
		count, err := strconv.Atoi(row["source_count"])
		if err != nil || count <= 0 {
			t.Fatalf("row %d source_count %q is not a positive integer", line, row["source_count"])
		}
		dir := filepath.Join(repoRoot(t), filepath.FromSlash(row["census_package"]))
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("row %d census_package %q is not a directory under the repo root", line, row["census_package"])
		}
	}
}

func TestExternalFailureCensusTestsExist(t *testing.T) {
	rows := externalFailureCensusRows(t)
	testFuncsByPackage := map[string]map[string]bool{}
	var missing []string
	for i, row := range rows {
		pkg := row["census_package"]
		funcs, ok := testFuncsByPackage[pkg]
		if !ok {
			funcs = packageTestFunctions(t, filepath.Join(repoRoot(t), filepath.FromSlash(pkg)))
			testFuncsByPackage[pkg] = funcs
		}
		if !funcs[row["census_test"]] {
			missing = append(missing, "row "+strconv.Itoa(i+2)+": family "+strconv.Quote(row["family_pattern"])+" expects "+row["census_test"]+" in "+pkg)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("external failure census tests missing:\n  %s\nadd the named census test to the census_package before landing the matrix row", strings.Join(missing, "\n  "))
	}
}

func externalFailureCensusRows(t *testing.T) []map[string]string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "analysis", "architecture", "external_failure_census.csv")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open external failure census matrix: %v", err)
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read external failure census matrix: %v", err)
	}
	if len(records) < 2 {
		t.Fatalf("external failure census matrix has %d rows, want header plus at least one family", len(records))
	}
	header := records[0]
	for _, name := range externalFailureCensusColumns {
		if !csvHeaderHas(header, name) {
			t.Fatalf("external failure census matrix missing required column %q", name)
		}
	}
	rows := make([]map[string]string, 0, len(records)-1)
	for rowIndex, record := range records[1:] {
		if len(record) != len(header) {
			t.Fatalf("row %d has %d fields, want %d", rowIndex+2, len(record), len(header))
		}
		values := map[string]string{}
		for i, name := range header {
			values[name] = record[i]
		}
		rows = append(rows, values)
	}
	return rows
}

func packageTestFunctions(t *testing.T, dir string) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}
	funcs := map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue
			}
			funcs[fn.Name.Name] = true
		}
	}
	return funcs
}
