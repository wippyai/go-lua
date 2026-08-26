// Package memberdefinition is a synthetic fixture mirroring the real
// domain/*/memberdefinition contribution shape, used only by
// inventory_test.go. It is not loaded by anything else.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const fixturePackagePath = "github.com/wippyai/go-lua/internal/cutovermanifest/inventory/testdata/fixture/impl"

func fixtureAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "fixture"}
}

func fixtureMethod(name, receiver string, resultIndex int8) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: fixturePackagePath,
		Name:        name,
		Receiver:    definition.GoType{PackagePath: fixturePackagePath, Name: receiver},
		ResultIndex: resultIndex,
	}
}

// Contribution is the fixture's synthetic declaration surface: one relation,
// one projection over it, and one reducer, matching the real
// domain/heap/allocation/empty shape closely enough to exercise the walker.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "fixture",
		Rule: "fixture-rule",
		Relations: []definition.Relation{{
			Name:              "FixturePredecessors",
			Key:               "fixture/predecessors",
			Subject:           "FixtureFactCarrier",
			Inputs:            []definition.RelationInput{{Carrier: "FixtureKeyCarrier"}},
			CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: fixtureAxis(), Member: "fixture/candidates"}),
		}},
		Projections: []definition.Projection{{
			Name:              "FixturePredecessorKey",
			Key:               "fixture/predecessor-key",
			Relation:          "FixturePredecessors",
			CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: fixtureAxis(), Member: "fixture/candidates"}),
			Role:              member.Key,
			Result:            "FixtureKeyCarrier",
			Accessor:          fixtureMethod("FixtureKey", "Key", -1),
		}},
		Reducers: []definition.Reducer{{
			Name:      "FixtureReducer",
			Key:       "fixture/reducer",
			Candidate: "FixtureKeyCarrier",
			Inputs: []definition.ReducerInput{{
				Axis:         fixtureAxis(),
				Carrier:      "FixtureFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    fixtureAxis(),
				Carrier: "FixtureFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: fixturePackagePath, Name: "FixtureFact", ResultIndex: 0},
		}},
	}
}
