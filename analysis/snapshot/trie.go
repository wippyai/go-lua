package snapshot

import "math/bits"

// The trie is the one storage shape of this package. Rows, denominator
// members, the directory, the mount bindings and the query publication are
// all stored in it, so there is exactly one implementation of persistence to
// reason about and exactly one read path to keep allocation free.
//
// # Shape
//
// It is a compressed hash-array-mapped trie: each node splits five bits of a
// key's hash into a 32-way branch and keeps two bitmaps, one for the branches
// that hold a row and one for the branches that hold a subtrie. Only occupied
// branches are stored, so a node is as wide as its occupancy rather than as
// wide as its branch factor.
//
// # Why this shape
//
// The cost law is the reason. A publication that changes d rows must copy the
// nodes on those rows' paths and nothing else: every node the change did not
// touch is shared with the previous publication by pointer. Path length is
// bounded by the hash width rather than by the row count, so one changed row
// costs a constant number of node copies whether the column holds ten rows or
// ten million, and an unchanged column or denominator costs none at all
// because the publication keeps the pointer it already had.
//
// A flat map cannot state that law: sharing it publishes a mutable structure,
// and copying it prices every publication at the size of the column. A fixed
// chunk table cannot state it either: with a fixed chunk count the chunk
// itself grows with the column, so one changed row again copies a share of
// the whole. Path copying is what makes the change set, not the column, the
// unit of cost.
//
// # Reads
//
// Lookup walks the path iteratively and returns stored values by copy. It
// boxes nothing, closes over nothing, and allocates nothing on any outcome.

const (
	// trieBits is the hash width one node consumes, and trieWidth is the
	// branch factor that follows from it.
	trieBits  = 5
	trieWidth = 1 << trieBits
	trieMask  = trieWidth - 1
	// trieFinal is the shift at which the hash is exhausted. A node reached
	// at that depth holds keys whose hashes collide over their whole width
	// and is scanned rather than branched.
	trieFinal = 64
)

// trie is one node. Its fields are never written after construction: an
// update builds new nodes along the path it touches and shares every other
// node with the value it was derived from.
type trie[K comparable, V any] struct {
	// entryMap marks the branches that hold a row directly, nodeMap the
	// branches that hold a subtrie. The two are disjoint.
	entryMap uint32
	nodeMap  uint32
	// entries and nodes hold only occupied branches, in branch order, so a
	// branch's position is the population count of the bitmap below its bit.
	entries []trieEntry[K, V]
	nodes   []*trie[K, V]
}

// trieEntry is one stored row. It retains the key's hash so an update never
// recomputes it while it walks, and so a collision node can compare cheaply.
type trieEntry[K comparable, V any] struct {
	hash  uint64
	key   K
	value V
}

// trieLookup answers key against node. It returns the stored value by copy
// and allocates nothing.
func trieLookup[K comparable, V any](node *trie[K, V], hash uint64, key K) (V, bool) {
	shift := uint(0)
	for node != nil {
		if shift >= trieFinal {
			for index := range node.entries {
				if node.entries[index].key == key {
					return node.entries[index].value, true
				}
			}
			break
		}
		bit := uint32(1) << ((hash >> shift) & trieMask)
		if node.entryMap&bit != 0 {
			entry := &node.entries[bits.OnesCount32(node.entryMap&(bit-1))]
			if entry.key == key {
				return entry.value, true
			}
			break
		}
		if node.nodeMap&bit == 0 {
			break
		}
		node = node.nodes[bits.OnesCount32(node.nodeMap&(bit-1))]
		shift += trieBits
	}
	var zero V
	return zero, false
}

