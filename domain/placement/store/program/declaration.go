// Package program owns Placement Store's callback-free rule declaration.
//
// This package is deliberately separate from store's reducer/data package.
// It names the sealed Value candidate relation, the exact foreign Value source
// read, the dependent selected Placement route read, and the routed Placement
// publication. It contains no engine slot, runtime callback, route planner, or
// compatibility path. The declaration is the cold half of the Store family;
// store.StorageFold remains the one domain reducer implementation named by
// Placement's member contribution.
package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// The Store family identities. These are declaration keys, not runtime
// handles: composition resolves them against the sealed axis/member surfaces.
const (
	AxisKey     schema.Key = "placement"
	OutputKey   schema.Key = "placement/facts"
	RuleKey     schema.Key = "placement-storage"
	FactorRole             = "factor/placement"
	RuleRole               = "rule/placement/storage"
	OperandRole            = "operand/placement/storage"

	valueAxisKey     schema.Key = "value"
	placementAxisKey schema.Key = AxisKey

	// Placement owns the route relation and its projections. Value owns the
	// candidate/source relation and its key projection; these aliases keep the
	// foreign side explicit in every declaration law.
	StorageRoutes           schema.Key = placementdomain.StorageRoutes
	StorageRouteKey         schema.Key = placementdomain.StorageRouteKey
	StorageRouteTag         schema.Key = placementdomain.StorageRouteTag
	StorageRouteDestination schema.Key = placementdomain.StorageRouteDestination
	StorageReducer          schema.Key = placementdomain.StorageReducer
	// StorageRouteSelection is the operation Placement publishes the storage
	// route rows through; they are produced from the transfer source the
	// earlier read delivered rather than enumerated.
	StorageRouteSelection     schema.Key = placementdomain.StorageRouteSelection
	StorageTransferCandidates schema.Key = valuedomain.StorageTransferCandidates
	StorageTransferSources    schema.Key = valuedomain.StorageTransferSources
	StorageTransferSourceKey  schema.Key = valuedomain.StorageTransferSourceKey
)

func axisReference(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func denominatorReference(key schema.Key) ruleprogram.DenominatorRef {
	return ruleprogram.DenominatorRef{Surface: schema.SurfaceKindDenominator, Key: key}
}

// RuleIssues is Store's mounted issuance geometry. A storage bind has both a
// local successor and a call-effect (tail-transfer) form; a storage write has
// the local predecessor form. Returning a fresh slice keeps the declaration
// value independent of callers that inspect or sort the rows.
func RuleIssues() []rule.Issuance {
	return []rule.Issuance{
		{Occurrence: "occurrence/storage-bind-transfer", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"},
		{Occurrence: "occurrence/storage-bind-transfer", Requirement: "program-requirement/tail-transfer-result", Form: "program-form/call-effect"},
		{Occurrence: "occurrence/storage-write", Requirement: "program-requirement/unrestricted", Form: "program-form/local-predecessor"},
	}
}

// RuleEntry is the canonical callback-free Store rule declaration. The Store
// family is installed through the generated RuleFamily seam; this value is
// what Program composition consumes.
func RuleEntry() rule.Spec {
	return rule.Spec{
		Key:      RuleKey,
		Writes:   AxisKey,
		Owner:    AxisKey,
		Issues:   RuleIssues(),
		Lane:     rule.LaneMounted,
		Semantic: vocabulary.RoleKey(RuleRole),
		Roles:    []schema.Key{vocabulary.RoleKey(OperandRole)},
		Program:  Storage(),
	}
}

// StructureSpecs contributes Store's rule and operand semantic roles. The
// factor role is Placement's axis-owner declaration and is therefore not
// re-authored by this rule package.
func StructureSpecs() []structure.Spec {
	return vocabulary.RoleSpecs(RuleRole, OperandRole)
}

// Storage returns the immutable Store rule declaration.
//
// Join 0 is the one exact foreign Value source read keyed by the candidate's
// source coordinate. Join 1 is the dependent Placement route relation: it
// consumes the same candidate and Join 0's Value fact, then performs one
// selected read over the Placement denominator. The route join is explicit on
// the output; no selected-join inference is permitted by the ABI. Both reads
// use canonical order, explicit sparsity, one-cell multiplicity, and refusal
// on opaque evidence. Store has no fallback/Unknown branch in its declaration.
func Storage() ruleprogram.Program {
	valueAxis := axisReference(valueAxisKey)
	placementAxis := axisReference(placementAxisKey)
	placementDenominator := denominatorReference("coordinates/placement")
	return ruleprogram.Program{
		OperandRole: vocabulary.RoleKey(OperandRole),
		Candidate: member.AxisRelationCandidate(member.RelationRef{
			Axis:   valueAxis,
			Member: StorageTransferCandidates,
		}),
		Joins: []ruleprogram.JoinDecl{
			{
				Sources: []ruleprogram.SourceRef{ruleprogram.CandidateSource()},
				Relation: member.RelationRef{
					Axis:   valueAxis,
					Member: StorageTransferSources,
				},
				Key: member.ProjectionRef{
					Axis:   valueAxis,
					Member: StorageTransferSourceKey,
				},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(valueAxis),
					Form:       ruleprogram.Exact,
					Contract: ruleprogram.ReadContract{
						Order:        ruleprogram.OrderCanonical,
						Sparse:       ruleprogram.SparseExplicit,
						OnOpaque:     ruleprogram.OnOpaqueRefuse,
						Multiplicity: ruleprogram.MultiplicityOne,
					},
				},
			},
			{
				Sources: []ruleprogram.SourceRef{
					ruleprogram.CandidateSource(),
					ruleprogram.PriorSource(0),
				},
				Relation: member.RelationRef{
					Axis:   placementAxis,
					Member: StorageRoutes,
				},
				Key: member.ProjectionRef{
					Axis:   placementAxis,
					Member: StorageRouteKey,
				},
				Predicate: member.ProjectionRef{
					Axis:   placementAxis,
					Member: StorageRouteTag,
				},
				Selection: member.SelectionRef{
					Axis:   placementAxis,
					Member: StorageRouteSelection,
				},
				Read: ruleprogram.ReadDecl{
					PointBound: ruleprogram.PointBound,
					Input:      0,
					Axis:       ruleprogram.AxisRef(placementAxis),
					Form:       ruleprogram.Selected,
					Contract: ruleprogram.ReadContract{
						Order:          ruleprogram.OrderCanonical,
						Sparse:         ruleprogram.SparseExplicit,
						OnOpaque:       ruleprogram.OnOpaqueRefuse,
						Multiplicity:   ruleprogram.MultiplicityOne,
						DenominatorRef: placementDenominator,
					},
				},
			},
		},
		Fold: ruleprogram.FoldDecl{
			Reducer: member.ReducerRef{
				Axis:   placementAxis,
				Member: StorageReducer,
			},
			Inputs: []ruleprogram.JoinRef{0, 1},
			Outputs: []ruleprogram.OutputDecl{
				{
					Column: axis.OutputRef{
						Axis: placementAxis,
						Key:  OutputKey,
					},
					Destination: member.ProjectionRef{
						Axis:   placementAxis,
						Member: StorageRouteDestination,
					},
					Mode:             ruleprogram.ModeRoute,
					ValueSlot:        0,
					RouteJoin:        1,
					RouteJoinPresent: true,
				},
			},
		},
	}
}
