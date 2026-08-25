// Package memberdefinition is the generator-only owner source for the
// allocation rule's own fold. It is imported by the member definition roster
// and by nothing at runtime, so the allocation package keeps its judgment and
// none of the source-level symbol metadata.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const allocationPackagePath = "github.com/wippyai/go-lua/domain/value/allocation"

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

func judgmentType() definition.GoType {
	return definition.GoType{PackagePath: allocationPackagePath, Name: "Judgment"}
}

// reducer is the shape of this rule's fold: the allocation receipt it is
// indexed by, the Value fact it publishes, and the sealed judgment that answers
// it. It declares no input, because the rule declares no read - the receipt is
// the whole of the evidence its answer rests on.
//
// The candidate carrier, the directory that subjects it, the coordinate the
// publication lands at, and the transition the carry applies are Value's own
// declarations, so this contribution names them rather than restating them.
func reducer() definition.Reducer {
	return definition.Reducer{
		Name:      "AllocationResultReducer",
		Key:       "value/allocation/reducer",
		Candidate: "AllocationResultCarrier",
		Outputs: []definition.ReducerOutput{{
			Axis:    axisReference("value"),
			Carrier: "ValueFactCarrier",
		}},
		Derivation: definition.ReducerDerivation{
			State:      judgmentType(),
			Build:      definition.GoSymbol{PackagePath: allocationPackagePath, Name: "Derive", ResultIndex: 0},
			StaticAxes: []schema.EntryReference{axisReference("value")},
		},
		Implementation: definition.GoSymbol{
			PackagePath: allocationPackagePath, Name: "Result",
			Receiver: judgmentType(), ResultIndex: 0,
		},
	}
}

// Contribution is the allocation rule's whole share of the member vocabulary:
// the fold that answers from Value's sealed constructor receipts.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis:     "value",
		Rule:     "value-allocation",
		Reducers: []definition.Reducer{reducer()},
	}
}