// trieInsert returns the trie that holds key at value alongside everything
// node holds, and reports whether the key is new. It copies the nodes on the
// key's path and shares every other node with node.
func trieInsert[K comparable, V any](node *trie[K, V], shift uint, entry trieEntry[K, V]) (*trie[K, V], bool) {
	if node == nil {
		fresh := &trie[K, V]{entries: []trieEntry[K, V]{entry}}
		if shift < trieFinal {
			fresh.entryMap = 1 << ((entry.hash >> shift) & trieMask)
		}
		return fresh, true
	}
	if shift >= trieFinal {
		for index := range node.entries {
			if node.entries[index].key == entry.key {
				return &trie[K, V]{entries: replaced(node.entries, index, entry)}, false
			}
		}
		return &trie[K, V]{entries: inserted(node.entries, len(node.entries), entry)}, true
	}
	bit := uint32(1) << ((entry.hash >> shift) & trieMask)
	if node.entryMap&bit != 0 {
		index := bits.OnesCount32(node.entryMap & (bit - 1))
		held := node.entries[index]
		if held.key == entry.key {
			return &trie[K, V]{
				entryMap: node.entryMap,
				nodeMap:  node.nodeMap,
				entries:  replaced(node.entries, index, entry),
				nodes:    node.nodes,
			}, false
		}
		return &trie[K, V]{
			entryMap: node.entryMap &^ bit,
			nodeMap:  node.nodeMap | bit,
			entries:  excluded(node.entries, index),
			nodes: inserted(
				node.nodes,
				bits.OnesCount32(node.nodeMap&(bit-1)),
				trieMerge(shift+trieBits, held, entry),
			),
		}, true
	}
	if node.nodeMap&bit != 0 {
		index := bits.OnesCount32(node.nodeMap & (bit - 1))
		child, added := trieInsert(node.nodes[index], shift+trieBits, entry)
		return &trie[K, V]{
			entryMap: node.entryMap,
			nodeMap:  node.nodeMap,
			entries:  node.entries,
			nodes:    replaced(node.nodes, index, child),
		}, added
	}
	return &trie[K, V]{
		entryMap: node.entryMap | bit,
		nodeMap:  node.nodeMap,
		entries:  inserted(node.entries, bits.OnesCount32(node.entryMap&(bit-1)), entry),
		nodes:    node.nodes,
	}, true
}

// trieRemove returns the trie that holds everything node holds except key,
// and reports whether a row was removed. It copies the removed key's path and
// nothing else, and it lifts a subtrie left holding a single row back into
// its parent so a removal never leaves a chain that only exists because of
// what the trie used to hold.
func trieRemove[K comparable, V any](node *trie[K, V], shift uint, hash uint64, key K) (*trie[K, V], bool) {
	if node == nil {
		return nil, false
	}
	if shift >= trieFinal {
		for index := range node.entries {
			if node.entries[index].key == key {
				if len(node.entries) == 1 {
					return nil, true
				}
				return &trie[K, V]{entries: excluded(node.entries, index)}, true
			}
		}
		return node, false
	}
	bit := uint32(1) << ((hash >> shift) & trieMask)
	if node.entryMap&bit != 0 {
		index := bits.OnesCount32(node.entryMap & (bit - 1))
		if node.entries[index].key != key {
			return node, false
		}
		if len(node.entries) == 1 && len(node.nodes) == 0 {
			return nil, true
		}
		return &trie[K, V]{
			entryMap: node.entryMap &^ bit,
			nodeMap:  node.nodeMap,
			entries:  excluded(node.entries, index),
			nodes:    node.nodes,
		}, true
	}
	if node.nodeMap&bit == 0 {
		return node, false
	}
	index := bits.OnesCount32(node.nodeMap & (bit - 1))
	child, removed := trieRemove(node.nodes[index], shift+trieBits, hash, key)
	if !removed {
		return node, false
	}
	if child == nil {
		shrunk := &trie[K, V]{
			entryMap: node.entryMap,
			nodeMap:  node.nodeMap &^ bit,
			entries:  node.entries,
			nodes:    excluded(node.nodes, index),
		}
		if shrunk.entryMap == 0 && shrunk.nodeMap == 0 {
			return nil, true
		}
		return shrunk, true
	}
	if len(child.entries) == 1 && len(child.nodes) == 0 {
		return &trie[K, V]{
			entryMap: node.entryMap | bit,
			nodeMap:  node.nodeMap &^ bit,
			entries:  inserted(node.entries, bits.OnesCount32(node.entryMap&(bit-1)), child.entries[0]),
			nodes:    excluded(node.nodes, index),
		}, true
	}
	return &trie[K, V]{
		entryMap: node.entryMap,
		nodeMap:  node.nodeMap,
		entries:  node.entries,
		nodes:    replaced(node.nodes, index, child),
	}, true
}

