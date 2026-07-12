package architecture

import (
	"encoding/csv"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// censusHeader is the exact column layout of the semantic surface census.
var censusHeader = []string{
	"package",
	"file",
	"symbol",
	"kind",
	"current_owner",
	"final_owner",
	"classification",
	"semantic_id",
	"schema",
	"differential",
}

const censusUnclassified = "unclassified"

// censusClassifications is the closed classification vocabulary from the
// semantic engine v2 execution plan.
var censusClassifications = map[string]struct{}{
	"retained-lexical":     {},
	"moved-primitive":      {},
	"observation-compiler": {},
	"boundary-provider":    {},
	"deleted":              {},
	censusUnclassified:     {},
}

// censusFamily is one censused package family root, relative to the
// repository root. Every package under the root belongs to the family.
type censusFamily struct {
	Name string
	Root string
}

// censusFamilies lists the package families covered by the census in the
// order used for the migration progress gauge.
var censusFamilies = []censusFamily{
	{Name: "lua/transferfacts", Root: "analysis/lua/transferfacts"},
	{Name: "engine/factflow", Root: "analysis/engine/factflow"},
	{Name: "engine/operationplan", Root: "analysis/engine/operationplan"},
	{Name: "engine/factquery", Root: "analysis/engine/factquery"},
	{Name: "engine/visibility", Root: "analysis/engine/visibility"},
	{Name: "engine/sourcevalue", Root: "analysis/engine/sourcevalue"},
	{Name: "engine/sourceprojection", Root: "analysis/engine/sourceprojection"},
	{Name: "engine/typenarrow", Root: "analysis/engine/typenarrow"},
	{Name: "engine/factapply", Root: "analysis/engine/factapply"},
	{Name: "engine/effectlowering", Root: "analysis/engine/effectlowering"},
	{Name: "engine/callboundary", Root: "analysis/engine/callboundary"},
	{Name: "engine/callpayload", Root: "analysis/engine/callpayload"},
	{Name: "engine/calloutcome", Root: "analysis/engine/calloutcome"},
	{Name: "engine/callproducer", Root: "analysis/engine/callproducer"},
	{Name: "engine/transfer", Root: "analysis/engine/transfer"},
	{Name: "engine/solve", Root: "analysis/engine/solve"},
	{Name: "engine/workplan", Root: "analysis/engine/workplan"},
	{Name: "check/body", Root: "analysis/check/body"},
	{Name: "check/fixpoint", Root: "analysis/check/fixpoint"},
	{Name: "check/readmodel", Root: "analysis/check/readmodel"},
	{Name: "check/internal/readmodel", Root: "analysis/check/internal/readmodel"},
	{Name: "check/obligation", Root: "analysis/check/obligation"},
	{Name: "check/diagnostics", Root: "analysis/check/diagnostics"},
	{Name: "check/placementplan", Root: "analysis/check/placementplan"},
	{Name: "check/exportmanifest", Root: "analysis/check/exportmanifest"},
	{Name: "check/service", Root: "analysis/check/service"},
	{Name: "check/judgment", Root: "analysis/check/judgment"},
}

// censusRow is one (package, file, exported symbol) surface entry.
type censusRow struct {
	Package        string
	File           string
	Symbol         string
	Kind           string
	CurrentOwner   string
	FinalOwner     string
	Classification string
	SemanticID     string
	Schema         string
	Differential   string
}

func (r censusRow) key() string {
	return r.Package + "\x00" + r.File + "\x00" + r.Symbol + "\x00" + r.Kind
}

func (r censusRow) keyString() string {
	return fmt.Sprintf("%s %s %s (%s)", r.Package, r.File, r.Symbol, r.Kind)
}

func (r censusRow) record() []string {
	return []string{
		r.Package,
		r.File,
		r.Symbol,
		r.Kind,
		r.CurrentOwner,
		r.FinalOwner,
		r.Classification,
		r.SemanticID,
		r.Schema,
		r.Differential,
	}
}

func censusRowLess(a, b censusRow) bool {
	if a.Package != b.Package {
		return a.Package < b.Package
	}
	if a.File != b.File {
		return a.File < b.File
	}
	if a.Symbol != b.Symbol {
		return a.Symbol < b.Symbol
	}
	return a.Kind < b.Kind
}

// censusFamilyFor resolves the family owning a census package path. The
// longest matching root wins so nested roots stay distinct.
func censusFamilyFor(pkg string) (censusFamily, bool) {
	var best censusFamily
	found := false
	for _, family := range censusFamilies {
		if pkg != family.Root && !strings.HasPrefix(pkg, family.Root+"/") {
			continue
		}
		if !found || len(family.Root) > len(best.Root) {
			best = family
			found = true
		}
	}
	return best, found
}

// generateSemanticSurfaceCensus walks every census family and emits one row
// per exported symbol in production Go files. Test files are excluded;
// generated zz_* files are surfaces and stay included.
func generateSemanticSurfaceCensus(root string) ([]censusRow, error) {
	fset := token.NewFileSet()
	var rows []censusRow
	for _, family := range censusFamilies {
		familyRoot := filepath.Join(root, filepath.FromSlash(family.Root))
		err := filepath.WalkDir(familyRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			fileRows, err := censusRowsForFile(fset, path, filepath.ToSlash(rel), name)
			if err != nil {
				return err
			}
			rows = append(rows, fileRows...)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk census family %s: %w", family.Name, err)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return censusRowLess(rows[i], rows[j]) })
	return rows, nil
}

// censusRowsForFile extracts exported package-level symbols from one file.
// Methods carry their receiver base type in the symbol as Receiver.Name;
// exported methods on unexported receivers stay included because they carry
// interface-implementation semantics.
func censusRowsForFile(fset *token.FileSet, path, pkg, file string) ([]censusRow, error) {
	parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	var rows []censusRow
	add := func(symbol, kind string) {
		rows = append(rows, censusRow{
			Package:        pkg,
			File:           file,
			Symbol:         symbol,
			Kind:           kind,
			CurrentOwner:   pkg,
			Classification: censusUnclassified,
		})
	}
	for _, decl := range parsed.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				recv := receiverBaseTypeName(d.Recv.List[0].Type)
				if recv == "" {
					return nil, fmt.Errorf("%s: method %s has no resolvable receiver base type", path, d.Name.Name)
				}
				add(recv+"."+d.Name.Name, "method")
				continue
			}
			add(d.Name.Name, "func")
		case *ast.GenDecl:
			switch d.Tok {
			case token.TYPE:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if ts.Name.IsExported() {
						add(ts.Name.Name, "type")
					}
				}
			case token.CONST, token.VAR:
				kind := "const"
				if d.Tok == token.VAR {
					kind = "var"
				}
				for _, spec := range d.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, ident := range vs.Names {
						if ident.IsExported() {
							add(ident.Name, kind)
						}
					}
				}
			}
		}
	}
	return rows, nil
}

