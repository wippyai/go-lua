package program

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/relationcall"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCallRoutePartitionClassifiesCompleteSurfaceAndHasStableDigest(t *testing.T) {
	snapshot, owner, points := callRoutePartitionFixture(t)
	first, err := newCallRoutePartition(snapshot, owner)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newCallRoutePartition(snapshot, owner)
	if err != nil {
		t.Fatal(err)
	}
	if !first.routingDigest.available() || first.routingDigest != second.routingDigest {
		t.Fatalf("route routing digest unavailable or unstable: %x != %x", first.routingDigest, second.routingDigest)
	}
	for want, point := range points {
		got, ok := first.route(point)
		if !ok || got != want {
			t.Fatalf("route at %d = %d/%v, want %d", point, got, ok, want)
		}
	}
	for raw := 0; raw < first.pointCount; raw++ {
		point := cfg.Point(raw)
		_, classified := first.route(point)
		_, call := owner.Prepared.OperationPlan().Facts().CallSiteView(point)
		if classified != call {
			t.Fatalf("point %d classified=%v call=%v; partition must be total over calls only", point, classified, call)
		}
	}
}

func TestCallRouteRoutingDigestIsNotASemanticCacheFence(t *testing.T) {
	snapshot, owner, points := callRoutePartitionFixture(t)
	contextKey := summary.DefaultSummaryKey(ref.FromSymbol(991_001))
	withSpecialized := func(payload summary.Summary) relationRunSnapshot {
		changed := snapshot
		changed.consumers.entries = append([]relationConsumerEntry(nil), snapshot.consumers.entries...)
		consumer := changed.consumers.byKey[owner.Summary]
		entry := changed.consumers.entries[consumer]
		entry.dependencyKeys = append([]summary.SummaryKey(nil), entry.dependencyKeys...)
		if len(entry.dependencyKeys) == 0 {
			entry.dependencyKeys = make([]summary.SummaryKey, owner.Prepared.OperationPlan().PointCount())
		}
		entry.dependencyKeys[points[callRouteRelationLexical]] = contextKey
		changed.consumers.entries[consumer] = entry
		changed.contexts = []relationContextSummary{{context: contextKey, summary: payload}}
		return changed
	}

	withoutSuspend, err := newCallRoutePartition(withSpecialized(summary.Summary{}), owner)
	if err != nil {
		t.Fatal(err)
	}
	withSuspend, err := newCallRoutePartition(withSpecialized(summary.Summary{MaySuspend: true}), owner)
	if err != nil {
		t.Fatal(err)
	}
	if withoutSuspend.routingDigest != withSuspend.routingDigest {
		t.Fatal("routing-only digest unexpectedly included semantic Summary payload")
	}
	// This equality is the contract: routingDigest may identify provider routing
	// but must never be wired into a result cache as its semantic validity fence.
}

