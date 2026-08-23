// Package memberdefinition is the generator-only owner source for the Pack
// source rule's own reducer. It is imported by the member definition roster
// and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const packPackagePath = "github.com/wippyai/go-lua/domain/pack"

func packAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "pack"}
}

// Contribution is the Pack source rule's reducer definition: a zero-input
// fold over one owner-issued Source row.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "pack",
		Rule: "pack-source",
		Reducers: []definition.Reducer{{
			Name:      "SourceReducer",
			Key:       "pack/reducer/source",
			Candidate: "SourceCarrier",
			Outputs: []definition.ReducerOutput{{
				Axis:    packAxis(),
				Carrier: "FactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: packPackagePath, Name: "SourceFact", ResultIndex: 0},
		}},
	}
}
