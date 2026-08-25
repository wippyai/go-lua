// Package memberdefinition declares authored-allocation birth's owner-scoped
// read, destination, and reducer surfaces.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath = "github.com/wippyai/go-lua/domain/value"
	birthPackagePath = "github.com/wippyai/go-lua/domain/placement/birth"
)

func axis(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func provider() member.RelationRef {
	return member.RelationRef{Axis: axis("value"), Member: "value/allocation/candidates"}
}

func valueType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func valuePointerType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name, Pointer: true}
}

func valueMethod(name, receiver string, pointer bool, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: valuePackagePath, Name: name, Receiver: valueType(receiver), ReceiverPointer: pointer, ResultIndex: resultIndex}
}

func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement", Rule: "placement-allocation-birth",
		Carriers: []definition.Carrier{
			{Name: "AllocationResultCarrier", Key: "carrier/value/allocation-result", Type: valuePointerType("AllocationResult")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: valueType("Coordinate")},
		},
		Relations: []definition.Relation{
			{Name: "AllocationFacts", Key: "value/allocation/facts", Axis: "value", Subject: "ValueFactCarrier", Inputs: []definition.RelationInput{{Carrier: "AllocationResultCarrier"}}, CandidateProvider: member.AxisRelationCandidate(provider())},
			{Name: "AllocationBirthDestinations", Key: "placement/allocation-birth/destinations", Subject: "AllocationResultCarrier", CandidateProvider: member.AxisRelationCandidate(provider())},
		},
		Projections: []definition.Projection{
			{Name: "AllocationFactKey", Key: "value/allocation/fact-key", Axis: "value", Relation: "AllocationFacts", Role: member.Key, Result: "ValueCoordinateCarrier", Accessor: valueMethod("Coordinate", "AllocationResult", true, -1), CandidateProvider: member.AxisRelationCandidate(provider())},
			{Name: "AllocationBirthDestination", Key: "placement/allocation-birth/destination", Relation: "AllocationBirthDestinations", Role: member.Destination, Result: "PlacementKeyCarrier", Accessor: valueMethod("Key", "AllocationResult", true, -1), CandidateProvider: member.AxisRelationCandidate(provider())},
		},
		Reducers: []definition.Reducer{{
			Name: "AllocationBirthReducer", Key: "placement/allocation-birth/reducer",
			Candidate:      "AllocationResultCarrier",
			Inputs:         []definition.ReducerInput{{Axis: axis("value"), Carrier: "ValueFactCarrier", Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne}},
			Outputs:        []definition.ReducerOutput{{Axis: axis("placement"), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: birthPackagePath, Name: "Allocation", ResultIndex: 0},
		}},
	}
}
