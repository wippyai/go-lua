package containment

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// kernelInput is the deliberately small boundary between the owner-specific
// relation emitters and the dense graph proof.  Emitters provide the complete
// final family denominator, candidate child-to-parent rows, the terms which
// are permitted to be roots, and the exact static-expression marks.  The
// kernel has no knowledge of any owner, source position, CFG, or row shape.
//
// The type is private on purpose.  A generic graph/CFG API would make the
// relation itself a second public authority; Prove and its owner-local
// emitters are the only intended callers.
type kernelInput struct {
	counts   [keyspace.FamilyCount]uint32
	edges    []kernelEdge
	fallback []kernelEdge
	roots    []keyspace.Term
	static   []keyspace.Term
}

// kernelEdge is one candidate directed child-to-parent relation.  Every edge
// is a producer claim: duplicate rows and distinct parents for one child are
// hard errors.
type kernelEdge struct {
	child  keyspace.Term
	parent keyspace.Term
	// role/rank are an owner-issued semantic label for consumers that need
	// the edge identity after the kernel has sealed Parent. They do not
	// participate in graph construction; zero means that the edge has no
	// additional published role.
	role uint32
	rank uint32
}

// buildKernel validates and seals one dense containment forest.  Every term
// must have exactly one parent unless it is explicitly listed in roots.  The
// resulting intervals are built from canonical family/ordinal order, so edge
// emission order cannot affect queries or the retained proof.
func buildKernel(input kernelInput) (*Result, error) {
	offsets, total, err := kernelLayout(input.counts)
	if err != nil {
		return nil, err
	}

	parents, err := kernelParents(input.counts, input.edges)
	if err != nil {
		return nil, err
	}
	rootState, err := kernelRoots(input.counts, total, offsets, input.roots, parents)
	if err != nil {
		return nil, err
	}
	if err := kernelFallbackParents(input.counts, offsets, total, input.fallback, rootState, parents); err != nil {
		return nil, err
	}

	parentNodes, err := kernelParentNodes(input.counts, total, offsets, parents, rootState)
	if err != nil {
		return nil, err
	}
	if err := kernelCheckCycles(parentNodes, total, rootState); err != nil {
		return nil, err
	}

	pre, post, err := kernelIntervals(parentNodes, input.counts, offsets, total, rootState)
	if err != nil {
		return nil, err
	}
	static, err := kernelStatic(input.counts, input.static)
	if err != nil {
		return nil, err
	}

	return &Result{
		total:   total,
		parents: parents,
		pre:     pre,
		post:    post,
		static:  static,
	}, nil
}

// kernelLayout validates the dense denominator and computes zero-based global
// offsets.  Each family ordinal is independently checked by construction;
// the uint64 accumulator prevents a wrapped uint32 denominator.
func kernelLayout(counts [keyspace.FamilyCount]uint32) ([keyspace.FamilyCount]uint32, uint32, error) {
	var offsets [keyspace.FamilyCount]uint32
	if counts[keyspace.FamilyInvalid] != 0 {
		return offsets, 0, errors.New("program/flow/containment: Invalid family cardinality is nonzero")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := counts[family]
		if uint64(count) > uint64(keyspace.MaxTermOrdinal) {
			return offsets, 0, errors.New("program/flow/containment: family cardinality exceeds Term ordinal")
		}
		if total > uint64(^uint32(0)) {
			return offsets, 0, errors.New("program/flow/containment: Term denominator overflow")
		}
		offsets[family] = uint32(total)
		total += uint64(count)
	}
	if total > uint64(^uint32(0)) {
		return offsets, 0, errors.New("program/flow/containment: Term denominator overflow")
	}
	// The interval worker needs a total+1 prefix slice.  Reject only a
	// denominator that cannot be represented as a Go slice length on this
	// architecture; this is an indexability check, not a semantic graph cap.
	if total >= uint64(^uint(0)>>1) {
		return offsets, 0, errors.New("program/flow/containment: Term denominator is not indexable")
	}
	return offsets, uint32(total), nil
}

func kernelParents(
	counts [keyspace.FamilyCount]uint32,
	edges []kernelEdge,
) ([keyspace.FamilyCount][]keyspace.Term, error) {
	var parents [keyspace.FamilyCount][]keyspace.Term
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if counts[family] != 0 {
			parents[family] = make([]keyspace.Term, int(counts[family]))
		}
	}
	for _, edge := range edges {
		if !validTerm(edge.child, counts) || !validTerm(edge.parent, counts) || edge.child == edge.parent {
			return parents, errors.New("program/flow/containment: invalid containment edge")
		}
		family := keyspace.TermFamily(edge.child)
		ordinal := keyspace.TermOrdinal(edge.child)
		slot := &parents[family][ordinal-1]
		if *slot != 0 {
			if *slot == edge.parent {
				return parents, errors.New("program/flow/containment: duplicate containment edge")
			}
			return parents, errors.New("program/flow/containment: conflicting containment parents")
		}
		*slot = edge.parent
	}
	return parents, nil
}

