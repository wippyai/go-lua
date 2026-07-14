package evaluated

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type rootFixture struct {
	parts        Parts
	requirements operationplan.ObservationRequirements
}

func TestShadowRootRejectsMissingDuplicateWrongStageOrderAndForgery(t *testing.T) {
	fixture := validRootFixture(t)
	tests := []struct {
		name   string
		mutate func(*Parts)
	}{
		{"missing", func(parts *Parts) { parts.Points = parts.Points[:len(parts.Points)-1] }},
		{"duplicate", func(parts *Parts) { parts.Points = append(parts.Points, parts.Points[len(parts.Points)-1]) }},
		{"wrong-stage", func(parts *Parts) { parts.Edges[0].Slot = parts.Points[0].Slot }},
		{"order", func(parts *Parts) { parts.Points[0], parts.Points[1] = parts.Points[1], parts.Points[0] }},
		{"forged-equal-authority", func(parts *Parts) {
			parts.Identity.Relation = availableTestAuthority(1)
			parts.Identity.Entry = availableTestAuthority(2)
			parts.Identity.Lineage = availableTestAuthority(3)
			parts.Identity.Registry = availableTestAuthority(4)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parts := cloneParts(fixture.parts)
			test.mutate(&parts)
			root, err := NewShadowRoot(context.Background(), standard.Registry(), fixture.requirements, false, parts)
			if err == nil || root.Coverage() != (Coverage{}) {
				t.Fatalf("invalid root published: %#v %v", root.Coverage(), err)
			}
		})
	}
}

func TestShadowRootOwnsTypedSlicesAndStaysNonAuthoritative(t *testing.T) {
	fixture := validRootFixture(t)
	root, err := NewShadowRoot(context.Background(), standard.Registry(), fixture.requirements, false, fixture.parts)
	if err != nil {
		t.Fatal(err)
	}
	if root.Authoritative() || !root.Coverage().Complete() {
		t.Fatalf("authority/coverage = %v/%#v", root.Authoritative(), root.Coverage())
	}
	want := root.Points()[0].Point
	fixture.parts.Points[0].Point++
	if root.Points()[0].Point != want {
		t.Fatal("root retained mutable constructor slice")
	}
	got := root.Points()
	got[0].Point++
	if root.Points()[0].Point != want {
		t.Fatal("root getter exposed mutable slice")
	}
}

func TestShadowRootRejectsProjectionViewMismatch(t *testing.T) {
	fixture := validRootFixture(t)
	fixture.parts.Identity.View.Mode = ProjectionWithCallOutcome
	root, err := NewShadowRoot(context.Background(), standard.Registry(), fixture.requirements, false, fixture.parts)
	if err == nil || root.Coverage() != (Coverage{}) {
		t.Fatalf("foreign projection view published: %#v %v", root.Coverage(), err)
	}
}

func TestProjectionViewSeparatesFilteredCallOutcomeInventory(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	lowered := wir.NewBody("call-view")
	start := lowered.Len()
	lowered.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	lowered.SetPointRange(call, start, lowered.Len())
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 89
	site := factflow.NewCallSite(factflow.CallSiteConfig{Point: call, HasPoint: true, Final: true})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{call: site}}).WithObservationIdentity(owner, lowered, graph)
	requirements, ok := plan.ObservationRequirements()
	if !ok {
		t.Fatal("call requirements")
	}
	filtered, err := SealProjectionView(requirements, false)
	if err != nil {
		t.Fatal(err)
	}
	full, err := SealProjectionView(requirements, true)
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Mode == full.Mode || filtered.Digest == full.Digest || filtered.Slots+1 != full.Slots {
		t.Fatalf("filtered/full views = %#v/%#v", filtered, full)
	}
}

func TestShadowRootRejectsUnsafeObservationActualAndExpected(t *testing.T) {
	fixture := validObservationRootFixture(t)
	reg, unsafe := unsafeObservationRegistry(t)
	for _, test := range []struct {
		name   string
		mutate func(*Observation)
	}{
		{name: "actual", mutate: func(item *Observation) { item.Actual = unsafe }},
		{name: "expected", mutate: func(item *Observation) { item.Expected, item.HasExpected = unsafe, true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			parts := cloneParts(fixture.parts)
			parts.Observations = cloneObservations(fixture.parts.Observations)
			test.mutate(&parts.Observations[0].Observed[0])
			root, err := NewShadowRoot(context.Background(), reg, fixture.requirements, false, parts)
			if err == nil || root.Coverage() != (Coverage{}) {
				t.Fatalf("unsafe observation published root %#v, %v", root.Coverage(), err)
			}
		})
	}
}

