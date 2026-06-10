package typ

// Freeze marks the recursive node and its reachable recursive descendants as
// immutable input. After freezing, SetBody is a no-op, so a shared stdlib,
// manifest, DB, or cache type graph cannot be mutated by any compilation that
// references it.
func (r *Recursive) Freeze() {
	FreezeType(r)
}

// FreezeType walks t and freezes every reachable recursive node, marking the whole
// type graph as immutable input. Stdlib types are frozen once at library init so
// any recursive body reached through them is immutable to every compilation.
func FreezeType(t Type) {
	if t == nil {
		return
	}
	// Contains is the canonical cycle-safe structural scanner; a predicate that
	// freezes each recursive node it visits and never short-circuits walks the
	// whole graph exactly once.
	Contains(t, func(n Type) bool {
		if rec, ok := n.(*Recursive); ok && rec != nil {
			rec.frozen = true
		}
		return false
	})
}
