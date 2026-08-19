// Package executable seals Flow's pre-Outcome runtime membership.
//
// The result is a Source/Flow/Static/Module-identity-bound, dense per-family bitset. Source
// control supplies the reachable direct roots; containment supplies static
// classification and the complete pre-Outcome denominator. Authored Flow
// operands are then closed iteratively. No source position table, causal
// edge, Outcome, or consumer-specific projection is retained here beyond the
// dense per-Body executable-root rows issued from the same seed. The
// containment and source-control owners retain only scalar owner value fences
// and expose narrow Matches checks; there is no pointer authority
// or generic assembly token to splice. This package validates its complete
// denominator and self-membership before closing the runtime operands.
package executable

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type root struct {
	id     identity.ContentID
	family keyspace.Family
}

// Result is the immutable pre-Outcome executable membership proof.  Counts
// are copied from Source's exact pre-Outcome identity and bits are dense by
// family ordinal. The four scalar owner IDs are the narrow provenance fence
// for this projection; no Source graph or authored input authority is retained.
type Result struct {
	counts   [keyspace.FamilyCount]uint32
	bits     [keyspace.FamilyCount][]uint64
	roots    [][]root
	members  uint32
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
}

// Matches reports whether r belongs to the supplied final Source, authored
// Flow, Static, and Module identities. It is the narrow downstream splice
// fence for this proof.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

// Executable reports whether term is an admitted runtime occurrence.  It is
// allocation-free and O(1); malformed, foreign, static, global-Cell, and
// Outcome terms fail closed.
func (r *Result) Executable(term keyspace.Term) bool {
	if r == nil || !r.sourceID.Available() || !r.flowID.Available() || !r.staticID.Available() || !r.moduleID.Available() {
		return false
	}
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		family == keyspace.FamilyOutcome || ordinal == 0 || ordinal > r.counts[family] {
		return false
	}
	word := (ordinal - 1) >> 6
	bits := r.bits[family]
	return uint64(word) < uint64(len(bits)) && bits[word]&(uint64(1)<<((ordinal-1)&63)) != 0
}

// Count reports the number of executable pre-Outcome terms.
func (r *Result) Count() int {
	if r == nil || !r.sourceID.Available() || !r.flowID.Available() || !r.staticID.Available() || !r.moduleID.Available() {
		return 0
	}
	return int(r.members)
}

// FamilyCount reports Source's exact pre-Outcome denominator for family.
// Outcome and invalid families always report zero.
func (r *Result) FamilyCount(family keyspace.Family) int {
	if r == nil || !r.sourceID.Available() || !r.flowID.Available() || !r.staticID.Available() || !r.moduleID.Available() || family <= keyspace.FamilyInvalid ||
		family >= keyspace.FamilyCount || family == keyspace.FamilyOutcome {
		return 0
	}
	return int(r.counts[family])
}

// RootCount returns the already-issued dense executable Source roots for one
// Body. A valid Body may have no roots; malformed or foreign Bodies fail
// closed separately.
func (r *Result) RootCount(body keyspace.Term) (int, bool) {
	if r == nil || r.roots == nil || len(r.roots) != int(r.counts[keyspace.FamilyBody]) || !r.sourceID.Available() || !r.flowID.Available() || !r.staticID.Available() || !r.moduleID.Available() ||
		keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 || uint64(keyspace.TermOrdinal(body)) > uint64(len(r.roots)) {
		return 0, false
	}
	return len(r.roots[keyspace.TermOrdinal(body)-1]), true
}

func (r *Result) RootAt(body keyspace.Term, index int) (identity.ContentID, keyspace.Family, bool) {
	count, ok := r.RootCount(body)
	if !ok || index < 0 || index >= count {
		return identity.ContentID{}, keyspace.FamilyInvalid, false
	}
	row := r.roots[keyspace.TermOrdinal(body)-1][index]
	return row.id, row.family, row.id.Available() && row.family != keyspace.FamilyInvalid
}

func (r *Result) installRoots(terms [][]keyspace.Term, paths *semanticpath.Certificate) error {
	if r == nil || !Matches(r, r.sourceID, r.flowID, r.staticID, r.moduleID) || !paths.Matches(r.sourceID, r.flowID, r.staticID, r.moduleID) || len(terms) != int(r.counts[keyspace.FamilyBody]) {
		return errors.New("program/flow/executable: root path owners disagree")
	}
	rows := make([][]root, len(terms))
	for bodyIndex, bodyTerms := range terms {
		rootRows := make([]root, 0, len(bodyTerms))
		for _, authored := range bodyTerms {
			if !r.Executable(authored) {
				return errors.New("program/flow/executable: executable root membership changed")
			}
			family, ordinal := keyspace.TermFamily(authored), keyspace.TermOrdinal(authored)
			id, idOK := paths.TermPathAt(r.sourceID, r.flowID, r.staticID, r.moduleID, family, ordinal)
			if !idOK || !id.Available() || family == keyspace.FamilyInvalid {
				return errors.New("program/flow/executable: executable root identity is unavailable")
			}
			rootRows = append(rootRows, root{id: id, family: family})
		}
		rows[bodyIndex] = rootRows
	}
	r.roots = rows
	return nil
}

func (r *Result) mark(term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount ||
		family == keyspace.FamilyOutcome || ordinal == 0 || ordinal > r.counts[family] {
		return false
	}
	word := (ordinal - 1) >> 6
	bits := r.bits[family]
	if uint64(word) >= uint64(len(bits)) {
		return false
	}
	mask := uint64(1) << ((ordinal - 1) & 63)
	if bits[word]&mask != 0 {
		return false
	}
	bits[word] |= mask
	r.members++
	return true
}

func newResult(counts [keyspace.FamilyCount]uint32, sourceID, flowID, staticID, moduleID identity.ContentID) *Result {
	r := &Result{counts: counts, sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		// The denominator is retained for every Source family, but membership
		// planes exist only for the closed runtime vocabulary. Static,
		// namespace, fault, key, import, and Outcome families can never be
		// executable and therefore retain no dead bit storage.
		if family == keyspace.FamilyOutcome || counts[family] == 0 || !runtimeFamily(family) {
			continue
		}
		r.bits[family] = make([]uint64, (uint64(counts[family])+63)/64)
	}
	return r
}
