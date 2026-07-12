package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestRelationContextEntryCertificateIdentityPrimitive(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local function identity(value) return value end`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	functions := bindings.NestedFunctions(nil)
	if len(functions) != 1 {
		t.Fatalf("nested functions = %d, want 1", len(functions))
	}
	fn := functions[0]
	slot := bindings.ParamSlots(fn)[0]
	value := typevalue.FromType(reg, typ.String)
	ks := keyspace.New()
	root := ks.FromPath(path.Path{Symbol: slot.Symbol})
	entry := state.State{}.
		WriteValue(reg, statekey.SymbolValue(slot.Symbol), value).
		WriteLocalPathKey(reg, root, value)
	base := summary.DefaultSummaryKey(ref.FromSymbol(101))
	keys := programKeys{bindings: bindings, contexts: newContextIndex(), certifyRelationContexts: true}

	contextKey, changed := keys.upsertCallContext(reg, callContextRef{expr: factflow.ExprRef(1)}, base, fn, entry, ks, nil, 0xabc)
	if !changed {
		t.Fatal("identity context was not created")
	}
	context, ok := keys.contexts.contextByKey(contextKey)
	if !ok {
		t.Fatal("identity context missing")
	}
	context.relationContextEntry = certifyRelationContextEntry(reg, bindings, fn, bindings.ParamSymbols(fn), nil, contextKey, base, 0xabc, keys.contexts.nextEntryDiscoveryGeneration(), context.entryState, context.entryKeys)
	if context.relationContextEntry == nil {
		t.Fatal("exact identity primitive entry did not produce a certificate")
	}
	certificate := context.relationContextEntry
	if certificate.context != contextKey || certificate.base != base || certificate.preparedBodyDigest != 0xabc || certificate.discoveryGeneration == 0 {
		t.Fatalf("certificate identity = %#v", certificate)
	}
	if len(certificate.params) != 1 || certificate.params[0].slot != statekey.SymbolValue(slot.Symbol) ||
		!product.Equal(reg, certificate.params[0].value, value) {
		t.Fatalf("certificate params = %#v", certificate.params)
	}
	if len(certificate.rootRefinements) != 1 || !certificate.rootRefinements[0] {
		t.Fatalf("certificate root refinements = %v, want [true]", certificate.rootRefinements)
	}
}

func TestRelationContextEntryCertificateDoesNotInventRootRefinement(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	context := base
	context.Entry.Values = 1
	certificate := certifyRelationContextEntry(reg, bindings, fn, bindings.ParamSymbols(fn), nil, context, base, 1, 1, entry, ks)
	if certificate == nil {
		t.Fatal("value-only exact entry was not certified")
	}
	if len(certificate.rootRefinements) != 1 || certificate.rootRefinements[0] {
		t.Fatalf("value-only certificate root refinements = %v, want [false]", certificate.rootRefinements)
	}
}

func TestRelationContextEntryCertificateLegacyPathDoesNoProofWork(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	keys := programKeys{bindings: bindings, contexts: newContextIndex()}
	contextKey, changed := keys.upsertCallContext(reg, callContextRef{expr: 99}, base, fn, entry, ks, nil, 0xabc)
	if !changed {
		t.Fatal("legacy context was not created")
	}
	context, ok := keys.contexts.contextByKey(contextKey)
	if !ok {
		t.Fatal("legacy context missing")
	}
	if context.relationContextEntry != nil || keys.contexts.discoveryGeneration != 0 {
		t.Fatalf("legacy path produced relation proof work: certificate=%v generation=%d", context.relationContextEntry, keys.contexts.discoveryGeneration)
	}
}

func TestRelationContextEntryCertificateRejectsHiddenLane(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	hidden := entry.WritePlacement(identity.ID{Kind: "test", Site: "hidden", Index: 1}, placement.OwnedHeap)
	keys := programKeys{bindings: bindings, contexts: newContextIndex(), certifyRelationContexts: true}

	contextKey, changed := keys.upsertCallContext(reg, callContextRef{expr: 2}, base, fn, hidden, ks, nil, 0xdef)
	if !changed {
		t.Fatal("context was not created")
	}
	context, ok := keys.contexts.contextByKey(contextKey)
	if !ok {
		t.Fatal("context missing")
	}
	context.relationContextEntry = certifyRelationContextEntry(reg, bindings, fn, bindings.ParamSymbols(fn), nil, contextKey, base, 0xdef, keys.contexts.nextEntryDiscoveryGeneration(), context.entryState, context.entryKeys)
	if context.relationContextEntry != nil {
		t.Fatal("placement-lane fact was hidden by the certificate projection")
	}
}

func TestRelationContextEntryCertificateRejectsTopDescendantPathAndCapture(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	slot := bindings.ParamSlots(fn)[0]
	context := base
	context.Entry.Values = 1
	if got := certifyRelationContextEntry(reg, bindings, fn, bindings.ParamSymbols(fn), nil, context, base, 1, 1,
		state.State{}.WriteValue(reg, statekey.SymbolValue(slot.Symbol), product.Top()), ks); got != nil {
		t.Fatal("top-valued parameter was certified")
	}
	value := entry.ReadValue(reg, statekey.SymbolValue(slot.Symbol))
	descendant := ks.FromPath(path.NewPath(slot.Symbol, "value").Field("member"))
	withDescendant := entry.WriteLocalPathKey(reg, descendant, value)
	if got := certifyRelationContextEntry(reg, bindings, fn, bindings.ParamSymbols(fn), nil, context, base, 1, 1, withDescendant, ks); got != nil {
		t.Fatal("descendant parameter path was certified")
	}

	stmts := parseChunk(t, `local suffix = "!"; local function captured(value) return value .. suffix end`)
	captureBindings := bind.BindChunk(stmts, bind.Options{})
	captured := captureBindings.NestedFunctions(nil)[0]
	capturedSlot := captureBindings.ParamSlots(captured)[0]
	capturedEntry := state.State{}.WriteValue(reg, statekey.SymbolValue(capturedSlot.Symbol), value)
	capture := captureBindings.DirectCaptures(captured)[0].Captured
	if got := certifyRelationContextEntry(reg, captureBindings, captured, captureBindings.ParamSymbols(captured), []symbol.ID{capture}, context, base, 1, 1, capturedEntry, keyspace.New()); got != nil {
		t.Fatal("captured function entry was certified")
	}
}

func TestRelationContextEntryCertificateRefreshCannotStayStale(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	keys := programKeys{bindings: bindings, contexts: newContextIndex(), certifyRelationContexts: true}
	call := callContextRef{expr: 3}

	contextKey, changed := keys.upsertCallContext(reg, call, base, fn, entry, ks, nil, 0x123)
	if !changed {
		t.Fatal("context was not created")
	}
	context, _ := keys.contexts.contextByKey(contextKey)
	context.relationContextEntry = certifyRelationContextEntry(reg, bindings, fn, bindings.ParamSymbols(fn), nil, contextKey, base, 0x123, keys.contexts.nextEntryDiscoveryGeneration(), context.entryState, context.entryKeys)
	if context.relationContextEntry == nil {
		t.Fatal("initial exact entry was not certified")
	}
	oldGeneration := context.relationContextEntry.discoveryGeneration
	refreshed := entry.WritePlacement(identity.ID{Kind: "test", Site: "refresh", Index: 1}, placement.OwnedHeap)
	gotKey, changed := keys.upsertCallContext(reg, call, base, fn, refreshed, ks, nil, 0x123)
	if !changed || gotKey != contextKey {
		t.Fatalf("refresh = (%v, %v), want (%v, true)", gotKey, changed, contextKey)
	}
	context, _ = keys.contexts.contextByKey(contextKey)
	if context.relationContextEntry != nil {
		t.Fatal("entry refresh retained or rebuilt a stale certificate")
	}
	if keys.contexts.discoveryGeneration <= oldGeneration {
		t.Fatalf("discovery generation = %d, want newer than %d", keys.contexts.discoveryGeneration, oldGeneration)
	}
}

func TestRelationContextEntryCertificateCaptureOnlyKeepsExactVersionedPath(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `local suffix = "!"; local function captured() return suffix end`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fn := bindings.NestedFunctions(nil)[0]
	capture := bindings.DirectCaptures(fn)[0]
	value := typevalue.LiteralString(reg, "!")
	ks := keyspace.New()
	versioned, ok := ks.FromResolverKey(capture.Captured, 7, nil)
	if !ok {
		t.Fatal("versioned capture key missing")
	}
	entry := state.State{}.
		WriteValue(reg, statekey.SymbolValue(capture.Captured), value).
		WriteLocalPathKey(reg, versioned, value)
	base := summary.DefaultSummaryKey(ref.FromSymbol(303))
	context := base
	context.Entry.Values = 1
	certificate := certifyRelationContextEntry(reg, bindings, fn, nil, []symbol.ID{capture.Captured}, context, base, 9, 2, entry, ks)
	if certificate == nil || len(certificate.captures) != 1 || certificate.captures[0].path.Version != 7 ||
		certificate.captures[0].path.Symbol != capture.Captured || !certificate.captureRootRefinements[0] {
		t.Fatalf("capture-only versioned certificate = %#v", certificate)
	}
}

func TestRelationContextEntryCertificateRejectsWrittenAndHiddenCaptureState(t *testing.T) {
	reg := standard.Registry()
	stmts := parseChunk(t, `
local suffix = "!"
local function captured() return suffix end
suffix = "changed"
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fn := bindings.NestedFunctions(nil)[0]
	capture := bindings.DirectCaptures(fn)[0]
	value := typevalue.LiteralString(reg, "!")
	base := summary.DefaultSummaryKey(ref.FromSymbol(304))
	context := base
	context.Entry.Values = 1
	entry := state.State{}.WriteValue(reg, statekey.SymbolValue(capture.Captured), value)
	if got := certifyRelationContextEntry(reg, bindings, fn, nil, []symbol.ID{capture.Captured}, context, base, 10, 3, entry, keyspace.New()); got != nil {
		t.Fatal("written capture was certified")
	}

	immutableStmts := parseChunk(t, `local suffix = "!"; local function captured() return suffix end`)
	immutableBindings := bind.BindChunk(immutableStmts, bind.Options{})
	immutableFn := immutableBindings.NestedFunctions(nil)[0]
	immutableCapture := immutableBindings.DirectCaptures(immutableFn)[0]
	hidden := state.State{}.
		WriteValue(reg, statekey.SymbolValue(immutableCapture.Captured), value).
		WritePlacement(identity.ID{Kind: "test", Site: "capture-hidden", Index: 1}, placement.OwnedHeap)
	if got := certifyRelationContextEntry(reg, immutableBindings, immutableFn, nil, []symbol.ID{immutableCapture.Captured}, context, base, 11, 4, hidden, keyspace.New()); got != nil {
		t.Fatal("capture entry with hidden state lane was certified")
	}
}

