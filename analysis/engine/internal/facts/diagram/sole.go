package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// SoleScratch is caller-owned reusable traversal storage for pure operations
// over one Factor. It is deliberately local to one caller: it has no global
// cache, pool, or semantic authority. Clear resets logical work while keeping
// exact storage available for the next operation.
//
// It is not safe for concurrent use. A pure comparison must not observe how
// often an equal suffix is reached; the exact (left, right, support) cache
// therefore lets a shared suffix be proved once without changing the older
// visitor API's callback multiplicity.
type SoleScratch[K scalar.Key, V any] struct {
	// checkpoint is evaluator-owned opaque liveness polling. It intentionally
	// carries no Context, domain, or scheduling vocabulary into the diagram;
	// a false probe only abandons the caller's unsealed construction.
	checkpoint        func() bool
	left, right       avlCursor[K, V]
	leftKey, rightKey *keyNode[K, V]
	hasLeft, hasRight bool
	frames            []soleFrame[V]
	seenInline        [relationInlineDepth]soleTriple[V]
	seenCount         int
	seen              map[soleTriple[V]]struct{}

	mergeFrames []soleMergeFrame[V]
	merge       map[soleMergeTriple[V]]soleMergeResult[V]
	output      []soleOutput[K, V]
	patches     []soleOutput[K, V]
	treeFrames  []soleTreeFrame[K, V]

	trackedFrames []trackedFrame[V]
	tracked       map[trackedTriple[V]]trackedResult[V]

	// many is the operation-local storage for one synchronized fixed-order
	// fold.  It is deliberately embedded in the caller-owned SoleScratch:
	// neither Diagram nor a typed Domain retains operand roots after the
	// transaction.  Flat tuple storage avoids one slice allocation per FDD
	// state, while the exact memo prevents shared suffixes from being expanded
	// repeatedly.
	manyCursors  []avlCursor[K, V]
	manyHeads    []*keyNode[K, V]
	manyHeap     []int
	manyNodes    []*node[V]
	manyWidth    int
	manySupports []support.Mask
	manyPresent  []bool
	manyIDs      []terminal.ID[V]

	manyLowNodes     []*node[V]
	manyHighNodes    []*node[V]
	manyLowSupports  []support.Mask
	manyHighSupports []support.Mask
	manyTupleNodes   []*node[V]
	manyTupleSupport []support.Mask
	manyStates       []soleManyState[V]
	manyStack        []int
	manyMemo         map[uint64]int
	manyNodeIDs      map[*node[V]]uint32
	manyMaskIDs      map[support.Mask]uint32
}

type soleManyState[V any] struct {
	reference *node[V]
	offset    int
	next      int
	low, high int
	atom      guard.Atom
	result    *node[V]
	phase     uint8
}

// SetCheckpoint installs an opaque evaluator liveness probe. It is called
// once while the enclosing SlotWork is idle, so the normal nil path remains a
// single predictable nil check with no per-node allocation.
func (scratch *SoleScratch[K, V]) SetCheckpoint(checkpoint func() bool) {
	if scratch != nil {
		scratch.checkpoint = checkpoint
	}
}

func (scratch *SoleScratch[K, V]) live() bool {
	return scratch != nil && (scratch.checkpoint == nil || scratch.checkpoint())
}

// NewSoleScratch makes explicit reusable storage for direct one-Factor
// structural work. The zero value is usable too, but this constructor permits
// callers to warm allocation before a hot comparison loop.
func NewSoleScratch[K scalar.Key, V any]() *SoleScratch[K, V] {
	return &SoleScratch[K, V]{}
}

