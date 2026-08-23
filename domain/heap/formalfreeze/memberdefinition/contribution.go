// Package memberdefinition is the generator-only owner source for the Heap
// formal-freeze rule's own reducer. It is imported by the member definition
// roster and by nothing at runtime.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const heapPackagePath = "github.com/wippyai/go-lua/domain/heap"

func heapAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}
}

// Contribution is the Heap formal-freeze rule's own member declaration.
//
// The fold is handed one member of the route relation its rule declares: the
// coordinate that member routes to, and the Heap world already published at
// that coordinate. It states no plan of its own - which routes a mounted call
// justifies is the relation's answer, not the judgment's - and it recovers the
// schema it decides in from the coordinate, which carries its own owner. That
// is why the declared signature is two carriers and nothing else.
//
// The read is Selected because the routes are a selection over the Heap axis,
// and it is untagged because the route relation declares no predicate: a
// route's identity for this judgment is its coordinate, which arrives as the
// route carrier. The Heap Factor's declared default is Bottom and the freeze
// judgment publishes the same empty normal image for a Bottom predecessor as
// for an unwritten one, so the read carries the Factor's default rather than a
// presence distinction the fold would have to draw.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "heap",
		Rule: "heap-formal-freeze",
		Reducers: []definition.Reducer{{
			Name: "FormalFreezeReducer",
			Key:  "heap/reducer/formal-freeze",
			Inputs: []definition.ReducerInput{{
				Axis:         heapAxis(),
				Carrier:      "HeapFactCarrier",
				Form:         member.ReadFormSelected,
				Multiplicity: member.MultiplicityOne,
				Route:        "HeapKeyCarrier",
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    heapAxis(),
				Carrier: "HeapFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: heapPackagePath, Name: "FormalFreezeFact", ResultIndex: 0},
		}},
	}
}
