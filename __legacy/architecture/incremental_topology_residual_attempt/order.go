package topology

// blockOrder is the one private order-maintenance primitive shared by the
// residual DAG and full condensation. It is an implicit AVL rope of bounded
// blocks: local edits move at most blockCapacity entries, while rank/order
// comparisons traverse only the block tree. No graph-wide position array is
// rewritten when a compatible search repairs an interval.
const (
	blockCapacity = 192
	blockSplit    = blockCapacity / 2
)

type orderEntry struct {
	id    int
	block *orderBlock
	slot  uint16
}

type blockOrder struct {
	root       *orderBlock
	first, last *orderBlock
	n          int
}

type orderBlock struct {
	left, right, parent *orderBlock
	prev, next          *orderBlock
	height              int8
	sub                 int
	n                   uint16
	items               [blockCapacity]*orderEntry
}

func (o *blockOrder) Len() int { return o.n }

func (o *blockOrder) At(index int) *orderEntry {
	if index < 0 || index >= o.n {
		return nil
	}
	for block := o.root; block != nil; {
		left := blockSub(block.left)
		if index < left {
			block = block.left
			continue
		}
		index -= left
		if index < int(block.n) {
			return block.items[index]
		}
		index -= int(block.n)
		block = block.right
	}
	return nil
}

func (o *blockOrder) Before(left, right *orderEntry) bool {
	if left == nil || right == nil || left.block == nil || right.block == nil {
		return false
	}
	if left.block == right.block {
		return left.slot < right.slot
	}
	return o.blockRank(left.block)+int(left.slot) < o.blockRank(right.block)+int(right.slot)
}

func (o *blockOrder) Prev(entry *orderEntry) *orderEntry {
	if entry == nil || entry.block == nil {
		return nil
	}
	block := entry.block
	if entry.slot != 0 {
		return block.items[int(entry.slot)-1]
	}
	if block.prev == nil {
		return nil
	}
	return block.prev.items[int(block.prev.n)-1]
}

func (o *blockOrder) Next(entry *orderEntry) *orderEntry {
	if entry == nil || entry.block == nil {
		return nil
	}
	block := entry.block
	next := int(entry.slot) + 1
	if next < int(block.n) {
		return block.items[next]
	}
	if block.next == nil {
		return nil
	}
	return block.next.items[0]
}

func (o *blockOrder) Append(entry *orderEntry) { o.InsertBefore(nil, entry) }

// InsertAfter inserts entry after anchor; nil anchor denotes the beginning.
func (o *blockOrder) InsertAfter(anchor, entry *orderEntry) {
	if entry == nil || entry.block != nil {
		panic("topology: invalid order insertion")
	}
	if anchor == nil {
		if o.first == nil {
			o.installFirst(entry)
			return
		}
		o.insertInto(o.first, 0, entry)
		return
	}
	if anchor.block == nil {
		panic("topology: detached order anchor")
	}
	o.insertInto(anchor.block, int(anchor.slot)+1, entry)
}

// InsertBefore inserts entry before anchor; nil anchor denotes the end.
func (o *blockOrder) InsertBefore(anchor, entry *orderEntry) {
	if entry == nil || entry.block != nil {
		panic("topology: invalid order insertion")
	}
	if anchor == nil {
		if o.last == nil {
			o.installFirst(entry)
			return
		}
		o.insertInto(o.last, int(o.last.n), entry)
		return
	}
	if anchor.block == nil {
		panic("topology: detached order anchor")
	}
	o.insertInto(anchor.block, int(anchor.slot), entry)
}

func (o *blockOrder) Remove(entry *orderEntry) {
	if entry == nil || entry.block == nil {
		panic("topology: detached order removal")
	}
	block := entry.block
	index := int(entry.slot)
	copy(block.items[index:], block.items[index+1:int(block.n)])
	block.n--
	block.items[block.n] = nil
	for slot := index; slot < int(block.n); slot++ {
		block.items[slot].slot = uint16(slot)
	}
	entry.block = nil
	entry.slot = 0
	o.n--
	if block.n == 0 {
		o.removeBlock(block)
		return
	}
	o.rebalanceFrom(block)
	o.mergeSmall(block)
}

func (o *blockOrder) installFirst(entry *orderEntry) {
	block := &orderBlock{height: 1, sub: 1, n: 1}
	block.items[0] = entry
	entry.block, entry.slot = block, 0
	o.root, o.first, o.last, o.n = block, block, block, 1
}

func (o *blockOrder) insertInto(block *orderBlock, index int, entry *orderEntry) {
	if block.n == blockCapacity {
		right := o.splitBlock(block)
		if index > int(block.n) {
			index -= int(block.n)
			block = right
		}
	}
	copy(block.items[index+1:], block.items[index:int(block.n)])
	block.items[index] = entry
	block.n++
	entry.block, entry.slot = block, uint16(index)
	for slot := index + 1; slot < int(block.n); slot++ {
		block.items[slot].slot = uint16(slot)
	}
	o.n++
	o.rebalanceFrom(block)
}

func (o *blockOrder) splitBlock(block *orderBlock) *orderBlock {
	right := &orderBlock{height: 1}
	count := int(block.n) - blockSplit
	copy(right.items[:count], block.items[blockSplit:int(block.n)])
	right.n = uint16(count)
	right.sub = count
	for slot := 0; slot < count; slot++ {
		entry := right.items[slot]
		entry.block, entry.slot = right, uint16(slot)
	}
	clear(block.items[blockSplit:int(block.n)])
	block.n = blockSplit
	o.insertBlockAfter(block, right)
	o.rebalanceFrom(block)
	return right
}

