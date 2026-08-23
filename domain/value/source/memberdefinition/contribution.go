// Package memberdefinition is the generator-only owner source for the Value
// source rule's own reducer. It is imported by the member definition roster
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

// Contribution is the Value source rule's reducer definition: a zero-input
// fold over one owner-issued source seed, which rederives the immutable fact
// the seed was sealed for.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "value",
		Rule: "value-source",
		Reducers: []definition.Reducer{{
			Name:      "SourceReducer",
			Key:       "value/reducer/source",
			Candidate: "SourceSeedCarrier",
			Outputs: []definition.ReducerOutput{{
				Axis:    valueAxis(),
				Carrier: "ValueFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "SourceFact", ResultIndex: 0},
		}},
	}
}
