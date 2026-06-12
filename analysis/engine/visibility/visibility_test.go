package visibility

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
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

func TestResolverKeyAtUsesVisibleVersion(t *testing.T) {
	point := cfg.Point(1)
	sym := symbol.ID(100)
	resolver := NewResolver(NewTable(map[cfg.Point]map[symbol.ID]ssa.Version{
		point: {
			sym: {Root: "x", Symbol: sym, ID: 3},
		},
	}))
	path := pathdom.NewPath(sym, "x").Field("field")

	if got, want := resolver.KeyAt(point, path), pathdom.PathKey("sym100@3.field"); got != want {
		t.Fatalf("KeyAt(versioned path) = %q, want %q", got, want)
	}
}

func TestResolverRejectsMissingVersionAndUnresolvedRoot(t *testing.T) {
	resolver := NewResolver(NewTable(nil))
	if got := resolver.KeyAt(1, pathdom.NewPath(100, "x")); got != "" {
		t.Fatalf("KeyAt without version = %q, want empty", got)
	}
	if got := resolver.KeyAt(1, pathdom.Path{Root: "x"}); got != "" {
		t.Fatalf("KeyAt unresolved root = %q, want empty", got)
	}
	if got := resolver.KeyAt(1, pathdom.Path{}); got != "" {
		t.Fatalf("KeyAt empty = %q, want empty", got)
	}
}

func TestResolverPlaceholderUsesCurrentPathKey(t *testing.T) {
	resolver := NewResolver(nil)
	path := pathdom.NewPlaceholder(0).IndexStr("item")

	if got, want := resolver.KeyAt(1, path), pathdom.PathKey("$0[\"item\"]"); got != want {
		t.Fatalf("KeyAt(placeholder) = %q, want %q", got, want)
	}
}

func TestResolverKeyAtUsesVisibleVersionNotPathSyntaxVersion(t *testing.T) {
	point := cfg.Point(6)
	sym := symbol.ID(101)
	resolver := NewResolver(NewTable(map[cfg.Point]map[symbol.ID]ssa.Version{
		point: {
			sym: {Root: "x", Symbol: sym, ID: 3},
		},
	}))
	path := pathdom.NewPath(sym, "x").Field("field")
	path.Version = 99

	if got, want := resolver.KeyAt(point, path), pathdom.PathKey("sym101@3.field"); got != want {
		t.Fatalf("KeyAt(versioned path) = %q, want visible version key %q", got, want)
	}
}

func TestResolverKeyForVersionUsesExplicitVersion(t *testing.T) {
	resolver := NewResolver(nil)
	path := pathdom.NewPath(100, "x").Field("field")

	if got, want := resolver.KeyForVersion(100, 7, path.Segments), pathdom.PathKey("sym100@7.field"); got != want {
		t.Fatalf("KeyForVersion = %q, want %q", got, want)
	}
	if got := resolver.KeyForVersion(100, 0, path.Segments); got != "" {
		t.Fatalf("KeyForVersion with zero version = %q, want empty", got)
	}
}

func TestResolverKeepsSameSymbolDifferentVersionsDistinctWhileStableIdentityIgnoresVersion(t *testing.T) {
	pointV1 := cfg.Point(10)
	pointV2 := cfg.Point(11)
	sym := symbol.ID(100)
	resolver := NewResolver(NewTable(map[cfg.Point]map[symbol.ID]ssa.Version{
		pointV1: {
			sym: {Root: "x", Symbol: sym, ID: 1},
		},
		pointV2: {
			sym: {Root: "x", Symbol: sym, ID: 2},
		},
	}))

	pathV1 := pathdom.NewPath(sym, "x").Field("field")
	pathV1.Version = 1
	pathV2 := pathdom.NewPath(sym, "x").Field("field")
	pathV2.Version = 2

	if got, want := resolver.KeyAt(pointV1, pathV1), pathdom.PathKey("sym100@1.field"); got != want {
		t.Fatalf("KeyAt(version 1) = %q, want %q", got, want)
	}
	if got, want := resolver.KeyAt(pointV2, pathV2), pathdom.PathKey("sym100@2.field"); got != want {
		t.Fatalf("KeyAt(version 2) = %q, want %q", got, want)
	}
	if resolver.KeyAt(pointV1, pathV2) == resolver.KeyAt(pointV2, pathV1) {
		t.Fatal("different versions resolved to the same local key")
	}

	stableV1, ok := pathaddr.StableOfPath(pathV1)
	if !ok {
		t.Fatal("StableOfPath(version 1) failed")
	}
	stableV2, ok := pathaddr.StableOfPath(pathV2)
	if !ok {
		t.Fatal("StableOfPath(version 2) failed")
	}
	if !stableV1.Equal(stableV2) {
		t.Fatalf("stable identity changed across versions: %s vs %s", stableV1.Key(), stableV2.Key())
	}
	if got := pathaddr.StablePathKey(pathV1); got != pathaddr.StablePathKey(pathV2) {
		t.Fatalf("StablePathKey changed across versions: %q vs %q", pathaddr.StablePathKey(pathV1), pathaddr.StablePathKey(pathV2))
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
