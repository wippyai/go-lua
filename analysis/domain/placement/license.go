package placement

// LicenseState is the evidence state for a placement or optimization license.
// Unknown is deliberately distinct from Refuted: it means the analysis did not
// establish the license, while Refuted records evidence that it cannot hold.
type LicenseState uint8

const (
	LicenseUnknown LicenseState = iota
	LicenseRefuted
	LicenseProven
)

// Known reports whether the state carries either positive or negative evidence.
func (s LicenseState) Known() bool {
	return s == LicenseRefuted || s == LicenseProven
}

// Proven reports whether the license was established.
func (s LicenseState) Proven() bool {
	return s == LicenseProven
}

// Join combines must-proof evidence from alternate sources. A refutation
// absorbs, and a proof must survive every source to remain proven.
func (s LicenseState) Join(other LicenseState) LicenseState {
	if s == LicenseRefuted || other == LicenseRefuted {
		return LicenseRefuted
	}
	if s == LicenseUnknown || other == LicenseUnknown {
		return LicenseUnknown
	}
	return LicenseProven
}

// LicenseStateFor returns the evidence state for a known boolean proof.
func LicenseStateFor(proven bool) LicenseState {
	if proven {
		return LicenseProven
	}
	return LicenseRefuted
}

// LicenseKind identifies one allocation-site license. The set is deliberately
// closed so all projections can be checked for coverage.
type LicenseKind uint8

const (
	LicenseAllocationSite LicenseKind = iota
	LicenseDecomposable
	LicenseFrameLocalUse
	LicenseFrameLocal
	LicenseDiesBeforeSuspension
	licenseKindLimit
)

// AllocationSiteLicenseKinds returns every allocation-site license kind in
// stable order.
func AllocationSiteLicenseKinds() []LicenseKind {
	return []LicenseKind{
		LicenseAllocationSite,
		LicenseDecomposable,
		LicenseFrameLocalUse,
		LicenseFrameLocal,
		LicenseDiesBeforeSuspension,
	}
}

// AllocationSiteLicenses is the canonical per-allocation-site proof record.
// It stays independent of checker DTOs so body facts and placement plans use
// the same tri-state join.
type AllocationSiteLicenses struct {
	allocationSite       LicenseState
	decomposable         LicenseState
	frameLocalUse        LicenseState
	frameLocal           LicenseState
	diesBeforeSuspension LicenseState
}

// NewAllocationSiteLicenses derives the canonical record from solved
// allocation-site evidence. Missing placement and lifetime facts remain
// unknown rather than being silently converted into refutations.
func NewAllocationSiteLicenses(
	decomposable bool,
	frameLocalUse bool,
	diesBeforeSuspension bool,
	hasDiesBeforeSuspension bool,
	value Value,
	hasPlacement bool,
) AllocationSiteLicenses {
	licenses := AllocationSiteLicenses{}.
		With(LicenseAllocationSite, LicenseProven).
		With(LicenseDecomposable, LicenseStateFor(decomposable)).
		With(LicenseFrameLocalUse, LicenseStateFor(frameLocalUse))
	if hasDiesBeforeSuspension {
		licenses = licenses.With(LicenseDiesBeforeSuspension, LicenseStateFor(diesBeforeSuspension))
	}
	frameLocal := licenses.State(LicenseFrameLocalUse)
	if hasPlacement {
		frameLocal = frameLocal.Join(LicenseStateFor(value == Stack))
	} else {
		frameLocal = frameLocal.Join(LicenseUnknown)
	}
	return licenses.With(LicenseFrameLocal, frameLocal.Join(licenses.State(LicenseDiesBeforeSuspension)))
}

// AllocationSiteLicenseProjection is the total boolean DTO projection of an
// AllocationSiteLicenses record. Unknown projects conservatively to false;
// the lifetime presence bit preserves the one wire field that distinguishes
// unavailable evidence from a proven-negative lifetime.
type AllocationSiteLicenseProjection struct {
	AllocationSite          bool
	Decomposable            bool
	FrameLocalUseProof      bool
	FrameLocal              bool
	DiesBeforeSuspension    bool
	HasDiesBeforeSuspension bool
}

// Projection converts every allocation-site license into its wire/plan DTO
// field. Keeping this here makes the projection surface exhaustive and shared.
func (l AllocationSiteLicenses) Projection() AllocationSiteLicenseProjection {
	diesBeforeSuspension := l.State(LicenseDiesBeforeSuspension)
	return AllocationSiteLicenseProjection{
		AllocationSite:          l.State(LicenseAllocationSite).Proven(),
		Decomposable:            l.State(LicenseDecomposable).Proven(),
		FrameLocalUseProof:      l.State(LicenseFrameLocalUse).Proven(),
		FrameLocal:              l.State(LicenseFrameLocal).Proven(),
		DiesBeforeSuspension:    diesBeforeSuspension.Proven(),
		HasDiesBeforeSuspension: diesBeforeSuspension.Known(),
	}
}

// State returns the evidence state for kind. Invalid kinds are unknown.
func (l AllocationSiteLicenses) State(kind LicenseKind) LicenseState {
	switch kind {
	case LicenseAllocationSite:
		return l.allocationSite
	case LicenseDecomposable:
		return l.decomposable
	case LicenseFrameLocalUse:
		return l.frameLocalUse
	case LicenseFrameLocal:
		return l.frameLocal
	case LicenseDiesBeforeSuspension:
		return l.diesBeforeSuspension
	default:
		return LicenseUnknown
	}
}

// With returns a copy with the state for kind set to state. Invalid kinds are
// ignored so callers cannot manufacture a partial record.
func (l AllocationSiteLicenses) With(kind LicenseKind, state LicenseState) AllocationSiteLicenses {
	switch kind {
	case LicenseAllocationSite:
		l.allocationSite = state
	case LicenseDecomposable:
		l.decomposable = state
	case LicenseFrameLocalUse:
		l.frameLocalUse = state
	case LicenseFrameLocal:
		l.frameLocal = state
	case LicenseDiesBeforeSuspension:
		l.diesBeforeSuspension = state
	}
	return l
}

// Join combines every license using the shared must-proof evidence law.
func (l AllocationSiteLicenses) Join(other AllocationSiteLicenses) AllocationSiteLicenses {
	for _, kind := range AllocationSiteLicenseKinds() {
		l = l.With(kind, l.State(kind).Join(other.State(kind)))
	}
	return l
}
