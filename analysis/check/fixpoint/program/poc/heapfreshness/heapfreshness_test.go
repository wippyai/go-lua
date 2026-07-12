package heapfreshness

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TestCurrentSummaryBoundaryAliasesFreshReturns is a differential regression
// probe. It records the current (wrong) production result and contrasts it with
// allocation-site-sensitive inline semantics. The assertions intentionally pass
// while the defect exists, so the isolated POC remains runnable in CI.
func TestCurrentSummaryBoundaryAliasesFreshReturns(t *testing.T) {
	reg := standard.Registry()
	fixture := newFixture(t, reg)

	callA := cfg.Point(101)
	callB := cfg.Point(202)
	outA := fixture.outcome(callA)
	outB := fixture.outcome(callB)
	currentA := resultID(t, reg, outA)
	currentB := resultID(t, reg, outB)
	if currentA != fixture.templateID || currentB != fixture.templateID {
		t.Fatalf("current outcomes unexpectedly instantiate allocation: A=%v B=%v template=%v", currentA, currentB, fixture.templateID)
	}

	wantA := inlineAllocationID(fixture.templateID, callA)
	wantB := inlineAllocationID(fixture.templateID, callB)
	if wantA == wantB {
		t.Fatal("reference inline allocation identities collided across call sites")
	}
	if currentA == wantA || currentB == wantB {
		t.Fatalf("current boundary unexpectedly matches inline identities: current=(%v,%v) inline=(%v,%v)", currentA, currentB, wantA, wantB)
	}

	// Model `a = make(); a.x = "changed"; b = make()`. Because B carries the
	// same template identity, its initial object replaces A's mutation.
	current := applyCurrent(reg, state.State{}, outA)
	current = current.WriteHeapTableObject(reg, currentA, fixture.objectFor(currentA, fixture.stringValue))
	current = applyCurrent(reg, current, outB)
	if got := fixture.member(t, current, currentA); !product.Equal(reg, got, fixture.numberValue) {
		t.Fatalf("current replacement no longer reproduces alias loss: got %v", got)
	}

	// The inline reference keeps both objects and both placement facts.
	inline := state.State{}
	inline = applyFresh(reg, inline, outA, fixture.templateID, wantA, false)
	inline = inline.WriteHeapTableObject(reg, wantA, fixture.objectFor(wantA, fixture.stringValue))
	inline = applyFresh(reg, inline, outB, fixture.templateID, wantB, false)
	if got := fixture.member(t, inline, wantA); !product.Equal(reg, got, fixture.stringValue) {
		t.Fatalf("inline A mutation = %v, want retained string", got)
	}
	if got := fixture.member(t, inline, wantB); !product.Equal(reg, got, fixture.numberValue) {
		t.Fatalf("inline B initial member = %v, want number", got)
	}
	if inline.ReadPlacement(wantA) != placement.OwnedHeap || inline.ReadPlacement(wantB) != placement.OwnedHeap {
		t.Fatalf("inline placements = (%s,%s), want two owned-heap facts", inline.ReadPlacement(wantA), inline.ReadPlacement(wantB))
	}
}

func TestSameCallSiteReferenceWeakJoinsAndStabilizes(t *testing.T) {
	reg := standard.Registry()
	fixture := newFixture(t, reg)
	call := cfg.Point(303)
	out := fixture.outcome(call)
	id := inlineAllocationID(fixture.templateID, call)

	first := applyFresh(reg, state.State{}, out, fixture.templateID, id, true)
	mutated := first.WriteHeapTableObject(reg, id, fixture.objectFor(id, fixture.stringValue))
	second := applyFresh(reg, mutated, out, fixture.templateID, id, true)
	member := fixture.member(t, second, id)
	want := product.Join(reg, fixture.numberValue, fixture.stringValue)
	if !product.Equal(reg, member, want) {
		t.Fatalf("weak repeated-site member = %v, want number|string join", member)
	}
	third := applyFresh(reg, second, out, fixture.templateID, id, true)
	if !heapidentity.ObjectDomain(reg).Equal(second.ReadHeapTableObject(reg, id), third.ReadHeapTableObject(reg, id)) {
		t.Fatal("repeated-site weak materialization did not reach a finite fixed point")
	}

	currentFirst := applyCurrent(reg, state.State{}, out)
	currentMutated := currentFirst.WriteHeapTableObject(reg, fixture.templateID, fixture.objectFor(fixture.templateID, fixture.stringValue))
	current := applyCurrent(reg, currentMutated, out)
	if got := fixture.member(t, current, fixture.templateID); !product.Equal(reg, got, fixture.numberValue) {
		t.Fatalf("current replacement behavior changed: got %v, want reset number", got)
	}
}

