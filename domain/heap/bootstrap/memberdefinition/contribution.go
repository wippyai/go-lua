// Package memberdefinition is the generator-only owner source for the Heap
// bootstrap rule's own reducer. It is imported by the member definition roster
// and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const heapPackagePath = "github.com/wippyai/go-lua/domain/heap"

func heapAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
}

// Contribution is the Heap bootstrap rule's reducer definition: a zero-input
// fold over one sealed bootstrap root, which restates the complete immutable
// image Heap sealed for it.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-bootstrap",
		Reducers: []definition.Reducer{{
			Name:      "BootReducer",
			Key:       "heap/reducer/boot",
			Candidate: "HeapKeyCarrier",
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "BootFact", ResultIndex: 0},
		}},
	}
}
