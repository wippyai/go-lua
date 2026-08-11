package rule

// Family identifies one Typestate-owned provider lifecycle judgment. It is a
// finite audit vocabulary, not a second Rule registry or execution path.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyAcquire
	FamilyTransition
	FamilyRetainRelease
	FamilySuspendResumeHandoff
	FamilyCancelCleanup
	FamilyExternalHandoff
)

// Projection identifies the exact existing Link range needed for the Rule's
// immutable operand identity.
type Projection uint8

const (
	ProjectionInvalid Projection = iota
	ProjectionOriginAcquisitionDeclaration
	ProjectionOriginApplicationTransitionDeclaration
	ProjectionResourceHolderActionIdentity
	ProjectionSuspensionResourceHandoffOutcome
	ProjectionCancellationResourceCleanupOutcome
	ProjectionExternalResourceHandoffOutcome
)

// Availability distinguishes a complete existing Link operand projection
// from a specifically named structural identity Link does not yet publish.
type Availability uint8

const (
	AvailabilityInvalid Availability = iota
	AvailabilityPresent
	AvailabilityAbsent
)

// LedgerEntry reports the sealed source/declaration authority required for a
// typed operand. Acquisition and transition use Typestate-derived Contract
// declarations; neither projects a Link protocol-composition relation.
type LedgerEntry struct {
	Family     Family
	Projection Projection
	Available  Availability
	Declared   bool
}

var ledger = [...]LedgerEntry{
	{Family: FamilyAcquire, Projection: ProjectionOriginAcquisitionDeclaration, Available: AvailabilityPresent, Declared: true},
	{Family: FamilyTransition, Projection: ProjectionOriginApplicationTransitionDeclaration, Available: AvailabilityPresent, Declared: true},
	{Family: FamilyRetainRelease, Projection: ProjectionResourceHolderActionIdentity, Available: AvailabilityAbsent},
	{Family: FamilySuspendResumeHandoff, Projection: ProjectionSuspensionResourceHandoffOutcome, Available: AvailabilityAbsent},
	{Family: FamilyCancelCleanup, Projection: ProjectionCancellationResourceCleanupOutcome, Available: AvailabilityAbsent},
	{Family: FamilyExternalHandoff, Projection: ProjectionExternalResourceHandoffOutcome, Available: AvailabilityAbsent},
}

// Ledger returns a detached finite inventory fragment with no lookup,
// registry, or composition authority.
func Ledger() []LedgerEntry { return append([]LedgerEntry(nil), ledger[:]...) }
