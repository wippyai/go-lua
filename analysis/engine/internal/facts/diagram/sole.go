package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
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
	treeFrames  []soleTreeFrame[K, V]

	trackedFrames []trackedFrame[V]
	tracked       map[trackedTriple[V]]trackedResult[V]
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
	clear(scratch.treeFrames)
	scratch.treeFrames = scratch.treeFrames[:0]
	clear(scratch.trackedFrames)
	scratch.trackedFrames = scratch.trackedFrames[:0]
	clear(scratch.tracked)
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
