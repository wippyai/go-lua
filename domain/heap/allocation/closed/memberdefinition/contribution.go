// Package memberdefinition is the generator-only owner source for the Heap
// closed-allocation rule's own reducer. It is imported by the member
// definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	closedPackagePath = "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	valuePackagePath  = "github.com/wippyai/go-lua/domain/value"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func valueGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func valueMethod(name, receiver string, receiverPointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath:     valuePackagePath,
		Name:            name,
		Receiver:        valueGoType(receiver),
		ReceiverPointer: receiverPointer,
		ResultIndex:     resultIndex,
	}
}

func valueRelation(key string) member.RelationRef {
	return member.RelationRef{Axis: axisReference("value"), Member: schema.Key(key)}
}

// Contribution is the Heap closed-allocation rule's reducer definition: the
// sealed scalar constructor candidate, the exact Heap predecessor it extends,
// and the Value summary over the constructor's own coordinate vector. The
// fold's whole answer is the Heap world the constructor denotes together with
// the outcome that world is delivered under.
//
// The declaration states the fold's shape, not its world semantics: CX-28 -
// the correction to how the enumerated coordinate product accumulates
// Cartesian worlds - is an open change to that fold and is deliberately not
// made here.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-closed",
		Carriers: []definition.Carrier{
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: valueGoType("Value")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: valueGoType("Coordinate")},
			{Name: "ClosedOperandsCarrier", Key: "carrier/value/closed-operands", Type: valueGoType("ClosedOperands")},
			{Name: "ClosedOperandCarrier", Key: "carrier/value/closed-operand", Type: valueGoType("ClosedOperand")},
		},
		// The rows this fold reads are VALUE rows, and they are declared here
		// because this rule is what reads them - the axis states per row which
		// axis's data it is, so a rule contributes the rows it folds over
		// without the axis base becoming the file every new rule edits.
		//
		// Which coordinates a constructor consumes is a fact in Value's own
		// numbering: a coordinate's dense key is the position Value's
		// normalizer assigned it, and Heap, which is upstream, holds no index
		// into it. So the span is published by a Value row addressed by the
		// same occurrence the Heap constructor is, and the correspondence
		// between the two directories is what lets this rule - whose candidate
		// is the Heap constructor - reach it.
		Relations: []definition.Relation{
			{
				Name:              "ClosedOperandParents",
				Key:               "value/closed-allocation/parents",
				Axis:              "value",
				Subject:           "ClosedOperandsCarrier",
				CandidateProvider: member.AxisRelationCandidate(valueRelation("value/closed-allocation/parents")),
				CandidateResolver: valueMethod("ClosedOperandsForMountedOccurrence", "Schema", true, 0),
				CandidateOrdinal:  valueMethod("ClosedOperandsOrdinal", "Schema", true, 0),
				CandidateAt:       valueMethod("ClosedOperandsAt", "Schema", true, 0),
				KeyVectorCount:    valueMethod("KeyVectorCount", "ClosedOperands", false, 0),
				KeyVectorAt:       valueMethod("KeyVectorAt", "ClosedOperands", false, 0),
				Correspondences: []member.RelationRef{{
					Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates",
				}},
			},
			{
				// The cells that vector spans. They are addressed by the
				// parent's published keys rather than by a directory of their
				// own, because an operand IS a Value coordinate: giving each a
				// row identity would number the same reads a second time.
				Name:              "ClosedOperandCells",
				Key:               "value/closed-allocation/operands",
				Axis:              "value",
				Subject:           "ClosedOperandCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "ClosedOperandsCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(valueRelation("value/closed-allocation/parents")),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "ClosedOperandKey",
				Key:               "value/closed-allocation/operand-key",
				Axis:              "value",
				Relation:          "ClosedOperandCells",
				CandidateProvider: member.AxisRelationCandidate(valueRelation("value/closed-allocation/parents")),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "ClosedOperand", false, -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name:      "ClosedAllocationReducer",
			Key:       "heap/reducer/closed",
			Candidate: "ClosedAllocationCarrier",
			Inputs: []definition.ReducerInput{
				{
					Axis:         axisReference("heap"),
					Carrier:      "HeapFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
				{
					Axis:         axisReference("value"),
					Carrier:      "ValueFactCarrier",
					Form:         member.ReadFormSummary,
					Multiplicity: member.MultiplicityMany,
					Tag:          "ValueCoordinateCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{
				Axis:    axisReference("heap"),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: closedPackagePath, Name: "resultClosed", ResultIndex: 0},
		}},
	}
}
