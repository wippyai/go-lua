package suspension

// WidenRank is the family witness. At validates the exact occurrence support
// selected by Key while the lattice itself remains constant across keys.
type WidenRank struct{ owner *schema }

func (schema Schema) WidenRank() (WidenRank, bool) {
	if !schema.Valid() {
		return WidenRank{}, false
	}
	return WidenRank{owner: schema.owner}, true
}
func (rank WidenRank) Width() int {
	if rank.owner == nil {
		return 0
	}
	return 1
}
func (rank WidenRank) At(key Key, value Value, component int) (uint64, bool) {
	owner, support, keyOK := key.support()
	if rank.owner == nil || component != 0 || !keyOK || owner != rank.owner || value.owner != rank.owner {
		return 0, false
	}
	if value.top {
		return 0, true
	}
	used := uint64(0)
	for _, lifecycle := range value.lifecycles {
		if !lifecycle.role.Valid() {
			return 0, false
		}
		if lifecycle.live {
			used++
		}
		if lifecycle.consumed {
			used++
		}
		for _, retention := range lifecycle.retained {
			if !rank.owner.containsAtom(support.retained, retainedAtom{kind: retention.subject.kind, value: retention.subject.value}) {
				return 0, false
			}
			if retention.roles&classPrivate != 0 {
				used++
			}
			if retention.roles&classShared != 0 {
				used++
			}
		}
	}
	capacity := uint64(3 * (2 + len(support.retained)*2))
	if used > capacity {
		return 0, false
	}
	return capacity - used + 1, true
}
