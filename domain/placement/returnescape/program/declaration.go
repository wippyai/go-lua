// Package program owns ReturnEscape's callback-free rule declaration.
//
// The package deliberately carries only schema keys.  The Value and Placement
// member catalogs are being extended by the FT-25 RouteMember work; keeping
// those references local lets this declaration land before the shared catalog
// does, without minting a second owner or a compatibility read.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	valueAxisKey     schema.Key = "value"
	placementAxisKey schema.Key = "placement"
	RuleKey                     = "placement-return-escape"
	RuleRole                    = "rule/placement/return-escape"
	OperandRole                 = "operand/placement/return-escape"

	// Value owns the return-boundary candidate and its heterogeneous member
	// rows.  These are intentionally local until Value publishes the rows in
	// its generated member catalog.
	returnBoundaryCandidates schema.Key = "value/return-boundary/candidates"
	returnBoundaryRoots      schema.Key = "value/return-boundary/roots"
	returnBoundaryRootKey    schema.Key = "value/return-boundary/root-key"
	returnBoundaryMembers    schema.Key = "value/return-boundary/members"
	returnBoundaryMemberKey  schema.Key = "value/return-boundary/member-key"

	// Placement owns the route member relation.  A RouteMember is the paired
	// read coordinate/write destination; the selected join names its key and
	// tag while the output names the same row's destination.
	returnEscapeRoutes           schema.Key = "placement/return-escape/routes"
	returnEscapeRouteKey         schema.Key = "placement/return-escape/route-key"
	returnEscapeRouteTag         schema.Key = "placement/return-escape/route-tag"
	returnEscapeRouteDestination schema.Key = "placement/return-escape/route-destination"
	returnEscapeReducer          schema.Key = "placement/return-escape/reducer"
	// The route rows are produced by the operation named here: which members
	// escape depends on the boundary vector the earlier reads delivered.
	returnEscapeRouteSelection schema.Key = "placement/return-escape/route-selection"

	valueCoordinateColumn      schema.Key = "value/facts"
	placementFactsColumn       schema.Key = "placement/facts"
	valueCoordinateDenominator schema.Key = "coordinates/value"
	placementDenominator       schema.Key = "coordinates/placement"
)

func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/return-boundary",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/local-successor",
	}}
}

func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: placementAxisKey, Owner: placementAxisKey,
		Issues: RuleIssues(), Lane: rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  ReturnEscape(),
	}
}

func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
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
		PointBound: ruleprogram.PointBound,
		Input:      0,
		Axis:       ruleprogram.AxisRef(valueAxis()),
		Form:       ruleprogram.Exact,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
			Multiplicity:   ruleprogram.MultiplicityOne,
			DenominatorRef: denominatorReference(valueCoordinateDenominator),
		},
	}
}

// summaryValueRead is the whole delivered vector of one return's fixed member
// set. The set is a closed denominator its own owner publishes - MemberCount
// and MemberAt ARE that denominator - so the read spans all of it, and the
// declaration that says so is a Summary read correlated by the parent ordinal
// rather than by a selection tag.
func summaryValueRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		PointBound: ruleprogram.PointBound,
		Input:      0,
		Axis:       ruleprogram.AxisRef(valueAxis()),
		Form:       ruleprogram.Summary,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
			Multiplicity:   ruleprogram.MultiplicityMany,
			DenominatorRef: denominatorReference(valueCoordinateDenominator),
		},
	}
}

func selectedPlacementRead() ruleprogram.ReadDecl {
	return ruleprogram.ReadDecl{
		PointBound: ruleprogram.PointBound,
		Input:      0,
		Axis:       ruleprogram.AxisRef(placementAxis()),
		Form:       ruleprogram.Selected,
		Contract: ruleprogram.ReadContract{
			Order:          ruleprogram.OrderCanonical,
			Sparse:         ruleprogram.SparseDefault,
			OnOpaque:       ruleprogram.OnOpaqueRefuse,
			Multiplicity:   ruleprogram.MultiplicityOne,
			DenominatorRef: denominatorReference(placementDenominator),
		},
	}
}

// ReturnEscape returns the three-read J/WR declaration, run by the generated
// ReturnEscape family:
//
//	Value return-boundary candidate
//	    -> exact root/anchor Value fact
//	    -> the delivered vector of its fixed member set
//	    -> selected Placement RouteMember rows
//
// The last join is the only route source and is named explicitly by the
// output.  Its destination is therefore the paired RouteMember destination,
// not a second inferred vector. The predecessor state is transported by the
// engine; this routed rule declares no duplicate identity carry. Empty
// selections settle through the routed form, with no fallback output or
// Unknown compensation.
func ReturnEscape() ruleprogram.Program {
	value := valueAxis()
	placement := placementAxis()
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   value,
			Member: returnBoundaryCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: value, Member: returnBoundaryRoots},
				Key:      member.ProjectionRef{Axis: value, Member: returnBoundaryRootKey},
				Read:     exactValueRead(),
			},
			{
				// ReturnBoundaryMembers is a self-provided nested member set
				// (Parent/Ordinal): its census densifies through its own
				// directory, not through this Source. The Source still names the
				// same candidate row join 0 read, exactly as ReturnBoundaryRoots
				// does, so the dependent read is addressable at all.
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
				},
				Relation: member.RelationRef{Axis: value, Member: returnBoundaryMembers},
				Key:      member.ProjectionRef{Axis: value, Member: returnBoundaryMemberKey},
				Parent:   member.RelationRef{Axis: value, Member: returnBoundaryCandidates},
				Read:     summaryValueRead(),
			},
			{
				// Candidate, then the exact root read (join 0), then the
				// delivered Value member vector (join 1): the root is an
				// authenticated candidate prerequisite the route algebra does
				// not fold into its own value, exactly as Store's StorageFold
				// declares its unused source input, but it must still be a
				// reachable join.
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation:  member.RelationRef{Axis: placement, Member: returnEscapeRoutes},
				Key:       member.ProjectionRef{Axis: placement, Member: returnEscapeRouteKey},
				Predicate: member.ProjectionRef{Axis: placement, Member: returnEscapeRouteTag},
				Selection: member.SelectionRef{Axis: placement, Member: returnEscapeRouteSelection},
				Read:      selectedPlacementRead(),
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: placement, Member: returnEscapeReducer},
			Inputs:  []ruleprogram.JoinRef{2},
			Outputs: []ruleprogram.OutputDecl{{
				Column: axis.OutputRef{Axis: placement, Key: placementFactsColumn},
				Destination: member.ProjectionRef{
					Axis:   placement,
					Member: returnEscapeRouteDestination,
				},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
				RouteJoinPresent: true,
			}},
		},
	}
}
