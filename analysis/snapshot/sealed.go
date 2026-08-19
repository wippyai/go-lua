package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// Denominators is the snapshot's sealed denominator publication: which
// published denominator identity proves absence on which columns. Membership
// itself lives in one sealed set per identity, referenced by every column
// that is total over it, so this value carries a directory and never a second
// copy of a key universe.
//
// A denominator proves as many columns as declare it. Two columns that answer
// over one key universe share one set: the second column costs a reference,
// not a copy, and neither column's edits can reach the set.
type Denominators struct {
	index *trie[identity.ContentID, denominatorEntry]
	count int
}

// denominatorEntry is one published denominator: the sealed membership set,
// erased because its key type belongs to the columns it proves, the size of
// that set, and the column slots that read against it.
type denominatorEntry struct {
	set   any
	size  int
	slots []uint32
}

// Published reports whether id names a denominator this snapshot publishes.
func (d Denominators) Published(id identity.ContentID) bool {
	_, published := trieLookup(d.index, hashKey(identityPlan, id), id)
	return published
}

// Proves reports whether the denominator id proves absence on the column at
// slot. It allocates nothing.
func (d Denominators) Proves(id identity.ContentID, slot uint32) bool {
	entry, published := trieLookup(d.index, hashKey(identityPlan, id), id)
	if !published {
		return false
	}
	for _, proved := range entry.slots {
		if proved == slot {
			return true
		}
	}
	return false
}

// Size reports how many keys the denominator id covers. It is the sealed
// cardinality of a key universe, which is what a reader iterating a column
// that is total over that universe needs and what a column keyed by a dense
// ordinal has no other way to state. A denominator the publication does not
// hold reports nothing rather than zero, so ignorance stays distinguishable
// from an empty universe.
func (d Denominators) Size(id identity.ContentID) (int, bool) {
	entry, published := trieLookup(d.index, hashKey(identityPlan, id), id)
	if !published {
		return 0, false
	}
	return entry.size, true
}

// Len reports how many denominators the snapshot publishes.
func (d Denominators) Len() int { return d.count }

// Queries is the snapshot's sealed query publication: the registered query
// families this snapshot answers. A family that is not published here has no
// authority against this snapshot, which is what keeps a local combination
// from passing itself off as a registered answer and what makes a result
// column openable as a plan.
type Queries struct {
	plans *trie[identity.ContentID, struct{}]
	count int
}

// Published reports whether plan is registered against this snapshot.
func (q Queries) Published(plan identity.ContentID) bool {
	_, registered := trieLookup(q.plans, hashKey(identityPlan, plan), plan)
	return registered
}

// Len reports how many query families the snapshot publishes.
func (q Queries) Len() int { return q.count }
