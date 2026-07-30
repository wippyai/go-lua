package state

import (
	"fmt"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// TypestateResourceQuery is one sealed lifecycle observation. It is keyed by
// the exact receiver identity and protocol, so consumers cannot turn a
// one-resource question into authority over the complete typestate and path
// evidence lanes.
//
// The query is also the single semantic program for concrete and symbolic
// execution: both paths call ObserveFactors after supplying the two registered
// source factors. Symbolic compilation may factor this program's result, but
// must not reproduce its alias or resource-resolution law.
type TypestateResourceQuery struct {
	capability TypestateQueryCapability
	target     pathaddr.StateKey
	protocol   typestate.Protocol
}

// SealTypestateResourceQuery binds one exact resource observation to the
// registered composite typestate/path-equality capability.
func SealTypestateResourceQuery(
	domain ProductDomain,
	capability TypestateQueryCapability,
	target pathaddr.StateKey,
	protocol typestate.Protocol,
) (TypestateResourceQuery, error) {
	if !capability.ValidFor(domain) || target == "" || protocol == "" {
		return TypestateResourceQuery{}, fmt.Errorf("%w: invalid typestate resource query", ErrInvalidLaneFactor)
	}
	if _, ok := capability.keys.FromStateKey(target.PathKey()); !ok {
		return TypestateResourceQuery{}, fmt.Errorf("%w: unresolved typestate resource query target", ErrInvalidLaneFactor)
	}
	return TypestateResourceQuery{capability: capability, target: target, protocol: protocol}, nil
}

func (q TypestateResourceQuery) ValidFor(domain ProductDomain) bool {
	return q.capability.ValidFor(domain) && q.target != "" && q.protocol != ""
}

func (q TypestateResourceQuery) valid() bool {
	return q.capability.seal != nil && q.capability.keys != nil && q.capability.keys.Valid() &&
		q.target != "" && q.protocol != ""
}

// SourceLanes returns the registered source factors consumed by this query.
// They are compilation inputs, not raw provider authority.
func (q TypestateResourceQuery) SourceLanes() []ProductLane {
	if q.capability.seal == nil {
		return nil
	}
	return []ProductLane{q.capability.typestate, q.capability.path}
}

func (q TypestateResourceQuery) Target() pathaddr.StateKey    { return q.target }
func (q TypestateResourceQuery) Protocol() typestate.Protocol { return q.protocol }

func (q TypestateResourceQuery) Equal(other TypestateResourceQuery) bool {
	return q.capability.seal == other.capability.seal && q.capability.keys == other.capability.keys &&
		q.target == other.target && q.protocol == other.protocol
}

// Less supplies a deterministic site-capability order.
func (q TypestateResourceQuery) Less(other TypestateResourceQuery) bool {
	if q.target != other.target {
		return q.target < other.target
	}
	return q.protocol < other.protocol
}

// TypestateResourceObservation is the exact finite result of a keyed query.
// Resource identity is deliberately absent: the query already owns it and the
// provider observes only whether its slot exists and, if so, that slot.
type TypestateResourceObservation struct {
	query TypestateResourceQuery
	slot  typestate.Slot
	found bool
}

func (o TypestateResourceObservation) ValidFor(query TypestateResourceQuery) bool {
	return query.valid() && o.query.Equal(query)
}

func (o TypestateResourceObservation) Slot() (typestate.Slot, bool) {
	return o.slot, o.found
}

func (o TypestateResourceObservation) Equal(other TypestateResourceObservation) bool {
	return o.query.Equal(other.query) && o.slot == other.slot && o.found == other.found
}

func (o TypestateResourceObservation) Fingerprint() uint64 {
	hash := internal.FnvString("typestate-resource-observation")
	hash = internal.MixHash(hash, internal.FnvString(o.query.target.String()))
	hash = internal.MixHash(hash, internal.FnvString(string(o.query.protocol)))
	if o.found {
		hash = internal.MixHash(hash, 1)
		hash = internal.MixHash(hash, internal.FnvString(string(o.slot.Current)))
		hash = internal.MixHash(hash, internal.FnvString(string(o.slot.Obligation.Final)))
		hash = internal.MixHash(hash, internal.FnvString(string(o.slot.Obligation.Finals)))
		hash = internal.MixHash(hash, uint64(o.slot.Locality))
	}
	return hash
}

// ObserveFactors executes the canonical query over its registered source
// factors. This is the only implementation of alias-aware resource lookup.
func (q TypestateResourceQuery) ObserveFactors(
	domain ProductDomain,
	typestateFactor, pathFactor LaneFactor,
) (TypestateResourceObservation, error) {
	if !q.ValidFor(domain) {
		return TypestateResourceObservation{}, fmt.Errorf("%w: foreign typestate resource query", ErrInvalidLaneFactor)
	}
	_, slot, found, err := domain.CanonicalTypestateResourceFactor(
		q.capability, typestateFactor, pathFactor, q.target, q.protocol,
	)
	if err != nil {
		return TypestateResourceObservation{}, err
	}
	if !found {
		slot = typestate.Slot{}
	}
	return TypestateResourceObservation{query: q, slot: slot, found: found}, nil
}

// ObserveState is the concrete adapter for the same factor-native query.
func (q TypestateResourceQuery) ObserveState(domain ProductDomain, input State) (TypestateResourceObservation, error) {
	lanes := q.SourceLanes()
	factors, err := domain.DecomposeLanes(input, lanes)
	if err != nil || len(factors) != 2 {
		if err == nil {
			err = fmt.Errorf("%w: incomplete typestate resource query factors", ErrIncompleteLaneFactors)
		}
		return TypestateResourceObservation{}, err
	}
	return q.ObserveFactors(domain, factors[0], factors[1])
}
