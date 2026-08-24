package empty

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
)

// ruleAuthorities is the sealed authority set this rule installs its family
// against. It names only a peer type this package already speaks, so the
// composition that supplies the authority record satisfies it structurally and
// neither side learns the other's shape.
type ruleAuthorities interface {
	HeapAuthority() *heapowner.HotOwner
}

// RuleEntry is this package's heap-empty rule declaration: the predecessor
// world it folds over, the constructor directory it draws candidates from, and
// the allocation transition its carry applies. The composition derives and
// binds the generated engine slot from this Program; the fold itself is the
// family this package installs at its own bind, because the engine has no
// generic builder for a transformed carry.
func RuleEntry() rule.Spec {
	heapAxis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
	return rule.Spec{
		Key:    "heap-empty",
		Writes: "heap",
		Owner:  "heap",
		Issues: []rule.Issuance{
			{Occurrence: "occurrence/allocation-empty", Requirement: "program-requirement/unrestricted", Form: "program-form/local-finish"},
		},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/heap/allocation-empty",
		Roles:    []schema.Key{"semantic/operand/heap/allocation-empty"},
		Program: program.Program{
			OperandRole: "semantic/operand/heap/allocation-empty",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: heapAxis, Member: heapdomain.EmptyAllocations}),
			Joins: []program.JoinDecl{{
				Sources:  []program.SourceRef{program.CandidateSource()},
				Relation: member.RelationRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationPredecessors},
				Key:      member.ProjectionRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationPredecessorKey},
				Read: program.ReadDecl{
					PointBound: program.PointBound,
					Input:      0,
					Axis:       program.AxisRef(heapAxis),
					Form:       program.Exact,
					Contract: program.ReadContract{
						Order: program.OrderCanonical, Sparse: program.SparseExplicit,
						OnOpaque: program.OnOpaqueRefuse, Multiplicity: program.MultiplicityOne,
					},
				},
			}},
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationReducer},
				Inputs:  []program.JoinRef{0},
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: heapAxis, Key: "heap/facts"},
					Destination: member.ProjectionRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationCoordinate},
					Mode:        program.ModeExact,
					ValueSlot:   0,
				}},
			},
			Carry: &program.CarryDecl{
				Input:     0,
				Mode:      program.CarryTransform,
				Transform: member.CarryTransformRef{Axis: heapAxis, Member: heapdomain.EmptyAllocationCarryTransform},
			},
		},
	}
}

// InstallFamily is the generated lane's bind arm for this rule. It resolves
// the one axis schema the emitted installer is sealed against, and claims the
// rule's sealed ordinal against the Factor it writes.
//
// This is the whole of what a family cutover still authors: how an authority
// record is reached is the composition's knowledge and not the rule's, so it
// cannot be a function of the declaration the family is emitted from.
func InstallFamily[A ruleAuthorities](binding *engine.SchemaBinding, slot *engine.GeneratedRuleSlot, authorities A) bool {
	owner := authorities.HeapAuthority()
	if owner == nil || !owner.Schema().Valid() {
		return false
	}
	installer, installerOK := NewFamilyInstaller(owner.Schema())
	if !installerOK {
		return false
	}
	return engine.BindRuleFamily[heapdomain.DenseCoordinate](binding, slot, owner.FactorRef(), installer)
}

// StructureSpecs is this package's contribution to the analyzer's semantic
// role vocabulary: the two roles its rule is identified by. A role is declared
// where it is used, so the row and the reference that names it are one
// package's statement.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs("rule/heap/allocation-empty", "operand/heap/allocation-empty")
}