func kernelRoots(
	counts [keyspace.FamilyCount]uint32,
	total uint32,
	offsets [keyspace.FamilyCount]uint32,
	rootTerms []keyspace.Term,
	parents [keyspace.FamilyCount][]keyspace.Term,
) ([]uint8, error) {
	// This plane begins as the root set, is reused for fallback provenance,
	// then becomes the cycle-state scratch.  A separate dense fallback table
	// or cycle plane would overlap the final parent proof for no benefit.
	allowed := make([]uint8, int(total))
	for _, root := range rootTerms {
		if !validTerm(root, counts) {
			return allowed, errors.New("program/flow/containment: invalid containment root")
		}
		index, ok := kernelIndex(root, counts, offsets, total)
		if !ok {
			return allowed, errors.New("program/flow/containment: invalid containment root index")
		}
		if allowed[index] != 0 {
			return allowed, errors.New("program/flow/containment: duplicate containment root")
		}
		allowed[index] = 1
		family := keyspace.TermFamily(root)
		ordinal := keyspace.TermOrdinal(root)
		if parents[family][ordinal-1] != 0 {
			return allowed, errors.New("program/flow/containment: root has a parent")
		}
	}
	return allowed, nil
}

// kernelFallbackParents admits the secondary relation source only after the
// ordinary producer rows and explicit roots have been checked.  A fallback
// producer may be reused verbatim, but it may not compete with another
// fallback parent or with an ordinary parent already installed for the same
// child.  A fallback child cannot be a designated root; its parent may be any
// valid term, including an explicit root such as Entry.
func kernelFallbackParents(
	counts [keyspace.FamilyCount]uint32,
	offsets [keyspace.FamilyCount]uint32,
	total uint32,
	edges []kernelEdge,
	rootState []uint8,
	parents [keyspace.FamilyCount][]keyspace.Term,
) error {
	if len(edges) == 0 {
		return nil
	}
	for _, edge := range edges {
		if !validTerm(edge.child, counts) || !validTerm(edge.parent, counts) || edge.child == edge.parent {
			return errors.New("program/flow/containment: invalid fallback containment edge")
		}
		childIndex, childOK := kernelIndex(edge.child, counts, offsets, total)
		_, parentOK := kernelIndex(edge.parent, counts, offsets, total)
		if !childOK || !parentOK || rootState[childIndex] == 1 {
			return errors.New("program/flow/containment: fallback edge requires a non-root child")
		}
		family := keyspace.TermFamily(edge.child)
		ordinal := keyspace.TermOrdinal(edge.child)
		slot := &parents[family][ordinal-1]
		if rootState[childIndex] == 2 {
			if *slot == edge.parent {
				continue
			}
			return errors.New("program/flow/containment: conflicting fallback containment parents")
		}
		if *slot != 0 {
			return errors.New("program/flow/containment: fallback overlaps ordinary parent")
		}
		*slot = edge.parent
		rootState[childIndex] = 2
	}
	return nil
}

func kernelParentNodes(
	counts [keyspace.FamilyCount]uint32,
	total uint32,
	offsets [keyspace.FamilyCount]uint32,
	parents [keyspace.FamilyCount][]keyspace.Term,
	rootState []uint8,
) ([]uint32, error) {
	invalid := total
	parentNodes := make([]uint32, int(total))
	for index := range parentNodes {
		parentNodes[index] = invalid
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			index := offsets[family] + ordinal - 1
			parent := parents[family][ordinal-1]
			if parent == 0 {
				if rootState[index] != 1 {
					return nil, fmt.Errorf("program/flow/containment: term %08x is missing a parent", uint32(keyspace.MakeTerm(family, ordinal)))
				}
				continue
			}
			parentIndex, ok := kernelIndex(parent, counts, offsets, total)
			if !ok {
				return nil, errors.New("program/flow/containment: parent is outside denominator")
			}
			if rootState[index] == 1 {
				return nil, errors.New("program/flow/containment: designated root has a parent")
			}
			parentNodes[index] = parentIndex
		}
	}
	return parentNodes, nil
}

// kernelCheckCycles follows each parent chain iteratively.  A state of one is
// the current chain and a state of two is a chain whose termination was
// already proved.  No recursion or depth cap is needed, and each node is
// entered and finalized once.
func kernelCheckCycles(parentNodes []uint32, total uint32, state []uint8) error {
	for index := range state {
		state[index] = 0
	}
	path := make([]uint32, 0)
	for start := uint32(0); start < total; start++ {
		if state[start] == 2 {
			continue
		}
		path = path[:0]
		current := start
		for current != total && state[current] == 0 {
			state[current] = 1
			path = append(path, current)
			current = parentNodes[current]
		}
		if current != total && state[current] == 1 {
			return errors.New("program/flow/containment: containment cycle")
		}
		for index := len(path); index > 0; index-- {
			state[path[index-1]] = 2
		}
	}
	return nil
}

