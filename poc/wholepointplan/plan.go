// Package wholepointplan is an isolated proof that all point-local factflow
// facts can be compiled into one explicit, deterministic operation row before
// execution. Production does not import this package.
package wholepointplan

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Phase identifies which concrete transfer consumes a fact.
type Phase uint8

const (
	Node Phase = iota + 1
	Edge
)

// Barrier is a semantic sequencing boundary in the current concrete
// applicators. Its numeric value is deliberately irrelevant: Cursor ordering
// is defined by canonicalBarriers, not by declaration or FactsInput field
// order.
type Barrier uint8

const (
	N0Materialize Barrier = iota + 1
	N1NoReturn
	N2ImplicationClosure
	N3Postconditions
	N4Writes
	N5Return
	N6CovariantFinalizer
	E0Reachability
	E1Refinements
	E2ImplicationClosure
	E3Relations
	E4Evidence
	E5CallEffects
)

var canonicalBarriers = [...]Barrier{
	N0Materialize, N1NoReturn, N2ImplicationClosure, N3Postconditions,
	N4Writes, N5Return, N6CovariantFinalizer,
	E0Reachability, E1Refinements, E2ImplicationClosure, E3Relations,
	E4Evidence, E5CallEffects,
}

// BarrierSet records the exact (possibly non-contiguous) barriers owned by a
// composite transaction. It avoids pretending that call processing executes
// in every barrier between N0 materialization and E5 call-edge effects.
type BarrierSet uint16

func barriers(values ...Barrier) BarrierSet {
	var set BarrierSet
	for _, barrier := range values {
		set |= 1 << (barrier - 1)
	}
	return set
}

// Has reports whether the transaction participates in barrier.
func (s BarrierSet) Has(barrier Barrier) bool {
	return barrier != 0 && s&(1<<(barrier-1)) != 0
}

// OperationRole distinguishes executable semantic facts from auxiliary data
// owned by a composite transaction. A sidecar is emitted by Cursor so the
// plan is exhaustive, but a future executor must consume it through Owner
// rather than execute it as an independent state transition.
type OperationRole uint8

const (
	Semantic OperationRole = iota + 1
	Sidecar
)

// Kind names a point-local semantic fact in canonical execution order.
// A row records each fact exactly once. Some production kernels use a fact at
// more than one internal stage (notably channel-select materialization); that
// staging stays inside the shared concrete kernel rather than duplicating the
// fact in the row.
type Kind string

const (
	OpCallSite                    Kind = "call-site"
	OpNoNormalReturn              Kind = "no-normal-return"
	OpPathPresenceImplication     Kind = "path-presence-implication"
	OpPathDescendantInvalidation  Kind = "path-descendant-invalidation"
	OpPostconditionRefinement     Kind = "postcondition-refinement"
	OpPostconditionPathRelation   Kind = "postcondition-path-relation"
	OpChannelSelect               Kind = "channel-select"
	OpDynamicIndexWrite           Kind = "dynamic-index-write"
	OpRootAssignment              Kind = "root-assignment"
	OpPathAssignment              Kind = "path-assignment"
	OpPathStaticMemberWrite       Kind = "path-static-member-write"
	OpReturn                      Kind = "return"
	OpCovariantExposure           Kind = "covariant-exposure"
	OpCallResultValue             Kind = "call-result-value"
	OpReturnPresenceRelation      Kind = "return-presence-relation"
	OpBranchReachability          Kind = "branch-reachability"
	OpBranchConditionSource       Kind = "branch-condition-source"
	OpBranchRefinement            Kind = "branch-refinement"
	OpBranchLengthRefinement      Kind = "branch-length-refinement"
	OpBranchNumberFloor           Kind = "branch-number-floor"
	OpBranchNumberCeil            Kind = "branch-number-ceil"
	OpBranchDifferenceConstraint  Kind = "branch-difference-constraint"
	OpBranchPresenceRelation      Kind = "branch-presence-relation"
	OpBranchPathRelation          Kind = "branch-path-relation"
	OpBranchPathEvidence          Kind = "branch-path-evidence"
	OpBranchSufficientLiteralCase Kind = "branch-sufficient-literal-case"
)

// Operation is one immutable fact occurrence. Kind and Ordinal address the
// occurrence in Plan's private immutable Facts snapshot; the row never retains
// caller-owned maps or slices. Ordinal preserves order within a fact set.
type Operation struct {
	Phase   Phase
	Barrier Barrier
	Stages  BarrierSet
	Kind    Kind
	Role    OperationRole
	Owner   Kind
	Ordinal int
}

