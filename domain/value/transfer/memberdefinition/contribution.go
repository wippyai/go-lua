// Package memberdefinition is the generator-only owner source for the Value
// storage-transfer rule's own reducer. It is imported by the member
// definition roster and by nothing at runtime, so a source-level Go symbol
// descriptor never reaches the runtime Value package.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const valuePackagePath = "github.com/wippyai/go-lua/domain/value"

func valueAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
}

// Contribution is the Value storage-transfer rule's reducer definition: the
// exact Value read it consumes and the Value fact it republishes. Transfer is
// the identity fold, so the whole judgment is carrying an authenticated fact
// from one coordinate to another.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "value",
		Rule: "value-transfer",
		Reducers: []definition.Reducer{{
			Name: "IdentityReducer",
			Key:  "value/reducer/identity",
			Inputs: []definition.ReducerInput{{
				Axis:         valueAxis(),
				Carrier:      "ValueFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{
				Axis:    valueAxis(),
				Carrier: "ValueFactCarrier",
			}},
			Implementation: definition.GoSymbol{PackagePath: valuePackagePath, Name: "IdentityValue", ResultIndex: 0},
		}},
	}
}
