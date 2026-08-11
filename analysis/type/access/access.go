// Package access provides Lua table read access projections.
package access

import "github.com/wippyai/go-lua/analysis/type/typ"

// Field resolves a dot-field projection against a type.
func Field(t typ.Type, name string) (typ.Type, bool) {
	return newQuery().resolveField(t, name).materialize()
}

// MissingFieldReadsNil reports whether a missing field read on t has defined
// Lua table semantics and produces nil instead of an indexing error.
func MissingFieldReadsNil(t typ.Type) bool {
	return newQuery().resolveMissing(t)
}

type query struct {
	inline   [8]queryKey
	inlineN  uint8
	overflow map[queryKey]struct{}
}

type queryKey struct {
	op   uint8
	t    typ.Type
	key  typ.Type
	name string
	mode indexMode
}

func newQuery() *query { return &query{} }

func (q *query) enter(key queryKey) bool {
	for i := range q.inlineN {
		if q.inline[i] == key {
			return false
		}
	}
	if _, ok := q.overflow[key]; ok {
		return false
	}
	if q.inlineN < uint8(len(q.inline)) {
		q.inline[q.inlineN] = key
		q.inlineN++
		return true
	}
	if q.overflow == nil {
		q.overflow = make(map[queryKey]struct{})
	}
	q.overflow[key] = struct{}{}
	return true
}

func (q *query) leave(key queryKey) {
	if _, ok := q.overflow[key]; ok {
		delete(q.overflow, key)
		return
	}
	for i := int(q.inlineN) - 1; i >= 0; i-- {
		if q.inline[i] != key {
			continue
		}
		last := int(q.inlineN) - 1
		q.inline[i] = q.inline[last]
		q.inline[last] = queryKey{}
		q.inlineN--
		return
	}
}
