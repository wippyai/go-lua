package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// column is the sealed storage behind one axis. It is unexported and has no
// exported surface anywhere in the package, so the only way a value leaves it
// is a read that returns a copy. Nothing outside a Builder ever holds a
// *column, and a builder edit never writes into one: an edit publishes a new
// column value that shares every untouched node with the one it derived from.
type column[K comparable, V any] struct {
	// plan is the hashing schedule of K. It is derived once per key type, so
	// the rows and any denominator keyed by K answer under one hash.
	plan *keyPlan
	rows *trie[K, V]
	// members is the sealed denominator this column is total over. A nil
	// denominator means the column publishes none and can never prove
	// absence. The set is shared by pointer: several columns can be total
	// over one denominator without copying its membership, and no column
	// edit can reach it.
	members *denominator[K]
}

// read answers key against the sealed rows and the sealed denominator. A
// stored row is a hit; a key the denominator covers without a row is proven
// absent; anything else is a miss. The key is hashed once and answers both
// questions.
func (c *column[K, V]) read(key K) (V, ReadStatus) {
	hash := hashKey(c.plan, key)
	if value, stored := trieLookup(c.rows, hash, key); stored {
		return value, ReadHit
	}
	var zero V
	if c.members != nil {
		if _, covered := trieLookup(c.members.members, hash, key); covered {
			return zero, ReadProvenAbsent
		}
	}
	return zero, ReadMiss
}

// denominator is one sealed membership set: the key universe a column can be
// total over, under the identity that published it. It is immutable once
// sealed and is referenced, never copied, by every column it proves.
type denominator[K comparable] struct {
	id      identity.ContentID
	members *trie[K, struct{}]
}
