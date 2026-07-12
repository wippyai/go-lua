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
	Kind    Kind
	Ordinal int
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
		add := func(phase Phase, kind Kind, count int) {
			for i := 0; i < count; i++ {
				row = append(row, Operation{Phase: phase, Kind: kind, Ordinal: i})
			}
		}
		add(Node, OpCallSite, boolCount(has(input.CallSites, point)))
		add(Node, OpNoNormalReturn, boolCount(has(input.NoNormalReturns, point)))
		add(Node, OpPathPresenceImplication, len(input.PathValuePresenceImplications[point].Implications()))
		add(Node, OpPathDescendantInvalidation, boolCount(has(input.PathDescendantInvalidations, point)))
		add(Node, OpPostconditionRefinement, len(input.PostconditionRefinements[point].Refinements()))
		add(Node, OpPostconditionPathRelation, len(input.PostconditionPathRelations[point]))
		add(Node, OpChannelSelect, len(input.ChannelSelects[point].Events()))
		add(Node, OpDynamicIndexWrite, boolCount(has(input.DynamicIndexWrites, point)))
		add(Node, OpRootAssignment, boolCount(has(input.RootAssignments, point)))
		add(Node, OpPathAssignment, boolCount(has(input.PathAssignments, point)))
		add(Node, OpPathStaticMemberWrite, boolCount(has(input.PathStaticMemberWrites, point)))
		add(Node, OpReturn, boolCount(has(input.Returns, point)))
		add(Node, OpCovariantExposure, len(input.CovariantExposures[point]))
		// These are call/return materialization sidecars. Their relative order is
		// explicit even though the concrete kernel consumes them lazily.
		add(Node, OpCallResultValue, len(input.CallResultValues[point].Values()))
		add(Node, OpReturnPresenceRelation, len(input.ReturnPresenceRelations[point].Relations()))

		add(Edge, OpBranchReachability, boolCount(has(input.BranchEdgeReachability, point)))
		add(Edge, OpBranchConditionSource, boolCount(has(input.BranchConditionSources, point)))
		set := input.BranchRefinements[point]
		add(Edge, OpBranchRefinement, len(set.Refinements()))
		add(Edge, OpBranchLengthRefinement, len(set.LenRefinements()))
		add(Edge, OpBranchNumberFloor, len(set.NumFloorRefinements()))
		add(Edge, OpBranchNumberCeil, len(set.NumCeilRefinements()))
		add(Edge, OpBranchDifferenceConstraint, len(set.DiffConstraints()))
		add(Edge, OpBranchPresenceRelation, len(input.BranchPresenceRelations[point].Relations()))
		add(Edge, OpBranchPathRelation, len(input.BranchPathRelations[point].Relations()))
		add(Edge, OpBranchPathEvidence, len(input.BranchPathEvidence[point].Evidence()))
		add(Edge, OpBranchSufficientLiteralCase, len(input.BranchSufficientLiteralCases[point].Cases()))
		if len(row) != 0 {
			rows[point] = row
		}
	}
	return rows
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
