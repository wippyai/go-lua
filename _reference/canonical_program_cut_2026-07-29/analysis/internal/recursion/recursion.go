package recursion

// Guard tracks recursion depth.
type Guard struct {
	depth    int
	maxDepth int
}

// NewGuard creates a guard with the given max depth.
func NewGuard(maxDepth int) Guard {
	return Guard{
		depth:    0,
		maxDepth: maxDepth,
	}
}

// Enter advances the guard for a recursive step.
// Returns (nextGuard, true) if recursion should continue.
func (g Guard) Enter() (Guard, bool) {
	if g.depth > g.maxDepth {
		return g, false
	}

	next := g
	next.depth++
	return next, true
}
