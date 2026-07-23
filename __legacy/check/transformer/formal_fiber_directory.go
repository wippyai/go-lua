package transformer

import (
	"fmt"
	"math/bits"
	"sort"
)

// formalFiberOrdinal is one stable position in a frozen formal product
// inventory. It is an address inside formalFiberDirectoryArena, not a semantic
// axis, route, decision variable, or State coordinate.
type formalFiberOrdinal int

// formalFiberValue is an opaque structural token owned by the caller of the
// directory. Zero is the canonical per-fiber default. The directory never
// interprets a nonzero token.
type formalFiberValue uintptr

type formalFiberNodeRef int

// formalFiberDirectoryRoot is an immutable product-directory root. The owner
// pointer is part of the capability: process-local node references from two
// independently frozen arenas can never be mixed accidentally.
type formalFiberDirectoryRoot struct {
	owner *formalFiberDirectoryArena
	ref   formalFiberNodeRef
}

// formalFiberWrite is one point replacement. Writes are sealed into a sorted,
// duplicate-free formalFiberDelta before they can be applied.
type formalFiberWrite struct {
	ordinal formalFiberOrdinal
	value   formalFiberValue
}

// formalFiberDelta is an immutable, arena-owned sparse replacement. It has no
// carry semantics of its own: omitted ordinals retain the input root exactly.
type formalFiberDelta struct {
	owner  *formalFiberDirectoryArena
	writes []formalFiberWrite
}

// formalFiberZipLeaf is the only semantic seam in a binary directory zip. It
// is a concrete function type, so the structural recursion has no interface
// dispatch and cannot observe any product inventory besides ordinal and the
// two opaque leaf tokens.
type formalFiberZipLeaf func(formalFiberOrdinal, formalFiberValue, formalFiberValue) (formalFiberValue, error)

// formalFiberDirectoryStats are structural counters. They are suitable for
// always-on profiling because they count retained arena nodes and visited
// leaves rather than wall time.
type formalFiberDirectoryStats struct {
	NodesAdded     int
	LeafCalls      int
	EqualSubtrees  int
	InternedReuses int
}

type formalFiberDirectoryNode struct {
	height      uint
	left, right formalFiberNodeRef
	value       formalFiberValue
}

// formalFiberDirectoryArena is an append-only hash-consing authority for one
// fixed finite fiber inventory. Published roots are immutable. Point and batch
// updates path-copy only changed branches; the arena may retain unreachable
// interned nodes after an aborted higher-level transaction, but no root is
// published by this type until the complete structural operation succeeds.
//
// Ref zero denotes the canonical all-default subtree at every height. A
// nonzero leaf stores a non-default opaque token. A nonzero internal node is
// canonical for (height,left,right). The fixed height makes node identity
// independent of update order.
type formalFiberDirectoryArena struct {
	fibers    int
	leafBase  int
	height    uint
	pathNodes int
	nodes     []formalFiberDirectoryNode
	unique    map[formalFiberDirectoryNode]formalFiberNodeRef
}

// newFormalFiberDirectoryArena freezes a finite inventory width. Zero fibers
// is a valid empty product with one owned all-default root and no valid point
// update. The only limit is the host's addressable int range.
func newFormalFiberDirectoryArena(fibers int) (*formalFiberDirectoryArena, error) {
	if fibers < 0 {
		return nil, fmt.Errorf("transformer: negative formal fiber count")
	}
	arena := &formalFiberDirectoryArena{
		fibers: fibers,
		nodes:  []formalFiberDirectoryNode{{}},
		unique: make(map[formalFiberDirectoryNode]formalFiberNodeRef),
	}
	if fibers == 0 {
		return arena, nil
	}
	maxInt := int(^uint(0) >> 1)
	base := 1
	for base < fibers {
		if base > maxInt/2 {
			return nil, fmt.Errorf("transformer: formal fiber inventory exceeds address space")
		}
		base *= 2
	}
	arena.leafBase = base
	arena.height = uint(bits.Len(uint(base)) - 1)
	arena.pathNodes = int(arena.height) + 1
	return arena, nil
}

func (a *formalFiberDirectoryArena) defaultRoot() formalFiberDirectoryRoot {
	if a == nil {
		return formalFiberDirectoryRoot{}
	}
	return formalFiberDirectoryRoot{owner: a}
}

func (a *formalFiberDirectoryArena) fiberCount() int {
	if a == nil {
		return 0
	}
	return a.fibers
}

