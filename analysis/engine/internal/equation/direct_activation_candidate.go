package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// DirectActivationTransport is one candidate-local expanded factor edge.
// It is issued only from a body-relative transport set plus an exact trigger.
type DirectActivationTransport struct {
	Source PointRef
	Target PointRef
	Factor composition.Key
}

// DirectActivationTransportSet is the reusable body-relative denominator.
// It intentionally has no caller trigger: entries/exits are shared across
// every call occurrence, while each candidate owns its exact trigger point.
type DirectActivationTransportSet struct {
	data *directActivationTransportSetData
}

type directActivationTransportSetData struct {
	source  *composition.Composition
	base    *Batch
	entries []PointRef
	exits   []PointRef
	imports []composition.Key
	effect  composition.Key
	key     composition.Key
}

// DirectActivationCandidate is a compact dynamic locator over one body set.
type DirectActivationCandidate struct {
	data *directActivationCandidateData
}

type directActivationCandidateData struct {
	source    *composition.Composition
	base      *Batch
	origin    MaterializationOrigin
	trigger   PointRef
	transport DirectActivationTransportSet
	key       composition.Key
}

func NewDirectActivationTransportSet(source *composition.Composition, base *Batch, entries, exits []PointRef, imports []composition.Key, effect composition.Key) (DirectActivationTransportSet, bool) {
	if source == nil || base == nil || !base.Sealed() || len(entries) == 0 || len(exits) == 0 || len(imports) != 4 || !effect.Available() {
		return DirectActivationTransportSet{}, false
	}
	entryRows, entryOK := canonicalDirectActivationRefs(entries)
	exitRows, exitOK := canonicalDirectActivationRefs(exits)
	importRows, importsOK := canonicalDirectActivationFactors(source, imports)
	if !entryOK || !exitOK || !importsOK {
		return DirectActivationTransportSet{}, false
	}
	if _, known := source.FactorIndex(effect); !known {
		return DirectActivationTransportSet{}, false
	}
	for _, factor := range importRows {
		if factor == effect {
			return DirectActivationTransportSet{}, false
		}
	}
	key, keyed := identityKey("analysis/engine/equation/direct-activation-transport-set", func(writer *canonical.DigestWriter) bool {
		if writer.Count(uint64(len(entryRows))) != nil {
			return false
		}
		for _, ref := range entryRows {
			if writer.Uint(uint64(ref)) != nil {
				return false
			}
		}
		if writer.Count(uint64(len(exitRows))) != nil {
			return false
		}
		for _, ref := range exitRows {
			if writer.Uint(uint64(ref)) != nil {
				return false
			}
		}
		if writer.Count(uint64(len(importRows))) != nil {
			return false
		}
		for _, factor := range importRows {
			if !writeKey(writer, factor) {
				return false
			}
		}
		return writeKey(writer, effect)
	})
	if !keyed {
		return DirectActivationTransportSet{}, false
	}
	return DirectActivationTransportSet{data: &directActivationTransportSetData{source: source, base: base, entries: entryRows, exits: exitRows, imports: importRows, effect: effect, key: key}}, true
}

func canonicalDirectActivationRefs(values []PointRef) ([]PointRef, bool) {
	result := append([]PointRef(nil), values...)
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	for index, ref := range result {
		if ref == 0 || index != 0 && ref == result[index-1] {
			return nil, false
		}
	}
	return result, true
}

func canonicalDirectActivationFactors(source *composition.Composition, values []composition.Key) ([]composition.Key, bool) {
	result := append([]composition.Key(nil), values...)
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left], result[right]) })
	for index, factor := range result {
		if !factor.Available() || index != 0 && factor == result[index-1] {
			return nil, false
		}
		if _, known := source.FactorIndex(factor); !known {
			return nil, false
		}
	}
	return result, true
}

