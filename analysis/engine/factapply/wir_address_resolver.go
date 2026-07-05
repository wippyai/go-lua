package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// WIRAddressResolver is the production bridge from WIR source operands to the
// point-local state keys used by fact application. WIR owns operand identity;
// visibility owns state-cell identity.
type WIRAddressResolver struct {
	body       *wir.Body
	visibility *visibility.Resolver
}

// NewWIRAddressResolver returns a resolver for one WIR body and visibility
// table. It intentionally returns the WIR interface so transfer code depends on
// the closed access-mode contract, not this adapter's storage.
func NewWIRAddressResolver(body *wir.Body, resolver *visibility.Resolver) wir.AddressResolver {
	return WIRAddressResolver{body: body, visibility: resolver}
}

func (r WIRAddressResolver) Resolve(point cfg.Point, op wir.Operand, mode wir.AccessMode) (keyspace.Key, bool) {
	if r.body == nil || r.visibility == nil || op.Kind != wir.OperandPath {
		return keyspace.Key{}, false
	}
	p := r.body.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() {
		return keyspace.Key{}, false
	}
	address := visibility.AddressAt(r.visibility, point, p)
	switch mode {
	case wir.AccessReadBefore:
		return address.VisibleKeyspaceKey()
	case wir.AccessWriteLocal:
		return address.VisibleLocalKeyspaceKey()
	case wir.AccessRootOrVisible:
		return address.RootOrVisibleKeyspaceKey()
	case wir.AccessEvidence:
		return address.VisibleKeyspaceKey()
	default:
		return keyspace.Key{}, false
	}
}
