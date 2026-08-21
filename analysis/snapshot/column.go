package snapshot

import "github.com/wippyai/go-lua/analysis/identity"

// column is the sealed storage behind one axis. It is unexported and has no
// exported surface anywhere in the package, so the only way a value leaves it
// is a read that returns a copy. Nothing outside a Builder ever holds a
// *column, and a builder edit never writes into one: an edit publishes a new
// column value that shares every untouched node with the one it derived from.
//
// A column holds its rows in one of two shapes, chosen when it is sealed and
// invisible to every read. Rows keyed by a hash are the general shape and the
// one a revised publication needs, because a persistent trie prices an edit
// by the change set. Rows of a dense ordinal universe published once and
// never revised are held where their position already puts them: the sequence
// itself, indexed by the key, with no hash and no descent. Which shape a
// column holds changes what a read costs and never what it answers.
type column[K comparable, V any] struct {
	// plan is the hashing schedule of K and its index conversion. It is
	// derived once per key type, so the rows and any denominator keyed by K
	// answer under one hash and one notion of position.
	plan *keyPlan
	// sequence reports that this column holds its rows in values rather than
	// in rows. It is a fact about storage alone: a sequence column and a
	// keyed column built from the same rows answer identically.
	sequence bool
	// values holds the rows of a sequence column at their own ordinals. It
	// is sealed storage: the slice a read borrows is this one, and nothing
	// in the package writes to it after Seal.
	values []V
	// rows holds the rows of a keyed column.
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
// absent; anything else is a miss. A key is converted once, by index for a
// sequence column and by hash for a keyed one, and that one conversion
// answers both questions.
func (c *column[K, V]) read(key K) (V, ReadStatus) {
	var hash uint64
	if c.sequence {
		if index, positioned := keyOrdinal(c.plan, key); positioned && index < len(c.values) {
			return c.values[index], ReadHit
		}
	} else {
		hash = hashKey(c.plan, key)
		if value, stored := trieLookup(c.rows, hash, key); stored {
			return value, ReadHit
		}
	}
	var zero V
	if c.members == nil {
		return zero, ReadMiss
	}
	if c.sequence && !c.members.ordinal {
		// A sequence column that reads against a denominator naming its
		// members is the one case where the row lookup left no hash behind.
		hash = hashKey(c.plan, key)
	}
	if c.members.covers(c.plan, key, hash) {
		return zero, ReadProvenAbsent
	}
	return zero, ReadMiss
}

// denominator is one sealed membership set: the key universe a column can be
// total over, under the identity that published it. It is immutable once
// sealed and is referenced, never copied, by every column it proves.
//
// A universe states its membership in the form its keys have. A universe of
// positions is the range 0..width-1 and is stated by its width alone, so a
// dense key universe costs a word rather than a set. Any other universe names
// its members and holds them.
type denominator[K comparable] struct {
	id identity.ContentID
	// ordinal marks a universe that is the dense position range below width.
	ordinal bool
	width   int
	members *trie[K, struct{}]
	// order is the publisher's canonical member order.  The trie above is
	// deliberately only the membership authority; its hash layout is not a
	// stable enumeration order.
	order []K
}

// covers reports whether key is in the universe this denominator publishes.
// hash is the key's hash under plan, which the caller has already computed
// for the lookup that did not find a row.
func (d *denominator[K]) covers(plan *keyPlan, key K, hash uint64) bool {
	if d.ordinal {
		index, positioned := keyOrdinal(plan, key)
		return positioned && index < d.width
	}
	_, covered := trieLookup(d.members, hash, key)
	return covered
}
