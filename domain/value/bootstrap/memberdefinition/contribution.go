// Package memberdefinition is the generator-only owner source for the Value
// bootstrap rule's own reducer. It is imported by the member definition roster
// and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const valuePackagePath = "github.com/wippyai/go-lua/domain/value"

func valueAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
}

// Contribution is the Value bootstrap rule's reducer definition: a zero-input
// fold over one sealed Host global binding receipt, which restates the Value
// the binding was sealed with or the sealed absence of an initial value.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "value",
		Rule: "value-bootstrap",
		Reducers: []definition.Reducer{{
			Name:      "GlobalBootstrapReducer",
			Key:       "value/reducer/global-bootstrap",
			Candidate: "GlobalBootstrapResultCarrier",
			Outputs: []definition.ReducerOutput{{
				Axis:    valueAxis(),
				Carrier: "ValueFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "GlobalBootstrapFact", ResultIndex: 0},
		}},
	}
}
