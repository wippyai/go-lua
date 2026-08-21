package source

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// Version 4 adds Source-owned authored debug spellings after the canonical
// Key/fault rows. Cell spellings are dense; named Call spellings are sparse.
const contentVersion = 4

// contentRecordSpellings is part of the canonical authored ContentID stream.
// The retired artifact codec used to own the same numeric record tag; keeping
// the value here preserves the existing authored identity bytes without
// retaining a second codec representation.
const contentRecordSpellings uint64 = 6

// authoredContentID hashes only Source's owned authored rows. Position/root
// indexes are Seal projections and deliberately contribute no second identity.
func authoredContentID(a *authority) (id identity.ContentID) {
	if a == nil || a.identity.name == "" {
		return identity.ContentID{}
	}
	h := sha256.New()
	var w framing.Writer
	if w.Reset(h, "program/source", contentVersion) != nil ||
		writeAuthoredPayload(&w, a) != nil || w.Finish() != nil {
		return identity.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return identity.ContentID{}
	}
	return id
}

// writeAuthoredPayload is the one canonical Source-authored row writer for
// ContentID. The committed Outcome family, and every Source seal projection,
// are deliberately absent. Outcome is assigned only by Flow during Source
// Commit and therefore does not enter authored identity.
func writeAuthoredPayload(w *framing.Writer, a *authority) error {
	if w == nil || a == nil || a.identity.name == "" {
		return framing.ErrMalformed
	}
	if err := w.Record(1); err != nil {
		return err
	}
	if err := w.String(a.identity.name); err != nil {
		return err
	}
	termCount, ok := authoredTermCount(&a.identity)
	if !ok || termCount == 0 {
		return framing.ErrMalformed
	}
	if err := w.Count(uint64(termCount)); err != nil {
		return err
	}
	if !contentSpans(w, &a.identity) || !contentLiterals(w, &a.literals) ||
		!contentOrder(w, a) || !contentKeyFault(w, a) || !contentSpellings(w, &a.spellings) {
		return framing.ErrMalformed
	}
	return nil
}

func contentSpellings(w *framing.Writer, store *spellingStore) bool {
	if w == nil || store == nil || w.Record(contentRecordSpellings) != nil ||
		w.Count(uint64(len(store.cells))) != nil {
		return false
	}
	for index, name := range store.cells {
		if w.Uint(uint64(keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1)))) != nil || w.String(name) != nil {
			return false
		}
	}
	if w.Count(uint64(len(store.calls))) != nil {
		return false
	}
	for _, row := range store.calls {
		if row.Call == 0 || row.Name == "" || w.Uint(uint64(row.Call)) != nil || w.String(row.Name) != nil {
			return false
		}
	}
	return true
}

func contentKeyFault(w *framing.Writer, a *authority) bool {
	if w == nil || a == nil || w.Record(5) != nil ||
		w.Count(uint64(len(a.keys.exact.atoms))) != nil ||
		w.Count(uint64(len(a.keys.keys))) != nil ||
		w.Count(uint64(len(a.keys.faults))) != nil {
		return false
	}
	// Exact storage itself is in canonical order, making dense Key handles
	// stable across every child component and artifact encoder.
	for _, value := range a.keys.exact.atoms {
		if !contentExactValue(w, value) {
			return false
		}
	}
	for _, row := range a.keys.keys {
		_, ok := exactValue(a, row.exact)
		if row.owner == 0 || !ok || row.exact == 0 ||
			uint64(row.exact) > uint64(len(a.keys.exact.atoms)) ||
			w.Uint(uint64(row.owner)) != nil || w.Uint(uint64(row.form)) != nil ||
			w.Uint(uint64(row.exact)) != nil {
			return false
		}
	}
	for _, row := range a.keys.faults {
		if row.Owner == 0 || w.Uint(uint64(row.Owner)) != nil ||
			w.Uint(uint64(row.Kind)) != nil || w.Uint(uint64(row.Label)) != nil ||
			w.Uint(uint64(row.Blocker)) != nil {
			return false
		}
	}
	return true
}