// trieBuild constructs the trie holding entries, which is what sealing a
// whole column does. It partitions the rows by hash chunk level by level and
// allocates each node once, so a fresh column costs its own nodes rather than
// the path copies an insertion sequence would leave behind.
//
// scratch is a buffer as long as entries. The two swap roles at every level:
// a level scatters what it read into the other buffer, and the range it read
// is then free for its own children to scatter into. Both buffers are
// consumed: neither holds meaningful contents when the build returns, and a
// caller that still needs its rows passes a copy.
func trieBuild[K comparable, V any](entries, scratch []trieEntry[K, V], shift uint) *trie[K, V] {
	if len(entries) == 0 {
		return nil
	}
	if shift >= trieFinal {
		// Equal keys hash equally, so a key offered twice reaches this one
		// node and is stored once: a row set is a set.
		held := make([]trieEntry[K, V], 0, len(entries))
		for _, entry := range entries {
			stored := false
			for index := range held {
				if held[index].key == entry.key {
					held[index] = entry
					stored = true
					break
				}
			}
			if !stored {
				held = append(held, entry)
			}
		}
		return &trie[K, V]{entries: held}
	}
	var counts, offsets, cursors [trieWidth]int
	for index := range entries {
		counts[(entries[index].hash>>shift)&trieMask]++
	}
	placed := 0
	node := &trie[K, V]{}
	for chunk := 0; chunk < trieWidth; chunk++ {
		offsets[chunk] = placed
		cursors[chunk] = placed
		placed += counts[chunk]
		switch {
		case counts[chunk] == 0:
		case counts[chunk] == 1:
			node.entryMap |= 1 << chunk
		default:
			node.nodeMap |= 1 << chunk
		}
	}
	for index := range entries {
		chunk := (entries[index].hash >> shift) & trieMask
		scratch[cursors[chunk]] = entries[index]
		cursors[chunk]++
	}
	node.entries = make([]trieEntry[K, V], 0, bits.OnesCount32(node.entryMap))
	node.nodes = make([]*trie[K, V], 0, bits.OnesCount32(node.nodeMap))
	for chunk := 0; chunk < trieWidth; chunk++ {
		start, end := offsets[chunk], offsets[chunk]+counts[chunk]
		switch {
		case counts[chunk] == 0:
		case counts[chunk] == 1:
			node.entries = append(node.entries, scratch[start])
		default:
			node.nodes = append(node.nodes, trieBuild(scratch[start:end], entries[start:end], shift+trieBits))
		}
	}
	return node
}

// trieMerge returns the subtrie holding two rows whose hashes agree above
// shift. It descends until the hashes disagree, and stores both rows in one
// scanned node when they agree over the whole hash width.
func trieMerge[K comparable, V any](shift uint, first, second trieEntry[K, V]) *trie[K, V] {
	if shift >= trieFinal {
		return &trie[K, V]{entries: []trieEntry[K, V]{first, second}}
	}
	firstBit := uint32(1) << ((first.hash >> shift) & trieMask)
	secondBit := uint32(1) << ((second.hash >> shift) & trieMask)
	if firstBit == secondBit {
		return &trie[K, V]{nodeMap: firstBit, nodes: []*trie[K, V]{trieMerge(shift+trieBits, first, second)}}
	}
	if firstBit < secondBit {
		return &trie[K, V]{entryMap: firstBit | secondBit, entries: []trieEntry[K, V]{first, second}}
	}
	return &trie[K, V]{entryMap: firstBit | secondBit, entries: []trieEntry[K, V]{second, first}}
}

// inserted returns items with item at index. The result is exactly sized, so
// no later insertion can write into a published node's storage.
func inserted[T any](items []T, index int, item T) []T {
	grown := make([]T, len(items)+1)
	copy(grown, items[:index])
	grown[index] = item
	copy(grown[index+1:], items[index:])
	return grown
}

// replaced returns items with index set to item.
func replaced[T any](items []T, index int, item T) []T {
	copied := make([]T, len(items))
	copy(copied, items)
	copied[index] = item
	return copied
}

// excluded returns items without index.
func excluded[T any](items []T, index int) []T {
	shrunk := make([]T, len(items)-1)
	copy(shrunk, items[:index])
	copy(shrunk[index:], items[index+1:])
	return shrunk
}