func TestContextEntryCaptureValueUsefulAcceptsOnlyPrimitiveScalars(t *testing.T) {
	reg := standard.Registry()
	for name, value := range map[string]product.Value{
		"table":    typevalue.FromType(reg, typetable.NewRecord().Field("value", typ.String).Build()),
		"function": typevalue.FromType(reg, typ.Func().Returns(typ.String).Build()),
		"any":      typevalue.FromType(reg, typ.Any),
	} {
		if contextEntryCaptureValueUseful(reg, value) {
			t.Fatalf("%s capture value was admitted", name)
		}
	}
	for name, value := range map[string]product.Value{
		"nil":     typevalue.Nil(reg),
		"boolean": typevalue.LiteralBool(reg, true),
		"integer": typevalue.LiteralInt(reg, 7),
		"string":  typevalue.LiteralString(reg, "value"),
	} {
		if !contextEntryCaptureValueUseful(reg, value) {
			t.Fatalf("%s primitive capture value was rejected", name)
		}
	}
}

func relationCertificateFixture(t *testing.T) (*axis.Registry, *bind.Result, *ast.FunctionExpr, state.State, *keyspace.KeySpace, summary.SummaryKey) {
	t.Helper()
	reg := standard.Registry()
	stmts := parseChunk(t, `local function identity(value) return value end`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	fn := bindings.NestedFunctions(nil)[0]
	slot := bindings.ParamSlots(fn)[0]
	entry := state.State{}.WriteValue(reg, statekey.SymbolValue(slot.Symbol), typevalue.FromType(reg, typ.String))
	return reg, bindings, fn, entry, keyspace.New(), summary.DefaultSummaryKey(ref.FromSymbol(202))
}
