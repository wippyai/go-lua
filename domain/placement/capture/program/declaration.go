// Package program owns the callback-free cold declaration for Placement's
// closure-capture rule. It remains a family-local pre-stage until the
// dependent-relation runtime seam can bind its route relation atomically.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	valueAxisKey     schema.Key = "value"
	placementAxisKey schema.Key = "placement"
	RuleKey                     = "placement-closure-capture"
	RuleRole                    = "rule/placement/closure-capture"
	OperandRole                 = "operand/placement/closure-capture"

	captureParents     schema.Key = "placement/closure-capture/parents"
	captureParentKey   schema.Key = "placement/closure-capture/parent-key"
	captureSources     schema.Key = "value/closure-capture/sources"
	captureSourceKey   schema.Key = "value/closure-capture/source-key"
	captureSourceTag   schema.Key = "value/closure-capture/source-tag"
	captureRoutes      schema.Key = "placement/closure-capture/routes"
	captureRouteKey    schema.Key = "placement/closure-capture/route-key"
	captureRouteTag    schema.Key = "placement/closure-capture/route-tag"
	captureDestination schema.Key = "placement/closure-capture/route-destination"
	captureReducer     schema.Key = "placement/closure-capture/reducer"
	captureOutput      schema.Key = "placement/facts"
)

func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/allocation",
		Requirement: "program-requirement/closure-capture",
		Form:        "program-form/local-successor",
	}}
}

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: placementAxisKey, Owner: placementAxisKey,
		Issues: RuleIssues(), Lane: rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  ClosureCapture(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func relation(axisKey, memberKey schema.Key) member.RelationRef {
	return member.RelationRef{Axis: axisReference(axisKey), Member: memberKey}
}

func projection(axisKey, memberKey schema.Key) member.ProjectionRef {
	return member.ProjectionRef{Axis: axisReference(axisKey), Member: memberKey}
}

func denominator(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// ClosureCapture declares the exact Placement parent, the selected Value
// capture-source vector, and the selected Placement route relation. The
// parent and routed predecessor are the reducer's semantic inputs; the Value
// selection is a dependency of the single route materialization.
func ClosureCapture() ruleprogram.Program {
	valueAxis := axisReference(valueAxisKey)
	placementAxis := axisReference(placementAxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		// Issuance already authenticates the unique closure-proof row reached
		// from this allocation occurrence. Reusing it keeps Program as the one
		// candidate authority; Heap must not publish a role-specific mirror.
		Candidate: member.IssuedRowCandidate(programissuance.RelationOccurrenceClosureProof),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: relation(placementAxisKey, captureParents),
				Key:      projection(placementAxisKey, captureParentKey),
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(placementAxis), Form: ruleprogram.Exact,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
						OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
					},
				},
			},
			{
				Sources:   []ruleprogram.SourceRef{ruleprogram.CandidateSource(), ruleprogram.PriorSource(0)},
				Relation:  relation(valueAxisKey, captureSources),
				Key:       projection(valueAxisKey, captureSourceKey),
				Predicate: projection(valueAxisKey, captureSourceTag),
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(valueAxis), Form: ruleprogram.Selected,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
						OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
						DenominatorRef: denominator("coordinates/value"),
					},
				},
			},
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource(), ruleprogram.PriorSource(0), ruleprogram.PriorSource(1)},
				Relation: relation(placementAxisKey, captureRoutes),
				Key:      projection(placementAxisKey, captureRouteKey),
				Read: ruleprogram.ReadDecl{
					Input: 0, Axis: ruleprogram.AxisRef(placementAxis), Form: ruleprogram.Selected,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order: ruleprogram.OrderCanonical, Sparse: ruleprogram.SparseExplicit,
						OnOpaque: ruleprogram.OnOpaqueRefuse, Multiplicity: ruleprogram.MultiplicityOne,
						DenominatorRef: denominator("coordinates/placement"),
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: placementAxis, Member: captureReducer},
			Inputs:  []ruleprogram.JoinRef{0, 2},
			Outputs: []ruleprogram.OutputDecl{{
				Column:      axis.OutputRef{Axis: placementAxis, Key: captureOutput},
				Destination: projection(placementAxisKey, captureDestination),
				Mode:        ruleprogram.ModeRoute, ValueSlot: 0, RouteJoin: 2, RouteJoinPresent: true,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}
