// Package memberdefinition is the generator-only contribution for Value's
// binary-arithmetic rule. Value owns the candidate endpoint directory; this
// contribution owns the two exact source joins, their coordinate projections,
// and the reducer signature that folds the arithmetic judgment. The fold
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
		Member: "value/binary-arithmetic/candidates",
	}
}

// Contribution declares the binary-arithmetic member vocabulary. Both exact
// joins are keyed by the same candidate input and differ only in the
// owner-issued coordinate projection they use. The reducer receives the
// candidate followed by those two Value facts and returns one Value fact plus
// the sealed ReductionOutcome; the implementation is the domain fold named
// by Value.ArithmeticValue, never an engine callback, rule handle, or
// arithmetic-package adapter.
func Contribution() definition.Contribution {
	provider := candidateProvider()
	return definition.Contribution{
		Axis: "value",
		Rule: "value-binary-arithmetic",
		Relations: []definition.Relation{{
			Name:              "BinaryArithmeticSources",
			Key:               "value/binary-arithmetic/sources",
			Subject:           "ValueFactCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "BinaryArithmeticCarrier"}},
			CandidateProvider: member.AxisRelationCandidate(provider),
		}},
		Projections: []definition.Projection{
			{
				Name:              "BinaryArithmeticLeft",
				Key:               "value/binary-arithmetic/left",
				Relation:          "BinaryArithmeticSources",
				CandidateProvider: member.AxisRelationCandidate(provider),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Left", "BinaryArithmetic", -1),
			},
			{
				Name:              "BinaryArithmeticRight",
				Key:               "value/binary-arithmetic/right",
				Relation:          "BinaryArithmeticSources",
				CandidateProvider: member.AxisRelationCandidate(provider),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Right", "BinaryArithmetic", -1),
			},
			{
				Name:              "BinaryArithmeticWrite",
				Key:               "value/binary-arithmetic/write",
				Relation:          "BinaryArithmeticCandidates",
				CandidateProvider: member.AxisRelationCandidate(provider),
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Write", "BinaryArithmetic", -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "BinaryArithmeticReducer",
			Key:       "value/binary-arithmetic/reducer",
			Candidate: "BinaryArithmeticCarrier",
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
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "ArithmeticValue", ResultIndex: 0},
		}},
	}
}
