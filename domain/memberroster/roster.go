// Package memberroster is the composition roster of axis member sources: the
// one ordered list that says which axes have a member vocabulary and which
// rules contribute a reducer to each.
//
// It exists so the member generator selects a source from a registry rather
// than from a switch it holds itself. A rule that folds is declared in its own
// package as a definition.Contribution and appears here as one line; an axis
// that has a member vocabulary declares its base - carriers, relations,
// projections, carry transforms, key binding - in its owner's generator-only
// source and appears here as one Source. Nothing registers itself.
//
// Every package imported here is generator-only. The roster reaches no runtime
// domain package, so building the generator does not build the analyzer.
package memberroster

import (
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	calldispatch "github.com/wippyai/go-lua/domain/call/dispatch/memberdefinition"
	callbase "github.com/wippyai/go-lua/domain/call/memberdefinition"
	effectcallsite "github.com/wippyai/go-lua/domain/effect/callsite/memberdefinition"
	effectbase "github.com/wippyai/go-lua/domain/effect/memberdefinition"
	heapbase "github.com/wippyai/go-lua/domain/heap/memberdefinition"

	heapclosed "github.com/wippyai/go-lua/domain/heap/allocation/closed/memberdefinition"
	heapempty "github.com/wippyai/go-lua/domain/heap/allocation/empty/memberdefinition"
	heapingress "github.com/wippyai/go-lua/domain/heap/allocation/ingress/memberdefinition"
	heapbootstrap "github.com/wippyai/go-lua/domain/heap/bootstrap/memberdefinition"
	heapfreeze "github.com/wippyai/go-lua/domain/heap/formalfreeze/memberdefinition"
	packbase "github.com/wippyai/go-lua/domain/pack/memberdefinition"
	packsource "github.com/wippyai/go-lua/domain/pack/source/memberdefinition"
	placementbase "github.com/wippyai/go-lua/domain/placement/memberdefinition"
	placementreturnescape "github.com/wippyai/go-lua/domain/placement/returnescape/memberdefinition"
	placementstore "github.com/wippyai/go-lua/domain/placement/store/memberdefinition"
	staticbase "github.com/wippyai/go-lua/domain/static/memberdefinition"
	statictransfer "github.com/wippyai/go-lua/domain/static/transfer/memberdefinition"
	valuearithmetic "github.com/wippyai/go-lua/domain/value/arithmetic/memberdefinition"
	valuebootstrap "github.com/wippyai/go-lua/domain/value/bootstrap/memberdefinition"
	valueequality "github.com/wippyai/go-lua/domain/value/equality/memberdefinition"
	valuebase "github.com/wippyai/go-lua/domain/value/memberdefinition"
	valuemoduleload "github.com/wippyai/go-lua/domain/value/moduleload/memberdefinition"
	valueorder "github.com/wippyai/go-lua/domain/value/order/memberdefinition"
	valuerefinement "github.com/wippyai/go-lua/domain/value/refinement/memberdefinition"
	valuesource "github.com/wippyai/go-lua/domain/value/source/memberdefinition"
	valuetransfer "github.com/wippyai/go-lua/domain/value/transfer/memberdefinition"
)

// Composition returns the roster. A roster that does not admit is a
// declaration defect and is reported as one rather than half applied.
func Composition() (definition.Roster, bool) {
	return definition.NewRoster(
		definition.Source{
			Package: "value",
			Name:    "value",
			Base:    valuebase.StorageTransfer(),
			Contributions: []definition.Contribution{
				valuetransfer.Contribution(),
				valuesource.Contribution(),
				valuebootstrap.Contribution(),
				valuearithmetic.Contribution(),
				valueequality.Contribution(),
				valuerefinement.Contribution(),
				valueorder.Contribution(),
				valuemoduleload.Contribution(),
			},
		},
		definition.Source{
			Package: "static",
			Name:    "static-type",
			Base:    staticbase.TypeFactTransfer(),
			Contributions: []definition.Contribution{
				statictransfer.Contribution(),
			},
		},
		definition.Source{
			Package: "pack",
			Name:    "pack",
			Base:    packbase.Source(),
			Contributions: []definition.Contribution{
				packsource.Contribution(),
			},
		},
		definition.Source{
			Package: "placement",
			Name:    "placement",
			Base:    placementbase.Storage(),
			Contributions: []definition.Contribution{
				placementstore.Contribution(),
				placementreturnescape.Contribution(),
			},
		},
		definition.Source{
			Package: "heap",
			Name:    "heap",
			Base:    heapbase.AllocationCarry(),
			Contributions: []definition.Contribution{
				heapingress.Contribution(),
				heapbootstrap.Contribution(),
				heapclosed.Contribution(),
				heapempty.Contribution(),
				heapfreeze.Contribution(),
			},
		},
		definition.Source{
			Package: "call",
			Name:    "call",
			Base:    callbase.MountedCall(),
			Contributions: []definition.Contribution{
				calldispatch.Contribution(),
			},
		},
		definition.Source{
			Package: "effect",
			Name:    "effect",
			Base:    effectbase.MountedCall(),
			Contributions: []definition.Contribution{
				effectcallsite.SelectedContribution(),
				effectcallsite.OpaqueContribution(),
				effectcallsite.BodyContribution(),
			},
		},
	)
}
