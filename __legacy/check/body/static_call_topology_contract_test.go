package body

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TestStaticCallTopologyCollectObservesDirectMethodAndSuffixCallLocations
// exercises census/collect/collectCalls/observeOperand/operandLocation/
// observeLocation/observeSuffix directly against a two-function forest whose
// caller performs one direct call, one method call, and one dot-suffix call.
func TestStaticCallTopologyCollectObservesDirectMethodAndSuffixCallLocations(t *testing.T) {
	config := Config{Registry: standard.Registry()}
	stmts := parseChunk(t, `
local function helper()
	return 1
end
local t = {}
local u = {}
local function main()
	helper()
	t:method_call()
	u.member()
end
`)
	forest, bindings, err := prepareCallTopologyForest(t, stmts, config)
	if err != nil {
		t.Fatalf("prepareCallTopologyForest: %v", err)
	}
	helperFn := localFunctionAt(t, stmts, 0)
	mainFn := localFunctionAt(t, stmts, 3)
	helperStatic, mainStatic := forest.Function(helperFn), forest.Function(mainFn)
	if helperStatic == nil || mainStatic == nil {
		t.Fatalf("forest is missing helper or main static")
	}

	b := newCallTopologyTestBuilder(t, forest, bindings)
	if _, err := b.census(); err != nil {
		t.Fatalf("census: %v", err)
	}
	if err := b.collect(); err != nil {
		t.Fatalf("collect: %v", err)
	}

	mainIndex := findStaticIndex(t, b, mainStatic)
	helperIndex := findStaticIndex(t, b, helperStatic)

	direct := callSiteBySuffix(t, b.sites, mainIndex, "")
	if direct.exactTarget != helperIndex {
		t.Fatalf("direct call exactTarget = %d, want helper index %d", direct.exactTarget, helperIndex)
	}
	if direct.refine {
		t.Fatalf("direct call to a known lexical function must not need refinement")
	}

	methodSuffix := segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentField, Name: "method_call"}})
	method := callSiteBySuffix(t, b.sites, mainIndex, methodSuffix)
	if method.exactTarget != -1 || !method.refine {
		t.Fatalf("method call site = %+v, want unresolved receiver requiring refinement", method)
	}
	if canonical := b.suffixes[methodSuffix]; len(canonical) != 1 || canonical[0].Name != "method_call" {
		t.Fatalf("method call suffix canonical segments = %v, want single field method_call", canonical)
	}

	fieldSuffix := segment.FormatSegments([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	field := callSiteBySuffix(t, b.sites, mainIndex, fieldSuffix)
	if field.exactTarget != -1 || !field.refine {
		t.Fatalf("suffix call site = %+v, want unresolved callee requiring refinement", field)
	}
	if field.source.root == "" || field.source.root == method.source.root {
		t.Fatalf("suffix call and method call must observe distinct non-empty receiver locations, got %q and %q", field.source.root, method.source.root)
	}
	if direct.source.root == "" {
		t.Fatalf("direct call must observe a non-empty callee location")
	}
}

// boundaryGlobalContract locates the contract product.Value published for a
// named global inside one body's frozen boundary.
func boundaryGlobalContract(t testing.TB, bindings *bind.Result, globals []symbol.ID, contracts []product.Value, name string) product.Value {
	t.Helper()
	for index, global := range globals {
		if bindings.Name(global) == name {
			return contracts[index]
		}
	}
	t.Fatalf("boundary globals %v missing %q", globals, name)
	return product.Value{}
}

// TestStaticCallTopologyCensusSharedGlobalCarrierIsConsistent is the green
// twin of TestStaticCallTopologyGlobalCarrierContractsConsistentAcrossSharedGlobalReaders
// below: a helper and its caller both read the same declared ambient global
// and nothing else. The call topology boundary census must publish the exact
// same carrier contract for that global on both bodies.
func TestStaticCallTopologyCensusSharedGlobalCarrierIsConsistent(t *testing.T) {
	reg := standard.Registry()
	config := Config{
		Registry:    reg,
		GlobalTypes: map[string]typ.Type{"mystr": typ.String},
		Signatures:  signaturelookup.Source{IncludeStdlib: true},
	}
	stmts := parseChunk(t, `
local function helper()
	local m = mystr
end
local function main()
	helper()
	local m = mystr
end
main()
`)
	forest, bindings, err := prepareCallTopologyForest(t, stmts, config)
	if err != nil {
		t.Fatalf("prepareCallTopologyForest: %v", err)
	}
	helperFn := localFunctionAt(t, stmts, 0)
	mainFn := localFunctionAt(t, stmts, 1)
	helperStatic, mainStatic := forest.Function(helperFn), forest.Function(mainFn)
	if helperStatic == nil || mainStatic == nil {
		t.Fatalf("forest is missing helper or main static")
	}

	topology := forest.CallTopology()
	_, helperGlobals, helperContracts, ok := topology.Boundary(helperStatic.lexicalBodyID)
	if !ok {
		t.Fatalf("call topology has no boundary for helper")
	}
	_, mainGlobals, mainContracts, ok := topology.Boundary(mainStatic.lexicalBodyID)
	if !ok {
		t.Fatalf("call topology has no boundary for main")
	}

	helperContract := boundaryGlobalContract(t, bindings, helperGlobals, helperContracts, "mystr")
	mainContract := boundaryGlobalContract(t, bindings, mainGlobals, mainContracts, "mystr")
	if !product.Equal(reg, helperContract, mainContract) {
		t.Fatalf("mystr carrier contract differs between helper and main: %v vs %v", helperContract, mainContract)
	}
}

// TestStaticCallTopologyGlobalCarrierContractsConsistentAcrossSharedGlobalReaders
// mirrors, at the body-package grain, the forest-level reproducer
// TestExternalCensusGlobalCarrierConflict in
// analysis/check/fixpoint/program/external_forest_census_test.go: a helper
// and its caller both read the same declared ambient global (mystr), and the
// caller additionally reads an interface-typed global (errors) after it.
// This must not produce "prepare lexical forest: global carrier contracts
// conflict" for the shared mystr carrier.
//
// This is a known-red twin of the census family: PrepareBoundChunkForest
// currently fails here exactly as CheckChunk does at the fixpoint layer. Do
// not relax this assertion to tolerate the error.
func TestStaticCallTopologyGlobalCarrierContractsConsistentAcrossSharedGlobalReaders(t *testing.T) {
	luaError := typ.NewInterface("Error", []typ.Method{
		{Name: "kind", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	config := Config{
		Registry: standard.Registry(),
		GlobalTypes: map[string]typ.Type{
			"mystr":  typ.String,
			"errors": luaError,
		},
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	}
	stmts := parseChunk(t, `
local function helper()
	local m = mystr
end
local function main()
	helper()
	local m = mystr
	local e = errors
end
main()
`)
	_, _, err := prepareCallTopologyForest(t, stmts, config)
	if err != nil {
		t.Fatalf("prepareCallTopologyForest: %v, want consistent mystr carrier contracts across helper and main", err)
	}
}