// Clear drops the logical contents of the scratch without discarding its
// capacity. It never changes a Diagram, a Root, or a support region.
func (scratch *SoleScratch[K, V]) Clear() {
	if scratch == nil {
		return
	}
	scratch.left.clear()
	scratch.right.clear()
	scratch.leftKey, scratch.rightKey = nil, nil
	scratch.hasLeft, scratch.hasRight = false, false
	clear(scratch.frames)
	scratch.frames = scratch.frames[:0]
	clear(scratch.seenInline[:])
	scratch.seenCount = 0
	clear(scratch.seen)
	clear(scratch.mergeFrames)
	scratch.mergeFrames = scratch.mergeFrames[:0]
	clear(scratch.merge)
	clear(scratch.output)
	scratch.output = scratch.output[:0]
	clear(scratch.patches)
	scratch.patches = scratch.patches[:0]
	clear(scratch.treeFrames)
	scratch.treeFrames = scratch.treeFrames[:0]
	clear(scratch.trackedFrames)
	scratch.trackedFrames = scratch.trackedFrames[:0]
	clear(scratch.tracked)
	for index := range scratch.manyCursors {
		scratch.manyCursors[index].clear()
	}
	clear(scratch.manyCursors)
	scratch.manyCursors = scratch.manyCursors[:0]
	clear(scratch.manyHeads)
	scratch.manyHeads = scratch.manyHeads[:0]
	clear(scratch.manyHeap)
	scratch.manyHeap = scratch.manyHeap[:0]
	clear(scratch.manyNodes)
	scratch.manyNodes = scratch.manyNodes[:0]
	scratch.manyWidth = 0
	clear(scratch.manySupports)
	scratch.manySupports = scratch.manySupports[:0]
	clear(scratch.manyPresent)
	scratch.manyPresent = scratch.manyPresent[:0]
	clear(scratch.manyIDs)
	scratch.manyIDs = scratch.manyIDs[:0]
	clear(scratch.manyLowNodes)
	scratch.manyLowNodes = scratch.manyLowNodes[:0]
	clear(scratch.manyHighNodes)
	scratch.manyHighNodes = scratch.manyHighNodes[:0]
	clear(scratch.manyLowSupports)
	scratch.manyLowSupports = scratch.manyLowSupports[:0]
	clear(scratch.manyHighSupports)
	scratch.manyHighSupports = scratch.manyHighSupports[:0]
	scratch.clearManyStates()
}

func (scratch *SoleScratch[K, V]) clearManyStates() {
	if scratch == nil {
		return
	}
	clear(scratch.manyTupleNodes)
	scratch.manyTupleNodes = scratch.manyTupleNodes[:0]
	clear(scratch.manyTupleSupport)
	scratch.manyTupleSupport = scratch.manyTupleSupport[:0]
	clear(scratch.manyStates)
	scratch.manyStates = scratch.manyStates[:0]
	clear(scratch.manyStack)
	scratch.manyStack = scratch.manyStack[:0]
	clear(scratch.manyMemo)
	clear(scratch.manyNodeIDs)
	clear(scratch.manyMaskIDs)
}

func resizeClear[T any](values []T, count int) []T {
	clear(values)
	if cap(values) < count {
		return make([]T, count)
	}
	values = values[:count]
	clear(values)
	return values
}

// prepareMany opens a k-way ascending sparse-key zipper.  Each cursor owns
// only borrowed immutable nodes.  Normal exhaustion is represented solely by
// an empty heap; cancellation remains the caller's separate live check.
func (scratch *SoleScratch[K, V]) prepareMany(roots []*keyNode[K, V]) bool {
	if !scratch.live() || len(roots) == 0 {
		return false
	}
	scratch.Clear()
	count := len(roots)
	scratch.manyCursors = resizeClear(scratch.manyCursors, count)
	scratch.manyHeads = resizeClear(scratch.manyHeads, count)
	scratch.manyNodes = resizeClear(scratch.manyNodes, count)
	scratch.manySupports = resizeClear(scratch.manySupports, count)
	scratch.manyPresent = resizeClear(scratch.manyPresent, count)
	scratch.manyIDs = resizeClear(scratch.manyIDs, count)
	scratch.manyLowNodes = resizeClear(scratch.manyLowNodes, count)
	scratch.manyHighNodes = resizeClear(scratch.manyHighNodes, count)
	scratch.manyLowSupports = resizeClear(scratch.manyLowSupports, count)
	scratch.manyHighSupports = resizeClear(scratch.manyHighSupports, count)
	for index, root := range roots {
		scratch.manyCursors[index].begin(root)
		head, present := scratch.manyCursors[index].next()
		if !present {
			continue
		}
		scratch.manyHeads[index] = head
		scratch.manyHeapPush(index)
	}
	return scratch.live()
}

func (scratch *SoleScratch[K, V]) manyHeapLess(left, right int) bool {
	leftIndex, rightIndex := scratch.manyHeap[left], scratch.manyHeap[right]
	leftHead, rightHead := scratch.manyHeads[leftIndex], scratch.manyHeads[rightIndex]
	return leftHead.key < rightHead.key || leftHead.key == rightHead.key && leftIndex < rightIndex
}

