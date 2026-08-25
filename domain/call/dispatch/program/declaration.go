// Package program owns Call dispatch's callback-free rule declaration.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	AxisKey     schema.Key = "call"
	OutputKey   schema.Key = "call/facts"
	RuleKey     schema.Key = "call-dispatch"
	RuleRole               = "rule/call/dispatch"
	OperandRole            = "operand/call/dispatch"

	MountedCallCandidates schema.Key = calldomain.MountedCallCandidates
	MountedCallCoordinate schema.Key = calldomain.MountedCallCoordinate
	MountedCallParents    schema.Key = valuedomain.MountedCallParents
	MountedCallCalleeKey  schema.Key = valuedomain.MountedCallCalleeKey
	DispatchReducer       schema.Key = calldomain.DispatchReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call", Requirement: "program-requirement/unrestricted", Form: "program-form/call-dispatch"}},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Dispatch(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// Dispatch declares one join: Value's own image of the callee this mounted
// application applies, read exactly at the coordinate Value's parent relation
// addresses it by. The publication is exact at the candidate's own Call
// coordinate, which is where a dispatch fact belongs - the rule refines the
// cell of the application it is indexed by.
//
// The callee's alternatives are not declared here. A dispatch fact is the join
// of every alternative the callee reaches, and that join is the fold's own
// judgment over the authorities its state seals; declaring it as a selection
// over the Call axis would make this rule read the axis it publishes into, and
// a rule whose authored region is derived from its own output is not a
// monotone operator - the fixpoint refuses its ascent rather than converging.
func Dispatch() ruleprogram.Program {
	callAxis := axisReference(AxisKey)
	valueAxis := axisReference("value")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   callAxis,
			Member: MountedCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{{
			Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
			Relation: member.RelationRef{Axis: valueAxis, Member: MountedCallParents},
			Key:      member.ProjectionRef{Axis: valueAxis, Member: MountedCallCalleeKey},
			Read: ruleprogram.ReadDecl{
				Input:      0,
				Axis:       ruleprogram.AxisRef(valueAxis),
				Form:       ruleprogram.Exact,
				PointBound: ruleprogram.PointBound,
				Contract: ruleprogram.ReadContract{
					Order:        ruleprogram.OrderCanonical,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaqueRefuse,
					Multiplicity: ruleprogram.MultiplicityOne,
				},
			},
		}},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: callAxis, Member: DispatchReducer},
			Inputs:  []ruleprogram.JoinRef{0},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: callAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: callAxis, Member: MountedCallCoordinate},
				Mode:        ruleprogram.ModeExact,
				ValueSlot:   0,
			}},
		},
	}
}
