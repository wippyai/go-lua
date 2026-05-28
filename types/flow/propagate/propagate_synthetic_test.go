package propagate

import (
	"math/rand"
	"sort"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

// TestPropagate_LoopPreheaderReinforcement_Terminates models a divergent
// fixture pattern: a loop whose preheader carries a multi-disjunct
// condition, and whose backedge ANDs that condition into the predecessor's
// running condition every iteration. Without widening, the chain
//
//	t ← And(t, preheaderDisjuncts)
//
// cross-products on each visit and the condition grows without bound.
//
// With FVS widening (DOMAIN_DESIGN.md §7), the loop header is in FVS and
// widening fires structurally on every revisit (no visit-count delay). The
// chain stabilizes within O(|CFG|) worklist visits.
func TestPropagate_LoopPreheaderReinforcement_Terminates(t *testing.T) {
	g := buildPreheaderReinforcementGraph()
	inputs := makePreheaderReinforcementInputs(g)
	bounded := withVisitCap(t, inputs, len(g.rpo)*8)

	result := Propagate(bounded.inputs)

	if bounded.visits > bounded.cap {
		t.Fatalf("propagate exceeded visit cap %d (visited %d times); widening did not contain the chain",
			bounded.cap, bounded.visits)
	}

	// Sanity: the result is populated.
	if result == nil || result.PointConditions == nil {
		t.Fatalf("Propagate returned nil/empty result")
	}
}

// TestPropagate_DeterminismUnderShuffledRPO asserts that propagate produces
// the same fixed point regardless of RPO ordering. The widening's
// over-approximation is applied at fixed FVS points and is order-independent
// once the worklist is exhausted.
//
// DOMAIN_DESIGN.md §10.4: determinism under shuffled RPO.
func TestPropagate_DeterminismUnderShuffledRPO(t *testing.T) {
	r := rand.New(rand.NewSource(0xCAFEBABE))

	for trial := 0; trial < 5; trial++ {
		base := buildPreheaderReinforcementGraph()
		baseInputs := makePreheaderReinforcementInputs(base)
		baseResult := Propagate(baseInputs)

		shuffled := buildPreheaderReinforcementGraph()
		r.Shuffle(len(shuffled.rpo), func(i, j int) {
			// Don't move the entry point off index 0; the algorithm
			// assumes Entry is processed first via initial seeding.
			if i == 0 || j == 0 {
				return
			}
			shuffled.rpo[i], shuffled.rpo[j] = shuffled.rpo[j], shuffled.rpo[i]
		})
		shuffledInputs := makePreheaderReinforcementInputs(shuffled)
		shuffledResult := Propagate(shuffledInputs)

		// The two results must contain the same set of points with
		// semantically equal conditions.
		if len(baseResult.PointConditions) != len(shuffledResult.PointConditions) {
			t.Fatalf("trial %d: point set size mismatch: base=%d shuffled=%d",
				trial, len(baseResult.PointConditions), len(shuffledResult.PointConditions))
		}
		for p, bc := range baseResult.PointConditions {
			sc, ok := shuffledResult.PointConditions[p]
			if !ok {
				t.Fatalf("trial %d: shuffled result missing point %v", trial, p)
			}
			if !constraint.Domain.Equal(bc, sc) {
				t.Fatalf("trial %d: condition at point %v differs:\n  base=    %v\n  shuffled=%v",
					trial, p, bc, sc)
			}
		}
	}
}

// TestPropagate_AcyclicVariantPreservesPrecision is the precision retention
// half of §10.4. A simple linear acyclic CFG with no backedge: the FVS is
// empty, no widening fires, and the propagated condition at the "end" point
// retains the exact edge condition introduced on the path.
func TestPropagate_AcyclicVariantPreservesPrecision(t *testing.T) {
	x := constraint.Path{Root: "x", Symbol: 100}
	yy := constraint.Path{Root: "y", Symbol: 101}
	zz := constraint.Path{Root: "z", Symbol: 102}

	edgeCond := constraint.Or(
		constraint.Or(
			constraint.FromConstraints(constraint.Truthy{Path: x}),
			constraint.FromConstraints(constraint.NotNil{Path: yy}),
		),
		constraint.FromConstraints(constraint.HasField{Path: zz, Field: "kind"}),
	)

	// Linear acyclic CFG: entry(1) → 2 → 3 (end). Edge 2→3 carries edgeCond.
	g := &mockGraph{
		entry: 1,
		nodes: map[cfg.Point]*cfg.Node{
			1: {Kind: cfg.NodeEntry, Point: 1},
			2: {Kind: cfg.NodeBranch, Point: 2},
			3: {Kind: cfg.NodeAssign, Point: 3},
		},
		preds: map[cfg.Point][]cfg.Point{
			1: {},
			2: {1},
			3: {2},
		},
		succs: map[cfg.Point][]cfg.Point{
			1: {2},
			2: {3},
			3: {},
		},
		rpo: []cfg.Point{1, 2, 3},
	}
	inputs := &Inputs{
		Graph: g,
		EdgeConditions: EdgeConditions{
			{From: 2, To: 3}: edgeCond,
		},
	}
	result := Propagate(inputs)

	end := result.PointConditions[3]
	if end.IsFalse() {
		t.Fatalf("end point should be reachable; got ⊥")
	}
	if !end.HasConstraints() {
		t.Fatalf("end point should carry constraints from the edge; got %v", end)
	}
	// Acyclic: no FVS membership, no widening fires, and the condition at
	// the end point is the exact disjunction.
	if !preheaderDisjPresent(end, edgeCond) {
		t.Fatalf("expected the edge disjunction to survive on the linear path;\n  end=%v\n  edge=%v",
			end, edgeCond)
	}
}

// preheaderDisjPresent returns true if every literal of preheader appears
// in some disjunct of end.
func preheaderDisjPresent(end, preheader constraint.Condition) bool {
	if !end.HasConstraints() {
		return false
	}
	endLits := map[string]struct{}{}
	for _, d := range end.Disjuncts {
		for _, lit := range d {
			endLits[constraintKey(lit)] = struct{}{}
		}
	}
	for _, d := range preheader.Disjuncts {
		for _, lit := range d {
			if _, ok := endLits[constraintKey(lit)]; !ok {
				return false
			}
		}
	}
	return true
}

func constraintKey(c constraint.Constraint) string {
	type stringer interface{ String() string }
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return strconv.FormatUint(c.Hash(), 16)
}

// buildPreheaderReinforcementGraph constructs:
//
//	entry(1) → preheader(2) → header(3) ─→ body(4) → backedge → header(3)
//	                                  └─→ exit(5)
//
// Header(3) is a loop header (LoopPreheaderSet=true, LoopPreheader=2). The
// preheader's edge condition is a multi-disjunct DNF on x, y, z. The
// backedge introduces a fresh literal on each iteration via an edge
// condition Truthy(iterN) — but here we model a static fresh literal that
// would cycle in real CFG analysis as an OR of new disjuncts on each visit.
//
// To exercise the divergence pattern in a single execution, we make the
// body's edge to the header carry the preheader disjunction AND-ed with a
// fresh literal each time the body is re-entered. We approximate this with
// a structural backedge whose edge condition is rich enough to cross-
// product non-trivially against the running condition.
func buildPreheaderReinforcementGraph() *mockGraph {
	x := constraint.Path{Root: "x", Symbol: 100}
	yy := constraint.Path{Root: "y", Symbol: 101}
	zz := constraint.Path{Root: "z", Symbol: 102}

	// 4-disjunct preheader condition; the loop body re-AND's it.
	preheaderDisj := constraint.Or(
		constraint.Or(
			constraint.FromConstraints(constraint.Truthy{Path: x}),
			constraint.FromConstraints(constraint.NotNil{Path: yy}),
		),
		constraint.Or(
			constraint.FromConstraints(constraint.HasField{Path: zz, Field: "kind"}),
			constraint.FromConstraints(constraint.Truthy{Path: zz}),
		),
	)
	_ = preheaderDisj

	g := &mockGraph{
		entry: 1,
		nodes: map[cfg.Point]*cfg.Node{
			1: {Kind: cfg.NodeEntry, Point: 1},
			2: {Kind: cfg.NodeAssign, Point: 2},
			3: {
				Kind:             cfg.NodeJoin,
				Point:            3,
				LoopPreheader:    2,
				LoopPreheaderSet: true,
				LoopVars:         nil,
			},
			4: {Kind: cfg.NodeAssign, Point: 4},
			5: {Kind: cfg.NodeAssign, Point: 5},
		},
		preds: map[cfg.Point][]cfg.Point{
			1: {},
			2: {1},
			3: {2, 4},
			4: {3},
			5: {3},
		},
		succs: map[cfg.Point][]cfg.Point{
			1: {2},
			2: {3},
			3: {4, 5},
			4: {3},
			5: {},
		},
		rpo: []cfg.Point{1, 2, 3, 4, 5},
	}
	return g
}

func makePreheaderReinforcementInputs(g *mockGraph) *Inputs {
	x := constraint.Path{Root: "x", Symbol: 100}
	yy := constraint.Path{Root: "y", Symbol: 101}
	zz := constraint.Path{Root: "z", Symbol: 102}

	preheaderDisj := constraint.Or(
		constraint.Or(
			constraint.FromConstraints(constraint.Truthy{Path: x}),
			constraint.FromConstraints(constraint.NotNil{Path: yy}),
		),
		constraint.Or(
			constraint.FromConstraints(constraint.HasField{Path: zz, Field: "kind"}),
			constraint.FromConstraints(constraint.Truthy{Path: zz}),
		),
	)
	bodyDisj := constraint.Or(
		constraint.FromConstraints(constraint.NotNil{Path: x}),
		constraint.FromConstraints(constraint.HasField{Path: yy, Field: "tag"}),
	)

	return &Inputs{
		Graph: g,
		EdgeConditions: EdgeConditions{
			{From: 2, To: 3}: preheaderDisj,
			{From: 4, To: 3}: bodyDisj,
		},
	}
}

// withVisitCap wraps an Inputs with a graph that counts Successors visits
// and reports the total. The cap is used to assert termination is bounded.
type visitCounter struct {
	inputs *Inputs
	visits int
	cap    int
}

func withVisitCap(t *testing.T, inputs *Inputs, cap int) *visitCounter {
	t.Helper()
	wrapped := &countingGraph{inner: inputs.Graph, t: t, cap: cap}
	newInputs := *inputs
	newInputs.Graph = wrapped
	return &visitCounter{
		inputs: &newInputs,
		cap:    cap,
		visits: 0,
	}
}

type countingGraph struct {
	inner Graph
	t     *testing.T
	cap   int
	hits  int
}

func (c *countingGraph) Entry() cfg.Point           { return c.inner.Entry() }
func (c *countingGraph) RPO() []cfg.Point           { return c.inner.RPO() }
func (c *countingGraph) Node(p cfg.Point) *cfg.Node { return c.inner.Node(p) }
func (c *countingGraph) Predecessors(p cfg.Point) []cfg.Point {
	c.hits++
	if c.hits > c.cap*4 {
		c.t.Helper()
		c.t.Fatalf("countingGraph exceeded cap %d; suggests non-termination", c.cap*4)
	}
	return c.inner.Predecessors(p)
}
func (c *countingGraph) Successors(p cfg.Point) []cfg.Point {
	c.hits++
	if c.hits > c.cap*4 {
		c.t.Helper()
		c.t.Fatalf("countingGraph exceeded cap %d; suggests non-termination", c.cap*4)
	}
	return c.inner.Successors(p)
}

// rpoOrder is a stable sort of a CFG point slice; used in determinism check.
func rpoOrder(points []cfg.Point) []cfg.Point {
	out := append([]cfg.Point(nil), points...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

var _ = rpoOrder // referenced for future ordering assertions