func (scratch *SoleScratch[K, V]) manyHeapPush(operand int) {
	scratch.manyHeap = append(scratch.manyHeap, operand)
	for index := len(scratch.manyHeap) - 1; index > 0; {
		parent := (index - 1) / 2
		if !scratch.manyHeapLess(index, parent) {
			break
		}
		scratch.manyHeap[index], scratch.manyHeap[parent] = scratch.manyHeap[parent], scratch.manyHeap[index]
		index = parent
	}
}

func (scratch *SoleScratch[K, V]) manyHeapPop() (int, bool) {
	if len(scratch.manyHeap) == 0 {
		return 0, false
	}
	result := scratch.manyHeap[0]
	last := len(scratch.manyHeap) - 1
	scratch.manyHeap[0] = scratch.manyHeap[last]
	scratch.manyHeap[last] = 0
	scratch.manyHeap = scratch.manyHeap[:last]
	for index := 0; ; {
		left := index*2 + 1
		if left >= len(scratch.manyHeap) {
			break
		}
		right, smallest := left+1, left
		if right < len(scratch.manyHeap) && scratch.manyHeapLess(right, left) {
			smallest = right
		}
		if !scratch.manyHeapLess(smallest, index) {
			break
		}
		scratch.manyHeap[index], scratch.manyHeap[smallest] = scratch.manyHeap[smallest], scratch.manyHeap[index]
		index = smallest
	}
	return result, true
}

// nextMany returns the next key and a scratch-owned operand column vector.
// The vector is valid only until the next call.  It deliberately performs no
// liveness sampling so cancellation can never be mistaken for exhaustion.
func (scratch *SoleScratch[K, V]) nextMany() (K, []*node[V], bool) {
	var zero K
	if scratch == nil || len(scratch.manyHeap) == 0 {
		return zero, nil, false
	}
	clear(scratch.manyNodes)
	first := scratch.manyHeap[0]
	key := scratch.manyHeads[first].key
	for len(scratch.manyHeap) != 0 {
		operand := scratch.manyHeap[0]
		head := scratch.manyHeads[operand]
		if head == nil || head.key != key {
			break
		}
		_, _ = scratch.manyHeapPop()
		scratch.manyNodes[operand] = head.value
		next, present := scratch.manyCursors[operand].next()
		if present {
			scratch.manyHeads[operand] = next
			scratch.manyHeapPush(operand)
		} else {
			scratch.manyHeads[operand] = nil
		}
	}
	return key, scratch.manyNodes, true
}

func (scratch *SoleScratch[K, V]) manyNodeID(value *node[V]) uint32 {
	if value == nil {
		return 0
	}
	if id := scratch.manyNodeIDs[value]; id != 0 {
		return id
	}
	if scratch.manyNodeIDs == nil {
		scratch.manyNodeIDs = make(map[*node[V]]uint32)
	}
	id := uint32(len(scratch.manyNodeIDs) + 1)
	scratch.manyNodeIDs[value] = id
	return id
}

func (scratch *SoleScratch[K, V]) manyMaskID(value support.Mask) uint32 {
	if id := scratch.manyMaskIDs[value]; id != 0 {
		return id
	}
	if scratch.manyMaskIDs == nil {
		scratch.manyMaskIDs = make(map[support.Mask]uint32)
	}
	id := uint32(len(scratch.manyMaskIDs) + 1)
	scratch.manyMaskIDs[value] = id
	return id
}

func (scratch *SoleScratch[K, V]) manyState(reference *node[V], nodes []*node[V], supports []support.Mask) (int, bool) {
	if scratch == nil || len(nodes) == 0 || len(nodes) != len(supports) {
		return 0, false
	}
	hash := uint64(1469598103934665603)
	hash ^= uint64(scratch.manyNodeID(reference))
	hash *= 1099511628211
	for index := range nodes {
		hash ^= uint64(scratch.manyNodeID(nodes[index]))
		hash *= 1099511628211
		hash ^= uint64(scratch.manyMaskID(supports[index]))
		hash *= 1099511628211
	}
	for link := scratch.manyMemo[hash]; link != 0; link = scratch.manyStates[link-1].next {
		state := scratch.manyStates[link-1]
		equal := state.reference == reference
		for index := range nodes {
			if scratch.manyTupleNodes[state.offset+index] != nodes[index] || scratch.manyTupleSupport[state.offset+index] != supports[index] {
				equal = false
				break
			}
		}
		if equal {
			return link - 1, true
		}
	}
	if scratch.manyMemo == nil {
		scratch.manyMemo = make(map[uint64]int)
	}
	offset := len(scratch.manyTupleNodes)
	scratch.manyTupleNodes = append(scratch.manyTupleNodes, nodes...)
	scratch.manyTupleSupport = append(scratch.manyTupleSupport, supports...)
	index := len(scratch.manyStates)
	scratch.manyStates = append(scratch.manyStates, soleManyState[V]{reference: reference, offset: offset, next: scratch.manyMemo[hash]})
	scratch.manyMemo[hash] = index + 1
	return index, true
}

