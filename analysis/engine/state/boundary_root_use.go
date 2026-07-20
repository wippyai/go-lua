package state

// BoundaryRootUse declares the complete way one registered factor consumes
// boundary roots.  It is dependency metadata, not a second implementation of
// the boundary law: the registered boundaryRoots operation remains the sole
// concrete semantic authority.
//
// Guarded execution uses this declaration to transpose roots independently.
// A factor that only needs reachability must never be aligned with root scalar
// values, and slot/path consumers must receive one joined destination root at
// a time rather than the Cartesian tuple of every root.
type BoundaryRootUse struct {
	declared     bool
	reachability bool
	slotValues   bool
	pathValues   bool
}

// EstablishesReachability reports whether a non-Bottom slot root or any path
// root changes the factor's reachable spelling.
func (u BoundaryRootUse) EstablishesReachability() bool { return u.reachability }

// ReadsSlotValues reports whether the factor consumes exact scalar values for
// destination roots that own a value slot.
func (u BoundaryRootUse) ReadsSlotValues() bool { return u.slotValues }

// ReadsPathValues reports whether the factor consumes exact scalar values for
// destination roots that own a structural path.
func (u BoundaryRootUse) ReadsPathValues() bool { return u.pathValues }

func boundaryRootUseNone() BoundaryRootUse {
	return BoundaryRootUse{declared: true}
}

func boundaryRootUseReachability() BoundaryRootUse {
	return BoundaryRootUse{declared: true, reachability: true}
}

func boundaryRootUseSlotValues() BoundaryRootUse {
	return BoundaryRootUse{declared: true, slotValues: true}
}

func boundaryRootUsePathValuesAndReachability() BoundaryRootUse {
	return BoundaryRootUse{declared: true, reachability: true, pathValues: true}
}

// BoundaryRootUse returns the exact registered root dependency of lane.
func (d ProductDomain) BoundaryRootUse(lane ProductLane) (BoundaryRootUse, error) {
	runtime, err := d.validateLane(lane)
	if err != nil {
		return BoundaryRootUse{}, err
	}
	if !runtime.ops.boundaryRootUse.declared {
		return BoundaryRootUse{}, ErrIncompleteLaneFactors
	}
	return runtime.ops.boundaryRootUse, nil
}
