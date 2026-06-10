package recursion

// Hashable is a minimal interface for types that can be tracked by a hash.
type Hashable interface {
	Hash() uint64
}

// Guard tracks recursion depth and optional cycle detection.
type Guard struct {
	depth    int
	maxDepth int
	seen     map[uint64]struct{}
}

// NewGuard creates a guard with the given max depth.
func NewGuard(maxDepth int) Guard {
	return Guard{
		depth:    0,
		maxDepth: maxDepth,
		seen:     nil,
	}
}

// WithSeen enables cycle detection using hashes.
func (g Guard) WithSeen() Guard {
	if g.seen == nil {
		g.seen = make(map[uint64]struct{})
	}

	return g
}

// Enter advances the guard for a recursive step.
// Returns (nextGuard, true) if recursion should continue.
func (g Guard) Enter(h Hashable) (Guard, bool) {
	if g.depth > g.maxDepth {
		return g, false
	}

	next := g
	next.depth++

	if h != nil && next.seen != nil {
		hash := h.Hash()
		if _, ok := next.seen[hash]; ok {
			return next, false
		}

		next.seen = copySeen(next.seen)
		next.seen[hash] = struct{}{}
	}

	return next, true
}

// Depth returns the current recursion depth of the guard.
func (g Guard) Depth() int {
	return g.depth
}

func copySeen(src map[uint64]struct{}) map[uint64]struct{} {
	if len(src) == 0 {
		return make(map[uint64]struct{})
	}

	dst := make(map[uint64]struct{}, len(src)+1)
	for k := range src {
		dst[k] = struct{}{}
	}

	return dst
}