// updateDepth is the maximum number of retained nodes a novel point update
// can add, including its leaf. It is derived from address width, not a semantic
// work budget.
func (a *formalFiberDirectoryArena) updateDepth() int {
	if a == nil {
		return 0
	}
	return a.pathNodes
}

func (a *formalFiberDirectoryArena) nodeCount() int {
	if a == nil || len(a.nodes) == 0 {
		return 0
	}
	return len(a.nodes) - 1
}

func (a *formalFiberDirectoryArena) validateRoot(root formalFiberDirectoryRoot) error {
	if a == nil || root.owner != a || root.ref < 0 || int(root.ref) >= len(a.nodes) {
		return fmt.Errorf("transformer: foreign formal fiber directory root")
	}
	if root.ref != 0 && a.nodes[root.ref].height != a.height {
		return fmt.Errorf("transformer: malformed formal fiber directory root")
	}
	return nil
}

func (a *formalFiberDirectoryArena) validateOrdinal(ordinal formalFiberOrdinal) error {
	if a == nil || int(ordinal) < 0 || int(ordinal) >= a.fibers {
		return fmt.Errorf("transformer: formal fiber ordinal is outside frozen inventory")
	}
	return nil
}

// sealDelta validates, sorts, and freezes a sparse replacement. Duplicate
// ordinals reject instead of acquiring order-dependent last-write semantics.
func (a *formalFiberDirectoryArena) sealDelta(writes []formalFiberWrite) (formalFiberDelta, error) {
	if a == nil {
		return formalFiberDelta{}, fmt.Errorf("transformer: formal fiber delta has no arena")
	}
	canonical := append([]formalFiberWrite(nil), writes...)
	for _, write := range canonical {
		if err := a.validateOrdinal(write.ordinal); err != nil {
			return formalFiberDelta{}, err
		}
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].ordinal < canonical[j].ordinal })
	for index := 1; index < len(canonical); index++ {
		if canonical[index-1].ordinal == canonical[index].ordinal {
			return formalFiberDelta{}, fmt.Errorf("transformer: formal fiber delta repeats ordinal %d", canonical[index].ordinal)
		}
	}
	return formalFiberDelta{owner: a, writes: canonical}, nil
}

// update returns a new immutable root with one point replaced. Existing roots
// remain valid. Replacing a leaf with its current token returns root verbatim
// and interns no node.
func (a *formalFiberDirectoryArena) update(root formalFiberDirectoryRoot, ordinal formalFiberOrdinal, value formalFiberValue) (formalFiberDirectoryRoot, formalFiberDirectoryStats, error) {
	if err := a.validateRoot(root); err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	if err := a.validateOrdinal(ordinal); err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	before := a.nodeCount()
	ref, reuses, err := a.updateAt(root.ref, a.height, 0, a.leafBase, int(ordinal), value)
	if err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	return formalFiberDirectoryRoot{owner: a, ref: ref}, formalFiberDirectoryStats{
		NodesAdded: a.nodeCount() - before, InternedReuses: reuses,
	}, nil
}

func (a *formalFiberDirectoryArena) updateAt(ref formalFiberNodeRef, height uint, start, span, target int, value formalFiberValue) (formalFiberNodeRef, int, error) {
	if height == 0 {
		current, err := a.leafValue(ref)
		if err != nil {
			return 0, 0, err
		}
		if current == value {
			return ref, 1, nil
		}
		next, reused, err := a.internLeaf(value)
		if err != nil {
			return 0, 0, err
		}
		return next, boolInt(reused), nil
	}
	left, right, err := a.children(ref, height)
	if err != nil {
		return 0, 0, err
	}
	half := span / 2
	reuses := 0
	if target < start+half {
		left, reuses, err = a.updateAt(left, height-1, start, half, target, value)
	} else {
		right, reuses, err = a.updateAt(right, height-1, start+half, half, target, value)
	}
	if err != nil {
		return 0, 0, err
	}
	if ref != 0 {
		current := a.nodes[ref]
		if current.left == left && current.right == right {
			return ref, reuses + 1, nil
		}
	}
	next, reused, err := a.internBranch(height, left, right)
	return next, reuses + boolInt(reused), err
}

