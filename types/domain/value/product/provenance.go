package product

// Provenance is the diagnostic explanation sidecar carried beside an
// AbstractValue.
//
// It records why a value holds the abstraction it does (the originating
// construct), purely for diagnostics. It is deliberately excluded from Equal and
// Hash: two values that describe the same abstraction are equivalent regardless of
// how they were derived, so provenance never affects interning, lattice
// equivalence, or the db red-green firewall. Phase 7 carriers attach richer
// provenance; this carries the minimal label scaffolding.
type Provenance struct {
	// Origin is a short human-readable label for where the value came from.
	Origin string
}

// provenance is the heap-held sidecar referenced by an AbstractValue handle. It is
// a pointer so the absence of provenance costs nothing and so attaching provenance
// never touches the interned node.
type provenance struct {
	data Provenance
}

// newProvenance boxes provenance data for attachment to a handle.
func newProvenance(p Provenance) *provenance {
	return &provenance{data: p}
}