func (value DirectActivationTransportSet) Available() bool {
	return value.data != nil && value.data.source != nil && value.data.base != nil && value.data.base.Sealed() && value.data.key.Available() && len(value.data.entries) != 0 && len(value.data.exits) != 0 && len(value.data.imports) == 4 && value.data.effect.Available()
}
func (value DirectActivationTransportSet) OwnedBy(source *composition.Composition, base *Batch) bool {
	return value.Available() && value.data.source == source && value.data.base == base
}
func (value DirectActivationTransportSet) Key() composition.Key {
	if !value.Available() {
		return composition.Key{}
	}
	return value.data.key
}

func NewDirectActivationCandidate(source *composition.Composition, base *Batch, origin MaterializationOrigin, trigger PointRef, transport DirectActivationTransportSet) (DirectActivationCandidate, bool) {
	if source == nil || base == nil || !directActivationOriginAvailable(origin) || trigger == 0 || !transport.OwnedBy(source, base) {
		return DirectActivationCandidate{}, false
	}
	key, keyed := identityKey("analysis/engine/equation/direct-activation-candidate", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, transport.Key()) && writer.Uint(uint64(trigger)) == nil && writeKey(writer, origin.Family) && writeKey(writer, origin.Application) && writeKey(writer, origin.Target) && writeKey(writer, origin.Endpoint) && writer.Uint(uint64(origin.TriggerOrdinal)) == nil
	})
	if !keyed {
		return DirectActivationCandidate{}, false
	}
	return DirectActivationCandidate{data: &directActivationCandidateData{source: source, base: base, origin: origin, trigger: trigger, transport: transport, key: key}}, true
}

func directActivationOriginAvailable(origin MaterializationOrigin) bool {
	return origin.Family.Available() && origin.Application.Available() && origin.Target.Available() && origin.Endpoint.Available() && origin.TriggerOrdinal >= 0
}
func (value DirectActivationCandidate) Available() bool {
	return value.data != nil && value.data.source != nil && value.data.base != nil && value.data.key.Available() && value.data.trigger != 0 && directActivationOriginAvailable(value.data.origin) && value.data.transport.OwnedBy(value.data.source, value.data.base)
}
func (value DirectActivationCandidate) OwnedBy(source *composition.Composition, base *Batch) bool {
	return value.Available() && value.data.source == source && value.data.base == base
}
func (value DirectActivationCandidate) Same(other DirectActivationCandidate) bool {
	return value.data != nil && value.data == other.data
}
func (value DirectActivationCandidate) Key() composition.Key {
	if !value.Available() {
		return composition.Key{}
	}
	return value.data.key
}
func (value DirectActivationCandidate) Origin() (MaterializationOrigin, bool) {
	if !value.Available() {
		return MaterializationOrigin{}, false
	}
	return value.data.origin, true
}
func (value DirectActivationCandidate) Trigger() (PointRef, bool) {
	if !value.Available() {
		return 0, false
	}
	return value.data.trigger, true
}
func (value DirectActivationCandidate) Transport() (DirectActivationTransportSet, bool) {
	if !value.Available() {
		return DirectActivationTransportSet{}, false
	}
	return value.data.transport, true
}
func (value DirectActivationCandidate) TransportCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.data.transport.data.entries)*len(value.data.transport.data.imports) + len(value.data.transport.data.exits)
}
func (value DirectActivationCandidate) TransportAt(index int) (DirectActivationTransport, bool) {
	if !value.Available() || index < 0 || index >= value.TransportCount() {
		return DirectActivationTransport{}, false
	}
	set := value.data.transport.data
	imports := len(set.entries) * len(set.imports)
	if index < imports {
		entry := index / len(set.imports)
		factor := index % len(set.imports)
		return DirectActivationTransport{Source: value.data.trigger, Target: set.entries[entry], Factor: set.imports[factor]}, true
	}
	return DirectActivationTransport{Source: set.exits[index-imports], Target: value.data.trigger, Factor: set.effect}, true
}
