package specimen

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// RouteFact is the typed payload one observed receiver route publishes.
type RouteFact struct {
	Kind indexdomain.RouteKind
	Role materialization.Role
}

// ReceiverRouteColumns are the owner column codecs the route expansion reads
// and publishes through.
type ReceiverRouteColumns struct {
	Receiver *relbindgen.Column[valuedomain.Value]
	Route    *relbindgen.Column[RouteFact]
}

// ReceiverRouteArgument is the decoded frame of one raw-get route observation.
type ReceiverRouteArgument struct {
	Receiver valuedomain.Value
}

// ReceiverRouteOperation is the owner's receiver-route observation. It holds
// the sealed topology, whose root support is frozen at seal, and it names each
// emitted row by the owner-issued content of that route's heap root. It cannot
// mint a row: a name outside the mounted denominator is refused by the
// denominator witness, never published.
type ReceiverRouteOperation struct {
	topology *indexdomain.Topology
}

// Evaluate expands one receiver into its rooted routes. The traversal is
// finite under the declared denominator: the emitter's capacity is the sealed
// signature's output bound, and the row that would exceed it refuses the whole
// invocation rather than truncating the expansion.
func (operation ReceiverRouteOperation) Evaluate(argument ReceiverRouteArgument, emitter *relbindgen.Emitter[RouteFact]) outcome.Code {
	admitted := true
	observed := operation.topology.VisitReceiver(argument.Receiver, nil, func(route indexdomain.Route) bool {
		key, role, rooted := route.Root()
		if !rooted {
			return true
		}
		content, named := key.ContentID()
		if !named || !emitter.PutAt(content, RouteFact{Kind: route.Kind(), Role: role}) {
			admitted = false
			return false
		}
		return true
	})
	if !observed || !admitted {
		return outcome.Refused
	}
	if emitter.Len() == 0 {
		return outcome.NoCandidate
	}
	return outcome.Produced
}

// BindReceiverRoutes admits the finite-expansion specimen: one scalar receiver
// in, denominator-bounded rooted routes out, each at the row the owner names.
func BindReceiverRoutes(operation signature.Signature, topology *indexdomain.Topology, columns ReceiverRouteColumns, refusal model.RefusalID) (binding.Factory, bool) {
	if topology == nil || !columns.Receiver.Available() || !columns.Route.Available() {
		return nil, false
	}
	return relbindgen.Bind(relbindgen.Spec[ReceiverRouteArgument, RouteFact]{
		Signature: operation,
		Decoder:   receiverRouteDecoder{receiver: columns.Receiver},
		Encoder:   receiverRouteEncoder{route: columns.Route},
		Operation: ReceiverRouteOperation{topology: topology},
		Address:   relbindgen.KeyedDestination,
		Refusal:   refusal,
	})
}

type receiverRouteDecoder struct {
	receiver *relbindgen.Column[valuedomain.Value]
}

func (decoder receiverRouteDecoder) Decode(inputs relbindgen.Inputs) (ReceiverRouteArgument, bool) {
	receiver, ok := relbindgen.ScalarAt(inputs, 0, decoder.receiver)
	if !ok {
		return ReceiverRouteArgument{}, false
	}
	return ReceiverRouteArgument{Receiver: receiver}, true
}

type receiverRouteEncoder struct {
	route *relbindgen.Column[RouteFact]
}

func (encoder receiverRouteEncoder) Encode(outputs relbindgen.Outputs, value RouteFact) bool {
	return relbindgen.PutColumn(outputs, 0, encoder.route, value)
}
