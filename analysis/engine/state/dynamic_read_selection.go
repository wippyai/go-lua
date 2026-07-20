package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// DynamicReadSelection is the immutable source-selection certificate shared
// by the executable binder and static producer support. It owns address
// validation, canonical path/heap aliases, and dynamic-fact relevance; those
// consumers may project this certificate but may not restate its predicates.
type DynamicReadSelection struct {
	seal        *productDomainSeal
	keys        *keyspace.KeySpace
	tables      map[keyspace.Key]struct{}
	pathMembers []keyspace.Key
	heapMembers []keyspace.Key
	keySegment  segment.Segment
	exactKey    bool
	query       DynamicReadQuery
	membership  DynamicReadMembershipMode
}

// DynamicReadMembershipMode records whether the canonical binder can ever
// prove the query's key/table membership pair. Conditional mode must remain a
// row-level decision; freeze-time flat identity support may not guess it.
type DynamicReadMembershipMode uint8

const (
	DynamicReadMembershipImpossible DynamicReadMembershipMode = iota + 1
	DynamicReadMembershipConditional
)

func (d ProductDomain) PrepareDynamicReadSelection(query DynamicReadQuery) (DynamicReadSelection, error) {
	if !d.Valid() || query.KeySpace == nil || !query.KeySpace.Valid() ||
		!product.BelongsToRegistry(d.reg, query.TableValue) || !product.BelongsToRegistry(d.reg, query.KeyValue) {
		return DynamicReadSelection{}, fmt.Errorf("%w: invalid dynamic-read selection", ErrInvalidLaneFactor)
	}
	out := DynamicReadSelection{seal: d.seal, keys: query.KeySpace, tables: make(map[keyspace.Key]struct{}, len(query.TableKeys)), query: query}
	if len(query.KeyKeys) != 0 && len(query.TableKeys) != 0 {
		out.membership = DynamicReadMembershipConditional
	} else {
		out.membership = DynamicReadMembershipImpossible
	}
	for index, raw := range query.TableKeys {
		if raw == "" {
			return DynamicReadSelection{}, fmt.Errorf("%w: empty dynamic-read table key %d", ErrInvalidLaneFactor, index)
		}
		table, ok := query.KeySpace.InternStateKey(raw)
		if !ok {
			return DynamicReadSelection{}, fmt.Errorf("%w: foreign dynamic-read table key %d", ErrInvalidLaneFactor, index)
		}
		out.tables[table] = struct{}{}
	}
	out.keySegment, out.exactKey = typevalue.ExactScalarKeySegment(d.reg, query.TypeValues, query.KeyValue)
	if out.exactKey && query.TablePath.Kind != keyspace.KindInvalid {
		member, ok := query.KeySpace.AppendSegment(query.TablePath, out.keySegment)
		if !ok {
			return DynamicReadSelection{}, fmt.Errorf("%w: dynamic-read path member address", ErrInvalidLaneFactor)
		}
		out.pathMembers = dynamicReadCanonicalPaths(query.KeySpace, member)
	}
	if out.exactKey {
		suffix, ok := query.KeySpace.FromRootlessSuffix([]segment.Segment{out.keySegment})
		if !ok {
			return DynamicReadSelection{}, fmt.Errorf("%w: dynamic-read heap member address", ErrInvalidLaneFactor)
		}
		out.heapMembers = append(out.heapMembers, suffix)
		if canonical, ok := heapidentity.FieldCanonicalStaticMemberSuffixKey(query.KeySpace, []segment.Segment{out.keySegment}); ok && canonical != suffix {
			out.heapMembers = append(out.heapMembers, canonical)
		}
	}
	return out, nil
}

func (s DynamicReadSelection) MembershipMode() DynamicReadMembershipMode { return s.membership }

func (s DynamicReadSelection) validFor(d ProductDomain) bool {
	return d.Valid() && s.seal == d.seal && s.keys != nil && s.keys.Valid() && s.tables != nil
}

func (s DynamicReadSelection) PathMembers() []keyspace.Key {
	return append([]keyspace.Key(nil), s.pathMembers...)
}

func (s DynamicReadSelection) HeapMembers(available []keyspace.Key) []keyspace.Key {
	if s.exactKey {
		out := make([]keyspace.Key, 0, len(s.heapMembers))
		for _, candidate := range s.heapMembers {
			if sortedHeapKeyContains(s.keys, available, candidate) {
				out = append(out, candidate)
			}
		}
		return out
	}
	return append([]keyspace.Key(nil), available...)
}

func (s DynamicReadSelection) SelectsTable(table keyspace.Key) bool {
	_, selected := s.tables[table]
	return selected
}

func (s DynamicReadSelection) FactRelevant(d ProductDomain, membershipProven bool, factKey dynamicindex.Key, fact dynamicindex.Fact) bool {
	if !s.validFor(d) || !s.SelectsTable(factKey.Table) || fact.Admission == dynamicindex.AdmissionRejected ||
		product.Equal(d.reg, fact.Value, product.Bottom(d.reg)) {
		return false
	}
	if membershipProven {
		return true
	}
	factSegment, exact := typevalue.ExactScalarKeySegment(d.reg, s.query.TypeValues, fact.KeyValue)
	return s.exactKey && exact && factSegment == s.keySegment &&
		presence.Equal(product.PresenceOf(fact.Value), presence.Present()) && !typevalue.HasOnlyNilType(d.reg, fact.Value)
}

func (s DynamicReadSelection) ExactKeySegment() (segment.Segment, bool) {
	return s.keySegment, s.exactKey
}