// operationSpec is the single classification registry for point-keyed facts.
// Field is tested against factflow.FactsInput by reflection. Stages is explicit
// for a composite transaction whose declaration participates in more than one
// non-contiguous concrete barrier; zero means only Barrier.
type operationSpec struct {
	Field   string
	Kind    Kind
	Phase   Phase
	Barrier Barrier
	Stages  BarrierSet
	Role    OperationRole
	Owner   Kind
}

var operationSpecs = [...]operationSpec{
	// Node transfer: materialize call/channel outcomes before every other fact.
	{Field: "CallSites", Kind: OpCallSite, Phase: Node, Barrier: N0Materialize, Stages: barriers(N0Materialize, E5CallEffects), Role: Semantic},
	{Field: "CallResultValues", Kind: OpCallResultValue, Phase: Node, Barrier: N0Materialize, Role: Sidecar, Owner: OpCallSite},
	{Field: "ChannelSelects", Kind: OpChannelSelect, Phase: Node, Barrier: N0Materialize, Stages: barriers(N0Materialize, N3Postconditions), Role: Semantic},
	{Field: "NoNormalReturns", Kind: OpNoNormalReturn, Phase: Node, Barrier: N1NoReturn, Role: Semantic},
	{Field: "PathValuePresenceImplications", Kind: OpPathPresenceImplication, Phase: Node, Barrier: N2ImplicationClosure, Role: Semantic},
	{Field: "PathDescendantInvalidations", Kind: OpPathDescendantInvalidation, Phase: Node, Barrier: N3Postconditions, Role: Semantic},
	{Field: "PostconditionRefinements", Kind: OpPostconditionRefinement, Phase: Node, Barrier: N3Postconditions, Role: Semantic},
	{Field: "PostconditionPathRelations", Kind: OpPostconditionPathRelation, Phase: Node, Barrier: N3Postconditions, Role: Semantic},
	{Field: "DynamicIndexWrites", Kind: OpDynamicIndexWrite, Phase: Node, Barrier: N4Writes, Role: Semantic},
	{Field: "RootAssignments", Kind: OpRootAssignment, Phase: Node, Barrier: N4Writes, Role: Semantic},
	{Field: "PathAssignments", Kind: OpPathAssignment, Phase: Node, Barrier: N4Writes, Role: Semantic},
	{Field: "PathStaticMemberWrites", Kind: OpPathStaticMemberWrite, Phase: Node, Barrier: N4Writes, Role: Semantic},
	{Field: "Returns", Kind: OpReturn, Phase: Node, Barrier: N5Return, Role: Semantic},
	{Field: "ReturnPresenceRelations", Kind: OpReturnPresenceRelation, Phase: Node, Barrier: N5Return, Role: Sidecar, Owner: OpReturn},
	{Field: "CovariantExposures", Kind: OpCovariantExposure, Phase: Node, Barrier: N6CovariantFinalizer, Role: Semantic},

	// Edge transfer: reachability and guard refinements precede implication
	// closure; scalar/path relations, evidence, and call effects then observe it.
	{Field: "BranchEdgeReachability", Kind: OpBranchReachability, Phase: Edge, Barrier: E0Reachability, Role: Semantic},
	{Field: "BranchConditionSources", Kind: OpBranchConditionSource, Phase: Edge, Barrier: E0Reachability, Role: Sidecar, Owner: OpBranchReachability},
	{Field: "BranchRefinements", Kind: OpBranchRefinement, Phase: Edge, Barrier: E1Refinements, Role: Semantic},
	{Field: "BranchSufficientLiteralCases", Kind: OpBranchSufficientLiteralCase, Phase: Edge, Barrier: E1Refinements, Role: Sidecar, Owner: OpBranchRefinement},
	// BranchRefinements owns five ordered sub-kinds. The latter four execute
	// after implication closure, so their explicit rows use E3Relations below.
	{Kind: OpBranchLengthRefinement, Phase: Edge, Barrier: E3Relations, Role: Semantic},
	{Kind: OpBranchNumberFloor, Phase: Edge, Barrier: E3Relations, Role: Semantic},
	{Kind: OpBranchNumberCeil, Phase: Edge, Barrier: E3Relations, Role: Semantic},
	{Kind: OpBranchDifferenceConstraint, Phase: Edge, Barrier: E3Relations, Role: Semantic},
	{Field: "BranchPresenceRelations", Kind: OpBranchPresenceRelation, Phase: Edge, Barrier: E3Relations, Role: Semantic},
	{Field: "BranchPathRelations", Kind: OpBranchPathRelation, Phase: Edge, Barrier: E3Relations, Role: Semantic},
	{Field: "BranchPathEvidence", Kind: OpBranchPathEvidence, Phase: Edge, Barrier: E4Evidence, Role: Semantic},
}

