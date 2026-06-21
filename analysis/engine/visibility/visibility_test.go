package visibility

import (
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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

func TestResolverStructKeyAtFormatsToKeyAt(t *testing.T) {
	point := cfg.Point(1)
	sym := symbol.ID(100)
	resolver := NewResolver(NewTable(map[cfg.Point]map[symbol.ID]ssa.Version{
		point: {
			sym: {Root: "x", Symbol: sym, ID: 3},
		},
	}))
	ks := resolver.KeySpace()

	paths := []pathdom.Path{
		pathdom.NewPath(sym, "x"),
		pathdom.NewPath(sym, "x").Field("field"),
		pathdom.NewPath(sym, "x").Field("a").Field("b"),
		pathdom.NewPath(sym, "x").IndexStr("k"),
		pathdom.NewPath(sym, "x").IndexInt(2),
	}
	for _, p := range paths {
		want := resolver.KeyAt(point, p)
		if want == "" {
			t.Fatalf("KeyAt(%v) empty, want resolvable", p)
		}
		got := resolver.StructKeyAt(point, p)
		if formatted := ks.Format(got); formatted != want {
			t.Fatalf("Format(StructKeyAt(%v)) = %q, want KeyAt %q", p, formatted, want)
		}
	}

	// Unresolved and non-point-local paths yield the invalid key (Format "").
	if got := resolver.StructKeyAt(point, pathdom.Path{}); ks.Format(got) != "" {
		t.Fatalf("StructKeyAt(empty) Format = %q, want empty", ks.Format(got))
	}
	if got := resolver.StructKeyAt(point, pathdom.NewPlaceholder(0).Field("item")); ks.Format(got) != "" {
		t.Fatalf("StructKeyAt(placeholder) Format = %q, want empty (not a point-local value-lane key)", ks.Format(got))
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

func TestRootOrVisibleKeyAtPolicy(t *testing.T) {
	point := cfg.Point(17)
	root := pathdom.NewPath(42, "x")
	member := root.Field("field")
	zeroSymbol := pathdom.Path{Root: "x"}
	calls := 0
	resolver := recordingPathKeyResolver{
		key: pathdom.PathKey("sym42@17.field"),
		onKeyAt: func(gotPoint cfg.Point, gotPath pathdom.Path) {
			calls++
			if gotPoint != point {
				t.Fatalf("resolver point = %v, want %v", gotPoint, point)
			}
			if gotPath.Key() != member.Key() {
				t.Fatalf("resolver path key = %q, want %q", gotPath.Key(), member.Key())
			}
			if gotPath.Symbol != member.Symbol || gotPath.Root != member.Root || gotPath.Version != member.Version {
				t.Fatalf("resolver path = %#v, want %#v", gotPath, member)
			}
		},
	}

	if got := RootOrVisibleKeyAt(resolver, point, pathdom.Path{}); got != "" {
		t.Fatalf("empty path key = %q, want empty", got)
	}
	if got := RootOrVisibleKeyAt(resolver, point, zeroSymbol); got != "" {
		t.Fatalf("symbol-zero path key = %q, want empty", got)
	}
	if got := RootOrVisibleKeyAt(nil, point, root); got != root.Key() {
		t.Fatalf("root path key = %q, want %q", got, root.Key())
	}
	if got := RootOrVisibleKeyAt(resolver, point, member); got != pathdom.PathKey("sym42@17.field") {
		t.Fatalf("member path key = %q, want sym42@17.field", got)
	}
	if calls != 1 {
		t.Fatalf("resolver KeyAt calls = %d, want 1", calls)
	}
}

type recordingPathKeyResolver struct {
	key     pathdom.PathKey
	onKeyAt func(cfg.Point, pathdom.Path)
}

func (r recordingPathKeyResolver) KeyAt(point cfg.Point, p pathdom.Path) pathdom.PathKey {
	if r.onKeyAt != nil {
		r.onKeyAt(point, p)
	}
	return r.key
}

func (r recordingPathKeyResolver) KeySpace() *keyspace.KeySpace {
	return keyspace.New()
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
	if got := stableV1.Key(); got != stableV2.Key() {
		t.Fatalf("stable key changed across versions: %q vs %q", stableV1.Key(), stableV2.Key())
	}
}

func TestBuildForwardPropagatesDefinitionToSuccessorPoints(t *testing.T) {
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	branch := graph.AddBranch()
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	sym := symbol.ID(200)
	table := BuildForward(BuildConfig{
		Graph: graph,
		Definitions: []Definition{
			{Point: assign, Symbol: sym, Root: "result"},
		},
	})
	want := ssa.Version{Root: "result", Symbol: sym, ID: 1}

	for _, point := range []cfg.Point{assign, branch, thenPoint, elsePoint, graph.Exit()} {
		if got := table.VisibleVersion(point, sym); got != want {
			t.Fatalf("VisibleVersion(%d) = %+v, want %+v", point, got, want)
		}
	}
	if got := table.VisibleVersion(graph.Entry(), sym); got != (ssa.Version{}) {
		t.Fatalf("entry VisibleVersion = %+v, want zero before definition", got)
	}
}

func TestBuildForwardRedefinitionDoesNotMutatePredecessorSnapshot(t *testing.T) {
	graph := cfg.New()
	firstAssign := graph.AddNode(cfg.NodeAssign)
	secondAssign := graph.AddNode(cfg.NodeAssign)
	after := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), firstAssign, false)
	graph.AddEdge(firstAssign, secondAssign, false)
	graph.AddEdge(secondAssign, after, false)
	graph.AddEdge(after, graph.Exit(), false)

	sym := symbol.ID(204)
	table := BuildForward(BuildConfig{
		Graph: graph,
		Definitions: []Definition{
			{Point: firstAssign, Symbol: sym, Root: "value"},
			{Point: secondAssign, Symbol: sym, Root: "value"},
		},
	})

	first := table.VisibleVersion(firstAssign, sym)
	second := table.VisibleVersion(secondAssign, sym)
	afterVersion := table.VisibleVersion(after, sym)

	if first != (ssa.Version{Root: "value", Symbol: sym, ID: 1}) {
		t.Fatalf("first assignment version = %+v, want id 1", first)
	}
	if second != (ssa.Version{Root: "value", Symbol: sym, ID: 2}) {
		t.Fatalf("second assignment version = %+v, want id 2", second)
	}
	if afterVersion != second {
		t.Fatalf("after version = %+v, want propagated second version %+v", afterVersion, second)
	}
}

func TestBuildForwardCreatesStableJoinVersionForDifferentIncomingDefinitions(t *testing.T) {
	graph := cfg.New()
	branch := graph.AddBranch()
	thenAssign := graph.AddNode(cfg.NodeAssign)
	elseAssign := graph.AddNode(cfg.NodeAssign)
	join := graph.AddNode(cfg.NodeJoin)
	after := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenAssign, true)
	graph.AddEdge(branch, elseAssign, false)
	graph.AddEdge(thenAssign, join, false)
	graph.AddEdge(elseAssign, join, false)
	graph.AddEdge(join, after, false)
	graph.AddEdge(after, graph.Exit(), false)

	sym := symbol.ID(201)
	table := BuildForward(BuildConfig{
		Graph: graph,
		Definitions: []Definition{
			{Point: thenAssign, Symbol: sym, Root: "value"},
			{Point: elseAssign, Symbol: sym, Root: "value"},
		},
	})

	thenVersion := table.VisibleVersion(thenAssign, sym)
	elseVersion := table.VisibleVersion(elseAssign, sym)
	joinVersion := table.VisibleVersion(join, sym)
	afterVersion := table.VisibleVersion(after, sym)

	if thenVersion.ID == 0 || elseVersion.ID == 0 || joinVersion.ID == 0 {
		t.Fatalf("versions = then %+v else %+v join %+v, want nonzero", thenVersion, elseVersion, joinVersion)
	}
	if thenVersion == elseVersion {
		t.Fatalf("branch definitions share version %+v, want distinct", thenVersion)
	}
	if joinVersion == thenVersion || joinVersion == elseVersion {
		t.Fatalf("join version %+v reused incoming versions then=%+v else=%+v", joinVersion, thenVersion, elseVersion)
	}
	if afterVersion != joinVersion {
		t.Fatalf("after VisibleVersion = %+v, want propagated join version %+v", afterVersion, joinVersion)
	}
}

