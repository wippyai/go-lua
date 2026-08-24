// Package memberdefinition is the generator-only contribution for Value's
// binary-equality rule. Value owns the candidate endpoint directory; this
// contribution owns the two exact source joins, their coordinate projections,
// and the reducer signature that folds the equality judgment. The fold
// implementation remains Value-owned; this package contributes only its
// declaration.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const valuePackagePath = "github.com/wippyai/go-lua/domain/value"

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
		Member: "value/binary-equality/candidates",
	}
}

// Contribution declares the binary-equality member vocabulary. Both exact
// joins are keyed by the same candidate input and differ only in the
// owner-issued coordinate projection they use. The reducer receives the
// candidate followed by those two Value facts and returns one Value fact plus
// the sealed ReductionOutcome; the implementation is the domain fold named
// by Value.EqualityValue, never an engine callback, rule handle, or equality
// package adapter.
func Contribution() definition.Contribution {
	provider := candidateProvider()
	return definition.Contribution{
		Axis: "value",
		Rule: "value-binary-equality",
		Relations: []definition.Relation{{
			Name:              "BinaryEqualitySources",
			Key:               "value/binary-equality/sources",
			Subject:           "ValueFactCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "BinaryEqualityCarrier"}},
			CandidateProvider: provider,
		}},
		Projections: []definition.Projection{
			{
				Name:              "BinaryEqualityLeft",
				Key:               "value/binary-equality/left",
				Relation:          "BinaryEqualitySources",
				CandidateProvider: provider,
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Left", "BinaryEquality", -1),
			},
			{
				Name:              "BinaryEqualityRight",
				Key:               "value/binary-equality/right",
				Relation:          "BinaryEqualitySources",
				CandidateProvider: provider,
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Right", "BinaryEquality", -1),
			},
			{
				Name:              "BinaryEqualityWrite",
				Key:               "value/binary-equality/write",
				Relation:          "BinaryEqualityCandidates",
				CandidateProvider: provider,
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Write", "BinaryEquality", -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "BinaryEqualityReducer",
			Key:       "value/binary-equality/reducer",
			Candidate: "BinaryEqualityCarrier",
			Inputs: []definition.ReducerInput{
				{
					Axis:         valueAxis(),
					Carrier:      "ValueFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
				{
					Axis:         valueAxis(),
					Carrier:      "ValueFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
			},
			Outputs: []definition.ReducerOutput{{
				Axis:    valueAxis(),
				Carrier: "ValueFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "EqualityValue", ResultIndex: 0},
		}},
	}
}
