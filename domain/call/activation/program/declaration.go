// Package program owns Call activation's callback-free rule declaration.
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
)

const (
	AxisKey        schema.Key = "call"
	OutputKey      schema.Key = "call/facts"
	RuleKey        schema.Key = "call-activation"
	RuleRole                  = "rule/call/activation"
	OperandRole               = "operand/call/activation"
	ActivationRole            = "activation-family/call/body"

	MountedCallCandidates schema.Key = calldomain.MountedCallCandidates
	MountedCallFacts      schema.Key = calldomain.MountedCallFacts
	MountedCallFactKey    schema.Key = calldomain.MountedCallFactKey

	ActivationBranches    schema.Key = calldomain.CallActivationBranches
	ActivationApplication schema.Key = calldomain.CallActivationApplication
	ActivationTarget      schema.Key = calldomain.CallActivationTarget
	ActivationEndpoint    schema.Key = calldomain.CallActivationEndpoint
	ActivationMount       schema.Key = calldomain.CallActivationMount
	ActivationBody        schema.Key = calldomain.CallActivationBody
	ActivationReducer     schema.Key = calldomain.CallActivationReducer
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func projection(key schema.Key) member.ProjectionRef {
	return member.ProjectionRef{Axis: axisReference(AxisKey), Member: key}
}

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   []rule.Issuance{{Occurrence: "occurrence/call-activation", Requirement: "program-requirement/unrestricted", Form: "program-form/call-summary"}},
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole), vocabulary.RoleKey(ActivationRole)},
		Program:  Activation(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole, ActivationRole)
}

// Activation declares the A form: one exact read of the Call fact at the
// trigger's own coordinate, and a structural publication of the branch set
// that trigger carries.
//
// There is no branch READ. The branches are a nested member set of the
// mounted-call directory, named by the vocabulary below and enumerated through
// that relation's own owner at issuance. A branch carries no Call fact any
// judgment consumes - the trigger's value and the branch's identity settle it
// - and a branch is a body rather than a call site, so it has no coordinate to
// be read at. Reading them would cost one Factor read per body in the PROGRAM
// on every invocation, where the judgment itself is over the handful of
// targets a value actually names.
//
// The transport vector is what one branch instantiates when it crosses its
// transition: the value, heap, pack, effect and placement lanes are carried
// into the mounted body and carried back out to the trigger, and the call lane
// is carried in only - a body publishes no call value across the edge.
func Activation() ruleprogram.Program {
	callAxis := axisReference(AxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   callAxis,
			Member: MountedCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{{
			Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
			Relation: member.RelationRef{Axis: callAxis, Member: MountedCallFacts},
			Key:      member.ProjectionRef{Axis: callAxis, Member: MountedCallFactKey},
			Read: ruleprogram.ReadDecl{
				Input:      0,
				Axis:       ruleprogram.AxisRef(callAxis),
				Form:       ruleprogram.Exact,
				PointBound: ruleprogram.PointBound,
				Contract: ruleprogram.ReadContract{
					Order:        ruleprogram.OrderCanonical,
					Sparse:       ruleprogram.SparseExplicit,
					OnOpaque:     ruleprogram.OnOpaquePropagateAuthenticated,
					Multiplicity: ruleprogram.MultiplicityOne,
				},
			},
		}},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: callAxis, Member: ActivationReducer},
			Inputs:  []ruleprogram.JoinRef{0},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: callAxis, Key: OutputKey},
				Destination: member.ProjectionRef{Axis: callAxis, Member: calldomain.MountedCallCoordinate},
				Mode:        ruleprogram.ModeStructural,
				ValueSlot:   0,
			}},
		},
		Transport: []ruleprogram.TransportDecl{
			{Axis: ruleprogram.AxisRef(axisReference("value")), Exported: true},
			{Axis: ruleprogram.AxisRef(axisReference("call"))},
			{Axis: ruleprogram.AxisRef(axisReference("heap")), Exported: true},
			{Axis: ruleprogram.AxisRef(axisReference("pack")), Exported: true},
			{Axis: ruleprogram.AxisRef(axisReference("effect")), Exported: true},
			{Axis: ruleprogram.AxisRef(axisReference("placement")), Exported: true},
		},
		ActivationRole: vocabulary.RoleKey(ActivationRole),
		Activation: &ruleprogram.ActivationDecl{
			Branch:      member.RelationRef{Axis: callAxis, Member: ActivationBranches},
			Application: projection(ActivationApplication),
			Target:      projection(ActivationTarget),
			Endpoint:    projection(ActivationEndpoint),
			Mount:       projection(ActivationMount),
			Body:        projection(ActivationBody),
			// The vector crosses as the branch set itself: it is instantiated
			// once per candidate branch, so the rows that cross the edge are
			// those branches and not a relation invented per transported axis.
			Transport: member.RelationRef{Axis: callAxis, Member: ActivationBranches},
		},
	}
}
