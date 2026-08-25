// Package program owns Suspension's callback-free mounted-rule declarations.
// Program issuance owns the subject-liveness candidate; Value, Call and the
// output axis own the relations the sealed rule joins around it. The member
// keys below remain local pre-stage spellings until those owner relations and
// vector delivery can replace the legacy Link catalog atomically.
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
	valueAxisKey        schema.Key = "value"
	placementAxisKey    schema.Key = "placement"
	RuleKey                        = "placement-suspension"
	EvidenceRuleKey                = "placement-suspension-evidence"
	RuleRole                       = "rule/placement/suspension"
	OperandRole                    = "operand/placement/suspension"
	EvidenceRuleRole               = "rule/placement/suspension-evidence"
	EvidenceOperandRole            = "operand/placement/suspension-evidence"

	// Value's exact anchor and selected source vector.
	suspensionAnchors   schema.Key = "value/suspension/anchors"
	suspensionAnchorKey schema.Key = "value/suspension/anchor-key"
	suspensionSources   schema.Key = "value/suspension/sources"
	suspensionSourceKey schema.Key = "value/suspension/source-key"
	suspensionSourceTag schema.Key = "value/suspension/source-tag"

	// Placement's paired RouteMember vector and reducer.
	suspensionRoutes           schema.Key = "placement/suspension/routes"
	suspensionRouteKey         schema.Key = "placement/suspension/route-key"
	suspensionRouteTag         schema.Key = "placement/suspension/route-tag"
	suspensionRouteDestination schema.Key = "placement/suspension/route-destination"
	suspensionReducer          schema.Key = "placement/suspension/reducer"

	placementFactsColumn       schema.Key = "placement/facts"
	valueCoordinateDenominator schema.Key = "coordinates/value"
	placementDenominator       schema.Key = "coordinates/placement"
)

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: placementAxisKey, Owner: placementAxisKey,
		Issues: RuleIssues(), Lane: rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Suspension(),
	}
}

func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/subject-liveness",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormCallSummary,
	}}
}

func EvidenceRuleEntry() rule.Spec {
	return rule.Spec{
		Key: EvidenceRuleKey, Writes: "placement-suspension-evidence", Owner: "placement-suspension-evidence",
		Issues: EvidenceRuleIssues(), Lane: rule.LaneMounted,
		Semantic: vocabulary.RoleKey(EvidenceRuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(EvidenceOperandRole)},
		Program:  SuspensionEvidence(),
	}
}

func EvidenceRuleIssues() []rule.Issuance { return RuleIssues() }

// StructureSpecs owns only the two rule/operand identities. The evidence
// Factor vocabulary remains at its axis owner and is not mirrored here.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole, EvidenceRuleRole, EvidenceOperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

func valueAxis() schema.EntryReference     { return axisReference(valueAxisKey) }
func placementAxis() schema.EntryReference { return axisReference(placementAxisKey) }

func exactValueRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		Input:      0,
		Axis:       ruleprogram.AxisRef(valueAxis()),
		Form:       ruleprogram.Exact,
		PointBound: ruleprogram.PointBound,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
			Multiplicity:   ruleprogram.MultiplicityOne,
			DenominatorRef: denominatorReference(valueCoordinateDenominator),
		},
	}
}

func selectedValueRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		Input:      0,
		Axis:       ruleprogram.AxisRef(valueAxis()),
		Form:       ruleprogram.Selected,
		PointBound: ruleprogram.PointBound,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
			Multiplicity:   ruleprogram.MultiplicityOne,
			DenominatorRef: denominatorReference(valueCoordinateDenominator),
		},
	}
}

func selectedPlacementRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		Input:      0,
		Axis:       ruleprogram.AxisRef(placementAxis()),
		Form:       ruleprogram.Selected,
		PointBound: ruleprogram.PointBound,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaqueRefuse,
			Multiplicity:   ruleprogram.MultiplicityOne,
			DenominatorRef: denominatorReference(placementDenominator),
		},
	}
}

