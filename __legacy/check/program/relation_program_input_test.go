package program

import (
	"bytes"
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestRelationProgramInputUsesPreparedLexicalForestAndOwnedCallSurfaces(t *testing.T) {
	reg := standard.Registry()
	const source = `
local function leaf(value: string): string
	return value
end
local function caller(value: string): string
	return leaf(value)
end
return caller("ok")
`
	stmts := parseRelationProgramInputChunk(t, source)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatalf("prepareBoundChunkBodies: %v", err)
	}

	units, err := relationProgramInput(prepared, relationProgramFactories(t, prepared, nil), nil)
	if err != nil {
		t.Fatalf("relationProgramInput: %v", err)
	}
	if len(units) != 3 {
		t.Fatalf("relation input units = %d, want chunk plus two lexical functions", len(units))
	}
	bodies := make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(units))
	for index, unit := range units {
		if index > 0 && bytes.Compare(units[index-1].Body[:], unit.Body[:]) >= 0 {
			t.Fatalf("relation input body order is not canonical at %d", index)
		}
		if unit.Registry != reg || unit.KeySpace == nil || unit.Graph == nil || unit.Plan == nil {
			t.Fatalf("relation input body %x lost prepared ownership", unit.Body)
		}
		if unit.Shape.Params != uint32(len(unit.Plan.BoundaryParams())) ||
			unit.Shape.Captures != uint32(len(unit.Plan.BoundaryCaptures())) ||
			unit.Shape.Globals != uint32(len(unit.Plan.BoundaryGlobals())) {
			t.Fatalf("relation input body %x shape drifted from plan boundary", unit.Body)
		}
		surface, exact := unit.Plan.CallSurface()
		if !exact || !surface.Complete() || surface.Owner() != unit.Body || surface.PointCount() != unit.Graph.Size() {
			t.Fatalf("relation input body %x call surface is not exact and owner-bound", unit.Body)
		}
		bodies[unit.Body] = struct{}{}
	}
	for _, unit := range units {
		surface, _ := unit.Plan.CallSurface()
		for _, site := range surface.Sites() {
			if site.Target.Kind() != operationplan.CallSurfaceTargetLexical {
				continue
			}
			target, ok := site.Target.LexicalBody()
			if !ok {
				t.Fatalf("body %x lexical call at %d has no body identity", unit.Body, site.Point)
			}
			if _, present := bodies[target]; !present {
				t.Fatalf("body %x lexical call at %d targets absent body %x", unit.Body, site.Point, target)
			}
		}
	}

	reparsed := parseRelationProgramInputChunk(t, source)
	rebound := bind.BindChunk(reparsed, bind.Options{})
	rekeys := collectKeys(rebound, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, reparsed)
	reprepared, err := prepareBoundChunkBodies(reparsed, rebound, body.Config{Registry: reg}, rekeys)
	if err != nil {
		t.Fatalf("repeat prepareBoundChunkBodies: %v", err)
	}
	repeated, err := relationProgramInput(reprepared, relationProgramFactories(t, reprepared, nil), nil)
	if err != nil {
		t.Fatalf("repeat relationProgramInput: %v", err)
	}
	if len(repeated) != len(units) {
		t.Fatalf("repeat relation input units = %d, want %d", len(repeated), len(units))
	}
	for index := range units {
		leftSurface, _ := units[index].Plan.CallSurface()
		rightSurface, _ := repeated[index].Plan.CallSurface()
		if units[index].Body != repeated[index].Body || leftSurface.Digest() != rightSurface.Digest() {
			t.Fatalf("repeat relation input drifted at canonical body %d", index)
		}
	}
}

func TestRelationProgramInputFindsDefinitionNestedInDynamicObjectEntry(t *testing.T) {
	const source = `
local key = "handler"
local registry = {
    handlers = {
        [key] = function(value: string): string
            return value
        end,
    },
}
return registry
`
	stmts := parseRelationProgramInputChunk(t, source)
	bindings := bind.BindChunk(stmts, bind.Options{})
	reg := standard.Registry()
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatalf("prepareBoundChunkBodies: %v", err)
	}
	if _, err := relationProgramInput(prepared, relationProgramFactories(t, prepared, nil), nil); err != nil {
		t.Fatalf("relationProgramInput: %v", err)
	}
}

