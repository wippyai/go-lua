// Package frame seals the domain-abstract access relation for one compiled
// rule frame. It knows only dense compiler-issued roots, equality, and
// directed boundary-following; it has no engine, factor, State, or identity
// authority.
package frame

// Root is a dense compiler-owned frame-root ordinal. Zero is permanently
// invalid; valid roots are in [1, Spec.Roots].
type Root uint32

// Equality says that Left and Right denote the same frame root. Equality is
// symmetric and transitive when Compile seals a Spec.
type Equality struct {
	Left  Root
	Right Root
}

// Follow is a directed access relation. It runs from an inner/formal role to
// the root it denotes at the enclosing/current boundary. Compile never adds
// the reverse relation.
type Follow struct {
	From Root
	To   Root
}

// Projection is one active Rule's statically known access summary. Known is
// deliberately explicit: false means that this Rule's access mapping is not
// bounded by the declaration, so its lists are not meaningful input.
type Projection struct {
	Known    bool
	MayRead  []Root
	MayWrite []Root
}

// Spec is the complete compiler-owned declaration for one frame closure.
// Roots is a cardinality, not a Root: the valid dense ordinals are 1 through
// Roots. Input slices are read during Compile and never retained.
type Spec struct {
	Roots       int
	Equalities  []Equality
	Follows     []Follow
	Projections []Projection
}

// Closure is an immutable, domain-abstract access answer. A valid unknown
// closure is intentionally non-authorizing: both membership methods return
// false. Invalid input returns nil, false from Compile rather than a partial
// closure.
type Closure struct {
	roots int
	read  []uint64
	write []uint64
	valid bool
	known bool
}

// Valid reports whether Compile accepted the complete declaration.
func (closure *Closure) Valid() bool {
	return closure != nil && closure.valid
}

// Known reports whether every active Rule supplied a bounded projection.
func (closure *Closure) Known() bool {
	return closure.Valid() && closure.known
}

// MayRead reports whether Root lies in the least read closure. Unknown and
// invalid closures fail closed.
func (closure *Closure) MayRead(root Root) bool {
	return closure.member(closure.read, root)
}

// MayWrite reports whether Root lies in the least write closure. Unknown and
// invalid closures fail closed.
func (closure *Closure) MayWrite(root Root) bool {
	return closure.member(closure.write, root)
}

func (closure *Closure) member(bits []uint64, root Root) bool {
	if !closure.Known() || !validRoot(root, closure.roots) {
		return false
	}
	index := int(root - 1)
	return bits[index>>6]&(uint64(1)<<uint(index&63)) != 0
}

func validRoot(root Root, roots int) bool {
	return roots >= 0 && root != 0 && uint64(root) <= uint64(roots)
}
