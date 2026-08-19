package typ

import (
	"context"
	"sync"
)

const (
	recursiveHashSmallVisitedCap = 8
	recursiveHashSmallMemoCap    = 16
	recursiveHashSmallActiveCap  = 16
	// recursiveHashPooledMapCap bounds the overflow map size a pooled scratch
	// retains between uses; a larger map is dropped so a single pathological type
	// does not pin its memory in the pool indefinitely.
	recursiveHashPooledMapCap = 1024
)

var recursiveHashScratchPool = sync.Pool{New: func() any { return &recursiveHashScratch{} }}

// getRecursiveHashScratch returns a clean scratch. putRecursiveHashScratch must
// be called when the traversal completes so the scratch (and any overflow maps)
// is reused instead of reallocated on the next structural hash.
func getRecursiveHashScratch() *recursiveHashScratch {
	return recursiveHashScratchPool.Get().(*recursiveHashScratch)
}

func putRecursiveHashScratch(s *recursiveHashScratch) {
	s.reset()
	recursiveHashScratchPool.Put(s)
}

// reset returns the scratch to a pristine state, niling pointer-bearing slots so
// the pool retains no type references, and retaining bounded overflow maps
// (cleared) for reuse while dropping oversized ones.
func (s *recursiveHashScratch) reset() {
	s.ctx = nil
	s.steps = 0
	s.err = nil
	s.sawCycle = false
	for i := 0; i < s.visitedLen; i++ {
		s.visited[i] = nil
	}
	s.visitedLen = 0
	for i := 0; i < s.visitedGenericLen; i++ {
		s.visitedGeneric[i] = nil
	}
	s.visitedGenericLen = 0
	for i := 0; i < s.activeLen; i++ {
		s.active[i] = nil
	}
	s.activeLen = 0
	for i := 0; i < s.memoLen; i++ {
		s.memo[i] = recursiveHashMemoEntry{}
	}
	s.memoLen = 0
	s.visitedMap = clearOrDropHashScratchMap(s.visitedMap)
	s.visitedGenericMap = clearOrDropHashScratchMap(s.visitedGenericMap)
	s.activeMap = clearOrDropHashScratchMap(s.activeMap)
	s.memoMap = clearOrDropHashScratchMap(s.memoMap)
}

func clearOrDropHashScratchMap[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return nil
	}
	if len(m) > recursiveHashPooledMapCap {
		return nil
	}
	clear(m)
	return m
}

type recursiveHashMemoEntry struct {
	t Type
	h uint64
}

type recursiveHashScratch struct {
	ctx   context.Context
	steps uint64
	err   error

	// sawCycle records whether this call emitted a $self/$cycle sentinel
	// anywhere in the walk: a productive back-edge was crossed, so at least
	// one contributing value depended on which node the walk started from.
	// See the equalityHashCache.interior field comment.
	sawCycle bool

	visited    [recursiveHashSmallVisitedCap]*Recursive
	visitedLen int
	visitedMap map[*Recursive]bool

	// visitedGeneric anchors cycle detection for a self-referential generic
	// declaration on the *Generic pointer itself, mirroring visited/visitedMap
	// for *Recursive. An Instantiated node is a transparent application of its
	// Generic, not an identity of its own, so anchoring on the declaration
	// (rather than on whichever Instantiated wrapper happens to start the
	// traversal) makes the closed hash independent of which application is
	// queried first.
	visitedGeneric    [recursiveHashSmallVisitedCap]*Generic
	visitedGenericLen int
	visitedGenericMap map[*Generic]bool

	active    [recursiveHashSmallActiveCap]Type
	activeLen int
	activeMap map[Type]bool

	memo    [recursiveHashSmallMemoCap]recursiveHashMemoEntry
	memoLen int
	memoMap map[Type]uint64
}

// checkpoint bounds cancellation latency while hashing a recursive type. The
// traversal enters this once per visited node, so deeply nested or wide type
// graphs cannot keep a canceled caller busy indefinitely.
func (s *recursiveHashScratch) checkpoint() bool {
	if s == nil || s.err != nil {
		return false
	}
	s.steps++
	if s.ctx == nil || s.steps%64 != 0 {
		return true
	}
	if err := s.ctx.Err(); err != nil {
		s.err = err
		return false
	}
	return true
}