// Extension describes a generic transfer contributed outside factflow. This
// POC deliberately has no generic operation ABI yet, so extensions fail closed
// before any executable plan is returned.
type Extension struct {
	Point cfg.Point
	Phase Phase
	Name  string
}

// Config supplies the concrete oracle dependencies. Compile replaces Facts in
// both configs with its own immutable snapshot.
type Config struct {
	Node       factapply.FactsNodeTransferConfig
	Edge       factapply.FactsEdgeTransferConfig
	Extensions []Extension
}

// Plan owns immutable rows and delegates execution to the current concrete
// kernels. This proves the compilation seam without creating a second semantic
// implementation prematurely.
type Plan struct {
	facts        factflow.Facts
	rows         map[cfg.Point][]Operation
	nodeTransfer transfer.NodeTransfer
	edgeTransfer transfer.EdgeTransfer
}

// Compile validates the entire input first, snapshots facts, and then builds
// all rows. On any unsupported extension it returns the zero Plan atomically.
func Compile(input factflow.FactsInput, config Config) (Plan, error) {
	for _, extension := range config.Extensions {
		if extension.Name == "" {
			return Plan{}, fmt.Errorf("wholepointplan: unnamed generic transfer at point %d", extension.Point)
		}
		return Plan{}, fmt.Errorf("wholepointplan: unsupported generic %s transfer %q at point %d", phaseName(extension.Phase), extension.Name, extension.Point)
	}
	facts := factflow.NewFacts(input)
	config.Node.Facts = facts
	config.Edge.Facts = facts
	return Plan{
		facts:        facts,
		rows:         compileRows(input),
		nodeTransfer: factapply.NewFactsNodeTransfer(config.Node),
		edgeTransfer: factapply.NewFactsEdgeTransfer(config.Edge),
	}, nil
}

func phaseName(phase Phase) string {
	if phase == Edge {
		return "edge"
	}
	return "node"
}

// Cursor walks a point's declarations in canonical semantic order without
// exposing Plan's backing row. It is metadata only: execution continues to use
// the unchanged production transfers below.
type Cursor struct {
	row  []Operation
	next int
}

// Cursor returns an allocation-free iterator over point's immutable row.
func (p Plan) Cursor(point cfg.Point) Cursor { return Cursor{row: p.rows[point]} }

// Next returns the next fact declaration in canonical barrier order.
func (c *Cursor) Next() (Operation, bool) {
	if c.next >= len(c.row) {
		return Operation{}, false
	}
	op := c.row[c.next]
	c.next++
	return op, true
}

// Row returns a defensive copy of the complete row for point.
func (p Plan) Row(point cfg.Point) []Operation {
	return append([]Operation(nil), p.rows[point]...)
}

// ExecuteNode runs the shared production concrete kernel over the immutable
// snapshot used to compile the row.
func (p Plan) ExecuteNode(ctx transfer.NodeContext, in state.State) state.State {
	if p.nodeTransfer == nil {
		return in
	}
	return p.nodeTransfer(ctx, in)
}

// ExecuteEdge runs the shared production concrete edge kernel.
func (p Plan) ExecuteEdge(ctx transfer.EdgeContext, out state.State) state.State {
	if p.edgeTransfer == nil {
		return out
	}
	return p.edgeTransfer(ctx, out)
}