func TestShadowRootCancelsInsideLargeObservationList(t *testing.T) {
	fixture := validObservationRootFixture(t)
	parts := cloneParts(fixture.parts)
	parts.Observations = cloneObservations(fixture.parts.Observations)
	item := parts.Observations[0].Observed[0]
	parts.Observations[0].Observed = make([]Observation, 4096)
	for index := range parts.Observations[0].Observed {
		parts.Observations[0].Observed[index] = item
	}
	ctx := &cancelRootAfterChecksContext{remaining: 20}
	root, err := NewShadowRoot(ctx, standard.Registry(), fixture.requirements, false, parts)
	if err == nil || !strings.Contains(err.Error(), "canceled") || root.Coverage() != (Coverage{}) {
		t.Fatalf("mid-observation cancellation published %#v, %v", root.Coverage(), err)
	}
}

type cancelRootAfterChecksContext struct{ remaining int }

func (*cancelRootAfterChecksContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*cancelRootAfterChecksContext) Done() <-chan struct{}       { return nil }
func (c *cancelRootAfterChecksContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}
func (*cancelRootAfterChecksContext) Value(any) any { return nil }

type unsafeObservationAxis struct{ safe bool }

func unsafeObservationRegistry(t *testing.T) (*axis.Registry, product.Value) {
	t.Helper()
	key := axis.NewKey[unsafeObservationAxis]("test.evaluated.observation-retention")
	reg := axis.NewRegistry()
	axis.Register(reg, axis.Spec[unsafeObservationAxis]{
		Key: key, Bottom: func() unsafeObservationAxis { return unsafeObservationAxis{} },
		Top:      func() unsafeObservationAxis { return unsafeObservationAxis{safe: true} },
		Equal:    func(a, b unsafeObservationAxis) bool { return a == b },
		LessOrEq: func(a, b unsafeObservationAxis) bool { return !a.safe || b.safe },
		Join: func(a, b unsafeObservationAxis) unsafeObservationAxis {
			return unsafeObservationAxis{safe: a.safe || b.safe}
		},
		Meet: func(a, b unsafeObservationAxis) unsafeObservationAxis {
			return unsafeObservationAxis{safe: a.safe && b.safe}
		},
		Hash: func(v unsafeObservationAxis) uint64 {
			if v.safe {
				return 1
			}
			return 0
		},
		Boundary:  axis.PortableIdentity,
		Retention: axis.ValidatedRetention(func(value unsafeObservationAxis) bool { return value.safe }),
	})
	reg.Freeze()
	return reg, product.Set(reg, product.Top(), key, unsafeObservationAxis{safe: false})
}