// applyDelta applies a sorted sparse replacement in one tree traversal. Empty
// deltas and deltas whose values are already present reuse root exactly.
func (a *formalFiberDirectoryArena) applyDelta(root formalFiberDirectoryRoot, delta formalFiberDelta) (formalFiberDirectoryRoot, formalFiberDirectoryStats, error) {
	if err := a.validateRoot(root); err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	if delta.owner != a {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, fmt.Errorf("transformer: foreign formal fiber delta")
	}
	for index, write := range delta.writes {
		if err := a.validateOrdinal(write.ordinal); err != nil {
			return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
		}
		if index != 0 && delta.writes[index-1].ordinal >= write.ordinal {
			return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, fmt.Errorf("transformer: formal fiber delta is not canonical")
		}
	}
	if len(delta.writes) == 0 {
		return root, formalFiberDirectoryStats{EqualSubtrees: 1}, nil
	}
	before := a.nodeCount()
	ref, reuses, err := a.applyDeltaAt(root.ref, a.height, 0, a.leafBase, delta.writes)
	if err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	return formalFiberDirectoryRoot{owner: a, ref: ref}, formalFiberDirectoryStats{
		NodesAdded: a.nodeCount() - before, InternedReuses: reuses,
	}, nil
}

func (a *formalFiberDirectoryArena) applyDeltaAt(ref formalFiberNodeRef, height uint, start, span int, writes []formalFiberWrite) (formalFiberNodeRef, int, error) {
	if len(writes) == 0 {
		return ref, 1, nil
	}
	if height == 0 {
		if len(writes) != 1 || int(writes[0].ordinal) != start {
			return 0, 0, fmt.Errorf("transformer: malformed formal fiber delta partition")
		}
		current, err := a.leafValue(ref)
		if err != nil {
			return 0, 0, err
		}
		if current == writes[0].value {
			return ref, 1, nil
		}
		next, reused, err := a.internLeaf(writes[0].value)
		return next, boolInt(reused), err
	}
	left, right, err := a.children(ref, height)
	if err != nil {
		return 0, 0, err
	}
	half, splitAt := span/2, start+span/2
	split := sort.Search(len(writes), func(index int) bool { return int(writes[index].ordinal) >= splitAt })
	left, leftReuses, err := a.applyDeltaAt(left, height-1, start, half, writes[:split])
	if err != nil {
		return 0, 0, err
	}
	right, rightReuses, err := a.applyDeltaAt(right, height-1, splitAt, half, writes[split:])
	if err != nil {
		return 0, 0, err
	}
	reuses := leftReuses + rightReuses
	if ref != 0 {
		current := a.nodes[ref]
		if current.left == left && current.right == right {
			return ref, reuses + 1, nil
		}
	}
	next, reused, err := a.internBranch(height, left, right)
	return next, reuses + boolInt(reused), err
}

// valueAt reads one opaque token without materializing any other fiber.
func (a *formalFiberDirectoryArena) valueAt(root formalFiberDirectoryRoot, ordinal formalFiberOrdinal) (formalFiberValue, error) {
	if err := a.validateRoot(root); err != nil {
		return 0, err
	}
	if err := a.validateOrdinal(ordinal); err != nil {
		return 0, err
	}
	ref, height, start, span := root.ref, a.height, 0, a.leafBase
	for height != 0 {
		left, right, err := a.children(ref, height)
		if err != nil {
			return 0, err
		}
		half := span / 2
		if int(ordinal) < start+half {
			ref = left
		} else {
			ref, start = right, start+half
		}
		height, span = height-1, half
	}
	return a.leafValue(ref)
}

// zip combines two complete directories of the same arena. Equal subtrees
// return by identity without visiting their leaves. Only unequal leaves invoke
// the concrete callback.
func (a *formalFiberDirectoryArena) zip(left, right formalFiberDirectoryRoot, combine formalFiberZipLeaf) (formalFiberDirectoryRoot, formalFiberDirectoryStats, error) {
	if err := a.validateRoot(left); err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	if err := a.validateRoot(right); err != nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, err
	}
	if combine == nil {
		return formalFiberDirectoryRoot{}, formalFiberDirectoryStats{}, fmt.Errorf("transformer: formal fiber zip has no leaf operation")
	}
	before := a.nodeCount()
	stats := formalFiberDirectoryStats{}
	ref, err := a.zipAt(left.ref, right.ref, a.height, 0, a.leafBase, combine, &stats)
	if err != nil {
		return formalFiberDirectoryRoot{}, stats, err
	}
	stats.NodesAdded = a.nodeCount() - before
	return formalFiberDirectoryRoot{owner: a, ref: ref}, stats, nil
}