func TestBuildForwardDoesNotCreateLoopPhiWithoutBackedgeRedefinition(t *testing.T) {
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	header := graph.AddBranch()
	body := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, header, false)
	graph.AddEdge(header, body, true)
	graph.AddEdge(body, header, false)
	graph.AddEdge(header, graph.Exit(), false)

	sym := symbol.ID(202)
	table := BuildForward(BuildConfig{
		Graph: graph,
		Definitions: []Definition{
			{Point: assign, Symbol: sym, Root: "item"},
		},
	})
	want := ssa.Version{Root: "item", Symbol: sym, ID: 1}

	for _, point := range []cfg.Point{assign, header, body, graph.Exit()} {
		if got := table.VisibleVersion(point, sym); got != want {
			t.Fatalf("VisibleVersion(%d) = %+v, want %+v", point, got, want)
		}
	}
}

func TestVersionMergeIgnoresRootTextWhenIdentityMatches(t *testing.T) {
	sym := symbol.ID(203)

	left := map[symbol.ID]ssa.Version{
		sym: {Root: "left-root", Symbol: sym, ID: 7},
	}
	right := map[symbol.ID]ssa.Version{
		sym: {Root: "right-root", Symbol: sym, ID: 7},
	}
	if !versionMapsEqual(left, right) {
		t.Fatalf("versionMapsEqual(%+v, %+v) = false, want true", left[sym], right[sym])
	}

	graph := cfg.New()
	leftNode := graph.AddNode(cfg.NodeNoop)
	rightNode := graph.AddNode(cfg.NodeNoop)
	joinNode := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), leftNode, false)
	graph.AddEdge(graph.Entry(), rightNode, false)
	graph.AddEdge(leftNode, joinNode, false)
	graph.AddEdge(rightNode, joinNode, false)
	graph.AddEdge(joinNode, graph.Exit(), false)

	merged := mergePredecessors(
		graph,
		joinNode,
		map[cfg.Point]map[symbol.ID]ssa.Version{
			leftNode:  left,
			rightNode: right,
		},
		map[cfg.Point]struct{}{
			leftNode:  {},
			rightNode: {},
		},
		map[lookup]ssa.Version{},
		map[symbol.ID]int{sym: 7},
	)

	got, ok := merged[sym]
	if !ok {
		t.Fatal("mergePredecessors dropped semantically identical incoming version")
	}
	if got.Symbol != sym || got.ID != 7 {
		t.Fatalf("merged version = %+v, want symbol %d version 7", got, sym)
	}
	if got.Root != left[sym].Root && got.Root != right[sym].Root {
		t.Fatalf("merged root = %q, want one of the incoming display roots", got.Root)
	}
	if len(merged) != 1 {
		t.Fatalf("merged symbol count = %d, want 1", len(merged))
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
