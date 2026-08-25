// Package memberdefinition declares the two owner-scoped surfaces consumed by
// Placement fresh birth. It contains metadata only; no runtime registry or
// callback is introduced.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	valuePackagePath     = "github.com/wippyai/go-lua/domain/value"
	placementPackagePath = "github.com/wippyai/go-lua/domain/placement"
	birthPackagePath     = "github.com/wippyai/go-lua/domain/placement/birth"
)

func axis(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func provider() member.RelationRef {
	return member.RelationRef{Axis: axis("value"), Member: "value/fresh-result/candidates"}
}

func valueType(name string) definition.GoType {
	return definition.GoType{PackagePath: valuePackagePath, Name: name}
}

func valueMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{PackagePath: valuePackagePath, Name: name, Receiver: valueType(receiver), ResultIndex: resultIndex}
}

// Contribution is one Placement rule declaration. Roster composition moves
// the explicitly foreign Value rows into Value's catalog; the destination and
// reducer remain Placement-owned.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "placement",
		Rule: "placement-fresh-birth",
		Carriers: []definition.Carrier{
			{Name: "FreshResultCallCarrier", Key: "carrier/value/fresh-result-call", Type: valueType("FreshResultCall")},
			{Name: "ValueCoordinateCarrier", Key: "carrier/value/coordinate", Type: valueType("Coordinate")},
		},
		Relations: []definition.Relation{
			{Name: "FreshResultFacts", Key: "value/fresh-result/facts", Axis: "value", Subject: "ValueFactCarrier", Inputs: []definition.RelationInput{{Carrier: "FreshResultCallCarrier"}}, CandidateProvider: member.AxisRelationCandidate(provider())},
			{Name: "FreshBirthDestinations", Key: "placement/fresh-birth/destinations", Subject: "FreshResultCallCarrier", CandidateProvider: member.AxisRelationCandidate(provider())},
		},
		Projections: []definition.Projection{
			{Name: "FreshResultFactKey", Key: "value/fresh-result/fact-key", Axis: "value", Relation: "FreshResultFacts", Role: member.Key, Result: "ValueCoordinateCarrier", Accessor: valueMethod("Coordinate", "FreshResultCall", -1), CandidateProvider: member.AxisRelationCandidate(provider())},
			{Name: "FreshBirthDestination", Key: "placement/fresh-birth/destination", Relation: "FreshBirthDestinations", Role: member.Destination, Result: "PlacementKeyCarrier", Accessor: valueMethod("Key", "FreshResultCall", -1), CandidateProvider: member.AxisRelationCandidate(provider())},
		},
		Reducers: []definition.Reducer{{
			Name: "FreshBirthReducer", Key: "placement/fresh-birth/reducer",
			Candidate: "FreshResultCallCarrier",
			Inputs: []definition.ReducerInput{{
				Axis: axis("value"), Carrier: "ValueFactCarrier",
				Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
			}},
			Outputs:        []definition.ReducerOutput{{Axis: axis("placement"), Carrier: "PlacementFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: birthPackagePath, Name: "Fresh", ResultIndex: 0},
		}},
	}
}
