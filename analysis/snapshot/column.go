package snapshot

// column is the sealed storage behind one axis. It is unexported and has no
// exported surface anywhere in the package, so the only way a value leaves it
// is a read that returns a copy. Nothing outside a Builder ever holds a
// *column, and a Builder loses its reach into one the moment it fills the
// slot: the builder exposes no way to reopen a filled slot.
type column[K comparable, V any] struct {
	rows map[K]V
	// members is the sealed denominator: the key set this column is total
	// over. A nil members set means the column publishes no denominator and
	// therefore can never prove absence.
	members map[K]struct{}
}

// read answers key against the sealed rows and the sealed denominator. A
// stored row is a hit; a key the denominator covers without a row is proven
// absent; anything else is a miss.
func (c *column[K, V]) read(key K) (V, ReadStatus) {
	if value, stored := c.rows[key]; stored {
		return value, ReadHit
	}
	var zero V
	if c.members != nil {
		if _, covered := c.members[key]; covered {
			return zero, ReadProvenAbsent
		}
	}
	return zero, ReadMiss
}
