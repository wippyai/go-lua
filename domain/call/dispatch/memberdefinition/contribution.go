// Package memberdefinition declares Call dispatch's generated relation and
// reducer vocabulary. It is generator-only; runtime code consumes the sealed
// catalog and direct generated calls.
package memberdefinition

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

const (
	callPackagePath     = "github.com/wippyai/go-lua/domain/call"
	valuePackagePath    = "github.com/wippyai/go-lua/domain/value"
	dispatchPackagePath = "github.com/wippyai/go-lua/domain/call/dispatch"
)

func goType(path, name string) definition.GoType {
	return definition.GoType{PackagePath: path, Name: name}
}

func axisReference(key string) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: schema.Key(key)}
}

// Contribution is Call dispatch's own fold. The candidate is Call's mounted
// application, the one exact read is Value's image of the callee that
// application applies, and the publication is exact at the candidate's own
// coordinate - which is where a dispatch fact has always gone, because
// dispatch refines the Call cell of the application it is indexed by.
//
// The alternatives a callee reduces to are the judgment's own business: they
// are derived inside the fold from the three authorities the declaration
// seals into its state, and their join is the one fact this rule publishes.
// Nothing about them crosses the call shape, so no route carrier, predicate
// tag or destination projection is declared here.
func Contribution() definition.Contribution {
	return definition.Contribution{
		Axis: "call",
		Rule: "call-dispatch",
		Carriers: []definition.Carrier{
			{Name: "ValueFactCarrier", Key: "carrier/value/fact", Type: goType(valuePackagePath, "Value")},
		},
		Reducers: []definition.Reducer{{
			Name:      "DispatchReducer",
			Key:       "call/dispatch/reducer",
			Candidate: "CallCoordinateCarrier",
			Inputs: []definition.ReducerInput{{
				Axis:         axisReference("value"),
				Carrier:      "ValueFactCarrier",
				Form:         member.ReadFormExact,
				Multiplicity: member.MultiplicityOne,
			}},
			Outputs: []definition.ReducerOutput{{Axis: axisReference("call"), Carrier: "CallFactCarrier"}},
			Derivation: definition.ReducerDerivation{
				State: goType(dispatchPackagePath, "Judgment"),
				Build: definition.GoSymbol{PackagePath: dispatchPackagePath, Name: "Derive", ResultIndex: 0},
				StaticAxes: []schema.EntryReference{
					axisReference("call"),
					axisReference("value"),
					axisReference("heap"),
				},
			},
			Implementation: definition.GoSymbol{
				PackagePath: dispatchPackagePath, Name: "Dispatch",
				Receiver: goType(dispatchPackagePath, "Judgment"), ResultIndex: 0,
			},
		}},
	}
}
