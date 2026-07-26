package equation

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

// FamilyValue is one family-scoped publication. Subject and qualifiers are
// parsed zero-copy views of the key; Payload aliases the immutable fact value.
type FamilyValue struct {
	Subject     factkey.SubjectRef
	Occurrence  string
	Payload     []byte
	payloadKind factkey.PayloadKind
	parsed      factkey.ParsedKey
	guarded     bool
}

func (v FamilyValue) Qualifier(index int) (factkey.SubjectRef, bool) {
	return v.parsed.Qualifier(index)
}

func (v FamilyValue) QualifierCount() int { return v.parsed.QualifierCount() }

// Guarded reports whether the publication belongs to a branch edge. It lets a
// family consumer preserve the publication's control-flow provenance without
// exposing or re-parsing the guard encoding.
func (v FamilyValue) Guarded() bool { return v.guarded }

// DecodedPayload opts a value-payload family into the shared codec. Families
// whose declarations name another payload kind fail closed; their bytes must
// be interpreted by the codec named in factkey.Family.PayloadKind.
func (v FamilyValue) DecodedPayload() (shapefact.Payload, bool) {
	if v.payloadKind != factkey.PayloadValue {
		return shapefact.Payload{}, false
	}
	return shapefact.Decode(v.Payload)
}

// DecodedTypestatePublication opts a typestate-payload family into the domain
// codec and verifies that the typed resource repeats the key's exact identity.
// A family-kind mismatch, malformed payload, or key/payload identity mismatch
// fails closed.
func (v FamilyValue) DecodedTypestatePublication() (typestate.Publication, bool) {
	if v.payloadKind != factkey.PayloadTypestate {
		return typestate.Publication{}, false
	}
	publication, ok := typestate.DecodePublication(v.Payload)
	if !ok {
		return typestate.Publication{}, false
	}
	identity, ok := v.Subject.Decode(nil)
	if !ok || publication.Resource.ID != typestate.ResourceID(string(identity)) {
		return typestate.Publication{}, false
	}
	return publication, true
}

// FamilyValueIterator walks one binary-searched prefix range without
// materializing a fact slice or splitting keys. It is a value iterator so
// constructing and advancing it does not allocate.
type FamilyValueIterator struct {
	family factkey.Family
	prefix string
	lane   []Fact
	active []Guard
	index  int
	all    bool
}

// FamilyValues returns the visible publications selected by a typed family
// prefix built with factkey.BuildKey.
func (p Partition) FamilyValues(prefix factkey.Key) FamilyValueIterator {
	return p.familyValues(prefix, false)
}

// AllFamilyValues is the unfiltered-history counterpart used by may-fact
// families whose guarded publication remains possible after reconvergence.
func (p Partition) AllFamilyValues(prefix factkey.Key) FamilyValueIterator {
	return p.familyValues(prefix, true)
}

func (p Partition) familyValues(prefix factkey.Key, all bool) FamilyValueIterator {
	family, ok := prefix.Family()
	if !ok {
		return FamilyValueIterator{}
	}
	text := prefix.String()
	view := p.view()
	lane := p.closure.Values
	if view.orderedValues() {
		start, end := prefixRange(lane, text)
		lane = lane[start:end:end]
	}
	var active []Guard
	if !all {
		active = view.activeGuards()
	}
	return FamilyValueIterator{
		family: family,
		prefix: text,
		lane:   lane,
		active: active,
		all:    all,
	}
}

// Next returns the next structurally valid row. Malformed keys fail closed and
// are skipped rather than being exposed as a different subject.
func (it *FamilyValueIterator) Next() (FamilyValue, bool) {
	for it.index < len(it.lane) {
		fact := it.lane[it.index]
		it.index++
		if !strings.HasPrefix(fact.Key, it.prefix) || (!it.all && !guardsIncluded(fact.Guards, it.active)) {
			continue
		}
		parsed, ok := it.family.ParseKey(fact.Key)
		if !ok {
			continue
		}
		return FamilyValue{
			Subject:     parsed.Subject,
			Occurrence:  parsed.Occurrence,
			Payload:     fact.Value,
			payloadKind: it.family.PayloadKind,
			parsed:      parsed,
			guarded:     len(fact.Guards) != 0,
		}, true
	}
	return FamilyValue{}, false
}
