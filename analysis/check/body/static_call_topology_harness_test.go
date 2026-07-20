package body

import (
	"bytes"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

// prepareCallTopologyForest binds stmts with config's declared globals and
// prepares the resulting lexical forest through the production entry point
// (PrepareBoundChunkForest), which seals the call topology as a side effect.
// The seal error is returned unexamined so contract tests can assert on it
// directly instead of failing at setup.
func prepareCallTopologyForest(t testing.TB, stmts []ast.Stmt, config Config) (*StaticForest, *bind.Result, error) {
	t.Helper()
	bindings := bind.BindChunk(stmts, bind.Options{Globals: configGlobals(config)})
	forest, err := PrepareBoundChunkForest(stmts, bindings, config)
	return forest, bindings, err
}

// localFunctionAt returns the function literal assigned by the single-value
// local function declaration at stmts[index] (i.e. "local function name()
// ... end").
func localFunctionAt(t testing.TB, stmts []ast.Stmt, index int) *ast.FunctionExpr {
	t.Helper()
	local, ok := stmts[index].(*ast.LocalAssignStmt)
	if !ok || len(local.Exprs) != 1 {
		t.Fatalf("stmt[%d] = %T, want single-value local function assignment", index, stmts[index])
	}
	fn, ok := local.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("stmt[%d] value = %T, want function literal", index, local.Exprs[0])
	}
	return fn
}

// newCallTopologyTestBuilder reconstructs the staticCallTopologyBuilder
// exactly as sealCallTopology assembles it (static_call_topology.go), without
// running census/collect/solve/freeze, so contract tests can invoke those
// steps directly and inspect the intermediate builder state they leave
// behind.
func newCallTopologyTestBuilder(t testing.TB, forest *StaticForest, bindings *bind.Result) *staticCallTopologyBuilder {
	t.Helper()
	statics := make([]*Static, 0, len(forest.functions)+1)
	if forest.root != nil {
		statics = append(statics, forest.root)
	}
	for _, static := range forest.functions {
		statics = append(statics, static)
	}
	sort.Slice(statics, func(i, j int) bool {
		return bytes.Compare(statics[i].lexicalBodyID[:], statics[j].lexicalBodyID[:]) < 0
	})
	b := &staticCallTopologyBuilder{
		statics:       statics,
		indexByBody:   make(map[lexicalidentity.StableLexicalBodyID]int, len(statics)),
		indexByProto:  make(map[wir.FunctionSymbolID]int, len(forest.functions)),
		locations:     make(map[staticCallLocation][]segment.Segment),
		suffixes:      make(map[string][]segment.Segment),
		copyByRoot:    make(map[string][]staticCallCopy),
		copySeen:      make(map[staticCallCopy]struct{}),
		values:        make(map[staticCallLocation]map[int]struct{}),
		watchers:      make(map[staticCallLocation][]int),
		returnSources: make([][][]staticCallLocation, len(statics)),
		returnOpen:    make([][]staticCallOpenSource, len(statics)),
		adjacency:     make([]map[int]struct{}, len(statics)),
	}
	for index, static := range statics {
		if _, duplicate := b.indexByBody[static.lexicalBodyID]; duplicate {
			t.Fatalf("call topology harness: duplicate lexical body at index %d", index)
		}
		b.indexByBody[static.lexicalBodyID] = index
		b.adjacency[index] = make(map[int]struct{})
	}
	for fn, static := range forest.functions {
		fnSymbol, ok := bindings.FunctionSymbol(fn)
		if !ok || fnSymbol == 0 {
			t.Fatalf("call topology harness: function has no bound symbol")
		}
		index, ok := b.indexByBody[static.lexicalBodyID]
		if !ok {
			t.Fatalf("call topology harness: function body is absent from index")
		}
		b.indexByProto[wir.FunctionSymbolID(fnSymbol)] = index
	}
	return b
}

// findStaticIndex returns the builder-local index for static within b.
func findStaticIndex(t testing.TB, b *staticCallTopologyBuilder, static *Static) int {
	t.Helper()
	index, ok := b.indexByBody[static.lexicalBodyID]
	if !ok {
		t.Fatalf("call topology harness: static is absent from builder index")
	}
	return index
}

// callSiteBySuffix returns the unique site owned by owner whose call source
// has the given suffix.
func callSiteBySuffix(t testing.TB, sites []staticCallSiteBuilder, owner int, suffix string) staticCallSiteBuilder {
	t.Helper()
	var found []staticCallSiteBuilder
	for _, site := range sites {
		if site.owner == owner && site.source.suffix == suffix {
			found = append(found, site)
		}
	}
	if len(found) != 1 {
		t.Fatalf("call sites for owner %d suffix %q = %d, want exactly 1", owner, suffix, len(found))
	}
	return found[0]
}