func contentExactValue(w *framing.Writer, value keyspace.LiteralValue) bool {
	if w == nil || w.Uint(uint64(value.Kind)) != nil {
		return false
	}
	switch value.Kind {
	case keyspace.LiteralBool:
		return w.Bool(value.Bool) == nil
	case keyspace.LiteralInteger:
		return w.Uint(uint64(value.Integer)) == nil
	case keyspace.LiteralFloat:
		return w.Uint(value.FloatBits) == nil
	case keyspace.LiteralString:
		return w.String(value.String) == nil
	default:
		return false
	}
}

func contentSpans(w *framing.Writer, identity *identityStore) bool {
	if w == nil || identity == nil || w.Record(2) != nil {
		return false
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		spans := identity.spans[family]
		if w.Uint(uint64(family)) != nil || w.Count(uint64(len(spans))) != nil {
			return false
		}
		for _, span := range spans {
			if w.Uint(uint64(span.startLine)) != nil || w.Uint(uint64(span.startCol)) != nil ||
				w.Uint(uint64(span.endLine)) != nil || w.Uint(uint64(span.endCol)) != nil {
				return false
			}
		}
	}
	return true
}

func contentLiterals(w *framing.Writer, store *literalStore) bool {
	if w == nil || store == nil || w.Record(3) != nil ||
		!contentNil(w, store.nil) || !contentBool(w, store.bool) ||
		!contentInteger(w, store.integer) || !contentFloat(w, store.float) || !contentString(w, store.string) {
		return false
	}
	return true
}

func contentNil(w *framing.Writer, rows []NilLiteral) bool {
	if w.Uint(uint64(keyspace.FamilyNil)) != nil || w.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if row.Owner == 0 || w.Uint(uint64(row.Owner)) != nil {
			return false
		}
	}
	return true
}

func contentBool(w *framing.Writer, rows []BoolLiteral) bool {
	if w.Uint(uint64(keyspace.FamilyBool)) != nil || w.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if row.Owner == 0 || w.Uint(uint64(row.Owner)) != nil || w.Bool(row.Value) != nil {
			return false
		}
	}
	return true
}

func contentInteger(w *framing.Writer, rows []IntegerLiteral) bool {
	if w.Uint(uint64(keyspace.FamilyInteger)) != nil || w.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if row.Owner == 0 || w.Uint(uint64(row.Owner)) != nil || w.Uint(uint64(row.Value)) != nil {
			return false
		}
	}
	return true
}

func contentFloat(w *framing.Writer, rows []FloatLiteral) bool {
	if w.Uint(uint64(keyspace.FamilyFloat)) != nil || w.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if row.Owner == 0 || w.Uint(uint64(row.Owner)) != nil || w.Uint(row.Bits) != nil {
			return false
		}
	}
	return true
}

func contentString(w *framing.Writer, rows []StringLiteral) bool {
	if w.Uint(uint64(keyspace.FamilyString)) != nil || w.Count(uint64(len(rows))) != nil {
		return false
	}
	for _, row := range rows {
		if row.Owner == 0 || w.Uint(uint64(row.Owner)) != nil || w.String(row.Value) != nil {
			return false
		}
	}
	return true
}

func contentOrder(w *framing.Writer, a *authority) bool {
	if w == nil || a == nil || w.Record(4) != nil ||
		!contentRanges(w, keyspace.FamilyBody, a.order.sourceTerms, a.order.bodyRanges) ||
		!contentRanges(w, keyspace.FamilyBind, a.order.bindTerms, a.order.bindRanges) ||
		!contentRanges(w, keyspace.FamilyFunction, a.order.formalTerms, a.order.formalRanges) {
		return false
	}
	return true
}

func contentRanges(w *framing.Writer, family keyspace.Family, pool []keyspace.Term, ranges []termRange) bool {
	if w.Uint(uint64(family)) != nil || w.Count(uint64(len(ranges))) != nil {
		return false
	}
	for _, r := range ranges {
		if !validRange(pool, r) || w.Count(uint64(r.end-r.start)) != nil {
			return false
		}
		for _, term := range pool[r.start:r.end] {
			if term == 0 || w.Uint(uint64(term)) != nil {
				return false
			}
		}
	}
	return true
}

func validRange(pool []keyspace.Term, r termRange) bool {
	return r.start <= r.end && uint64(r.end) <= uint64(len(pool))
}
