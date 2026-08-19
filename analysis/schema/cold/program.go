package cold

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Program is the published fact of one mounted module key: the frozen
// compiled program mounted there, together with the identities that
// authenticate it.
//
// The frozen program is carried by value. That is what makes one compiled
// program shareable across every mount of it without a copy: a Frozen shares
// its published structure on assignment and admits no derivation, so the same
// value can sit in the row of two mounts, in two Links, for as long as any of
// them lives, and address the same content throughout.
//
// The row carries the module key it is stored under so that a row handed
// onward is self-describing. A consumer that received the frozen program and
// the module key as two arguments would have to keep them consistent itself,
// which is the shape this column exists to remove.
type Program struct {
	Frozen     snapshot.Frozen
	ModuleKey  identity.ContentID
	ArtifactID identity.ContentID
	ProgramID  identity.ContentID
	SchemaID   identity.ContentID
}

// Available reports whether row names a mounted program. A row is available
// only when the frozen program is a sealed publication and every identity
// that authenticates it is present, so a partially assembled mount can never
// be mistaken for a mounted one.
func (row Program) Available() bool {
	return row.Frozen.Published() && row.ModuleKey.Available() &&
		row.ArtifactID.Available() && row.ProgramID.Available() && row.SchemaID.Available()
}

// catalog is the identity this program's cold publication is addressed under.
// It is derived rather than carried, so a row cannot be assembled with a
// catalog that disagrees with the declaration catalog it names.
func (row Program) catalog() (identity.ContentID, bool) {
	if !row.Available() {
		return identity.ContentID{}, false
	}
	return CatalogID(row.SchemaID)
}

// CallTargetCount is the sealed width of this program's call-target family.
func (row Program) CallTargetCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return CallTargetFamily().Count(&row.Frozen, catalog)
}

// CallTargetAt returns one closure-allocation-to-callable-body proof by its
// position in the emitted sequence.
func (row Program) CallTargetAt(index int) (CallTarget, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return CallTarget{}, false
	}
	return CallTargetFamily().At(&row.Frozen, catalog, index)
}

// ExactScalarSummaryCount is the sealed width of this program's exact scalar
// summary family.
func (row Program) ExactScalarSummaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return ExactScalarSummaryCount(&row.Frozen, catalog)
}

// ExactScalarSummaryAt returns one exact scalar proof by its emitted ordinal.
func (row Program) ExactScalarSummaryAt(index int) (ExactScalarSummary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return ExactScalarSummary{}, false
	}
	return ExactScalarSummaryAt(&row.Frozen, catalog, index)
}

// ArithmeticSummaryCount is the sealed width of this program's arithmetic
// summary family.
func (row Program) ArithmeticSummaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return ArithmeticSummaryCount(&row.Frozen, catalog)
}

// ArithmeticSummaryAt returns one arithmetic proof by its emitted ordinal.
func (row Program) ArithmeticSummaryAt(index int) (ArithmeticSummary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return ArithmeticSummary{}, false
	}
	return ArithmeticSummaryAt(&row.Frozen, catalog, index)
}

// UnarySummaryCount is the sealed width of this program's unary summary
// family.
func (row Program) UnarySummaryCount() (int, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return 0, false
	}
	return UnarySummaryCount(&row.Frozen, catalog)
}

// UnarySummaryAt returns one unary proof by its emitted ordinal.
func (row Program) UnarySummaryAt(index int) (UnarySummary, bool) {
	catalog, derived := row.catalog()
	if !derived {
		return UnarySummary{}, false
	}
	return UnarySummaryAt(&row.Frozen, catalog, index)
}