func (s *recursiveHashScratch) visitedContains(r *Recursive) bool {
	if s.visitedMap != nil {
		return s.visitedMap[r]
	}
	for i := 0; i < s.visitedLen; i++ {
		if s.visited[i] == r {
			return true
		}
	}
	return false
}

func (s *recursiveHashScratch) visitedPush(r *Recursive) {
	if s.visitedMap != nil {
		s.visitedMap[r] = true
		return
	}
	if s.visitedLen < len(s.visited) {
		s.visited[s.visitedLen] = r
		s.visitedLen++
		return
	}
	s.visitedMap = make(map[*Recursive]bool, len(s.visited)+1)
	for i := 0; i < s.visitedLen; i++ {
		s.visitedMap[s.visited[i]] = true
	}
	s.visitedMap[r] = true
}

func (s *recursiveHashScratch) visitedPop(r *Recursive) {
	if s.visitedMap != nil {
		delete(s.visitedMap, r)
		return
	}
	if s.visitedLen == 0 {
		return
	}
	s.visitedLen--
	s.visited[s.visitedLen] = nil
}

func (s *recursiveHashScratch) visitedContainsGeneric(g *Generic) bool {
	if s.visitedGenericMap != nil {
		return s.visitedGenericMap[g]
	}
	for i := 0; i < s.visitedGenericLen; i++ {
		if s.visitedGeneric[i] == g {
			return true
		}
	}
	return false
}

func (s *recursiveHashScratch) visitedPushGeneric(g *Generic) {
	if s.visitedGenericMap != nil {
		s.visitedGenericMap[g] = true
		return
	}
	if s.visitedGenericLen < len(s.visitedGeneric) {
		s.visitedGeneric[s.visitedGenericLen] = g
		s.visitedGenericLen++
		return
	}
	s.visitedGenericMap = make(map[*Generic]bool, len(s.visitedGeneric)+1)
	for i := 0; i < s.visitedGenericLen; i++ {
		s.visitedGenericMap[s.visitedGeneric[i]] = true
	}
	s.visitedGenericMap[g] = true
}

func (s *recursiveHashScratch) visitedPopGeneric(g *Generic) {
	if s.visitedGenericMap != nil {
		delete(s.visitedGenericMap, g)
		return
	}
	if s.visitedGenericLen == 0 {
		return
	}
	s.visitedGenericLen--
	s.visitedGeneric[s.visitedGenericLen] = nil
}

func (s *recursiveHashScratch) activeContains(t Type) bool {
	if s.activeMap != nil {
		return s.activeMap[t]
	}
	for i := 0; i < s.activeLen; i++ {
		if s.active[i] == t {
			return true
		}
	}
	return false
}

func (s *recursiveHashScratch) activePush(t Type) {
	if s.activeMap != nil {
		s.activeMap[t] = true
		return
	}
	if s.activeLen < len(s.active) {
		s.active[s.activeLen] = t
		s.activeLen++
		return
	}
	s.activeMap = make(map[Type]bool, len(s.active)+1)
	for i := 0; i < s.activeLen; i++ {
		s.activeMap[s.active[i]] = true
	}
	s.activeMap[t] = true
}

func (s *recursiveHashScratch) activePop(t Type) {
	if s.activeMap != nil {
		delete(s.activeMap, t)
		return
	}
	if s.activeLen == 0 {
		return
	}
	s.activeLen--
	s.active[s.activeLen] = nil
}

func (s *recursiveHashScratch) memoGet(t Type) (uint64, bool) {
	if s.memoMap != nil {
		h, ok := s.memoMap[t]
		return h, ok
	}
	for i := 0; i < s.memoLen; i++ {
		if s.memo[i].t == t {
			return s.memo[i].h, true
		}
	}
	return 0, false
}

func (s *recursiveHashScratch) memoSet(t Type, h uint64) {
	if s.memoMap != nil {
		s.memoMap[t] = h
		return
	}
	for i := 0; i < s.memoLen; i++ {
		if s.memo[i].t == t {
			s.memo[i].h = h
			return
		}
	}
	if s.memoLen < len(s.memo) {
		s.memo[s.memoLen] = recursiveHashMemoEntry{t: t, h: h}
		s.memoLen++
		return
	}
	s.memoMap = make(map[Type]uint64, len(s.memo)+1)
	for i := 0; i < s.memoLen; i++ {
		s.memoMap[s.memo[i].t] = s.memo[i].h
	}
	s.memoMap[t] = h
}
