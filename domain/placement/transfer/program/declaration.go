// Package program owns Target-transfer Placement's callback-free rule
// declaration.
//
// It names the foreign mounted-call candidate, the exact Call fact read, the
// call's ordered mounted actuals, the dependent Placement route relation, and
// the routed Placement publication. It holds no engine slot, runtime
// callback, route planner, or compatibility path; the transfer reducer and
// the route derivation remain the domain package's own source.
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
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const (
	valueAxisKey     schema.Key = "value"
	callAxisKey      schema.Key = "call"
	placementAxisKey schema.Key = "placement"
	RuleKey                     = "placement-transfer"
	RuleRole                    = "rule/placement/transfer"
	OperandRole                 = "operand/placement/transfer"

	// Placement owns the route relation and its three projections. Call owns
	// the mounted-call candidate directory and its fact key; Value owns the
	// actual member set, its parent row and its coordinate. Naming each
	// foreign member through its own axis package keeps every declaration law
	// able to see which owner published the row.
	TransferRoutes           schema.Key = placementdomain.TransferRoutes
	TransferRouteKey         schema.Key = placementdomain.TransferRouteKey
	TransferRouteTag         schema.Key = placementdomain.TransferRouteTag
	TransferRouteDestination schema.Key = placementdomain.TransferRouteDestination
	TransferReducer          schema.Key = placementdomain.TransferReducer
	// TransferRouteSelection is the operation Placement publishes the transfer
	// route rows through; which routes exist depends on the mounted actual
	// vector the earlier reads delivered.
	TransferRouteSelection   schema.Key = placementdomain.TransferRouteSelection
	MountedCallCandidates    schema.Key = calldomain.MountedCallCandidates
	MountedCallFacts         schema.Key = calldomain.MountedCallFacts
	MountedCallFactKey       schema.Key = calldomain.MountedCallFactKey
	MountedCallActualMembers schema.Key = valuedomain.MountedCallActualMembers
	MountedCallActualKey     schema.Key = valuedomain.MountedCallActualKey
	MountedCallParents       schema.Key = valuedomain.MountedCallParents

	placementFactsColumn       schema.Key = "placement/facts"
	valueCoordinateDenominator schema.Key = "coordinates/value"
	placementDenominator       schema.Key = "coordinates/placement"
)

func RuleIssues() []rule.Issuance {
	return []rule.Issuance{{
		Occurrence:  "occurrence/call",
		Requirement: "program-requirement/unrestricted",
		Form:        "program-form/call-effect",
	}}
}

// RuleEntry is the canonical callback-free transfer rule declaration. The
// call-effect cut is the invocation boundary at which Call alternatives, Pack
// actuals and Target transfer declarations are all authenticated.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key: RuleKey, Writes: placementAxisKey, Owner: placementAxisKey,
		Issues: RuleIssues(), Lane: rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Transfer(),
	}
}

// StructureSpecs contributes the transfer consumer's rule and operand
// semantic roles.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// Transfer returns the immutable Target-transfer rule declaration.
//
// Join 0 is the one exact Call fact read, keyed at the coordinate Call
// already projects for the mounted-call candidate. It is sparse-explicit
// because an absent Call fact and a present one are different answers here: a
// call whose fact was never written names no target and therefore no
// transfer. Join 1 is that call's ordered mounted actuals - a nested member
// set Value publishes by (parent, ordinal), so which actuals a call has is a
// membership its owner already sealed rather than geometry this rule rewalks.
// Its opaque policy is Propagate: an authenticated opaque actual is exactly
// the evidence the demand widens to every Send root on, and refusing it here
// would lose a displacement the Target declaration authorizes. Join 2 is the
// dependent Placement route relation those two make possible, read selected
// over the Placement denominator and tagged by the route coordinate the
// relation itself issues.
//
// Only join 2 is a fold argument. Joins 0 and 1 are the materialization the
// route relation depends on, and a prerequisite is not an argument. The
// publication is routed and declares no carry: a route displaces only the
// authenticated keys its own selection observed, and every Placement
// coordinate those routes did not name is already the predecessor's.
func Transfer() ruleprogram.Program {
	value := axisReference(valueAxisKey)
	call := axisReference(callAxisKey)
	placement := axisReference(placementAxisKey)
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   call,
			Member: MountedCallCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: call, Member: MountedCallFacts},
				Key:      member.ProjectionRef{Axis: call, Member: MountedCallFactKey},
				Read: ruleprogram.ReadDecl{
					Input:      0,
					Axis:       ruleprogram.AxisRef(call),
					Form:       ruleprogram.Exact,
					PointBound: ruleprogram.PointBound,
					Contract: ruleprogram.ReadContract{
						Order:        ruleprogram.OrderCanonical,
						Sparse:       ruleprogram.SparseExplicit,
						OnOpaque:     ruleprogram.OnOpaqueRefuse,
						Multiplicity: ruleprogram.MultiplicityOne,
					},
				},
			},
			{
				Sources:  []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{Axis: value, Member: MountedCallActualMembers},
				Key:      member.ProjectionRef{Axis: value, Member: MountedCallActualKey},
				Parent:   member.RelationRef{Axis: value, Member: MountedCallParents},
				Read: ruleprogram.ReadDecl{
					// Every read of this rule is bound to the one input port
					// the candidate arrives on. The port is what fixes the
					// point a read observes, and all three of these observe
					// the mounted call the candidate names: a Value read bound
					// to a second port would observe a point at which the
					// mounted actual's fact is not yet published, and answer
					// the Factor default in place of it.
					Input: 0,
					Axis:  ruleprogram.AxisRef(value),
					Form:  ruleprogram.Summary,
					// The vector resolves through Value's own member set at
					// this Input, not a transported occurrence, so Input 1's
					// slot shares the candidate's own point.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaquePropagateAuthenticated,
						Multiplicity:   ruleprogram.MultiplicityMany,
						DenominatorRef: denominatorReference(valueCoordinateDenominator),
					},
				},
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
					ruleprogram.PriorSource(1),
				},
				Relation:  member.RelationRef{Axis: placement, Member: TransferRoutes},
				Key:       member.ProjectionRef{Axis: placement, Member: TransferRouteKey},
				Predicate: member.ProjectionRef{Axis: placement, Member: TransferRouteTag},
				Selection: member.SelectionRef{Axis: placement, Member: TransferRouteSelection},
				Read: ruleprogram.ReadDecl{
					Input: 0,
					Axis:  ruleprogram.AxisRef(placement),
					Form:  ruleprogram.Selected,
					// Resolved through Placement's own route directory at this
					// Input, not a transported occurrence, the same as Input 1.
					PointBound: ruleprogram.PointBoundSelf,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseDefault,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: denominatorReference(placementDenominator),
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{Axis: placement, Member: TransferReducer},
			Inputs:  []ruleprogram.JoinRef{2},
			Outputs: []ruleprogram.OutputDecl{{
				Column:           axis.OutputRef{Axis: placement, Key: placementFactsColumn},
				Destination:      member.ProjectionRef{Axis: placement, Member: TransferRouteDestination},
				Mode:             ruleprogram.ModeRoute,
				ValueSlot:        0,
				RouteJoin:        2,
				RouteJoinPresent: true,
			}},
		},
	}
}
