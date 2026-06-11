package visibility

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestZeroVisibilityReturnsZeroVersion(t *testing.T) {
	var table Table
	if got := table.VisibleVersion(1, 10); got != (ssa.Version{}) {
		t.Fatalf("zero table VisibleVersion = %+v, want zero", got)
	}

	var nilTable *Table
	if got := nilTable.VisibleVersion(1, 10); got != (ssa.Version{}) {
		t.Fatalf("nil table VisibleVersion = %+v, want zero", got)
	}
}

func TestVisibleVersionReturnedForPointAndSymbol(t *testing.T) {
	builder := NewBuilder()
	sym := symbol.ID(10)
	def := builder.Define(1, sym, "x")
	builder.SetVisible(2, sym, def)

	table := builder.Build()
	if got := table.VisibleVersion(2, sym); got != def {
		t.Fatalf("VisibleVersion = %+v, want %+v", got, def)
	}
}

func TestShadowedSymbolsAreDistinct(t *testing.T) {
	builder := NewBuilder()
	outer := symbol.ID(10)
	inner := symbol.ID(11)
	outerVersion := builder.Define(1, outer, "x")
	innerVersion := builder.Define(1, inner, "x")

	table := builder.Build()
	if got := table.VisibleVersion(1, outer); got != outerVersion {
		t.Fatalf("outer VisibleVersion = %+v, want %+v", got, outerVersion)
	}
	if got := table.VisibleVersion(1, inner); got != innerVersion {
		t.Fatalf("inner VisibleVersion = %+v, want %+v", got, innerVersion)
	}
	if outerVersion == innerVersion {
		t.Fatal("shadowed symbols produced identical versions")
	}
}

func TestRootIsDisplayOnlyAndSymbolLookupIsAuthoritative(t *testing.T) {
	point := cfg.Point(3)
	outer := symbol.ID(10)
	inner := symbol.ID(11)

	builder := NewBuilder()
	builder.SetVisible(point, outer, ssa.Version{Root: "x", Symbol: 999, ID: 4})
	builder.SetVisible(point, inner, ssa.Version{Root: "x", Symbol: 999, ID: 5})

	table := builder.Build()
	gotOuter := table.VisibleVersion(point, outer)
	gotInner := table.VisibleVersion(point, inner)

	if gotOuter.Root != "x" || gotInner.Root != "x" {
		t.Fatalf("Root was not preserved for display: outer=%+v inner=%+v", gotOuter, gotInner)
	}
	if gotOuter.Symbol != outer || gotOuter.ID != 4 {
		t.Fatalf("outer lookup = %+v, want symbol %d version 4", gotOuter, outer)
	}
	if gotInner.Symbol != inner || gotInner.ID != 5 {
		t.Fatalf("inner lookup = %+v, want symbol %d version 5", gotInner, inner)
	}
	if got := table.VisibleVersion(point, symbol.ID(999)); got != (ssa.Version{}) {
		t.Fatalf("lookup by embedded non-authoritative symbol = %+v, want zero", got)
	}
}

func TestNewTableClonesPointSymbolMap(t *testing.T) {
	point := cfg.Point(4)
	sym := symbol.ID(20)
	input := map[cfg.Point]map[symbol.ID]ssa.Version{
		point: {
			sym: {Root: "value", ID: 7},
		},
	}

	table := NewTable(input)
	input[point][sym] = ssa.Version{Root: "changed", ID: 8}

	got := table.VisibleVersion(point, sym)
	want := ssa.Version{Root: "value", Symbol: sym, ID: 7}
	if got != want {
		t.Fatalf("VisibleVersion after input mutation = %+v, want %+v", got, want)
	}
}

func TestPackageDoesNotImportLuaPackages(t *testing.T) {
	pkgs, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse package imports: %v", err)
	}

	const forbidden = "github.com/wippyai/go-lua/analysis/lua"
	for _, pkg := range pkgs {
		for filename, file := range pkg.Files {
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					t.Fatalf("unquote import %s in %s: %v", imp.Path.Value, filename, err)
				}
				if strings.HasPrefix(path, forbidden) {
					t.Fatalf("%s imports forbidden Lua package %q", filename, path)
				}
			}
		}
	}
}