func declaration(outputAxisKey schema.Key, candidate member.CandidateRef, anchor, anchorKey, sources, sourceKey, sourceTag, routes, routeKey, routeTag, routeDestination, reducer, outputColumn schema.Key) ruleprogram.Program {
	value := valueAxis()
	outputAxis := axisReference(outputAxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate:   candidate,
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: value, Member: anchor},
				Key:      member.ProjectionRef{Axis: value, Member: anchorKey},
				Read:     exactValueRead(),
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
				},
				Relation:  member.RelationRef{Axis: value, Member: sources},
				Key:       member.ProjectionRef{Axis: value, Member: sourceKey},
				Predicate: member.ProjectionRef{Axis: value, Member: sourceTag},
				Read:      selectedValueRead(),
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation: member.RelationRef{Axis: outputAxis, Member: routes},
				Key:      member.ProjectionRef{Axis: outputAxis, Member: routeKey},
				Read:     selectedOutputRead(outputAxis),
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: outputAxis, Member: reducer},
			Inputs:  []ruleprogram.JoinRef{0, 1, 2},
			Outputs: []ruleprogram.OutputDecl{{
				Column:           axis.OutputRef{Axis: outputAxis, Key: outputColumn},
				Destination:      member.ProjectionRef{Axis: outputAxis, Member: routeDestination},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
				RouteJoinPresent: true,
			}},
		},
		Carry: &ruleprogram.CarryDecl{Input: 0, Mode: ruleprogram.CarryIdentity},
	}
}

func selectedOutputRead(outputAxis schema.EntryReference) ruleprogram.ReadDecl {
	read := selectedPlacementRead()
	read.Axis = ruleprogram.AxisRef(outputAxis)
	return read
}

// Suspension returns the mounted consumer's three-read pre-stage declaration.
// Its candidate is Program's authenticated subject-liveness row; the remaining
// reads are retained only until the Call-authenticated vector shape can replace
// this draft in the same cut that deletes the legacy Link implementation.
func Suspension() ruleprogram.Program {
	return declaration(
		placementAxisKey,
		member.IssuedRowCandidate(programissuance.RelationOccurrenceSubjectLiveness),
		suspensionAnchors, suspensionAnchorKey,
		suspensionSources, suspensionSourceKey, suspensionSourceTag,
		suspensionRoutes, suspensionRouteKey, suspensionRouteTag, suspensionRouteDestination,
		suspensionReducer, placementFactsColumn,
	)
}

// SuspensionEvidence returns the independent evidence producer's same
// Program/Value bridge and route selection.  It deliberately names a separate
// candidate/relation/reducer vocabulary and output column so the evidence
// producer cannot consume Placement class or masquerade as the suspension
// consumer's receipt.
func SuspensionEvidence() ruleprogram.Program {
	const (
		evidenceAnchors          schema.Key = "value/suspension-evidence/anchors"
		evidenceAnchorKey        schema.Key = "value/suspension-evidence/anchor-key"
		evidenceSources          schema.Key = "value/suspension-evidence/sources"
		evidenceSourceKey        schema.Key = "value/suspension-evidence/source-key"
		evidenceSourceTag        schema.Key = "value/suspension-evidence/source-tag"
		evidenceRoutes           schema.Key = "placement/suspension-evidence/routes"
		evidenceRouteKey         schema.Key = "placement/suspension-evidence/route-key"
		evidenceRouteTag         schema.Key = "placement/suspension-evidence/route-tag"
		evidenceRouteDestination schema.Key = "placement/suspension-evidence/route-destination"
		evidenceAxisKey          schema.Key = "placement-suspension-evidence"
		evidenceReducer          schema.Key = "placement-suspension-evidence/reducer"
		evidenceFactsColumn      schema.Key = "placement/suspension-evidence/facts"
	)
	declaration := declaration(
		evidenceAxisKey,
		member.IssuedRowCandidate(programissuance.RelationOccurrenceSubjectLiveness),
		evidenceAnchors, evidenceAnchorKey,
		evidenceSources, evidenceSourceKey, evidenceSourceTag,
		evidenceRoutes, evidenceRouteKey, evidenceRouteTag, evidenceRouteDestination,
		evidenceReducer, evidenceFactsColumn,
	)
	declaration.OperandRole = vocabulary.RoleKey(EvidenceOperandRole)
	return declaration
}