func (scratch *SoleScratch[K, V]) prepare(left, right *keyNode[K, V]) bool {
	if !scratch.live() {
		return false
	}
	scratch.Clear()
	scratch.left.begin(left)
	scratch.right.begin(right)
	scratch.leftKey, scratch.hasLeft = scratch.left.next()
	scratch.rightKey, scratch.hasRight = scratch.right.next()
	return true
}

// seenBefore records a pure comparison suffix.  The small inline set keeps a
// cold mismatch allocation-free; a larger valid graph gets the same exact
// identity memo without imposing a traversal bound.
func (scratch *SoleScratch[K, V]) seenBefore(value soleTriple[V]) bool {
	for index := 0; index < scratch.seenCount; index++ {
		if scratch.seenInline[index] == value {
			return true
		}
	}
	if scratch.seenCount < len(scratch.seenInline) {
		scratch.seenInline[scratch.seenCount] = value
		scratch.seenCount++
		return false
	}
	if scratch.seen == nil {
		scratch.seen = make(map[soleTriple[V]]struct{}, relationInlineDepth*2)
		for index := 0; index < scratch.seenCount; index++ {
			scratch.seen[scratch.seenInline[index]] = struct{}{}
		}
	}
	if _, found := scratch.seen[value]; found {
		return true
	}
	scratch.seen[value] = struct{}{}
	return false
}

// solePair is one ascending sparse coordinate from the exact key union. A
// nil side is the undefined column. It is structural data only: neither it
// nor SoleScratch assigns a terminal's semantic meaning.
type solePair[K scalar.Key, V any] struct {
	key         K
	left, right *node[V]
}

// nextPair is the reusable ordered zipper for later fused merge/output code.
// It never materializes the union or calls semantic code.
func (scratch *SoleScratch[K, V]) nextPair() (solePair[K, V], bool) {
	if scratch == nil || !scratch.hasLeft && !scratch.hasRight {
		return solePair[K, V]{}, false
	}
	switch {
	case !scratch.hasRight || scratch.hasLeft && scratch.leftKey.key < scratch.rightKey.key:
		pair := solePair[K, V]{key: scratch.leftKey.key, left: scratch.leftKey.value}
		scratch.leftKey, scratch.hasLeft = scratch.left.next()
		return pair, true
	case !scratch.hasLeft || scratch.rightKey.key < scratch.leftKey.key:
		pair := solePair[K, V]{key: scratch.rightKey.key, right: scratch.rightKey.value}
		scratch.rightKey, scratch.hasRight = scratch.right.next()
		return pair, true
	default:
		pair := solePair[K, V]{key: scratch.leftKey.key, left: scratch.leftKey.value, right: scratch.rightKey.value}
		scratch.leftKey, scratch.hasLeft = scratch.left.next()
		scratch.rightKey, scratch.hasRight = scratch.right.next()
		return pair, true
	}
}

type soleFrame[V any] struct {
	left, right *node[V]
	region      support.Mask
}

// soleTriple is an exact suffix identity. Mask is an immutable BDD handle;
// equality here is physical identity within the one guard manager, not an
// approximation of a Boolean region.
type soleTriple[V any] struct {
	left, right *node[V]
	region      support.Mask
}

func (scratch *SoleScratch[K, V]) push(frame soleFrame[V]) {
	scratch.frames = append(scratch.frames, frame)
}

func (scratch *SoleScratch[K, V]) pop() (soleFrame[V], bool) {
	last := len(scratch.frames) - 1
	if last < 0 {
		return soleFrame[V]{}, false
	}
	frame := scratch.frames[last]
	scratch.frames[last] = soleFrame[V]{}
	scratch.frames = scratch.frames[:last]
	return frame, true
}
