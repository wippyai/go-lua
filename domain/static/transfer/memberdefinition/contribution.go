// Package memberdefinition is the generator-only owner source for the Static
// type-fact transfer rule's own reducer. It is imported by the member
// definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const staticPackagePath = "github.com/wippyai/go-lua/domain/static"

func staticAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "static-type"}
}

// Contribution is the Static transfer rule's reducer definition: the exact
// TypeFact read it consumes and the TypeFact it republishes at the transfer's
// destination coordinate.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "static-type",
		Rule: "static-transfer",
		Reducers: []definition.Reducer{{
			Name: "IdentityTypeFactReducer",
			Key:  "static-type/reducer/identity",
			Inputs: []definition.ReducerInput{{
				Axis:         staticAxis(),
				Carrier:      "TypeFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    staticAxis(),
				Carrier: "TypeFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: staticPackagePath, Name: "IdentityTypeFact", ResultIndex: 0},
		}},
	}
}
