package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// IndexWriteAdmissionFunc reads one normalized index-write admission query.
type IndexWriteAdmissionFunc func(IndexWriteReadQuery) (typ.Type, bool)

// IndexWriteKeyAliases returns stable key-path aliases at a point.
type IndexWriteKeyAliases func(cfg.Point, StableAddress) []StableAddress

// IndexWriteAdmissionWithKeyAliases applies the index-write admission rule plus
// the canonical key-alias fallback. Callers supply how to read admissions and how
// to enumerate aliases; flow owns the retry semantics and query rewriting.
func IndexWriteAdmissionWithKeyAliases(
	q IndexWriteReadQuery,
	admit IndexWriteAdmissionFunc,
	keyAliases IndexWriteKeyAliases,
) (typ.Type, bool) {
	if admit == nil {
		return nil, false
	}
	if got, ok := admit(q); ok {
		return got, true
	}
	if !q.Admission.HasKeyPath || keyAliases == nil {
		return nil, false
	}
	for _, keyAddr := range keyAliases(q.Point, q.Admission.KeyPath) {
		if keyAddr.Equal(q.Admission.KeyPath) {
			continue
		}
		aliasQuery := q
		aliasQuery.Admission.KeyPath = keyAddr
		aliasQuery.Admission.HasKeyPath = true
		if got, ok := admit(aliasQuery); ok {
			return got, true
		}
	}
	return nil, false
}
