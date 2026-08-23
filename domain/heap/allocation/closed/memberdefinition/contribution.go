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
