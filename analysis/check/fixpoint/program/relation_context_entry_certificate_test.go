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
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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
	keys := programKeys{bindings: bindings, contexts: newContextIndex()}

	contextKey, changed := keys.upsertCallContext(reg, callContextRef{expr: factflow.ExprRef(1)}, base, fn, entry, ks, nil, 0xabc)
	if !changed {
		t.Fatal("identity context was not created")
	}
	context, ok := keys.contexts.contextByKey(contextKey)
	if !ok || context.relationContextEntry == nil {
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
}

func TestRelationContextEntryCertificateRejectsHiddenLane(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	hidden := entry.WritePlacement(identity.ID{Kind: "test", Site: "hidden", Index: 1}, placement.OwnedHeap)
	keys := programKeys{bindings: bindings, contexts: newContextIndex()}

	contextKey, changed := keys.upsertCallContext(reg, callContextRef{expr: 2}, base, fn, hidden, ks, nil, 0xdef)
	if !changed {
		t.Fatal("context was not created")
	}
	context, ok := keys.contexts.contextByKey(contextKey)
	if !ok {
		t.Fatal("context missing")
	}
	if context.relationContextEntry != nil {
		t.Fatal("placement-lane fact was hidden by the certificate projection")
	}
}

func TestRelationContextEntryCertificateRefreshCannotStayStale(t *testing.T) {
	reg, bindings, fn, entry, ks, base := relationCertificateFixture(t)
	keys := programKeys{bindings: bindings, contexts: newContextIndex()}
	call := callContextRef{expr: 3}

	contextKey, changed := keys.upsertCallContext(reg, call, base, fn, entry, ks, nil, 0x123)
	if !changed {
		t.Fatal("context was not created")
	}
	context, _ := keys.contexts.contextByKey(contextKey)
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