type fixture struct {
	reg         *axis.Registry
	provider    callpayload.CallOutcomeProvider
	templateID  identity.ID
	memberKey   keyspace.Key
	numberValue product.Value
	stringValue product.Value
}

func newFixture(t *testing.T, reg *axis.Registry) fixture {
	t.Helper()
	callee := symbol.ID(41)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 42})
	templateID := identity.LuaTableLiteral(7001, 17)
	root := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(templateID))
	numberValue := typevalue.FromType(reg, typ.Number)
	stringValue := typevalue.FromType(reg, typ.String)
	ks := keyspace.New()
	memberKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "x"}})
	if !ok {
		t.Fatal("member key")
	}
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, StaticMembers: map[keyspace.Key]product.Value{memberKey: numberValue}})
	provider := callresult.OutcomeProvider(callresult.ProviderConfig{
		Summaries: summary.NewSnapshot(reg, summary.EntrySummary{Key: key, Summary: summary.Summary{
			Returns: []product.Value{root}, HeapKeySpace: ks,
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{templateID: object},
		}}),
		KeyFor:   callresult.ByCalleeIdentity(map[symbol.ID]summary.SummaryKey{callee: key}),
		KeySpace: ks,
	})
	return fixture{reg: reg, provider: provider, templateID: templateID, memberKey: memberKey, numberValue: numberValue, stringValue: stringValue}
}

func (f fixture) outcome(point cfg.Point) callpayload.CallOutcome {
	return f.provider(transfer.NodeContext{Registry: f.reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{CalleeSymbol: symbol.ID(41)}).View(), state.State{}, nil)
}

func (f fixture) objectFor(id identity.ID, member product.Value) heapidentity.TableObject {
	root := product.Set(f.reg, product.NewWithPresence(f.reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(id))
	return heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, StaticMembers: map[keyspace.Key]product.Value{f.memberKey: member}})
}

func (f fixture) member(t *testing.T, s state.State, id identity.ID) product.Value {
	t.Helper()
	value, ok := s.ReadHeapTableObject(f.reg, id).StaticMember(f.memberKey)
	if !ok {
		t.Fatalf("heap object %v has no .x member", id)
	}
	return value
}

func resultID(t *testing.T, reg *axis.Registry, out callpayload.CallOutcome) identity.ID {
	t.Helper()
	if len(out.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(out.Results))
	}
	id, ok := product.Get(reg, out.Results[0].Value, identity.Key).ID()
	if !ok {
		t.Fatal("result has no exact identity")
	}
	return id
}

func applyCurrent(reg *axis.Registry, in state.State, out callpayload.CallOutcome) state.State {
	for id, object := range out.HeapTableObjects {
		in = in.WriteHeapTableObject(reg, id, object)
	}
	for id := range out.HeapTableObjects {
		in = in.WritePlacement(id, placement.OwnedHeap)
	}
	return in
}

func applyFresh(reg *axis.Registry, in state.State, out callpayload.CallOutcome, from, to identity.ID, weak bool) state.State {
	object := out.HeapTableObjects[from]
	root := product.Set(reg, object.Root(), identity.Key, identity.Singleton(to))
	object = heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root, StaticMembers: object.StaticMembers()})
	if weak {
		object = heapidentity.ObjectDomain(reg).Join(in.ReadHeapTableObject(reg, to), object)
	}
	in = in.WriteHeapTableObject(reg, to, object)
	return in.WritePlacement(to, placement.Join(in.ReadPlacement(to), placement.OwnedHeap))
}

func inlineAllocationID(template identity.ID, point cfg.Point) identity.ID {
	return identity.ID{Kind: template.Kind, Site: "call-site", Index: template.Index ^ uint64(point)*0x9e3779b97f4a7c15}
}
