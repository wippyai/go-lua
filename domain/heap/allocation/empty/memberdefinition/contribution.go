// Package memberdefinition is the generator-only owner source for the Heap
// empty-allocation rule's own relation, projection and reducer. It is imported
// by the member definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const heapPackagePath = "github.com/wippyai/go-lua/domain/heap"

func heapAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
}

func heapMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: heapPackagePath,
		Name:        name,
		Receiver:    definition.GoType{PackagePath: heapPackagePath, Name: receiver},
		ResultIndex: resultIndex,
	}
}

// Contribution is the Heap empty-allocation rule's own member declaration: the
// predecessor rows it folds over, the projection that addresses them, and the
// fold itself.
//
// The predecessor of an empty constructor is the Heap world at the very
// coordinate the constructor writes, so the read relation and the destination
// projection resolve through the same owner-issued allocation coordinate. The
// fold's whole answer is that world extended with the fresh object, delivered
// under the outcome it is extended at.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-empty",
		Relations: []definition.Relation{{
			Name:              "EmptyAllocationPredecessors",
			Key:               "heap/empty-allocation/predecessors",
			Subject:           "HeapFactCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "HeapKeyCarrier"}},
			CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: heapAxis(), Member: "heap/empty-allocation/candidates"}),
		}},
		Projections: []definition.Projection{{
			Name:              "EmptyAllocationPredecessorKey",
			Key:               "heap/empty-allocation/predecessor-key",
			Relation:          "EmptyAllocationPredecessors",
			CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: heapAxis(), Member: "heap/empty-allocation/candidates"}),
			Role:              member.Key,
			Result:            "HeapKeyCarrier",
			Accessor:          heapMethod("EmptyAllocation", "Key", -1),
		}},
		Reducers: []definition.Reducer{{
			Name:      "EmptyAllocationReducer",
			Key:       "heap/reducer/empty",
			Candidate: "HeapKeyCarrier",
			Inputs: []definition.ReducerInput{{
				Axis:         heapAxis(),
				Carrier:      "HeapFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "EmptyAllocationFact", ResultIndex: 0},
		}},
	}
}
