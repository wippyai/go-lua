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
	suspensionAnchors         schema.Key = "value/suspension/anchors"
	suspensionAnchorKey       schema.Key = "value/suspension/anchor-key"
	suspensionSources         schema.Key = "value/suspension/sources"
	suspensionSourceKey       schema.Key = "value/suspension/source-key"
	suspensionSourceTag       schema.Key = "value/suspension/source-tag"
	suspensionSourceSelection schema.Key = "value/suspension/source-selection"

	// Placement's paired RouteMember vector and reducer.
	suspensionRoutes           schema.Key = "placement/suspension/routes"
	suspensionRouteKey         schema.Key = "placement/suspension/route-key"
	suspensionRouteSelection   schema.Key = "placement/suspension/route-selection"
	suspensionRouteTag         schema.Key = "placement/suspension/route-tag"
	suspensionRouteDestination schema.Key = "placement/suspension/route-destination"
	suspensionReducer          schema.Key = "placement/suspension/reducer"
	// Both dependent vectors are produced: which Value cells the subject's
	// liveness is decided from is read out of the publication the candidate
	// was redeemed from, and the allocation it is published into is resolved
	// through the mounted call its boundary names.
	suspensionSourceSelection schema.Key = "value/suspension/source-selection"
	suspensionRouteSelection  schema.Key = "placement/suspension/route-selection"

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

// anchorRead is the denominator statement of the source vector: the closed
// Value world the subject's cells are complete against. It is not a value the
// fold consumes - nothing is folded from an anchor - so it names no fold input
// and the vector read below is complete against it.
func anchorRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		Input:      0,
		Axis:       ruleprogram.AxisRef(valueAxis()),
		Form:       ruleprogram.Complete,
		PointBound: ruleprogram.PointBound,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDense,
			OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
			Multiplicity:   ruleprogram.MultiplicityMany,
			DenominatorRef: denominatorReference(valueCoordinateDenominator),
		},
	}
}

// sourceVectorRead is the whole-vector delivery of the subject's Value cells.
// The judgment folds the vector, not one cell of it, so the read states the
// span it delivers rather than a per-cell selection.
func sourceVectorRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		Input:      0,
		Axis:       ruleprogram.AxisRef(valueAxis()),
		Form:       ruleprogram.Summary,
		PointBound: ruleprogram.PointBound,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
			Multiplicity:   ruleprogram.MultiplicityMany,
			DenominatorRef: denominatorReference(valueCoordinateDenominator),
		},
	}
}

// selectionRef names the operation a produced read is published through, and
// names none where the axis those rows land in publishes none: a reference to
// an axis with no member is a malformed row rather than an absent one.
func selectionRef(axis schema.EntryReference, key schema.Key) member.SelectionRef {
	if !key.Available() {
		return member.SelectionRef{}
	}
	return member.SelectionRef{Axis: axis, Member: key}
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

func declaration(outputAxisKey schema.Key, candidate member.CandidateRef, anchor, anchorKey, sources, sourceKey, sourceTag, sourceSelection, routes, routeKey, routeTag, routeSelection, routeDestination, reducer, outputColumn schema.Key) ruleprogram.Program {
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
				Read:     anchorRead(),
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
				},
				Relation:  member.RelationRef{Axis: value, Member: sources},
				Key:       member.ProjectionRef{Axis: value, Member: sourceKey},
				Predicate: member.ProjectionRef{Axis: value, Member: sourceTag},
				Selection: selectionRef(value, sourceSelection),
				Read:      sourceVectorRead(),
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation:  member.RelationRef{Axis: outputAxis, Member: routes},
				Key:       member.ProjectionRef{Axis: outputAxis, Member: routeKey},
				Selection: selectionRef(outputAxis, routeSelection),
				Read:      selectedOutputRead(outputAxis),
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: outputAxis, Member: reducer},
			// The anchor read states the denominator the vector is complete
			// against; a denominator is not a fold input, so the fold takes
			// the vector and the routed cell and nothing else.
			Inputs: []ruleprogram.JoinRef{1, 2},
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
		suspensionSources, suspensionSourceKey, suspensionSourceTag, suspensionSourceSelection,
		suspensionRoutes, suspensionRouteKey, suspensionRouteTag, suspensionRouteSelection, suspensionRouteDestination,
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
		evidenceSourceSelection  schema.Key = "value/suspension-evidence/source-selection"
		evidenceRoutes           schema.Key = "placement/suspension-evidence/routes"
		evidenceRouteKey         schema.Key = "placement/suspension-evidence/route-key"
		evidenceRouteTag         schema.Key = "placement/suspension-evidence/route-tag"
		evidenceRouteSelection   schema.Key = "placement/suspension-evidence/route-selection"
		evidenceRouteDestination schema.Key = "placement/suspension-evidence/route-destination"
		evidenceSourceSelection  schema.Key = "value/suspension-evidence/source-selection"
		evidenceRouteSelection   schema.Key = "placement/suspension-evidence/route-selection"
		evidenceAxisKey          schema.Key = "placement-suspension-evidence"
		evidenceReducer          schema.Key = "placement-suspension-evidence/reducer"
		evidenceFactsColumn      schema.Key = "placement/suspension-evidence/facts"
	)
	declaration := declaration(
		evidenceAxisKey,
		member.IssuedRowCandidate(programissuance.RelationOccurrenceSubjectLiveness),
		evidenceAnchors, evidenceAnchorKey,
		evidenceSources, evidenceSourceKey, evidenceSourceTag, evidenceSourceSelection,
		evidenceRoutes, evidenceRouteKey, evidenceRouteTag, evidenceRouteSelection, evidenceRouteDestination,
		evidenceReducer, evidenceFactsColumn,
	)
	declaration.OperandRole = vocabulary.RoleKey(EvidenceOperandRole)
	return declaration
}
