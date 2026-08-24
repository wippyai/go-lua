// Package memberdefinition is the generator-only contribution for Value's
// presence-refinement rule. Value owns the candidate endpoint directory; this
// contribution owns the one exact source join, its coordinate projections,
// and the reducer signature that folds the refinement judgment. The fold
// implementation remains Value-owned; this package contributes only its
// declaration.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath = "github.com/wippyai/go-lua/domain/value"
)

func valueAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
}

func valueGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func valueMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: valuePackagePath,
		Name:        name,
		Receiver:    valueGoType(receiver),
		ResultIndex: resultIndex,
	}
}

func candidateProvider() member.RelationRef {
	return member.RelationRef{
		Axis:   valueAxis(),
		Member: "value/presence-refinement/candidates",
	}
}

// Contribution declares the presence-refinement member vocabulary. The one
// exact join is keyed by the candidate's guarded-read coordinate projection.
// The reducer receives the candidate followed by that one Value fact and
// returns one Value fact plus the sealed ReductionOutcome; the implementation
// is the domain fold named by Value.PresenceRefinementValue, never an engine
// callback, rule handle, or refinement-package adapter.
func Contribution() definition.Contribution {
	provider := candidateProvider()
	return definition.Contribution{
		Axis: "value",
		Rule: "value-presence-refinement",
		Relations: []definition.Relation{{
			Name:              "PresenceRefinementSources",
			Key:               "value/presence-refinement/sources",
			Subject:           "ValueFactCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "PresenceRefinementCarrier"}},
			CandidateProvider: member.AxisRelationCandidate(provider),
		}},
		Projections: []definition.Projection{
			{
				Name:              "PresenceRefinementSourceKey",
				Key:               "value/presence-refinement/source-key",
				Relation:          "PresenceRefinementSources",
				CandidateProvider: member.AxisRelationCandidate(provider),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Left", "PresenceRefinement", -1),
			},
			{
				Name:              "PresenceRefinementWrite",
				Key:               "value/presence-refinement/write",
				Relation:          "PresenceRefinementCandidates",
				CandidateProvider: member.AxisRelationCandidate(provider),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Write", "PresenceRefinement", -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "PresenceRefinementReducer",
			Key:       "value/presence-refinement/reducer",
			Candidate: "PresenceRefinementCarrier",
			Inputs: []definition.ReducerInput{{
				Axis:         valueAxis(),
				Carrier:      "ValueFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    valueAxis(),
				Carrier: "ValueFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "PresenceRefinementValue", ResultIndex: 0},
		}},
	}
}