func (a *formalFiberDirectoryArena) zipAt(left, right formalFiberNodeRef, height uint, start, span int, combine formalFiberZipLeaf, stats *formalFiberDirectoryStats) (formalFiberNodeRef, error) {
	if left == right {
		stats.EqualSubtrees++
		return left, nil
	}
	if height == 0 {
		if start < 0 || start >= a.fibers {
			return 0, fmt.Errorf("transformer: non-default padded formal fiber")
		}
		leftValue, err := a.leafValue(left)
		if err != nil {
			return 0, err
		}
		rightValue, err := a.leafValue(right)
		if err != nil {
			return 0, err
		}
		stats.LeafCalls++
		value, err := combine(formalFiberOrdinal(start), leftValue, rightValue)
		if err != nil {
			return 0, err
		}
		ref, reused, err := a.internLeaf(value)
		stats.InternedReuses += boolInt(reused)
		return ref, err
	}
	leftLow, leftHigh, err := a.children(left, height)
	if err != nil {
		return 0, err
	}
	rightLow, rightHigh, err := a.children(right, height)
	if err != nil {
		return 0, err
	}
	half := span / 2
	low, err := a.zipAt(leftLow, rightLow, height-1, start, half, combine, stats)
	if err != nil {
		return 0, err
	}
	high, err := a.zipAt(leftHigh, rightHigh, height-1, start+half, half, combine, stats)
	if err != nil {
		return 0, err
	}
	if left != 0 {
		node := a.nodes[left]
		if node.left == low && node.right == high {
			stats.InternedReuses++
			return left, nil
		}
	}
	if right != 0 {
		node := a.nodes[right]
		if node.left == low && node.right == high {
			stats.InternedReuses++
			return right, nil
		}
	}
	ref, reused, err := a.internBranch(height, low, high)
	stats.InternedReuses += boolInt(reused)
	return ref, err
}

func (a *formalFiberDirectoryArena) leafValue(ref formalFiberNodeRef) (formalFiberValue, error) {
	if ref == 0 {
		return 0, nil
	}
	if a == nil || ref < 0 || int(ref) >= len(a.nodes) || a.nodes[ref].height != 0 || a.nodes[ref].left != 0 || a.nodes[ref].right != 0 || a.nodes[ref].value == 0 {
		return 0, fmt.Errorf("transformer: malformed formal fiber leaf")
	}
	return a.nodes[ref].value, nil
}

func (a *formalFiberDirectoryArena) children(ref formalFiberNodeRef, height uint) (formalFiberNodeRef, formalFiberNodeRef, error) {
	if height == 0 {
		return 0, 0, fmt.Errorf("transformer: formal fiber leaf has no children")
	}
	if ref == 0 {
		return 0, 0, nil
	}
	if a == nil || ref < 0 || int(ref) >= len(a.nodes) {
		return 0, 0, fmt.Errorf("transformer: malformed formal fiber branch")
	}
	node := a.nodes[ref]
	if node.height != height || node.value != 0 {
		return 0, 0, fmt.Errorf("transformer: malformed formal fiber branch")
	}
	return node.left, node.right, nil
}

func (a *formalFiberDirectoryArena) internLeaf(value formalFiberValue) (formalFiberNodeRef, bool, error) {
	if value == 0 {
		return 0, true, nil
	}
	return a.internNode(formalFiberDirectoryNode{value: value})
}

func (a *formalFiberDirectoryArena) internBranch(height uint, left, right formalFiberNodeRef) (formalFiberNodeRef, bool, error) {
	if height == 0 {
		return 0, false, fmt.Errorf("transformer: formal fiber branch has zero height")
	}
	if left == 0 && right == 0 {
		return 0, true, nil
	}
	return a.internNode(formalFiberDirectoryNode{height: height, left: left, right: right})
}

func (a *formalFiberDirectoryArena) internNode(node formalFiberDirectoryNode) (formalFiberNodeRef, bool, error) {
	if a == nil || a.unique == nil || len(a.nodes) == 0 {
		return 0, false, fmt.Errorf("transformer: formal fiber arena is unowned")
	}
	if prior, ok := a.unique[node]; ok {
		return prior, true, nil
	}
	maxInt := int(^uint(0) >> 1)
	if len(a.nodes) == maxInt {
		return 0, false, fmt.Errorf("transformer: formal fiber node arena exceeds address space")
	}
	ref := formalFiberNodeRef(len(a.nodes))
	a.nodes = append(a.nodes, node)
	a.unique[node] = ref
	return ref, false, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
