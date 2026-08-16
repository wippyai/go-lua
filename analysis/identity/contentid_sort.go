package identity

// SortByContentID stably orders rows by the ContentID that key reports, using
// a least-significant-byte-first radix over the fixed 32-byte digest width.
// The pass count is constant, so ordering is linear in len(rows) and never
// invokes a comparison callback. Identity owns this because the order it
// establishes is exactly ContentID's own lexicographic byte order; the key
// function keeps the digest the only thing this package needs to know about a
// caller's row.
func SortByContentID[T any](rows []T, key func(T) ContentID) {
	if len(rows) < 2 {
		return
	}
	scratch := make([]T, len(rows))
	source, target := rows, scratch
	for byteIndex := len(ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var offsets [256]int
		for _, row := range source {
			offsets[key(row)[byteIndex]]++
		}
		total := 0
		for index := range offsets {
			count := offsets[index]
			offsets[index] = total
			total += count
		}
		for _, row := range source {
			bucket := key(row)[byteIndex]
			target[offsets[bucket]] = row
			offsets[bucket]++
		}
		source, target = target, source
	}
	if &source[0] != &rows[0] {
		copy(rows, source)
	}
}

// SortContentIDs orders ids into ascending lexicographic digest order.
func SortContentIDs(ids []ContentID) {
	SortByContentID(ids, func(id ContentID) ContentID { return id })
}
