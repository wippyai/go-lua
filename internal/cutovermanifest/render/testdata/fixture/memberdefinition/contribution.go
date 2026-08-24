// Package memberdefinition is a synthetic fixture used only by
// render_test.go, mirroring the real domain/*/memberdefinition contribution
// shape. One symbol is deliberately left undeclared (FixtureMissing) so the
// visible-mismatches section has something confirmed to report.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const fixturePackagePath = "github.com/wippyai/go-lua/internal/cutovermanifest/render/testdata/fixture/impl"

func fixtureAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "fixture"}
}

// Contribution declares one reducer whose Implementation names a function
// that does not exist in the fixture impl package, on purpose.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "fixture",
		Rule: "fixture-rule",
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
			Implementation: definition.GoSymbol{PackagePath: fixturePackagePath, Name: "FixtureMissing", ResultIndex: 0},
		}},
	}
}