func validObservationRootFixture(t *testing.T) rootFixture {
	t.Helper()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	lowered := wir.NewBody("observation-root")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 93
	plan := operationplan.New(graph, factflow.FactsInput{
		RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: {}},
	}).WithObservationIdentity(owner, lowered, graph)
	requirements, ok := plan.ObservationRequirements()
	if !ok {
		t.Fatal("observation requirements")
	}
	surface, err := operationplan.SealCallSurface(owner, graph.Size(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, err := SealProjectionView(requirements, false)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := AuthorityDigest{Status: AuthorityUnavailable}
	identity := Identity{Body: owner, Relation: unavailable, Entry: unavailable, Lineage: unavailable, Registry: unavailable,
		CallSurface: surface.Digest(), Schema: requirements.SchemaID(), Inventory: requirements.ConsumerInventoryID(), View: view, PointCount: uint32(graph.Size())}
	parts := Parts{Identity: identity}
	for slot, requirement := range requirements.Entries(false) {
		slotID := uint32(slot)
		switch requirement.Stage() {
		case operationplan.RequirementPoint:
			parts.Points = append(parts.Points, PointReachability{Slot: slotID, Point: requirement.Point(), Worlds: WorldSet{Root: DecisionTrue}})
		case operationplan.RequirementBoundary:
			parts.Boundaries = append(parts.Boundaries, Boundary{Slot: slotID, Point: requirement.Point(), Fragments: []BoundaryFragment{{
				Worlds: WorldSet{Root: DecisionTrue}, Values: []IndexedValue{{Value: product.Top()}}, Summary: summary.Summary{},
			}}})
		case operationplan.RequirementEdge:
			to, _ := requirement.EdgeTarget()
			parts.Edges = append(parts.Edges, EdgeReachability{Slot: slotID, From: requirement.Point(), To: to, Worlds: WorldSet{Root: DecisionTrue}})
		case operationplan.RequirementObservation:
			anchor, hasAnchor := requirement.Anchor()
			kind, hasKind := requirement.ObservationKind()
			if !hasAnchor || !hasKind {
				t.Fatal("malformed observation requirement")
			}
			parts.Observations = append(parts.Observations, ObservationSlot{Slot: slotID, Point: requirement.Point(), Observed: []Observation{{
				Worlds: WorldSet{Root: DecisionTrue}, Owner: owner, Kind: kind, Anchor: anchor, Slot: anchor.Slot, Actual: product.Top(),
			}}})
		case operationplan.RequirementRoute:
			anchor, _ := requirement.Anchor()
			parts.Routes = append(parts.Routes, Route{Slot: slotID, Point: requirement.Point(), Anchor: anchor, Worlds: WorldSet{Root: DecisionTrue}})
		default:
			t.Fatalf("unexpected observation fixture stage %d", requirement.Stage())
		}
	}
	if len(parts.Observations) == 0 {
		t.Fatal("fixture has no observation slot")
	}
	return rootFixture{parts: parts, requirements: requirements}
}

func validRootFixture(t *testing.T) rootFixture {
	t.Helper()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeNoop)
	right := graph.AddNode(cfg.NodeNoop)
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, left, true)
	graph.AddEdge(branch, right, false)
	graph.AddEdge(left, ret, false)
	graph.AddEdge(right, ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, _ := factflow.NewValueSourceShape(false, false, false, false)
	source, _ := factflow.NewStringLiteralValueSource("root", 0, 0, 0, shape)
	lowered := wir.NewBody("root")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 71
	plan := operationplan.New(graph, factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{ret: factflow.NewReturn([]factflow.ValueSource{source})}}).WithObservationIdentity(owner, lowered, graph)
	requirements, ok := plan.ObservationRequirements()
	if !ok {
		t.Fatal("requirements")
	}
	surface, err := operationplan.SealCallSurface(owner, graph.Size(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	view, err := SealProjectionView(requirements, false)
	if err != nil {
		t.Fatal(err)
	}
	unavailable := AuthorityDigest{Status: AuthorityUnavailable}
	identity := Identity{Body: owner, Relation: unavailable, Entry: unavailable, Lineage: unavailable, Registry: unavailable,
		CallSurface: surface.Digest(), Schema: requirements.SchemaID(), Inventory: requirements.ConsumerInventoryID(), View: view, PointCount: uint32(graph.Size())}
	parts := Parts{Identity: identity, Proof: WorldProof{}, Summary: summary.Summary{Returns: []product.Value{product.Top()}}}
	for slot, requirement := range requirements.Entries(false) {
		switch requirement.Stage() {
		case operationplan.RequirementPoint:
			parts.Points = append(parts.Points, PointReachability{Slot: uint32(slot), Point: requirement.Point(), Worlds: WorldSet{Root: DecisionTrue}})
		case operationplan.RequirementBoundary:
			parts.Boundaries = append(parts.Boundaries, Boundary{Slot: uint32(slot), Point: requirement.Point(), Fragments: []BoundaryFragment{{Worlds: WorldSet{Root: DecisionTrue}, Values: []IndexedValue{{Value: product.Top()}}, Summary: summary.Summary{Returns: []product.Value{product.Top()}}}}})
		case operationplan.RequirementEdge:
			to, _ := requirement.EdgeTarget()
			parts.Edges = append(parts.Edges, EdgeReachability{Slot: uint32(slot), From: requirement.Point(), To: to, Worlds: WorldSet{Root: DecisionTrue}})
		default:
			t.Fatalf("stage %d", requirement.Stage())
		}
	}
	return rootFixture{parts: parts, requirements: requirements}
}

func cloneParts(in Parts) Parts {
	out := in
	out.Points = append([]PointReachability(nil), in.Points...)
	out.Edges = append([]EdgeReachability(nil), in.Edges...)
	out.Boundaries = cloneBoundaries(in.Boundaries)
	return out
}

func availableTestAuthority(seed byte) AuthorityDigest {
	var value Digest
	value[0] = seed
	return AuthorityDigest{Status: AuthorityAvailable, Value: value}
}
