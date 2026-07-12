// Package semanticplan is an isolated Stage-1 proof for compiling existing
// factflow DTOs into immutable semantic operations. Production transfers do not
// import or execute this package.
package semanticplan

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

type OperationKind string

const OpAssignPath OperationKind = "assign-path"

type AccessMode uint8

const (
	Read AccessMode = 1 << iota
	Write
)

type LaneAccess struct {
	Lane state.LaneID
	Mode AccessMode
}

type OwnershipRequirement string

const (
	OwnershipLexicalPaths OwnershipRequirement = "lexical-paths"
	OwnershipKeySpace     OwnershipRequirement = "resolver-keyspace"
	OwnershipHeapIdentity OwnershipRequirement = "heap-identity"
)

type RebaseRequirement string

const (
	RebaseSourceRoot RebaseRequirement = "source-root"
	RebaseTargetRoot RebaseRequirement = "target-root"
)

// PathAssignmentOp is syntax-free and owns cloned structural paths. It never
// stores resolver-versioned path keys or keyspace intern IDs.
type PathAssignmentOp struct {
	Point  cfg.Point
	Target pathdom.Path
	Source factflow.ValueSource
	// SourcePath is present when Source is a structural access path and the
	// symbolic interpreter can represent the assignment.
	SourcePath    pathdom.Path
	HasSourcePath bool
	// Same-point companions are explicit plan metadata. The concrete oracle
	// preserves their production ordering; Stage-1 symbolic lifting rejects
	// them atomically until their operation handlers exist.
	HasStaticMemberWrite bool
	CovariantExposures   int
}

type Plan struct {
	facts      factflow.Facts
	operations map[cfg.Point]PathAssignmentOp
}

var pathAssignmentAccesses = []LaneAccess{
	{Lane: state.LaneValues, Mode: Read | Write},
	{Lane: state.LanePathEvidence, Mode: Read | Write},
	{Lane: state.LaneHeapTableIdentity, Mode: Read | Write},
	{Lane: state.LaneDynamicIndex, Mode: Write},
	{Lane: state.LaneKeyMemberships, Mode: Write},
	{Lane: state.LaneLenFloors, Mode: Write},
	{Lane: state.LaneUserLattices, Mode: Read | Write},
}

func (PathAssignmentOp) Kind() OperationKind { return OpAssignPath }

func (PathAssignmentOp) Accesses() []LaneAccess {
	return append([]LaneAccess(nil), pathAssignmentAccesses...)
}

func (PathAssignmentOp) Ownership() []OwnershipRequirement {
	return []OwnershipRequirement{OwnershipLexicalPaths, OwnershipKeySpace, OwnershipHeapIdentity}
}

func (PathAssignmentOp) Rebasing() []RebaseRequirement {
	return []RebaseRequirement{RebaseSourceRoot, RebaseTargetRoot}
}

// CompilePathAssignments builds a plan only for standalone PathAssignment
// points. Companion point facts change production semantics and fail closed.
func CompilePathAssignments(input factflow.FactsInput) (Plan, error) {
	points := make([]cfg.Point, 0, len(input.PathAssignments))
	for point := range input.PathAssignments {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	operations := make(map[cfg.Point]PathAssignmentOp, len(points))
	compiled := factflow.FactsInput{
		PathAssignments:        make(map[cfg.Point]factflow.PathAssignment, len(points)),
		PathStaticMemberWrites: make(map[cfg.Point]factflow.PathStaticMemberWrite),
		CovariantExposures:     make(map[cfg.Point][]factflow.CovariantExposure),
		ExpressionPaths:        make(map[factflow.ExprRef]pathdom.Path),
	}
	for _, point := range points {
		if reason := unsupportedCompanionAt(input, point); reason != "" {
			return Plan{}, fmt.Errorf("semanticplan: point %d requires contextual transfer: %s", point, reason)
		}
		fact := input.PathAssignments[point]
		compiled.PathAssignments[point] = fact
		op := PathAssignmentOp{Point: point, Target: fact.TargetPath(), Source: fact.Source()}
		if companion, ok := input.PathStaticMemberWrites[point]; ok {
			op.HasStaticMemberWrite = true
			compiled.PathStaticMemberWrites[point] = companion
		}
		op.CovariantExposures = len(input.CovariantExposures[point])
		if op.CovariantExposures != 0 {
			compiled.CovariantExposures[point] = append([]factflow.CovariantExposure(nil), input.CovariantExposures[point]...)
		}
		if op.Source.Kind == factflow.ValueSourceExpression && op.Source.HasExpr {
			if sourcePath, ok := input.ExpressionPaths[op.Source.ExprRef]; ok {
				op.SourcePath = sourcePath.Clone()
				op.HasSourcePath = true
				compiled.ExpressionPaths[op.Source.ExprRef] = sourcePath.Clone()
			}
		}
		operations[point] = op
	}
	return Plan{facts: factflow.NewFacts(compiled), operations: operations}, nil
}

func unsupportedCompanionAt(input factflow.FactsInput, point cfg.Point) string {
	checks := []struct {
		name string
		has  bool
	}{
		{"root-assignment", has(input.RootAssignments, point)},
		{"dynamic-index-write", has(input.DynamicIndexWrites, point)},
		{"descendant-invalidation", has(input.PathDescendantInvalidations, point)},
		{"no-normal-return", has(input.NoNormalReturns, point)},
		{"path-presence-implication", has(input.PathValuePresenceImplications, point)},
		{"channel-select", has(input.ChannelSelects, point)},
		{"postcondition-refinement", has(input.PostconditionRefinements, point)},
		{"postcondition-path-relation", len(input.PostconditionPathRelations[point]) != 0},
		{"call-result", has(input.CallResultValues, point)},
		{"return-presence", has(input.ReturnPresenceRelations, point)},
		{"return", has(input.Returns, point)},
		{"call-site", has(input.CallSites, point)},
	}
	for _, check := range checks {
		if check.has {
			return check.name
		}
	}
	if fact, ok := input.PathAssignments[point]; ok && fact.Source().HasExpr {
		if _, object := input.ObjectLiterals[fact.Source().ExprRef]; object {
			return "object-literal"
		}
	}
	return ""
}

func has[K comparable, V any](values map[K]V, key K) bool {
	_, ok := values[key]
	return ok
}

func (p Plan) Operation(point cfg.Point) (PathAssignmentOp, bool) {
	op, ok := p.operations[point]
	if !ok {
		return PathAssignmentOp{}, false
	}
	op.Target = op.Target.Clone()
	op.SourcePath = op.SourcePath.Clone()
	return op, true
}

// BindConcrete delegates the operation semantics to the exported production
// fact applicator. This is oracle plumbing, not evidence that production has
// already been factored into a cheap operation interpreter.
func (p Plan) BindConcrete(config factapply.FactsNodeTransferConfig) transfer.NodeTransfer {
	config.Facts = p.facts
	delegate := factapply.NewFactsNodeTransfer(config)
	return func(ctx transfer.NodeContext, in state.State) state.State {
		if _, ok := p.operations[ctx.Point]; !ok {
			return in
		}
		return delegate(ctx, in)
	}
}
