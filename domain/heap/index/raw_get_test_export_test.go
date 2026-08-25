package index

import (
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// RawGetSemanticSourceLookupFixture is an internal-test bridge for the
// external mounted-artifact law. It keeps the actual RawGetRule/sourceValue
// call in this package while the fixture obtains Pack's owner-issued sources
// through the public mounted schema APIs.
type RawGetSemanticSourceLookupFixture struct {
	rule    *RawGetRule
	payload rawPayload
	sources []pack.SemanticSource
	facts   []valuedomain.Value
	reads   int
	view    rawGetView
}

// NewRawGetSemanticSourceLookupFixture builds its reverse source directory
// with the production appendRawSource path. The caller supplies only sources,
// coordinates, and Value facts already issued by the mounted Pack/Value
// schemas; no legacy source or selection-index adapter is accepted.
func NewRawGetSemanticSourceLookupFixture(sources []pack.SemanticSource, coordinates []valuedomain.Coordinate, facts []valuedomain.Value) (*RawGetSemanticSourceLookupFixture, bool) {
	if len(sources) == 0 || len(sources) != len(coordinates) || len(sources) != len(facts) {
		return nil, false
	}
	all := make([]rawSource, 0, len(sources))
	refs := make([]RawSourceTag, 0, len(sources))
	tags := make(map[pack.SemanticSource]RawSourceTag, len(sources))
	reverse := make(map[rawPayloadSource]RawSourceTag, len(sources))
	payload := rawPayload{kind: rawPayloadTail}
	for index, source := range sources {
		if !source.Available() || !coordinates[index].Valid() {
			return nil, false
		}
		if !appendRawSource(&all, &refs, tags, reverse, &payload, heapdomain.RawPayloadTag(1), source, coordinates[index]) {
			return nil, false
		}
	}
	fixture := &RawGetSemanticSourceLookupFixture{
		rule:    &RawGetRule{runtime: &rawGetRuntime{topology: &Topology{catalog: &rawCatalog{payloads: []rawPayload{{}, payload}, sources: all, sourceRefs: refs, byPayloadSource: reverse}}}},
		payload: payload,
		sources: append([]pack.SemanticSource(nil), sources...),
		facts:   append([]valuedomain.Value(nil), facts...),
	}
	fixture.view.source = fixture.readSource
	return fixture, true
}

func (fixture *RawGetSemanticSourceLookupFixture) readSource(tag RawSourceTag) rawSelected[valuedomain.Value] {
	index := int(tag) - 1
	if fixture == nil || index < 0 || index >= len(fixture.facts) {
		return rawSelected[valuedomain.Value]{}
	}
	fixture.reads++
	return rawSelected[valuedomain.Value]{value: fixture.facts[index], present: true, found: true, valid: true}
}

// Lookup runs one complete source frontier through RawGetRule.sourceValue and
// returns the number of owner Value reads. It is intentionally reusable so
// warm allocation and linear-frontier laws can measure the same rule/catalog.
func (fixture *RawGetSemanticSourceLookupFixture) Lookup() (int, bool) {
	if fixture == nil || fixture.rule == nil || len(fixture.sources) != len(fixture.facts) || len(fixture.sources) == 0 {
		return 0, false
	}
	fixture.reads = 0
	for _, source := range fixture.sources {
		selected := fixture.rule.sourceValue(fixture.view, heapdomain.RawPayloadTag(1), source)
		if !selected.valid || !selected.found || !selected.present {
			return fixture.reads, false
		}
	}
	return fixture.reads, true
}
