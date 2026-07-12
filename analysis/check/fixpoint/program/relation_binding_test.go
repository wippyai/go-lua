package program

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type fixedRelationSourceValues struct {
	values map[factflow.ValueSource]product.Value
	calls  int
}

func (s *fixedRelationSourceValues) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	s.calls++
	value, ok := s.values[source]
	return value, ok
}

func TestInactiveRelationResolverBindsExactScalarAndHandledEmpty(t *testing.T) {
	catalog := captureRelationCatalog(t, `
local function identity(x: any): any return x end
local function noop() end
local input = "ok"
local got = identity(input)
noop()
return got
`)
	snapshot, err := catalog.Freeze(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	owner, direct := activeRelationOwner(t, catalog)
	factory, ok := snapshot.inactiveRelationResolverFactory(owner)
	if !ok {
		t.Fatal("exact owner did not produce resolver factory")
	}

	reg := standard.Registry()
	want := typevalue.LiteralString(reg, "bound")
	sources := &fixedRelationSourceValues{values: make(map[factflow.ValueSource]product.Value)}
	var identitySite, noopSite factflow.CallSiteView
	for point := cfg.Point(0); int(point) < direct.PointCount(); point++ {
		target, routed := direct.Lookup(point)
		if !routed {
			continue
		}
		site, found := owner.Prepared.OperationPlan().Facts().CallSiteView(point)
		if !found {
			t.Fatalf("routed point %d has no call fact", point)
		}
		if target.Shape.Params == 0 {
			noopSite = site
		} else {
			identitySite = site
			source, _ := site.ArgumentSourceAt(0)
			sources.values[source] = want
		}
	}
	resolver := factory(body.CallOutcomeContext{Facts: owner.Prepared.OperationPlan().Facts(), Sources: sources})
	ctx := transfer.NodeContext{Registry: reg}
	in := state.State{}
	before := in.Snapshot()
	out, handled := resolver(ctx, identitySite, in, nil)
	if !handled || len(out.Results) != 1 || !product.Equal(reg, out.Results[0].Value, want) {
		point, hasPoint := identitySite.Point()
		source, _ := identitySite.ArgumentSourceAt(0)
		receiver, hasReceiver := identitySite.ReceiverPath()
		_, hasReceiverSource := identitySite.ReceiverSource()
		methodPath, hasMethodPath := identitySite.MethodPath()
		cursor, bound := relationBindings(body.CallOutcomeContext{Facts: owner.Prepared.OperationPlan().Facts(), Sources: sources})(ctx, identitySite, state.State{}, nil, transformer.Shape{Params: 1})
		t.Fatalf("identity outcome = %#v, handled=%v; point=%d/%v callee=%d path=%#v args=%d flags=%v/%v member=%v method=%q receiver=%#v/%v receiverSource=%v methodPath=%#v/%v source=%#v valid=%v canonical=%#v exact=%v bound=%v cursor=%#v", out, handled, point, hasPoint, identitySite.CalleeSymbol(), identitySite.CalleePathRef(), identitySite.ArgumentSourceCount(), identitySite.Expanded(), identitySite.OpenTail(), identitySite.CalleeMemberAccess(), identitySite.MethodName(), receiver, hasReceiver, hasReceiverSource, methodPath, hasMethodPath, source, source.Valid(), canonicalSourcePath(owner.Prepared.OperationPlan().Facts(), source), exactScalarCallSite(identitySite, 1), bound, cursor)
	}
	if sources.calls != 2 {
		t.Fatalf("ValueOfSource calls = %d, want binding plus return-alias materialization", sources.calls)
	}
	if !state.Domain(reg).Equal(in, before) {
		t.Fatal("relation resolution mutated one of the caller's state lanes")
	}
	zeroArgResolver := factory(body.CallOutcomeContext{Facts: owner.Prepared.OperationPlan().Facts()})
	if out, handled := zeroArgResolver(ctx, noopSite, state.State{}, nil); !handled || len(out.Results) != 0 || out.MaySuspend {
		t.Fatalf("noop outcome = %#v, handled=%v; empty is authoritative", out, handled)
	}
}

func TestInactiveRelationResolverAcceptsAdjustedValueOnlyBinding(t *testing.T) {
	catalog := captureRelationCatalog(t, `
local function identity(x: any): any return x end
local input = "ok"
local got = identity(input)
return got
`)
	snapshot, err := catalog.Freeze(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	owner, direct := activeRelationOwner(t, catalog)
	factory, ok := snapshot.inactiveRelationResolverFactory(owner)
	if !ok {
		t.Fatal("exact owner did not produce resolver factory")
	}
	var original factflow.CallSiteView
	for point := cfg.Point(0); int(point) < direct.PointCount(); point++ {
		if target, routed := direct.Lookup(point); routed && target.Shape.Params == 1 {
			original, _ = owner.Prepared.OperationPlan().Facts().CallSiteView(point)
			break
		}
	}
	shape, _ := factflow.NewValueSourceShape(true, false, true, false)
	literal, _ := factflow.NewStringLiteralValueSource("literal", 0, 0, 0, shape)
	point, _ := original.Point()
	valueOnly := factflow.NewCallSite(factflow.CallSiteConfig{
		Point: point, HasPoint: true,
		CalleeSymbol: original.CalleeSymbol(), CalleePath: original.CalleePath(),
		ArgumentSources: []factflow.ValueSource{literal},
	}).View()
	reg := standard.Registry()
	value := typevalue.LiteralString(reg, "literal")
	sources := &fixedRelationSourceValues{values: map[factflow.ValueSource]product.Value{literal: value}}

	// Dense value binding succeeds and records an intentionally empty optional
	// path for a scalar literal.
	cursor, bound := relationBindings(body.CallOutcomeContext{Sources: sources})(transfer.NodeContext{Registry: reg}, valueOnly, state.State{}, nil, transformer.Shape{Params: 1})
	if !bound {
		t.Fatal("adjusted scalar value-only binding failed")
	}
	if got, ok := cursor.Value(transformer.Root{Kind: transformer.RootParam}); !ok || !product.Equal(reg, got, value) {
		t.Fatalf("bound value = %#v/%v", got, ok)
	}
	if got, ok := cursor.Path(transformer.Root{Kind: transformer.RootParam}); !ok || !got.IsEmpty() {
		t.Fatalf("optional path = %#v/%v, want empty/true", got, ok)
	}

	resolver := factory(body.CallOutcomeContext{Facts: owner.Prepared.OperationPlan().Facts(), Sources: sources})
	if out, handled := resolver(transfer.NodeContext{Registry: reg}, valueOnly, state.State{}, nil); !handled || len(out.Results) != 1 || !product.Equal(reg, out.Results[0].Value, value) {
		t.Fatalf("value-only outcome = %#v, handled=%v", out, handled)
	}
}

func TestInactiveRelationResolverFailsClosedOnOwnerAndCallShapeDrift(t *testing.T) {
	catalog := captureRelationCatalog(t, `
local function identity(x: any): any return x end
local got = identity("ok")
return got
`)
	snapshot, err := catalog.Freeze(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	owner, direct := activeRelationOwner(t, catalog)
	for name, drift := range map[string]func(*relationConsumerIdentity){
		"summary":    func(id *relationConsumerIdentity) { id.Summary.Entry.Values++ },
		"digest":     func(id *relationConsumerIdentity) { id.BodyDigest++ },
		"prepared":   func(id *relationConsumerIdentity) { id.Prepared = nil },
		"generation": func(id *relationConsumerIdentity) { id.Generation = &relationCatalogGeneration{} },
	} {
		t.Run(name, func(t *testing.T) {
			bad := owner
			drift(&bad)
			if factory, ok := snapshot.inactiveRelationResolverFactory(bad); ok || factory != nil {
				t.Fatal("drifted owner was accepted")
			}
		})
	}
	// A route prepared with a different callee shape cannot borrow the frozen
	// producer identity even when point, cell, and summary key still match.
	driftedSnapshot := snapshot
	driftedSnapshot.consumers.entries = append([]relationConsumerEntry(nil), snapshot.consumers.entries...)
	consumerIndex := driftedSnapshot.consumers.byKey[owner.Summary]
	routes := make(map[cfg.Point]transformer.DirectCallTarget)
	for point := cfg.Point(0); int(point) < direct.PointCount(); point++ {
		if target, ok := direct.Lookup(point); ok {
			target.Shape.Params++
			routes[point] = target
		}
	}
	drifted, err := transformer.NewDirectCallCatalog(direct.PointCount(), routes)
	if err != nil {
		t.Fatal(err)
	}
	driftedSnapshot.consumers.entries[consumerIndex].direct = drifted
	if factory, ok := driftedSnapshot.inactiveRelationResolverFactory(owner); ok || factory != nil {
		t.Fatal("drifted target shape was accepted")
	}

	factory, _ := snapshot.inactiveRelationResolverFactory(owner)
	reg := standard.Registry()
	want := typevalue.LiteralString(reg, "bound")
	sources := &fixedRelationSourceValues{values: make(map[factflow.ValueSource]product.Value)}
	var site factflow.CallSiteView
	for point := cfg.Point(0); int(point) < direct.PointCount(); point++ {
		if target, ok := direct.Lookup(point); ok && target.Shape.Params == 1 {
			site, _ = owner.Prepared.OperationPlan().Facts().CallSiteView(point)
			source, _ := site.ArgumentSourceAt(0)
			sources.values[source] = want
			break
		}
	}
	resolver := factory(body.CallOutcomeContext{Facts: owner.Prepared.OperationPlan().Facts(), Sources: sources})
	base := factflow.CallSiteConfig{CalleeSymbol: site.CalleeSymbol(), CalleePath: site.CalleePath(), ArgumentSources: []factflow.ValueSource{}}
	for name, bad := range map[string]factflow.CallSiteView{
		"missing point": factflow.NewCallSite(base).View(),
		"point drift": factflow.NewCallSite(func() factflow.CallSiteConfig {
			c := base
			c.Point, c.HasPoint = cfg.Point(9999), true
			return c
		}()).View(),
		"arity drift": factflow.NewCallSite(func() factflow.CallSiteConfig {
			c := base
			point, _ := site.Point()
			c.Point, c.HasPoint = point, true
			return c
		}()).View(),
		"expanded": factflow.NewCallSite(func() factflow.CallSiteConfig {
			c := base
			point, _ := site.Point()
			source, _ := site.ArgumentSourceAt(0)
			source.Final, source.Expanded, source.Adjusted = true, true, false
			c.Point, c.HasPoint, c.ArgumentSources = point, true, []factflow.ValueSource{source}
			return c
		}()).View(),
		"open": factflow.NewCallSite(func() factflow.CallSiteConfig {
			c := base
			point, _ := site.Point()
			source, _ := site.ArgumentSourceAt(0)
			source.Final, source.Expanded, source.Adjusted, source.OpenTail = true, true, false, true
			c.Point, c.HasPoint, c.ArgumentSources = point, true, []factflow.ValueSource{source}
			return c
		}()).View(),
	} {
		t.Run(name, func(t *testing.T) {
			if out, handled := resolver(transfer.NodeContext{Registry: reg}, bad, state.State{}, nil); handled || !out.Empty() {
				t.Fatalf("drift outcome = %#v, handled=%v", out, handled)
			}
		})
	}
}

func TestCanonicalRelationSourcePathPreservesVersionAndSegments(t *testing.T) {
	want := pathdom.NewPath(42, "value").Field("child")
	want.Version = 7
	shape, _ := factflow.NewValueSourceShape(true, false, true, false)
	expr, _ := factflow.NewExpressionValueSource(9, 0, 0, 0, shape)
	facts := factflow.NewFacts(factflow.FactsInput{ExpressionPaths: map[factflow.ExprRef]pathdom.Path{9: want}})
	if got := canonicalSourcePath(facts, expr); !got.Equal(want) || got.Version != 7 {
		t.Fatalf("expression path = %#v, want %#v", got, want)
	}
	pathSource, _ := factflow.NewPathValueSource(want.Key(), 0, 0, 0, shape)
	if got := canonicalSourcePath(factflow.Facts{}, pathSource); !got.Equal(want) || got.Version != 7 {
		t.Fatalf("path source = %#v, want %#v", got, want)
	}
	if got := canonicalSourcePath(facts, factflow.NewNilValueSource(0)); !got.IsEmpty() {
		t.Fatalf("value-only source path = %#v, want empty", got)
	}
}

func TestRelationSpecializationDelegatesDynamicReadCallbacks(t *testing.T) {
	reg := standard.Registry()
	want := typevalue.LiteralBool(reg, true)
	dynamicCalls, tableCalls := 0, 0
	factory := relationSpecialization(body.CallOutcomeContext{
		DynamicRead: func(transfer.NodeContext, pathdom.Path, product.Value, product.Value, state.State) (product.Value, bool) {
			dynamicCalls++
			return want, true
		},
		DynamicTableRead: func(transfer.NodeContext, pathdom.Path, product.Value, product.Value, state.State) (product.Value, bool) {
			tableCalls++
			return want, true
		},
	})
	specialization, ok := factory(transfer.NodeContext{Registry: reg}, factflow.CallSiteView{}, state.State{}, nil)
	if !ok || specialization.DynamicRead == nil || specialization.DynamicTableRead == nil {
		t.Fatal("dynamic specialization callbacks missing")
	}
	if got, ok := specialization.DynamicRead(pathdom.Path{}, product.Top(), product.Top()); !ok || !product.Equal(reg, got, want) {
		t.Fatalf("dynamic read = %#v/%v", got, ok)
	}
	if got, ok := specialization.DynamicTableRead(pathdom.Path{}, product.Top(), product.Top()); !ok || !product.Equal(reg, got, want) {
		t.Fatalf("dynamic table read = %#v/%v", got, ok)
	}
	if dynamicCalls != 1 || tableCalls != 1 {
		t.Fatalf("callback calls = %d/%d, want 1/1", dynamicCalls, tableCalls)
	}
}

func activeRelationOwner(t *testing.T, catalog relationRunCatalog) (relationConsumerIdentity, transformer.DirectCallCatalog) {
	t.Helper()
	for _, owner := range catalog.ConsumerPolicy().Owners() {
		if !catalog.ConsumerPolicy().Active(owner) {
			continue
		}
		if direct, ok := catalog.ConsumerPolicy().DirectCalls(owner); ok && len(direct.Cells()) != 0 {
			return owner, direct
		}
	}
	t.Fatal("active relation consumer not found")
	return relationConsumerIdentity{}, transformer.DirectCallCatalog{}
}
