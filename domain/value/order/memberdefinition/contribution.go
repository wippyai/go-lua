// Package memberdefinition is the generator-only contribution for Value's
// binary-order rule. Value owns the candidate endpoint directory; this
// contribution owns the two exact source joins, their coordinate projections,
// and the reducer signature that folds the order judgment. The fold
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
		Member: "value/binary-order/candidates",
	}
}

// Contribution declares the binary-order member vocabulary. Both exact joins
// are keyed by the same candidate input and differ only in the owner-issued
// coordinate projection they use. The reducer receives the candidate followed
// by those two Value facts and returns one Value fact plus the sealed
// ReductionOutcome; the implementation is the domain fold named by
// Value.OrderValue, never an engine callback, rule handle, or order-package
// adapter.
func Contribution() definition.Contribution {
	provider := candidateProvider()
	return definition.Contribution{
		Axis: "value",
		Rule: "value-binary-order",
		Relations: []definition.Relation{{
			Name:              "BinaryOrderSources",
			Key:               "value/binary-order/sources",
			Subject:           "ValueFactCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "BinaryOrderCarrier"}},
			CandidateProvider: provider,
		}},
		Projections: []definition.Projection{
			{
				Name:              "BinaryOrderLeft",
				Key:               "value/binary-order/left",
				Relation:          "BinaryOrderSources",
				CandidateProvider: provider,
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Left", "BinaryOrder", -1),
			},
			{
				Name:              "BinaryOrderRight",
				Key:               "value/binary-order/right",
				Relation:          "BinaryOrderSources",
				CandidateProvider: provider,
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Right", "BinaryOrder", -1),
			},
			{
				Name:              "BinaryOrderWrite",
				Key:               "value/binary-order/write",
				Relation:          "BinaryOrderCandidates",
				CandidateProvider: provider,
				Role:              member.Destination,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Write", "BinaryOrder", -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "BinaryOrderReducer",
			Key:       "value/binary-order/reducer",
			Candidate: "BinaryOrderCarrier",
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
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "OrderValue", ResultIndex: 0},
		}},
	}
}