func TestRelationProgramFreezeConsumesBinderSealedDirectLexicalDeclarations(t *testing.T) {
	reg := standard.Registry()
	stmts := parseRelationProgramInputChunk(t, `
local function leaf(value: string): ()
end
leaf("exact")
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatal(err)
	}
	units, err := relationProgramInput(prepared, relationProgramFactories(t, prepared, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transformer.FreezeRelationProgram(units, prepared.callTopology); err != nil {
		t.Fatalf("FreezeRelationProgram with direct local function: %v", err)
	}
}

func TestRelationProgramInputIncludesFunctionRootAndNestedLiteralExactlyOnce(t *testing.T) {
	parsed := parseRelationProgramInputChunk(t, `
local outer = function(value)
	local nested = function()
		return value
	end
	return nested()
end
`)
	chunkBindings := bind.BindChunk(parsed, bind.Options{})
	chunkFunctions := chunkBindings.Functions()
	if len(chunkFunctions) != 2 {
		t.Fatalf("bound chunk functions = %d, want outer and nested literal", len(chunkFunctions))
	}
	rootFn := chunkFunctions[0]
	bindings := bind.BindFunction(rootFn, bind.Options{})
	reg := standard.Registry()
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, rootFn.Stmts)
	prepared, err := prepareBoundFunctionBodies(rootFn, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatalf("prepareBoundFunctionBodies: %v", err)
	}
	if prepared.root != nil {
		t.Fatal("function forest unexpectedly manufactured a chunk root")
	}
	units, err := relationProgramInput(prepared, relationProgramFactories(t, prepared, nil), nil)
	if err != nil {
		t.Fatalf("relationProgramInput: %v", err)
	}
	if len(units) != len(bindings.Functions()) {
		t.Fatalf("relation input units = %d, want every one of %d bound functions exactly once", len(units), len(bindings.Functions()))
	}
	want := make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(prepared.functions))
	for _, static := range prepared.functions {
		want[static.StableLexicalBodyID()] = struct{}{}
	}
	for _, unit := range units {
		if _, ok := want[unit.Body]; !ok {
			t.Fatalf("relation input contains foreign function body %x", unit.Body)
		}
		delete(want, unit.Body)
	}
	if len(want) != 0 {
		t.Fatalf("relation input omitted %d prepared function bodies", len(want))
	}
	bodies := make(map[lexicalidentity.StableLexicalBodyID]struct{}, len(units))
	for _, unit := range units {
		bodies[unit.Body] = struct{}{}
	}
	lexicalCalls := 0
	for _, unit := range units {
		surface, _ := unit.Plan.CallSurface()
		for _, site := range surface.Sites() {
			if site.Target.Kind() != operationplan.CallSurfaceTargetLexical {
				continue
			}
			lexicalCalls++
			target, _ := site.Target.LexicalBody()
			if _, ok := bodies[target]; !ok {
				t.Fatalf("function-root call at %d targets absent prepared body %x", site.Point, target)
			}
		}
	}
	if lexicalCalls == 0 {
		t.Fatal("function-root fixture produced no lexical call authority")
	}
}

func TestRelationProgramInputPreservesExecutionFactoryAllLaneDomain(t *testing.T) {
	reg := standard.Registry()
	stmts := parseRelationProgramInputChunk(t, `
local threshold = 37
return threshold
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatal(err)
	}
	lanes := state.DefaultLanes()
	if len(lanes) != 17 {
		t.Fatalf("State lane catalog = %d, want 17", len(lanes))
	}
	factories := relationProgramFactories(t, prepared, lanes)
	units, err := relationProgramInput(prepared, factories, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 {
		t.Fatalf("relation units = %d, want one chunk body", len(units))
	}
	unit := units[0]
	factory := factories[unit.Body]
	if factory == nil {
		t.Fatal("relation unit lost its execution factory")
	}
	unitDomain := unit.Domain.Lattice()
	for _, lane := range lanes {
		single, err := state.TryDomainWithLanes(reg, []state.LaneID{lane})
		if err != nil {
			t.Fatal(err)
		}
		normalized := unitDomain.Join(unitDomain.Bottom(), single.Top())
		if !single.Equal(normalized, single.Top()) {
			t.Fatalf("relation unit domain dropped selected State lane %q", lane)
		}
	}
	stateKey := pathaddr.StateKey("sym999@1.bound")
	previous := unitDomain.Bottom().WriteNumCeil(unit.KeySpace, stateKey, 5)
	next := unitDomain.Bottom().WriteNumCeil(unit.KeySpace, stateKey, 6)
	widened := unitDomain.Widen(previous, next)
	if ceil, ok := widened.ReadNumCeil(unit.KeySpace, stateKey); !ok || ceil != 37 {
		t.Fatalf("relation unit WIR threshold widening = %d/%t, want 37/true", ceil, ok)
	}
}

func TestRelationProgramInputCarriesCanonicalEntrySeedAuthority(t *testing.T) {
	reg := standard.Registry()
	ambient := manifest.New("ambient")
	ambient.SetExport(typ.String)
	stmts := parseRelationProgramInputChunk(t, `
local function target(value: string)
	return value, configured, ambient
end
return target("ok")
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"configured", "ambient"}})
	moduleExports := importlookup.Source{Manifests: []*manifest.Manifest{ambient}}
	config := body.Config{
		Registry:      reg,
		Globals:       []string{"configured", "ambient"},
		GlobalTypes:   map[string]typ.Type{"configured": typ.Number},
		ModuleExports: moduleExports,
	}
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, moduleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, config, keys)
	if err != nil {
		t.Fatal(err)
	}
	factories := relationProgramFactories(t, prepared, nil)
	units, err := relationProgramInput(prepared, factories, nil)
	if err != nil {
		t.Fatal(err)
	}
	functions := bindings.Functions()
	if len(functions) != 1 {
		t.Fatalf("bound functions = %d, want 1", len(functions))
	}
	static := prepared.function(functions[0])
	if static == nil {
		t.Fatal("target body is absent from prepared forest")
	}
	var plan state.EntrySeedPlan
	for _, unit := range units {
		if unit.Body == static.StableLexicalBodyID() {
			plan = unit.EntrySeedPlan
			break
		}
	}
	if !plan.Valid() || plan.Empty() || plan.Len() < 3 {
		t.Fatalf("handed-off entry seed plan = valid:%t empty:%t len:%d", plan.Valid(), plan.Empty(), plan.Len())
	}

	params := bindings.ParamSlots(functions[0])
	configured, configuredOK := bindings.GlobalSymbol("configured")
	ambientSymbol, ambientOK := bindings.GlobalSymbol("ambient")
	if len(params) != 1 || !configuredOK || !ambientOK {
		t.Fatal("entry seed fixture has incomplete parameter/global bindings")
	}
	seeded, err := plan.Apply(reg, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	for name, slot := range map[string]key.Value{
		"param":      key.SymbolValue(params[0].Symbol),
		"configured": key.SymbolValue(configured),
		"ambient":    key.SymbolValue(ambientSymbol),
	} {
		if got := seeded.ReadValue(reg, slot); product.Domain(reg).Equal(got, product.Bottom(reg)) {
			t.Fatalf("handed-off %s seed remained Bottom", name)
		}
	}
	actual := product.Top()
	paramSlot := key.SymbolValue(params[0].Symbol)
	seeded, err = plan.Apply(reg, state.State{}.WriteValue(reg, paramSlot, actual))
	if err != nil {
		t.Fatal(err)
	}
	if got := seeded.ReadValue(reg, paramSlot); !product.Equal(reg, got, actual) {
		t.Fatal("handed-off plan replaced a route-supplied parameter")
	}
	if factory := factories[static.StableLexicalBodyID()]; factory == nil || !factory.EntrySeedPlan().Valid() || factory.EntrySeedPlan().Len() != plan.Len() {
		t.Fatal("relation unit entry-seed authority drifted from its execution factory")
	}
}

func relationProgramFactories(t *testing.T, prepared preparedBodies, lanes []state.LaneID) relationProgramExecutionFactories {
	t.Helper()
	statics := make([]*body.Static, 0, 1+len(prepared.functions))
	if prepared.root != nil {
		statics = append(statics, prepared.root)
	}
	for _, static := range prepared.functions {
		statics = append(statics, static)
	}
	out := make(relationProgramExecutionFactories, len(statics))
	for _, static := range statics {
		ctx, session := cancellation.Attach(context.Background())
		factory, err := static.NewExecutionFactory(body.ExecutionFactoryConfig{Context: ctx, Session: session, StateLanes: state.CloneLanes(lanes)})
		if err != nil {
			t.Fatalf("execution factory for body %x: %v", static.StableLexicalBodyID(), err)
		}
		out[static.StableLexicalBodyID()] = factory
	}
	return out
}

func TestPreparedForestSeparatesLexicalNamespaceFromBodyContentDigest(t *testing.T) {
	const unchanged = `
local function stable(value)
	return value
end
local function sibling()
	return 1
end
return stable(sibling())
`
	const siblingEdited = `
local function stable(value)
	return value
end
local function sibling()
	return 2
end
return stable(sibling())
`
	first := prepareFirstRelationInputFunction(t, unchanged)
	repeated := prepareFirstRelationInputFunction(t, unchanged)
	edited := prepareFirstRelationInputFunction(t, siblingEdited)
	if first.StableLexicalBodyID() != repeated.StableLexicalBodyID() || first.IdentityDigest() != repeated.IdentityDigest() {
		t.Fatal("unchanged lexical unit produced nondeterministic body/cache identity")
	}
	if first.StableLexicalBodyID() == edited.StableLexicalBodyID() {
		t.Fatal("sibling edit did not change the shared lexical unit namespace")
	}
	if first.IdentityDigest() != edited.IdentityDigest() {
		t.Fatal("unchanged body content digest was invalidated by an unrelated sibling edit")
	}
}

func prepareFirstRelationInputFunction(t *testing.T, source string) *body.Static {
	t.Helper()
	stmts := parseRelationProgramInputChunk(t, source)
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.Functions()
	if len(functions) < 2 {
		t.Fatalf("bound functions = %d, want stable and sibling", len(functions))
	}
	reg := standard.Registry()
	keys := collectKeys(bindings, rootKey(summary.SummaryKey{}), reg, nil, body.Config{}.ModuleExports, stmts)
	prepared, err := prepareBoundChunkBodies(stmts, bindings, body.Config{Registry: reg}, keys)
	if err != nil {
		t.Fatalf("prepareBoundChunkBodies: %v", err)
	}
	static := prepared.function(functions[0])
	if static == nil {
		t.Fatal("first lexical function is absent from prepared forest")
	}
	return static
}

func parseRelationProgramInputChunk(t *testing.T, source string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(source, "relation_program_input_test.lua")
	if err != nil {
		t.Fatalf("parse relation-program input fixture: %v", err)
	}
	return stmts
}
