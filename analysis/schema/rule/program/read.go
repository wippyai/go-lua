package program

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// ReadForm is the closed read vocabulary produced by the five-shape census.
// The five normal forms are derived from this form and the projections on a
// JoinDecl; there is intentionally no shape enum in the ABI.
type ReadForm = member.ReadForm

const (
	ReadFormInvalid = member.ReadFormInvalid
	Exact           = member.ReadFormExact
	Selected        = member.ReadFormSelected
	Summary         = member.ReadFormSummary
	Complete        = member.ReadFormComplete
)

// Order is the owner-declared order of a read's cells.
type Order uint8

const (
	OrderInvalid Order = iota
	OrderCanonical
	OrderByTag
	OrderOwner
)

func (order Order) Available() bool { return order >= OrderCanonical && order <= OrderOwner }

// Sparse states whether an absent coordinate stays absent or is materialized
// through an owner-declared default/dense denominator.
type Sparse uint8

const (
	SparseInvalid Sparse = iota
	SparseExplicit
	SparseDefault
	SparseDense
)

func (sparse Sparse) Available() bool { return sparse >= SparseExplicit && sparse <= SparseDense }

// OnOpaque is the only permitted treatment of authenticated opaque evidence.
// Widening is deliberately absent from this ABI; it belongs at an SCC boundary.
type OnOpaque uint8

const (
	OnOpaqueInvalid OnOpaque = iota
	OnOpaqueRefuse
	OnOpaquePropagateAuthenticated
)

func (onOpaque OnOpaque) Available() bool {
	return onOpaque == OnOpaqueRefuse || onOpaque == OnOpaquePropagateAuthenticated
}

// Multiplicity is the declared width of a read result.
type Multiplicity = member.Multiplicity

const (
	MultiplicityInvalid  = member.MultiplicityInvalid
	MultiplicityOptional = member.MultiplicityOptional
	MultiplicityOne      = member.MultiplicityOne
	MultiplicityMany     = member.MultiplicityMany
)

// ReadContract is explicit even when a form has a narrow implementation. A
// denominator is optional only for an exact sparse read whose empty state is
// merely NoCandidate; selected, summary, complete, default, and dense reads
// validate it as part of normal-form sealing.
type ReadContract struct {
	Order          Order
	Sparse         Sparse
	OnOpaque       OnOpaque
	Multiplicity   Multiplicity
	DenominatorRef DenominatorRef
}

func (contract ReadContract) Available() bool {
	return contract.Order.Available() && contract.Sparse.Available() &&
		contract.OnOpaque.Available() && contract.Multiplicity.Available()
}

// RequiresFactorDenominator is the one denominator law shared by declaration,
// plan, generated descriptor, emitter and runtime shape fences.
//
// A direct Summary or Complete read closes over a Factor's denominator and
// therefore needs its explicit address. A Summary addressed by a Parent or a
// KeyVector does not: it already owns its span denominator - the parent's
// member set, or the directory that publishes the vector - so demanding a
// second Factor denominator beside it would be two authorities over one span.
// Complete cannot be member-addressed at all, but the parameter stays explicit
// so every caller asks this one law instead of spelling its own.
func RequiresFactorDenominator(form ReadForm, sparse Sparse, memberAddressed bool) bool {
	if form == Summary && memberAddressed {
		return false
	}
	return form == Selected || form == Summary || form == Complete ||
		sparse == SparseDefault || sparse == SparseDense
}

// PointBoundDecl states whether a read's Input slot is backed by its own
// distinct predecessor topology point transported into this rule, or shares
// the candidate's own point because it resolves through its Factor's own
// directory/route surface at solve time.
//
// It is authored on the read, never derived from Form: the declaration
// corpus already carries both Exact and Selected reads on either side of
// this distinction, so Form alone does not settle it. PointBound is the
// ordinary disposition - a read whose Input slot geometry provides its own
// distinct predecessor. PointBoundSelf is the rare disposition a rule states
// explicitly when its selected/summary read resolves through its Factor's
// directory instead, and the transported predecessor at that Input slot is
// the candidate's own point.
type PointBoundDecl uint8

const (
	PointBoundInvalid PointBoundDecl = iota
	PointBound
	PointBoundSelf
)

func (bound PointBoundDecl) Available() bool {
	return bound == PointBound || bound == PointBoundSelf
}

// ReadDecl is inline on each JoinDecl. It is intentionally not a detached
// table keyed by an authored read identifier; the join's list position is the only result
// identity in the cold ABI.
type ReadDecl struct {
	Input      InputRef
	Axis       AxisRef
	Form       ReadForm
	Contract   ReadContract
	PointBound PointBoundDecl
}

func (read ReadDecl) Available() bool {
	return read.Axis.Available() && read.Form.Available() && read.Contract.Available() && read.PointBound.Available()
}

func (read ReadDecl) References() schema.EntryReferences {
	var references schema.EntryReferences
	if read.Axis.Declared() {
		references = append(references, read.Axis.EntryReference())
	}
	if read.Contract.DenominatorRef.Declared() {
		references = append(references, read.Contract.DenominatorRef.EntryReference())
	}
	return references
}