func TestCallRoutePartitionRejectsOwnerWidthGenerationTargetAndShapeDrift(t *testing.T) {
	snapshot, owner, points := callRoutePartitionFixture(t)
	for name, mutate := range map[string]func(*relationConsumerIdentity){
		"summary":     func(identity *relationConsumerIdentity) { identity.Summary.Entry.Values++ },
		"body digest": func(identity *relationConsumerIdentity) { identity.BodyDigest++ },
		"prepared":    func(identity *relationConsumerIdentity) { identity.Prepared = nil },
		"generation":  func(identity *relationConsumerIdentity) { identity.Generation = &relationCatalogGeneration{} },
	} {
		t.Run(name, func(t *testing.T) {
			drifted := owner
			mutate(&drifted)
			if _, err := newCallRoutePartition(snapshot, drifted); err == nil {
				t.Fatal("drifted owner produced a partition")
			}
		})
	}

	consumerIndex := snapshot.consumers.byKey[owner.Summary]
	relationPoint := points[callRouteRelationLexical]
	original, _ := snapshot.consumers.entries[consumerIndex].direct.Lookup(relationPoint)

	withDirect := func(direct transformer.DirectCallCatalog) relationRunSnapshot {
		drifted := snapshot
		drifted.consumers.entries = append([]relationConsumerEntry(nil), snapshot.consumers.entries...)
		drifted.consumers.entries[consumerIndex].direct = direct
		return drifted
	}

	wrongWidth, err := transformer.NewDirectCallCatalog(owner.Prepared.OperationPlan().PointCount()-1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newCallRoutePartition(withDirect(wrongWidth), owner); err == nil || !strings.Contains(err.Error(), "width") {
		t.Fatalf("width drift error = %v", err)
	}

	shapeDrift := original
	shapeDrift.Shape.Params++
	shapeCatalog, err := transformer.NewDirectCallCatalog(owner.Prepared.OperationPlan().PointCount(), map[cfg.Point]transformer.DirectCallTarget{relationPoint: shapeDrift})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newCallRoutePartition(withDirect(shapeCatalog), owner); err == nil || !strings.Contains(err.Error(), "shape") {
		t.Fatalf("shape drift error = %v", err)
	}

	targetGeneration := snapshot
	targetGeneration.identities = make(map[transformer.CellRef]relationCellIdentity, len(snapshot.identities))
	for cell, identity := range snapshot.identities {
		targetGeneration.identities[cell] = identity
	}
	driftedTarget := targetGeneration.identities[original.Cell]
	driftedTarget.Generation = &relationCatalogGeneration{}
	targetGeneration.identities[original.Cell] = driftedTarget
	if _, err := newCallRoutePartition(targetGeneration, owner); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("target generation drift error = %v", err)
	}

	// A relation target at the sealed boundary point is an ownership violation,
	// even when the cell and shape are independently valid.
	boundaryPoint := points[callRouteBoundary]
	misclassified, err := transformer.NewDirectCallCatalog(owner.Prepared.OperationPlan().PointCount(), map[cfg.Point]transformer.DirectCallTarget{
		relationPoint: original,
		boundaryPoint: original,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newCallRoutePartition(withDirect(misclassified), owner); err == nil || !strings.Contains(err.Error(), "non-lexical") {
		t.Fatalf("boundary ownership drift error = %v", err)
	}
}

func TestCallRoutePartitionDispatchesExactlyOneProvider(t *testing.T) {
	relationCatalog, err := relationcall.NewCatalog(4, []relationcall.Route{{Point: 1, Target: relationcall.Target{
		Cell: transformer.CellRef{Function: 1}, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(1)),
		Specialized: summary.Summary{}, HasSpecialized: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	partition := callRoutePartition{
		owner:      relationConsumerIdentity{Summary: summary.DefaultSummaryKey(ref.FromSymbol(2))},
		pointCount: 4,
		kinds: []callRouteKind{
			0, callRouteRelationLexical, callRouteConcreteLexical, callRouteBoundary,
		},
		present:         []bool{false, true, true, true},
		relationCatalog: relationCatalog,
		routingDigest:   callRouteRoutingDigest{1},
		dispatchSeal:    &callRouteDispatchSeal{},
	}
	// A caller-supplied dynamic target is deliberately ignored: binding replaces
	// it with the partition-owned catalog before the resolver is constructed.
	relationProvider, err := partition.bindRelationProvider(relationcall.Config{TargetFor: func(transfer.NodeContext, factflow.CallSiteView) (relationcall.Target, bool) {
		return relationcall.Target{
			Cell: transformer.CellRef{Function: 99}, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(99)),
			Specialized: summary.Summary{MaySuspend: true}, HasSpecialized: true,
		}, true
	}})
	if err != nil {
		t.Fatal(err)
	}
	calls := map[callRouteKind]int{}
	rejections := make([]callRouteRejection, 0)
	providers := callRouteProviders{
		relation: relationProvider,
		concrete: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			calls[callRouteConcreteLexical]++
			return callpayload.CallOutcome{MaySuspend: true}
		},
		boundary: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			calls[callRouteBoundary]++
			return callpayload.CallOutcome{}
		},
		reject: func(rejection callRouteRejection) { rejections = append(rejections, rejection) },
	}
	provider, err := partition.outcomeProvider(providers)
	if err != nil {
		t.Fatal(err)
	}
	if out := provider(transfer.NodeContext{}, callRouteTestSite(1), state.State{}, nil); !out.SuspensionKnown || out.MaySuspend || len(out.Results) != 0 {
		t.Fatalf("partition-owned relation outcome = %#v", out)
	}
	calls[callRouteRelationLexical]++
	if out := provider(transfer.NodeContext{}, callRouteTestSite(2), state.State{}, nil); !out.MaySuspend {
		t.Fatalf("concrete outcome = %#v", out)
	}
	if out := provider(transfer.NodeContext{}, callRouteTestSite(3), state.State{}, nil); !out.Empty() {
		t.Fatalf("boundary outcome = %#v", out)
	}
	for _, kind := range []callRouteKind{callRouteRelationLexical, callRouteConcreteLexical, callRouteBoundary} {
		if calls[kind] != 1 {
			t.Fatalf("provider %d calls = %d, want 1", kind, calls[kind])
		}
	}
	if len(rejections) != 0 {
		t.Fatalf("valid routes rejected: %#v", rejections)
	}
}

func TestCallRoutePartitionRelationMissRejectsWithoutFallback(t *testing.T) {
	relationCatalog, err := relationcall.NewCatalog(2, []relationcall.Route{{Point: 1, Target: relationcall.Target{
		Cell: transformer.CellRef{Function: 1}, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(1)),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	partition := callRoutePartition{
		owner:           relationConsumerIdentity{Summary: summary.DefaultSummaryKey(ref.FromSymbol(2))},
		pointCount:      2,
		kinds:           []callRouteKind{0, callRouteRelationLexical},
		present:         []bool{false, true},
		relationCatalog: relationCatalog,
		routingDigest:   callRouteRoutingDigest{1},
		dispatchSeal:    &callRouteDispatchSeal{},
	}
	relationProvider, err := partition.bindRelationProvider(relationcall.Config{})
	if err != nil {
		t.Fatal(err)
	}
	concreteCalls, boundaryCalls := 0, 0
	var rejected callRouteRejection
	provider, err := partition.outcomeProvider(callRouteProviders{
		relation: relationProvider,
		concrete: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			concreteCalls++
			return callpayload.CallOutcome{MaySuspend: true}
		},
		boundary: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			boundaryCalls++
			return callpayload.CallOutcome{}
		},
		reject: func(got callRouteRejection) { rejected = got },
	})
	if err != nil {
		t.Fatal(err)
	}
	if out := provider(transfer.NodeContext{}, callRouteTestSite(1), state.State{}, nil); !out.Empty() {
		t.Fatalf("relation miss outcome = %#v, want empty rejected attempt", out)
	}
	if concreteCalls != 0 || boundaryCalls != 0 {
		t.Fatalf("fallback provider calls concrete/boundary = %d/%d, want 0/0", concreteCalls, boundaryCalls)
	}
	if rejected.Kind != callRouteRelationLexical || rejected.Point != 1 || rejected.Reason == "" {
		t.Fatalf("rejection = %#v", rejected)
	}

	// Missing and non-call points are rejected before any provider is selected.
	provider(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	if concreteCalls != 0 || boundaryCalls != 0 {
		t.Fatalf("unclassified point invoked a fallback provider: %d/%d", concreteCalls, boundaryCalls)
	}
}

func TestCallRoutePartitionRejectsForeignRelationProviderAndMissingLatch(t *testing.T) {
	partition := callRouteTestRelationPartition(t, callRouteRoutingDigest{1})
	// Routing digests may match while semantic Relation payloads differ. Only
	// the run-local partition seal may authorize a prepared resolver.
	foreign := callRouteTestRelationPartition(t, callRouteRoutingDigest{1})
	foreignProvider, err := foreign.bindRelationProvider(relationcall.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := partition.outcomeProvider(callRouteProviders{
		relation: foreignProvider,
		reject:   func(callRouteRejection) {},
	}); err == nil || !strings.Contains(err.Error(), "identity drift") {
		t.Fatalf("foreign relation provider error = %v", err)
	}

	exactProvider, err := partition.bindRelationProvider(relationcall.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := partition.outcomeProvider(callRouteProviders{relation: exactProvider}); err == nil || !strings.Contains(err.Error(), "latch") {
		t.Fatalf("missing rejection latch error = %v", err)
	}
}

func TestCallRoutePartitionMalformedShapeDoesNotPanic(t *testing.T) {
	catalog, err := relationcall.NewCatalog(2, nil)
	if err != nil {
		t.Fatal(err)
	}
	partition := callRoutePartition{
		pointCount: 2, kinds: []callRouteKind{0, callRouteBoundary}, present: []bool{false},
		relationCatalog: catalog,
	}
	if _, ok := partition.route(1); ok {
		t.Fatal("malformed partition classified a point outside its owned slices")
	}
	if _, err := partition.outcomeProvider(callRouteProviders{reject: func(callRouteRejection) {}}); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed partition error = %v", err)
	}
}

func TestCallRoutePartitionMissingSelectedProviderRejects(t *testing.T) {
	for _, kind := range []callRouteKind{callRouteConcreteLexical, callRouteBoundary} {
		t.Run(string(rune('0'+kind)), func(t *testing.T) {
			catalog, err := relationcall.NewCatalog(2, nil)
			if err != nil {
				t.Fatal(err)
			}
			partition := callRoutePartition{
				pointCount: 2, kinds: []callRouteKind{0, kind}, present: []bool{false, true},
				relationCatalog: catalog, dispatchSeal: &callRouteDispatchSeal{},
			}
			var rejected callRouteRejection
			provider, err := partition.outcomeProvider(callRouteProviders{reject: func(got callRouteRejection) { rejected = got }})
			if err != nil {
				t.Fatal(err)
			}
			if out := provider(transfer.NodeContext{}, callRouteTestSite(1), state.State{}, nil); !out.Empty() {
				t.Fatalf("missing provider outcome = %#v", out)
			}
			if rejected.Kind != kind || rejected.Point != 1 || !strings.Contains(rejected.Reason, "unavailable") {
				t.Fatalf("missing provider rejection = %#v", rejected)
			}
		})
	}
}

func callRouteTestRelationPartition(t *testing.T, digest callRouteRoutingDigest) callRoutePartition {
	t.Helper()
	catalog, err := relationcall.NewCatalog(2, []relationcall.Route{{Point: 1, Target: relationcall.Target{
		Cell: transformer.CellRef{Function: 1}, SummaryKey: summary.DefaultSummaryKey(ref.FromSymbol(1)),
		Specialized: summary.Summary{}, HasSpecialized: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return callRoutePartition{
		owner:      relationConsumerIdentity{Summary: summary.DefaultSummaryKey(ref.FromSymbol(2))},
		pointCount: 2, kinds: []callRouteKind{0, callRouteRelationLexical}, present: []bool{false, true},
		relationCatalog: catalog, routingDigest: digest, dispatchSeal: &callRouteDispatchSeal{},
	}
}

func callRoutePartitionFixture(t *testing.T) (relationRunSnapshot, relationConsumerIdentity, map[callRouteKind]cfg.Point) {
	t.Helper()
	const source = `
local function converted(x: any): any return x end
local function unconverted(x: any): any return x end
local one = converted("one")
local two = unconverted("two")
local boundary = string.match("abc", "a")
return one, two, boundary
`
	stmts := parseChunk(t, source)
	config := Config{Check: body.Config{
		Registry:      standard.Registry(),
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte(source)),
	}}
	var catalog relationRunCatalog
	config.relationCatalogAudit = func(got relationRunCatalog) error {
		catalog = got
		return nil
	}
	if _, err := RunChunk(stmts, config); err != nil {
		t.Fatal(err)
	}
	if catalog.generation == nil {
		t.Fatal("fixture catalog was not captured")
	}

	consumerIndex := -1
	var relationPoints []cfg.Point
	var boundaryPoint cfg.Point
	for index, entry := range catalog.consumers.entries {
		if entry.identity.Prepared == nil || entry.identity.Prepared.OperationPlan() == nil {
			continue
		}
		surface, exact := entry.identity.Prepared.OperationPlan().CallSurface()
		if !exact || len(surface.Sites()) != 3 {
			continue
		}
		for _, site := range surface.Sites() {
			if _, routed := entry.direct.Lookup(site.Point); routed {
				relationPoints = append(relationPoints, site.Point)
			} else if site.Target.Kind() != operationplan.CallSurfaceTargetLexical {
				boundaryPoint = site.Point
			}
		}
		if len(relationPoints) == 2 && boundaryPoint != 0 {
			consumerIndex = index
			break
		}
		relationPoints = relationPoints[:0]
		boundaryPoint = 0
	}
	if consumerIndex < 0 {
		t.Fatal("fixture did not produce a caller with two lexical relation routes and one boundary route")
	}

	entry := &catalog.consumers.entries[consumerIndex]
	convertedPoint, concretePoint := relationPoints[0], relationPoints[1]
	convertedTarget, _ := entry.direct.Lookup(convertedPoint)
	partial, err := transformer.NewDirectCallCatalog(entry.direct.PointCount(), map[cfg.Point]transformer.DirectCallTarget{convertedPoint: convertedTarget})
	if err != nil {
		t.Fatal(err)
	}
	entry.direct = partial
	entry.active = true
	snapshot, err := catalog.Freeze(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, entry.identity, map[callRouteKind]cfg.Point{
		callRouteRelationLexical: relationPoints[0],
		callRouteConcreteLexical: concretePoint,
		callRouteBoundary:        boundaryPoint,
	}
}

func callRouteTestSite(point cfg.Point) factflow.CallSiteView {
	return factflow.NewCallSite(factflow.CallSiteConfig{Point: point, HasPoint: true}).View()
}
