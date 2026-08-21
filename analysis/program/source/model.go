// Package source owns Program's immutable authored source preimage and its
// seal-derived source-position projection. It owns no evaluation geometry.
package source

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	sealedindex "github.com/wippyai/go-lua/analysis/program/source/index"
)

// Span is one source coordinate. The source name is owned once by Input.
type Span struct {
	File      string
	StartLine uint32
	StartCol  uint32
	EndLine   uint32
	EndCol    uint32
}

// Coordinate is a filename-free secondary source token coordinate. Source
// owns the filename once; consumers render a Coordinate only through an
// immutable Identity. Its zero value denotes absence and is never a token.
type Coordinate struct {
	startLine uint32
	startCol  uint32
	endLine   uint32
	endCol    uint32
}

// FamilySpans supplies the final root-allocated cardinality and one dense span
// per ordinal for a canonical Term family. Families must occur once in order.
type FamilySpans struct {
	Family keyspace.Family
	Spans  []Span
}

// NilLiteral, BoolLiteral, IntegerLiteral, FloatLiteral, and StringLiteral
// have no Kind field: canonical Term family is their only discriminator.
type NilLiteral struct{ Owner keyspace.Term }
type BoolLiteral struct {
	Owner keyspace.Term
	Value bool
}
type IntegerLiteral struct {
	Owner keyspace.Term
	Value int64
}
type FloatLiteral struct {
	Owner keyspace.Term
	Bits  uint64
}
type StringLiteral struct {
	Owner keyspace.Term
	Value string
}

// BodySource owns one Body's exact direct authored source order.
type BodySource struct {
	Body  keyspace.Term
	Terms []keyspace.Term
}

// BindCells owns one Bind's exact Cell order.
type BindCells struct {
	Bind  keyspace.Term
	Cells []keyspace.Term
}

// FunctionFormals owns one Function's exact formal Cell order.
type FunctionFormals struct {
	Function keyspace.Term
	Formals  []keyspace.Term
}

// CellSpelling is the authored debug name for one lexical Cell.  Cell rows
// are dense and keyed by the existing FamilyCell term; an empty Name means
// that the cell has no authored spelling (for example a compiler-created
// temporary or an anonymous capture).
type CellSpelling struct {
	Cell keyspace.Term
	Name string
}

// CallSpelling is an optional authored debug name for one Call.  Dynamic or
// otherwise unnamed calls have no row.  Rows are stored in FamilyCall ordinal
// order and never create a second call identity authority.
type CallSpelling struct {
	Call keyspace.Term
	Name string
}

// Input is the authored Source boundary. Final Terms are allocated by root;
// Source only validates dense family ownership and retains the supplied rows.
type Input struct {
	Name      string
	Families  []FamilySpans
	Nil       []NilLiteral
	Bool      []BoolLiteral
	Integer   []IntegerLiteral
	Float     []FloatLiteral
	String    []StringLiteral
	Bodies    []BodySource
	Binds     []BindCells
	Functions []FunctionFormals
	// CellSpellings is the dense authored debug-spelling column. A nil slice
	// is accepted as the all-absent dense column for generic non-Lua fixtures;
	// an explicit slice must contain exactly one row per Cell ordinal.
	CellSpellings []CellSpelling
	// CallSpellings is sparse: only statically named authored calls are
	// supplied. Dynamic/unknown calls are represented by no row.
	CallSpellings []CallSpelling
	// ExactAtoms is the complete dense exact-key denominator for Program.
	// Source normalizes and interns it once; all later components receive only
	// Source-owned Key handles and cannot create a second atom authority.
	ExactAtoms []keyspace.LiteralValue
	Keys       []KeyInput
	Faults     []ControlFault
}

// Position is one root-seal/Layout result. Frontier already contains the
// exact Repeat-loop adjustment; Source never reads Flow to reconstruct it.
type Position struct {
	Term           keyspace.Term
	Root           keyspace.Term
	Body           keyspace.Term
	Offset         uint32
	Cursor         uint32
	FrontierBody   keyspace.Term
	FrontierCursor uint32
	// Repeat is the sole exception to the ordinary direct-root frontier. Flow's
	// position seal sets it only for the admitted repeat kind and chooses the
	// exact Loop child; Source validates only the direct Loop root, Body forest
	// child relation, and child-tail frontier geometry.
	Repeat bool
}

// IndexInput is the sole typed containment/position handoff from Flow seal.
// It is validated, compacted into Source-private indexes, then discarded.
type IndexInput struct {
	// SourceID is the scalar Source owner fence for this sealed projection.
	// Commit accepts the payload only when it matches the authored authority's
	// content identity exactly; the index is never a second Source authority.
	SourceID identity.ContentID
	// Positions is Flow's exact ordered position batch for the reachable
	// containment closure. Position.Term is the sole identity of each row; rows
	// must be in explicit (TermFamily, TermOrdinal) order. Every actual direct
	// Body source occurrence must be covered, while a typed child Body may have
	// no direct source occurrence. Flow does not emit Terms outside the closure;
	// Source validates only the local rows and direct-root completeness without
	// importing Flow or reconstructing that closure. The batch is retained by
	// neither Draft nor Component.
	Positions []Position
	// OutcomeOrigins is Flow's one canonical ordered Outcome origin batch.
	// Outcomes are derived Terms: Source Build rejects authored Outcome rows,
	// and Finalize derives their count/spans from these Body origins before it
	// validates the sparse Positions batch. The batch is consumed
	// during Finalize and is never retained or content-addressed.
	OutcomeOrigins []keyspace.Term
}