// receiverBaseTypeName unwraps pointers, parentheses and type parameters to
// the receiver's base type identifier.
func receiverBaseTypeName(expr ast.Expr) string {
	for {
		switch t := expr.(type) {
		case *ast.StarExpr:
			expr = t.X
		case *ast.ParenExpr:
			expr = t.X
		case *ast.IndexExpr:
			expr = t.X
		case *ast.IndexListExpr:
			expr = t.X
		case *ast.Ident:
			return t.Name
		default:
			return ""
		}
	}
}

func semanticSurfaceCensusPath(root string) string {
	return filepath.Join(root, "analysis", "architecture", "semantic_surface_census.csv")
}

func readSemanticSurfaceCensus(path string) ([]censusRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	// Validate widths ourselves so malformed rows identify their exact CSV row
	// instead of depending on encoding/csv's inferred first-record width.
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s is empty, want header row %v", path, censusHeader)
	}
	header := records[0]
	if len(header) != len(censusHeader) {
		return nil, fmt.Errorf("%s header has %d columns, want %d", path, len(header), len(censusHeader))
	}
	for i, name := range censusHeader {
		if header[i] != name {
			return nil, fmt.Errorf("%s header column %d is %q, want %q", path, i+1, header[i], name)
		}
	}
	rows := make([]censusRow, 0, len(records)-1)
	for i, record := range records[1:] {
		if len(record) != len(censusHeader) {
			return nil, fmt.Errorf("%s row %d has %d columns, want %d", path, i+2, len(record), len(censusHeader))
		}
		rows = append(rows, censusRow{
			Package:        record[0],
			File:           record[1],
			Symbol:         record[2],
			Kind:           record[3],
			CurrentOwner:   record[4],
			FinalOwner:     record[5],
			Classification: record[6],
			SemanticID:     record[7],
			Schema:         record[8],
			Differential:   record[9],
		})
	}
	return rows, nil
}

func writeSemanticSurfaceCensus(path string, rows []censusRow) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(f)
	if err := writer.Write(censusHeader); err != nil {
		f.Close()
		return err
	}
	for _, row := range rows {
		if err := writer.Write(row.record()); err != nil {
			f.Close()
			return err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// mergeSemanticSurfaceCensus carries classification columns from the
// checked-in census onto freshly generated rows, so regeneration never
// discards assigned ownership.
func mergeSemanticSurfaceCensus(generated, existing []censusRow) []censusRow {
	assigned := make(map[string]censusRow, len(existing))
	for _, row := range existing {
		assigned[row.key()] = row
	}
	merged := make([]censusRow, 0, len(generated))
	for _, row := range generated {
		if prev, ok := assigned[row.key()]; ok {
			row.FinalOwner = prev.FinalOwner
			row.Classification = prev.Classification
			row.SemanticID = prev.SemanticID
			row.Schema = prev.Schema
			row.Differential = prev.Differential
		}
		merged = append(merged, row)
	}
	return merged
}
