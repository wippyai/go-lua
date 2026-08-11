package source

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Version 3 changes Key rows from repeated LiteralValue payloads to the
// canonical dense exact-Key ordinal. The ordinal is already Source-owned and
// the exact atom denominator is emitted immediately before all Key rows.
const contentVersion = 3

// authoredContentID hashes only Source's owned authored rows. Position/root
// indexes are Seal projections and deliberately contribute no second identity.
func authoredContentID(a *authority) (id keyspace.ContentID) {
	if a == nil || a.identity.name == "" || a.identity.termCount == 0 {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	var w canonical.Writer
	if w.Reset(h, "program/source", contentVersion) != nil ||
		writeAuthoredPayload(&w, a) != nil || w.Finish() != nil {
		return keyspace.ContentID{}
	}
	if sum := h.Sum(id[:0]); len(sum) != len(id) {
		return keyspace.ContentID{}
	}
	return id
}

// writeAuthoredPayload is the one canonical Source-authored row writer. It is
// shared by ContentID and artifact persistence so a child cannot accidentally
// acquire two subtly different row orders or payload encodings. The committed
// Outcome family, and every Source seal projection, are deliberately absent.
// Outcome is assigned only by Flow during Source Commit; its count therefore
// must not enter either the authored identity or the portable Source section.
func writeAuthoredPayload(w *canonical.Writer, a *authority) error {
	if w == nil || a == nil || a.identity.name == "" {
		return canonical.ErrMalformed
	}
	if err := w.Record(1); err != nil {
		return err
	}
	if err := w.String(a.identity.name); err != nil {
		return err
	}
	termCount, ok := authoredTermCount(a)
	if !ok || termCount == 0 {
		return canonical.ErrMalformed
	}
	if err := w.Count(uint64(termCount)); err != nil {
		return err
	}
	if !contentSpans(w, &a.identity) || !contentLiterals(w, &a.literals) ||
		!contentOrder(w, a) || !contentKeyFault(w, a) {
		return canonical.ErrMalformed
	}
	return nil
}

// authoredTermCount excludes the derived Outcome family from the stored
// denominator. The identity store's termCount includes Outcomes after Commit,
// so recomputing this value is what keeps post-Commit artifacts authored-only.
func authoredTermCount(a *authority) (uint32, bool) {
	if a == nil {
		return 0, false
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		total += uint64(a.identity.counts[family])
	}
	if total == 0 || total > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(total), true
}

func contentKeyFault(w *canonical.Writer, a *authority) bool {
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

func contentExactValue(w *canonical.Writer, value keyspace.LiteralValue) bool {
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

func contentSpans(w *canonical.Writer, identity *identityStore) bool {
	if w == nil || identity == nil || w.Record(2) != nil {
		return false
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		spans := identity.spans[family]
		if uint32(len(spans)) != identity.counts[family] || w.Uint(uint64(family)) != nil ||
			w.Count(uint64(len(spans))) != nil {
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

func contentLiterals(w *canonical.Writer, store *literalStore) bool {
	if w == nil || store == nil || w.Record(3) != nil ||
		!contentNil(w, store.nil) || !contentBool(w, store.bool) ||
		!contentInteger(w, store.integer) || !contentFloat(w, store.float) || !contentString(w, store.string) {
		return false
	}
	return true
}

func contentNil(w *canonical.Writer, rows []NilLiteral) bool {
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

func contentBool(w *canonical.Writer, rows []BoolLiteral) bool {
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

func contentInteger(w *canonical.Writer, rows []IntegerLiteral) bool {
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

func contentFloat(w *canonical.Writer, rows []FloatLiteral) bool {
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

func contentString(w *canonical.Writer, rows []StringLiteral) bool {
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

func contentOrder(w *canonical.Writer, a *authority) bool {
	if w == nil || a == nil || w.Record(4) != nil ||
		!contentRanges(w, keyspace.FamilyBody, a.order.sourceTerms, a.order.bodyRanges) ||
		!contentRanges(w, keyspace.FamilyBind, a.order.bindTerms, a.order.bindRanges) ||
		!contentRanges(w, keyspace.FamilyFunction, a.order.formalTerms, a.order.formalRanges) {
		return false
	}
	return true
}

func contentRanges(w *canonical.Writer, family keyspace.Family, pool []keyspace.Term, ranges []termRange) bool {
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