func kernelIntervals(
	parentNodes []uint32,
	counts [keyspace.FamilyCount]uint32,
	offsets [keyspace.FamilyCount]uint32,
	total uint32,
	rootState []uint8,
) ([keyspace.FamilyCount][]uint32, [keyspace.FamilyCount][]uint32, error) {
	var pre, post [keyspace.FamilyCount][]uint32
	if total == 0 {
		return pre, post, nil
	}

	// Count into the following slot so the in-place prefix sum becomes the
	// child CSR starts.  This avoids keeping distinct count and start planes.
	childStart := make([]uint32, int(total)+1)
	for child := uint32(0); child < total; child++ {
		parent := parentNodes[child]
		if parent == total {
			continue
		}
		if parent >= total {
			return pre, post, errors.New("program/flow/containment: invalid parent index")
		}
		if childStart[parent+1] == ^uint32(0) {
			return pre, post, errors.New("program/flow/containment: child count overflow")
		}
		childStart[parent+1]++
	}

	for node := uint32(0); node < total; node++ {
		if uint64(childStart[node])+uint64(childStart[node+1]) > uint64(^uint32(0)) {
			return pre, post, errors.New("program/flow/containment: child index overflow")
		}
		childStart[node+1] += childStart[node]
	}
	childTotal := childStart[total]
	children := make([]uint32, int(childTotal))
	// preGlobal becomes the retained pre interval after it has served as the
	// fill cursor.  The final family slices below are views of this storage.
	preGlobal := make([]uint32, int(total))
	copy(preGlobal, childStart[:total])
	for child := uint32(0); child < total; child++ {
		parent := parentNodes[child]
		if parent == total {
			continue
		}
		at := preGlobal[parent]
		if at >= childTotal {
			return pre, post, errors.New("program/flow/containment: child index out of range")
		}
		children[at] = child
		preGlobal[parent] = at + 1
	}

	for node, parent := range parentNodes {
		rootState[node] = 0
		if parent == total {
			rootState[node] = 1
		}
		preGlobal[node] = 0
	}
	// Parent indexes have completed their last use.  Reuse their backing for
	// the retained post interval instead of allocating another dense plane.
	postGlobal := parentNodes
	for index := range postGlobal {
		postGlobal[index] = 0
	}
	type frame struct {
		node uint32
		next uint32
	}
	stack := make([]frame, 0)
	clock := uint32(0)
	for root := uint32(0); root < total; root++ {
		if rootState[root] == 0 {
			continue
		}
		if preGlobal[root] != 0 {
			return pre, post, errors.New("program/flow/containment: repeated containment root")
		}
		if clock == ^uint32(0) {
			return pre, post, errors.New("program/flow/containment: containment interval overflow")
		}
		clock++
		preGlobal[root] = clock
		stack = append(stack, frame{node: root, next: childStart[root]})
		for len(stack) != 0 {
			top := &stack[len(stack)-1]
			end := childStart[top.node+1]
			if top.next < end {
				child := children[top.next]
				top.next++
				if child >= total || preGlobal[child] != 0 {
					return pre, post, errors.New("program/flow/containment: malformed containment tree")
				}
				if clock == ^uint32(0) {
					return pre, post, errors.New("program/flow/containment: containment interval overflow")
				}
				clock++
				preGlobal[child] = clock
				stack = append(stack, frame{node: child, next: childStart[child]})
				continue
			}
			postGlobal[top.node] = clock
			stack = stack[:len(stack)-1]
		}
	}
	if clock != total {
		return pre, post, errors.New("program/flow/containment: unrooted containment component")
	}

	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := counts[family]
		if count == 0 {
			continue
		}
		start := offsets[family]
		end := start + count
		if end > total {
			return pre, post, errors.New("program/flow/containment: invalid family interval index")
		}
		pre[family] = preGlobal[start:end]
		post[family] = postGlobal[start:end]
	}
	return pre, post, nil
}

func kernelStatic(counts [keyspace.FamilyCount]uint32, marks []keyspace.Term) ([keyspace.FamilyCount][]uint64, error) {
	var static [keyspace.FamilyCount][]uint64
	for _, term := range marks {
		if !validTerm(term, counts) {
			return static, errors.New("program/flow/containment: invalid static containment mark")
		}
		family := keyspace.TermFamily(term)
		ordinal := keyspace.TermOrdinal(term)
		if static[family] == nil {
			wordCount := (uint64(counts[family]) + 63) / 64
			static[family] = make([]uint64, int(wordCount))
		}
		static[family][(ordinal-1)>>6] |= uint64(1) << ((ordinal - 1) & 63)
	}
	return static, nil
}

func kernelIndex(
	term keyspace.Term,
	counts [keyspace.FamilyCount]uint32,
	offsets [keyspace.FamilyCount]uint32,
	total uint32,
) (uint32, bool) {
	family := keyspace.TermFamily(term)
	ordinal := keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || ordinal > counts[family] {
		return 0, false
	}
	index := offsets[family] + ordinal - 1
	if index >= total {
		return 0, false
	}
	return index, true
}