type termRange struct{ start, end uint32 }

type identityStore struct {
	name           string
	spans          [keyspace.FamilyCount][]storedSpan
	outcomeOrigins []keyspace.Term
}

// storedSpan omits the repeated source file. Public queries reconstruct File
// from identityStore.name exactly as the prior Program API did.
type storedSpan struct {
	startLine uint32
	startCol  uint32
	endLine   uint32
	endCol    uint32
}

type literalStore struct {
	nil     []NilLiteral
	bool    []BoolLiteral
	integer []IntegerLiteral
	float   []FloatLiteral
	string  []StringLiteral
}

type orderStore struct {
	sourceTerms  []keyspace.Term
	bodyRanges   []termRange
	bindTerms    []keyspace.Term
	bindRanges   []termRange
	formalTerms  []keyspace.Term
	formalRanges []termRange
}

type spellingStore struct {
	// cells is always dense after Build, including zero-value entries for
	// omitted generic-fixture metadata.
	cells []string
	// calls is sparse and remains in ascending FamilyCall ordinal order.
	calls []CallSpelling
}

// familyCount reads the canonical sealed row storage. Authored families are
// represented by their span rows; the derived Outcome family is represented by
// its ordered Body origins. No separate cardinality plane is retained.
func (identity *identityStore) familyCount(family keyspace.Family) int {
	if identity == nil || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount {
		return 0
	}
	if family == keyspace.FamilyOutcome {
		return len(identity.outcomeOrigins)
	}
	return len(identity.spans[family])
}

func (identity *identityStore) termCount() (uint32, bool) {
	if identity == nil {
		return 0, false
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		total += uint64(identity.familyCount(family))
	}
	if total == 0 || total > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(total), true
}

type authority struct {
	identity  identityStore
	literals  literalStore
	order     orderStore
	spellings spellingStore
	keys      keyFaultStore
	index     *sealedindex.Table
	cellRoles *cellRoleAuthority
	content   identity.ContentID
}

type draftPhase uint8

const (
	draftOpen draftPhase = iota
	draftFinalizerClaimed
	draftTerminal
)

// draftState is shared by every copied Draft and Finalizer value. A Draft is
// not an independently copyable lifecycle flag: claiming a Finalizer and the
// first terminal Commit/Abort consume the authority for every copy.
type draftState struct {
	mu        sync.Mutex
	authority *authority
	phase     draftPhase
}

// Draft fences construction-only Source access until its one-shot Finalizer
// is claimed. Copies share the same lifecycle state.
type Draft struct{ state *draftState }

// Finalizer is the owner-issued Source sealing capability. Its Preimage method
// exposes the authored views needed by the sibling Flow finalizer. A Finalizer
// value is copyable, but all copies share the Draft lifecycle and exactly one
// terminal Commit or Abort can consume it.
type Finalizer struct{ state *draftState }

// Preimage is the lifecycle-bound authored Source query surface issued by a
// claimed Finalizer. It retains only the shared Draft fence: every typed
// subview resolves its owner through that fence, so a caller cannot splice a
// foreign authority into a live Preimage. Preimage has no terminal operation;
// it expires when its issuing Finalizer (or any copy) commits or aborts.
type Preimage struct{ state *draftState }

// Component is the immutable Source owner published by Program root.
type Component struct{ authority *authority }

// View is the direct Source capability; subviews partition its owner surface.
type View struct{ authority *authority }
type Identity struct {
	authority *authority
	state     *draftState
}
type Order struct {
	authority *authority
	state     *draftState
}
type BindOrder struct {
	authority *authority
	state     *draftState
}
type FormalOrder struct {
	authority *authority
	state     *draftState
}
type Spellings struct {
	authority *authority
	state     *draftState
}
type Index struct{ authority *authority }
type Literals struct {
	authority *authority
	state     *draftState
}
type Nils struct {
	authority *authority
	state     *draftState
}
type Bools struct {
	authority *authority
	state     *draftState
}
type Integers struct {
	authority *authority
	state     *draftState
}
type Floats struct {
	authority *authority
	state     *draftState
}
type Strings struct {
	authority *authority
	state     *draftState
}

// Keys is Source's sole exact-key authority: authored key spelling rows and
// the complete normalized atom denominator share one capability.
type Keys struct {
	authority *authority
	state     *draftState
}
type Faults struct {
	authority *authority
	state     *draftState
}
