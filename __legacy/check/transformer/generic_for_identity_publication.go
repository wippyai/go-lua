package transformer

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
)

// genericForProjectionIdentityClass records whether the value synthesized by
// the canonical generic-for projection can itself publish a finite singleton
// identity. IteratorVariableValue and GenericForProtocolResult materialize a
// type-derived value, so today their projection has no finite identity. The
// separately recorded sources are the exact no-write/carry alternatives owned
// by the same canonical term constructor.
type genericForProjectionIdentityClass uint8

const (
	genericForProjectionIdentityInvalid genericForProjectionIdentityClass = iota
	genericForProjectionIdentityNoFinite
)

// frozenGenericForIdentityPublication is the complete identity-support
// publication for one canonical generic-for binding. It is compiled beside
// the executable Projection and copied into reduced relation code. Consumers
// must use finiteSources directly; reparsing Projection would create a second
// implementation of generic-for semantics.
type frozenGenericForIdentityPublication struct {
	target             statekey.Value
	projection         ValueTerm
	finiteSources      []ValueTerm
	projectionIdentity genericForProjectionIdentityClass
	sealed             bool
}

func sealGenericForIdentityPublication(arena *Arena, target statekey.Value, projection ValueTerm) (frozenGenericForIdentityPublication, error) {
	if arena == nil || target == 0 || !arena.validEnvironmentSlot(target) || projection == 0 || int(projection) >= len(arena.values) {
		return frozenGenericForIdentityPublication{}, fmt.Errorf("generic-for identity publication is unowned")
	}
	var finiteSources []ValueTerm
	node := arena.values[projection]
	switch node.op {
	case valueIteratorProjection:
		if len(node.args) < 1 || len(node.args) > 2 {
			return frozenGenericForIdentityPublication{}, fmt.Errorf("generic-for iterator identity projection is malformed")
		}
		if len(node.args) == 2 {
			finiteSources = append(finiteSources, node.args[1])
		}
	case valueGenericForResult:
		if len(node.args) != 4 {
			return frozenGenericForIdentityPublication{}, fmt.Errorf("generic-for protocol identity projection is malformed")
		}
		finiteSources = append(finiteSources, node.args[3])
	default:
		return frozenGenericForIdentityPublication{}, fmt.Errorf("generic-for identity publication has a noncanonical projection")
	}
	sources := make([]ValueTerm, len(finiteSources))
	for index, source := range finiteSources {
		if source == 0 || int(source) >= len(arena.values) {
			return frozenGenericForIdentityPublication{}, fmt.Errorf("generic-for identity publication has an unowned finite source")
		}
		sources[index] = source
	}
	return frozenGenericForIdentityPublication{
		target: target, projection: projection, finiteSources: sources,
		projectionIdentity: genericForProjectionIdentityNoFinite, sealed: true,
	}, nil
}

func (p frozenGenericForIdentityPublication) clone() frozenGenericForIdentityPublication {
	p.finiteSources = append([]ValueTerm(nil), p.finiteSources...)
	return p
}

func (p frozenGenericForIdentityPublication) valid(arena *Arena, shape Shape) bool {
	if arena == nil || !p.sealed || p.target == 0 || !arena.validEnvironmentSlot(p.target) ||
		p.projection == 0 || p.projectionIdentity != genericForProjectionIdentityNoFinite ||
		!arena.validValue(p.projection, shape, make(map[ValueTerm]bool)) {
		return false
	}
	for _, source := range p.finiteSources {
		if source == 0 || !arena.validValue(source, shape, make(map[ValueTerm]bool)) {
			return false
		}
	}
	return true
}
