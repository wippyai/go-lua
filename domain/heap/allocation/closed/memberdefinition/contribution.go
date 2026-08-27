// Package memberdefinition is the generator-only owner source for the Heap
// closed-allocation rule's own reducer. It is imported by the member
// definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

const (
	closedPackagePath   = "github.com/wippyai/go-lua/domain/heap/allocation/closed"
	keymatchPackagePath = "github.com/wippyai/go-lua/domain/heap/keymatch"
	valuePackagePath    = "github.com/wippyai/go-lua/domain/value"
	heapPackagePath     = "github.com/wippyai/go-lua/domain/heap"
)

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func valueGoType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func heapMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: heapPackagePath,
		Name:        name,
		Receiver:    definition.GoType{PackagePath: heapPackagePath, Name: receiver},
		ResultIndex: resultIndex,
	}
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
			{Name: "HeapKeyCarrier", Key: "carrier/heap/key", Type: definition.GoType{PackagePath: heapPackagePath, Name: "Key"}, Capability: carrier.Equatable},
		},
		CarrierRefs: []definition.CarrierReference{
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Ref: carrier.Ref{Owner: axisReference("value"), Carrier: "carrier/value/fact"}, Type: valueGoType("Value")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Ref: carrier.Ref{Owner: axisReference("value"), Carrier: "carrier/value/coordinate"}, Type: valueGoType("Coordinate")},
			{Name: "ClosedOperandsCarrier", Key: "carrier/value/closed-operands", Ref: carrier.Ref{Owner: axisReference("value"), Carrier: "carrier/value/closed-operands"}, Type: valueGoType("ClosedOperands")},
			{Name: "ClosedOperandCarrier", Key: "carrier/value/closed-operand", Ref: carrier.Ref{Owner: axisReference("value"), Carrier: "carrier/value/closed-operand"}, Type: valueGoType("ClosedOperand")},
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
				// The Heap world this constructor extends, read at the
				// allocation coordinate it writes. It is this rule's own axis,
				// declared here beside the rows it folds with for the same
				// reason: the predecessor is part of how this rule decides.
				Name:              "ClosedAllocationPredecessors",
				Key:               "heap/closed-allocation/predecessors",
				Subject:           "HeapFactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "HeapKeyCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates"}),
			},
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
				Name:    "ClosedOperandCells",
				Key:     "value/closed-allocation/operands",
				Axis:    "value",
				Subject: "ClosedOperandCarrier",
				// The cells are derived from the constructor this read is
				// joined from - the source the join names - and the span they
				// sit in is published by the Value row that corresponds to it.
				Inputs:            []definition.RelationInput{{Carrier: "HeapKeyCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates"}),
			},
		},
		Projections: []definition.Projection{
			{
				Name:              "ClosedAllocationPredecessorKey",
				Key:               "heap/closed-allocation/predecessor-key",
				Relation:          "ClosedAllocationPredecessors",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates"}),
				Role:              member.Key,
				Result:            "HeapKeyCarrier",
				Accessor:          heapMethod("ClosedAllocation", "Key", -1),
			},
			{
				Name:              "ClosedOperandKey",
				Key:               "value/closed-allocation/operand-key",
				Axis:              "value",
				Relation:          "ClosedOperandCells",
				CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axisReference("heap"), Member: "heap/closed-allocation/candidates"}),
				Role:              member.Key,
				Result:            "ValueCoordinateCarrier",
				Accessor:          valueMethod("Coordinate", "ClosedOperand", false, -1),
			},
		},
		Reducers: []definition.Reducer{{
			Name: "ClosedAllocationReducer",
			Key:  "heap/reducer/closed",
			// The candidate a rule folds is the SUBJECT of the relation it
			// draws candidates from, which for a constructor is the
			// allocation coordinate. The structural descriptor it denotes is
			// resolved by the judgment from the schemas it was sealed with -
			// one authority for what a constructor consists of, reached where
			// the fold already rests, rather than a second one carried beside
			// the row.
			Candidate: "HeapKeyCarrier",
			Inputs: []definition.ReducerInput{
				{
					Axis:         axisReference("heap"),
					Carrier:      "HeapFactCarrier",
					Form:         member.ReadFormExact,
					Multiplicity: member.MultiplicityOne,
				},
				{
					Axis:    axisReference("value"),
					Carrier: "ValueFactCarrier",
					// The tag is the coordinate each cell sits at: the span
					// the candidate published IS a list of them, so the
					// correlation this read proved is named by the read
					// axis's own key rather than by a second address.
					Form:         member.ReadFormSummary,
					Multiplicity: member.MultiplicityMany,
					Tag:          "ValueCoordinateCarrier",
				},
			},
			Outputs: []definition.ReducerOutput{{
				Axis:    axisReference("heap"),
				Carrier: "HeapFactCarrier",
			}},
			// The fold rests on cold owner knowledge - the two schemas the
			// constructor is fenced to, and the selector projection that says
			// which atoms select which slots - so that knowledge is SEALED as
			// the rule's state, and the judgment is what the fold is a method
			// on. Its arguments stay carriers.
			//
			// The projection is RECEIVED, not derived. It reads sealed
			// authorities from Heap and Value at once, so no axis is
			// answerable for it and the mount phase constructs it exactly
			// once; a rule that built its own from the same two schemas would
			// be a second authority over which atoms select which slots.
			Derivation: definition.ReducerDerivation{
				State:      definition.GoType{PackagePath: closedPackagePath, Name: "Judgment"},
				Build:      definition.GoSymbol{PackagePath: closedPackagePath, Name: "NewJudgment", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{axisReference("heap"), axisReference("value")},
				Composed: []definition.CompositionSeal{{
					Key:  "composition/heap/selector-projection",
					Name: "selectors",
					Type: definition.GoType{PackagePath: keymatchPackagePath, Name: "SelectorProjection", Pointer: true},
				}},
			},
			Implementation: definition.GoSymbol{
				PackagePath: closedPackagePath,
				Name:        "resultClosed",
				Receiver:    definition.GoType{PackagePath: closedPackagePath, Name: "Judgment"},
				ResultIndex: 0,
			},
		}},
	}
}