func (o *blockOrder) mergeSmall(block *orderBlock) {
	if previous := block.prev; previous != nil && int(previous.n)+int(block.n) <= blockCapacity {
		o.mergeBlocks(previous, block)
		return
	}
	if next := block.next; next != nil && int(block.n)+int(next.n) <= blockCapacity {
		o.mergeBlocks(block, next)
	}
}

func (o *blockOrder) mergeBlocks(left, right *orderBlock) {
	start := int(left.n)
	count := int(right.n)
	copy(left.items[start:start+count], right.items[:count])
	left.n += right.n
	for slot := start; slot < start+count; slot++ {
		entry := left.items[slot]
		entry.block, entry.slot = left, uint16(slot)
	}
	clear(right.items[:count])
	right.n = 0
	o.removeBlock(right)
	o.rebalanceFrom(left)
}

func (o *blockOrder) insertBlockAfter(anchor, block *orderBlock) {
	if anchor == nil {
		if o.root != nil {
			panic("topology: missing block anchor")
		}
		o.root, o.first, o.last = block, block, block
		return
	}
	block.prev, block.next = anchor, anchor.next
	if anchor.next != nil {
		anchor.next.prev = block
	} else {
		o.last = block
	}
	anchor.next = block

	parent := anchor
	if parent.right != nil {
		parent = parent.right
		for parent.left != nil {
			parent = parent.left
		}
		parent.left = block
	} else {
		parent.right = block
	}
	block.parent = parent
	o.rebalanceFrom(parent)
}

func (o *blockOrder) removeBlock(block *orderBlock) {
	if block.prev != nil {
		block.prev.next = block.next
	} else {
		o.first = block.next
	}
	if block.next != nil {
		block.next.prev = block.prev
	} else {
		o.last = block.prev
	}

	var firstRepair *orderBlock
	if block.left == nil {
		firstRepair = block.parent
		o.transplant(block, block.right)
	} else if block.right == nil {
		firstRepair = block.parent
		o.transplant(block, block.left)
	} else {
		successor := block.right
		for successor.left != nil {
			successor = successor.left
		}
		if successor.parent != block {
			firstRepair = successor.parent
			o.transplant(successor, successor.right)
			successor.right = block.right
			successor.right.parent = successor
		} else {
			firstRepair = successor
		}
		o.transplant(block, successor)
		successor.left = block.left
		successor.left.parent = successor
		updateBlock(successor)
	}
	block.left, block.right, block.parent = nil, nil, nil
	block.prev, block.next = nil, nil
	if firstRepair != nil {
		o.rebalanceFrom(firstRepair)
	}
	if o.root == nil {
		o.first, o.last = nil, nil
	}
}

func (o *blockOrder) transplant(old, next *orderBlock) {
	parent := old.parent
	if parent == nil {
		o.root = next
	} else if parent.left == old {
		parent.left = next
	} else {
		parent.right = next
	}
	if next != nil {
		next.parent = parent
	}
}

func (o *blockOrder) rebalanceFrom(block *orderBlock) {
	for block != nil {
		updateBlock(block)
		balance := blockHeight(block.left) - blockHeight(block.right)
		if balance > 1 {
			if blockHeight(block.left.left) < blockHeight(block.left.right) {
				o.rotateLeft(block.left)
			}
			block = o.rotateRight(block)
		} else if balance < -1 {
			if blockHeight(block.right.right) < blockHeight(block.right.left) {
				o.rotateRight(block.right)
			}
			block = o.rotateLeft(block)
		}
		block = block.parent
	}
}

func (o *blockOrder) rotateLeft(root *orderBlock) *orderBlock {
	pivot := root.right
	child := pivot.left
	parent := root.parent
	pivot.parent = parent
	if parent == nil {
		o.root = pivot
	} else if parent.left == root {
		parent.left = pivot
	} else {
		parent.right = pivot
	}
	pivot.left = root
	root.parent = pivot
	root.right = child
	if child != nil {
		child.parent = root
	}
	updateBlock(root)
	updateBlock(pivot)
	return pivot
}

func (o *blockOrder) rotateRight(root *orderBlock) *orderBlock {
	pivot := root.left
	child := pivot.right
	parent := root.parent
	pivot.parent = parent
	if parent == nil {
		o.root = pivot
	} else if parent.left == root {
		parent.left = pivot
	} else {
		parent.right = pivot
	}
	pivot.right = root
	root.parent = pivot
	root.left = child
	if child != nil {
		child.parent = root
	}
	updateBlock(root)
	updateBlock(pivot)
	return pivot
}

func (o *blockOrder) blockRank(block *orderBlock) int {
	rank := blockSub(block.left)
	for block.parent != nil {
		parent := block.parent
		if parent.right == block {
			rank += blockSub(parent.left) + int(parent.n)
		}
		block = parent
	}
	return rank
}

func blockHeight(block *orderBlock) int {
	if block == nil {
		return 0
	}
	return int(block.height)
}

func blockSub(block *orderBlock) int {
	if block == nil {
		return 0
	}
	return block.sub
}

func updateBlock(block *orderBlock) {
	if block == nil {
		return
	}
	left, right := blockHeight(block.left), blockHeight(block.right)
	if left > right {
		block.height = int8(left + 1)
	} else {
		block.height = int8(right + 1)
	}
	block.sub = blockSub(block.left) + int(block.n) + blockSub(block.right)
}
