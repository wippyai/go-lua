// Package executable seals Flow's pre-Outcome runtime membership.
//
// The result is a Source/Flow/Static/Module-identity-bound, dense per-family bitset. Source
// control supplies the reachable direct roots; containment supplies static
// classification and the complete pre-Outcome denominator. Authored Flow
// operands are then closed iteratively. No source position table, causal
// edge, Outcome, or consumer-specific projection is retained here. The
// containment and source-control owners retain only scalar owner value fences
// and expose narrow Matches checks; there is no pointer authority
// or generic assembly token to splice. This package validates its complete
// denominator and self-membership before closing the runtime operands.
package executable

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the immutable pre-Outcome executable membership proof.  Counts
// are copied from Source's exact pre-Outcome identity and bits are dense by
// family ordinal. The four scalar owner IDs are the narrow provenance fence
// for this projection; no Source graph or authored input authority is retained.
type Result struct {
	counts   [keyspace.FamilyCount]uint32
	bits     [keyspace.FamilyCount][]uint64
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
