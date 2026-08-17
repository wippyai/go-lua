package index

// rawSelectionIndex is a solve-local ordinal index for one authenticated
// staged Selection. Tags remain the semantic route identity; the index only
// removes repeated scans while the Selection is live.
type rawSelectionIndex struct {
	entries []rawSelectionIndexEntry
}

type rawSelectionIndexEntry struct {
	tag         uint64
	ordinalPlus uint32
}

func (index *rawSelectionIndex) build(count int, at func(int) (uint64, bool)) bool {
	if index == nil || count < 0 || count > int(^uint(0)>>1)/2 || at == nil || uint64(count) >= uint64(^uint32(0)) {
		return false
	}
	if count == 0 {
		index.entries = index.entries[:0]
		return true
	}
	size := 2
	for size < count*2 {
		size <<= 1
	}
	if cap(index.entries) < size {
		index.entries = make([]rawSelectionIndexEntry, size)
	} else {
		index.entries = index.entries[:size]
		clear(index.entries)
	}
	for ordinal := 0; ordinal < count; ordinal++ {
		tag, ok := at(ordinal)
		if !ok || tag == 0 || !index.insert(tag, uint32(ordinal+1)) {
			return false
		}
	}
	return true
}

func (index *rawSelectionIndex) insert(tag uint64, ordinalPlus uint32) bool {
	mask := uint64(len(index.entries) - 1)
	position := rawSelectionHash(tag) & mask
	for probes := 0; probes < len(index.entries); probes++ {
		entry := &index.entries[position]
		if entry.ordinalPlus == 0 {
			entry.tag, entry.ordinalPlus = tag, ordinalPlus
			return true
		}
		if entry.tag == tag {
			return false
		}
		position = (position + 1) & mask
	}
	return false
}

func (index *rawSelectionIndex) ordinal(tag uint64) (int, bool) {
	if index == nil || tag == 0 || len(index.entries) == 0 {
		return 0, false
	}
	mask := uint64(len(index.entries) - 1)
	position := rawSelectionHash(tag) & mask
	for probes := 0; probes < len(index.entries); probes++ {
		entry := index.entries[position]
		if entry.ordinalPlus == 0 {
			return 0, false
		}
		if entry.tag == tag {
			return int(entry.ordinalPlus - 1), true
		}
		position = (position + 1) & mask
	}
	return 0, false
}

func rawSelectionHash(tag uint64) uint64 {
	tag ^= tag >> 30
	tag *= 0xbf58476d1ce4e5b9
	tag ^= tag >> 27
	tag *= 0x94d049bb133111eb
	return tag ^ (tag >> 31)
}
