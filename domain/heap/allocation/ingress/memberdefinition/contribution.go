// Package memberdefinition is the generator-only owner source for the Heap
// allocation-ingress rule's own reducer. It is imported by the member
// definition roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const heapPackagePath = "github.com/wippyai/go-lua/domain/heap"

func heapAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
}

// Contribution is the Heap allocation-ingress rule's reducer definition: a
// zero-input fold over one owner-issued allocation key, which rederives the
// WorldZero image that key was sealed for.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-ingress",
		Reducers: []definition.Reducer{{
			Name:      "IngressReducer",
			Key:       "heap/reducer/ingress",
			Candidate: "HeapKeyCarrier",
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "IngressFact", ResultIndex: 0},
		}},
	}
}
