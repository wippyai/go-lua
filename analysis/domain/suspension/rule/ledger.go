package rule

// Family identifies one Suspension-owned writing judgment. It is an audit
// vocabulary only: Rule declarations remain the executable cold schema.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyModuleInit
	FamilyModuleInitCancel
	FamilyModuleInitSuccess
	FamilyYield
	FamilyResume
	FamilyCancel
	FamilyHandlerReentry
)

// Projection identifies the exact existing Link range required by a family.
// No entry introduces an inferred continuation/generation relation.
type Projection uint8

const (
	ProjectionInvalid Projection = iota
	ProjectionModuleInitGeneration
	ProjectionModuleInitTerminal
	ProjectionModuleInitOutcome
	ProjectionSuspensionOccurrence
	ProjectionResumeGenerationCorrespondence
	ProjectionCancelGenerationCorrespondence
	ProjectionHandlerReentryGenerationCorrespondence
)

// Availability distinguishes a present Link range awaiting its remaining
// explicit owner capabilities from a named structural projection that Link
// does not currently expose.
type Availability uint8

const (
	AvailabilityInvalid Availability = iota
	AvailabilityPresent
	AvailabilityAbsent
)

// LedgerEntry records whether this child can currently declare the complete
// unanchored schema from an existing finite Link projection. Missing entries
// are explicit projection gaps, never an authorization to infer a pairing.
type LedgerEntry struct {
	Family     Family
	Projection Projection
	Available  Availability
	Declared   bool
}

var ledger = [...]LedgerEntry{
	{Family: FamilyModuleInit, Projection: ProjectionModuleInitGeneration, Available: AvailabilityPresent, Declared: true},
	{Family: FamilyModuleInitCancel, Projection: ProjectionModuleInitTerminal, Available: AvailabilityPresent, Declared: true},
	{Family: FamilyModuleInitSuccess, Projection: ProjectionModuleInitOutcome, Available: AvailabilityPresent, Declared: true},
	// SuspensionOccurrence is structural (Application × operation × row), not
	// a selected Yield execution fact. Until canonical body
	// compilation binds that exact selected-operation support, admitting this
	// row as a zero-read generation source would let an unrelated dynamic
	// candidate mint a live continuation.
	{Family: FamilyYield, Projection: ProjectionSuspensionOccurrence, Available: AvailabilityPresent},
	{Family: FamilyResume, Projection: ProjectionResumeGenerationCorrespondence, Available: AvailabilityAbsent},
	{Family: FamilyCancel, Projection: ProjectionCancelGenerationCorrespondence, Available: AvailabilityAbsent},
	{Family: FamilyHandlerReentry, Projection: ProjectionHandlerReentryGenerationCorrespondence, Available: AvailabilityAbsent},
}

// Ledger returns a detached finite inventory fragment. It has no lookup,
// registration, or composition authority.
func Ledger() []LedgerEntry { return append([]LedgerEntry(nil), ledger[:]...) }