func compileRows(input factflow.FactsInput) map[cfg.Point][]Operation {
	points := collectPoints(input)
	rows := make(map[cfg.Point][]Operation, len(points))
	for _, point := range points {
		var row []Operation
		add := func(kind Kind, count int) {
			spec := specForKind(kind)
			for i := 0; i < count; i++ {
				stages := spec.Stages
				if stages == 0 {
					stages = barriers(spec.Barrier)
				}
				row = append(row, Operation{
					Phase: spec.Phase, Barrier: spec.Barrier, Stages: stages,
					Kind: kind, Role: spec.Role, Owner: spec.Owner, Ordinal: i,
				})
			}
		}
		// Deliberately collect in FactsInput vocabulary order. sortOperations is
		// the only authority for execution order, so declaration-order edits
		// cannot silently change the future executor contract.
		add(OpRootAssignment, boolCount(has(input.RootAssignments, point)))
		add(OpPathAssignment, boolCount(has(input.PathAssignments, point)))
		add(OpPathStaticMemberWrite, boolCount(has(input.PathStaticMemberWrites, point)))
		add(OpDynamicIndexWrite, boolCount(has(input.DynamicIndexWrites, point)))
		add(OpPathDescendantInvalidation, boolCount(has(input.PathDescendantInvalidations, point)))
		add(OpCovariantExposure, len(input.CovariantExposures[point]))
		add(OpNoNormalReturn, boolCount(has(input.NoNormalReturns, point)))
		add(OpBranchReachability, boolCount(has(input.BranchEdgeReachability, point)))
		add(OpBranchConditionSource, boolCount(has(input.BranchConditionSources, point)))
		set := input.BranchRefinements[point]
		add(OpBranchRefinement, len(set.Refinements()))
		add(OpBranchLengthRefinement, len(set.LenRefinements()))
		add(OpBranchNumberFloor, len(set.NumFloorRefinements()))
		add(OpBranchNumberCeil, len(set.NumCeilRefinements()))
		add(OpBranchDifferenceConstraint, len(set.DiffConstraints()))
		add(OpBranchPresenceRelation, len(input.BranchPresenceRelations[point].Relations()))
		add(OpBranchPathRelation, len(input.BranchPathRelations[point].Relations()))
		add(OpBranchPathEvidence, len(input.BranchPathEvidence[point].Evidence()))
		add(OpBranchSufficientLiteralCase, len(input.BranchSufficientLiteralCases[point].Cases()))
		add(OpPathPresenceImplication, len(input.PathValuePresenceImplications[point].Implications()))
		add(OpChannelSelect, len(input.ChannelSelects[point].Events()))
		add(OpPostconditionRefinement, len(input.PostconditionRefinements[point].Refinements()))
		add(OpPostconditionPathRelation, len(input.PostconditionPathRelations[point]))
		add(OpCallResultValue, len(input.CallResultValues[point].Values()))
		add(OpReturnPresenceRelation, len(input.ReturnPresenceRelations[point].Relations()))
		add(OpReturn, boolCount(has(input.Returns, point)))
		add(OpCallSite, boolCount(has(input.CallSites, point)))
		sortOperations(row)
		if len(row) != 0 {
			rows[point] = row
		}
	}
	return rows
}

func specForKind(kind Kind) operationSpec {
	for _, spec := range operationSpecs {
		if spec.Kind == kind {
			return spec
		}
	}
	panic("wholepointplan: unclassified operation kind " + kind)
}

func barrierRank(barrier Barrier) int {
	for rank, candidate := range canonicalBarriers {
		if candidate == barrier {
			return rank
		}
	}
	return len(canonicalBarriers)
}

func kindRank(kind Kind) int {
	for rank, spec := range operationSpecs {
		if spec.Kind == kind {
			return rank
		}
	}
	return len(operationSpecs)
}

func sortOperations(row []Operation) {
	sort.SliceStable(row, func(i, j int) bool {
		left, right := row[i], row[j]
		if rankI, rankJ := barrierRank(left.Barrier), barrierRank(right.Barrier); rankI != rankJ {
			return rankI < rankJ
		}
		if rankI, rankJ := kindRank(left.Kind), kindRank(right.Kind); rankI != rankJ {
			return rankI < rankJ
		}
		return left.Ordinal < right.Ordinal
	})
}

func collectPoints(input factflow.FactsInput) []cfg.Point {
	set := make(map[cfg.Point]struct{})
	collectMapKeys(set, input.RootAssignments)
	collectMapKeys(set, input.PathAssignments)
	collectMapKeys(set, input.PathStaticMemberWrites)
	collectMapKeys(set, input.DynamicIndexWrites)
	collectMapKeys(set, input.PathDescendantInvalidations)
	collectMapKeys(set, input.CovariantExposures)
	collectMapKeys(set, input.NoNormalReturns)
	collectMapKeys(set, input.BranchEdgeReachability)
	collectMapKeys(set, input.BranchConditionSources)
	collectMapKeys(set, input.BranchRefinements)
	collectMapKeys(set, input.BranchPresenceRelations)
	collectMapKeys(set, input.BranchPathRelations)
	collectMapKeys(set, input.BranchPathEvidence)
	collectMapKeys(set, input.BranchSufficientLiteralCases)
	collectMapKeys(set, input.PathValuePresenceImplications)
	collectMapKeys(set, input.ChannelSelects)
	collectMapKeys(set, input.PostconditionRefinements)
	collectMapKeys(set, input.PostconditionPathRelations)
	collectMapKeys(set, input.CallResultValues)
	collectMapKeys(set, input.ReturnPresenceRelations)
	collectMapKeys(set, input.Returns)
	collectMapKeys(set, input.CallSites)
	out := make([]cfg.Point, 0, len(set))
	for point := range set {
		out = append(out, point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func collectMapKeys[V any](set map[cfg.Point]struct{}, values map[cfg.Point]V) {
	for point := range values {
		set[point] = struct{}{}
	}
}

func has[K comparable, V any](m map[K]V, key K) bool { _, ok := m[key]; return ok }
func boolCount(ok bool) int {
	if ok {
		return 1
	}
	return 0
}
