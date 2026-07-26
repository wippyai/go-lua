package equation

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

// FamilyValue is one family-scoped publication. Subject and qualifiers are
// parsed zero-copy views of the key; Payload aliases the immutable fact value.
type FamilyValue struct {
	Subject    factkey.SubjectRef
	Occurrence string
	Payload    []byte
	parsed     factkey.ParsedKey
}

func (v FamilyValue) Qualifier(index int) (factkey.SubjectRef, bool) {
	return v.parsed.Qualifier(index)
}

func (v FamilyValue) QualifierCount() int { return v.parsed.QualifierCount() }

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
			Subject:    parsed.Subject,
			Occurrence: parsed.Occurrence,
			Payload:    fact.Value,
			parsed:     parsed,
		}, true
	}
	return FamilyValue{}, false
}
