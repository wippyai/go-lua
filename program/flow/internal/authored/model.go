// Package authored owns the construction-only authored Flow relations. It
// stores canonical Program Terms only and never imports Source, Static, Link,
// Target, or an analysis domain.
package authored

import (
	"sync"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Range selects one contiguous interval in an owner-local dense Term pool.
type Range struct{ Start, End uint32 }

func validFieldKind(value kind.FieldKind) bool {
	return value >= kind.FieldList && value <= kind.FieldKey
}

func validLoopKind(value kind.LoopKind) bool {
	return value >= kind.LoopWhile && value <= kind.LoopGenericFor
}

// Value is authored Values structure. Evaluation ports are deliberately not
// present: they await the eventual whole-Flow finalization boundary.
type Value struct {
	Owner keyspace.Term
	Fixed Range
	Tail  keyspace.Term
}

// ValuesInput is the complete authored dense Values relation.
type ValuesInput struct {
	Rows  []Value
	Terms []keyspace.Term
}

// Table is authored table-constructor structure.
type Table struct {
	Owner  keyspace.Term
	Fields Range
}

// Field is authored table-constructor structure. Entry, successor, and
// normalized exact-key ports are seal projections, never content-addressed
// source semantics.
type Field struct {
	Table  keyspace.Term
	Key    keyspace.Term
	Values keyspace.Term
	Kind   kind.FieldKind
}

// TablesInput is the complete authored Table and TableField relation.
type TablesInput struct {
	Rows   []Table
	Fields []Field
	Order  []keyspace.Term
}

// ExactLens is an authored static access operation. Source is either a
// Source-owned name Key or an exact static value candidate, according to Kind.
// Its normalized atom is a later Source-joined seal projection.
type ExactLens struct {
	Owner  keyspace.Term
	Base   keyspace.Term
	Source keyspace.Term
	Kind   kind.FieldKind
}

// DynamicLens is an authored base-then-dynamic-key access operation.
type DynamicLens struct {
	Owner keyspace.Term
	Base  keyspace.Term
	Key   keyspace.Term
}

// AccessInput is the complete authored access relation.
type AccessInput struct {
	Exact   []ExactLens
	Dynamic []DynamicLens
}

// CellKind explicitly distinguishes lexical storage from Program-global
// storage. A global Cell uses a Source-owned canonical atom Key; no
// invalid-family or sentinel Term encodes that distinction.
type CellKind uint8

const (
	CellLocal CellKind = iota + 1
	CellGlobal
)

func (kind CellKind) valid() bool { return kind == CellLocal || kind == CellGlobal }

// Cell is authored storage. Exactly one coordinate is populated: Body for a
// local Cell, or Key for a global Cell. Key is a canonical Source-owned atom
// handle; its Source spelling/form is validated only by whole-Flow seal.
type Cell struct {
	Kind CellKind
	Body keyspace.Term
	Key  keyspace.Key
}

// Read is one authored storage or access observation. Implicit is occurrence
// provenance for a binder-proven unresolved global; its sparse query index is
// derived from these rows and is never separately authored.
type Read struct {
	Owner    keyspace.Term
	Source   keyspace.Term
	Implicit bool
}

// Vararg is an authored open occurrence anchored to a local Cell.
type Vararg struct {
	Owner keyspace.Term
	Cell  keyspace.Term
}

// Bind owns only its evaluated Values relation. Source owns its Cell order.
type Bind struct {
	Owner  keyspace.Term
	Values keyspace.Term
}

// Assign owns its evaluated Values relation. Its Write range is derived from
// the authored Write parent rows.
type Assign struct {
	Owner  keyspace.Term
	Values keyspace.Term
}

// Write is one ordered assignment commit to a Cell or Lens target.
type Write struct {
	Assign keyspace.Term
	Target keyspace.Term
}

// StorageInput is the complete authored storage relation.
type StorageInput struct {
	Cells   []Cell
	Reads   []Read
	Varargs []Vararg
	Binds   []Bind
	Assigns []Assign
	Writes  []Write
}

// Function is authored closure structure. Source owns formals and Static owns
// its contract; Flow owns only the executable body, optional vararg Cell, and
// capture bindings.
type Function struct {
	Owner    keyspace.Term
	Body     keyspace.Term
	Vararg   keyspace.Term
	Captures Range
}

// Capture binds one fresh local Cell in a Function body to an outer local
// Cell. Captures have no Term family: their dense, binder-ordered pool is
// owned by their Function range.
type Capture struct {
	Inner keyspace.Term
	Outer keyspace.Term
}

// FunctionsInput is the complete authored Function and flat Capture relation.
type FunctionsInput struct {
	Rows     []Function
	Captures []Capture
}

// Call is an authored dynamic call. Receiver is zero for ordinary call syntax;
// a nonzero receiver is the evaluated base of the matching named ExactLens
// callee observation.
type Call struct {
	Owner    keyspace.Term
	Callee   keyspace.Term
	Receiver keyspace.Term
	Actuals  keyspace.Term
}

// Return, Break, Label, and Goto are direct authored control evidence.
// Source owns their per-Body source order; Flow does not infer positions,
// exits, or lexical targets beyond Goto's resolved Label identity.
type Return struct {
	Owner  keyspace.Term
	Values keyspace.Term
}

type Break struct{ Owner keyspace.Term }
type Label struct{ Owner keyspace.Term }

type Goto struct {
	Owner  keyspace.Term
	Target keyspace.Term
}

// Branch is an authored scalar decision with two distinct child Bodies.
type Branch struct {
	Owner               keyspace.Term
	Condition           keyspace.Term
	WhenTrue, WhenFalse keyspace.Term
}

// Loop is one authored loop header. Cells indexes ControlInput.Cells using a
// dense owner-local CSR range; every listed Cell is a local Cell of Body.
type Loop struct {
	Owner   keyspace.Term
	Body    keyspace.Term
	Kind    kind.LoopKind
	Control keyspace.Term
	Cells   Range
}

// ControlInput is the complete authored control relation. It deliberately
// excludes Source-owned body order and ControlFault rows.
type ControlInput struct {
	Returns  []Return
	Breaks   []Break
	Labels   []Label
	Gotos    []Goto
	Branches []Branch
	Loops    []Loop
	Cells    []keyspace.Term
}

// OperatorsInput is the complete authored scalar-operator relation. Candidate
// classification, evaluation ports, and metamethod applications are derived by
// the future whole-Flow finalizer and have no authored representation here.
type OperatorsInput struct {
	Unaries  []Unary
	Binaries []Binary
	Selects  []Select
}

// Input is the authored Flow boundary. Counts is an allocation plan used to
// validate canonical Terms. It is not retained by the component.
type Input struct {
	Counts     [keyspace.FamilyCount]uint32
	Values     ValuesInput
	Access     AccessInput
	Storage    StorageInput
	Tables     TablesInput
	Functions  FunctionsInput
	Calls      []Call
	Control    ControlInput
	Operators  OperatorsInput
	Claims     []ValueClaim
	TypeValues []TypeValue
}

type valueStore struct {
	rows  []Value
	terms []keyspace.Term
}

type tableStore struct {
	rows   []Table
	fields []Field
	order  []keyspace.Term
}

type accessStore struct {
	exact   []ExactLens
	dynamic []DynamicLens
}

type storageStore struct {
	cells       []Cell
	reads       []Read
	implicit    []keyspace.Term
	varargs     []Vararg
	binds       []Bind
	assigns     []Assign
	writes      []Write
	assignWrite []Range
}

type functionStore struct {
	rows     []Function
	captures []Capture
}

type callStore struct{ rows []Call }

type operatorStore struct {
	unaries  []Unary
	binaries []Binary
	selects  []Select
}

type authoredControlStore struct {
	returns  []Return
	breaks   []Break
	labels   []Label
	gotos    []Goto
	branches []Branch
	loops    []Loop
	cells    []keyspace.Term
}

// component is the immutable owner-fenced authored Flow authority. It is
// deliberately unexported: only a committed View may retain it.
type component struct {
	contentID       keyspace.ContentID
	values          valueStore
	access          accessStore
	storage         storageStore
	tables          tableStore
	functions       functionStore
	calls           callStore
	operators       operatorStore
	claims          claimStore
	authoredControl authoredControlStore
}

type draftPhase uint8

const (
	draftOpen draftPhase = iota + 1
	draftClaimed
	draftCommitted
	draftAborted
)

// draftState is intentionally shared by copied Draft and Finalizer values:
// claiming and terminally consuming the authored component is a linear
// capability rather than a copyable handle.
type draftState struct {
	mu        sync.Mutex
	component *component
	phase     draftPhase
}

// Draft owns only authored Flow until exactly one Finalizer claims and
// terminally consumes it.
type Draft struct{ state *draftState }

// Finalizer is the owner-defined one-shot publication capability. Copies
// share Draft state, so exactly one terminal Commit or Abort can win.
type Finalizer struct{ state *draftState }

// viewAccess is embedded by every typed view. A committed View has no state
// and is direct immutable storage; a Finalizer.View has state and expires when
// that shared state reaches either terminal phase.
type viewAccess struct {
	component *component
	state     *draftState
}

func (view viewAccess) active() bool {
	if view.component == nil {
		return false
	}
	if view.state == nil {
		return true
	}
	state := view.state
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.phase == draftClaimed && state.component != nil
}

// View is the only authored Flow surface that can be retained after Commit.
// A View returned by Finalizer.View is lifecycle-bound; the View returned by
// Finalizer.Commit is direct immutable storage and remains valid.
type View struct{ viewAccess }

// Values is the direct Values relation view.
type Values struct{ viewAccess }

// Tables is the direct table-constructor relation view.
type Tables struct{ viewAccess }

// Fields is the direct TableField relation view.
type Fields struct{ viewAccess }

// Access partitions Flow's authored access rows into exact and dynamic views.
type Access struct{ viewAccess }
type ExactLenses struct{ viewAccess }
type DynamicLenses struct{ viewAccess }

// Storage partitions Flow's authored storage rows into compact direct views.
type Storage struct{ viewAccess }
type Cells struct{ viewAccess }
type Reads struct{ viewAccess }
type Varargs struct{ viewAccess }
type Binds struct{ viewAccess }
type Assigns struct{ viewAccess }
type Writes struct{ viewAccess }

// Functions and Calls expose the compact authored closure and application
// relations. Capture rows remain function-local pairs rather than Terms.
type Functions struct{ viewAccess }
type Calls struct{ viewAccess }

// Operators partitions Flow's authored scalar operator rows by their canonical
// Term family. It exposes no evaluation or candidate projection.
type Operators struct{ viewAccess }
type Unaries struct{ viewAccess }
type Binaries struct{ viewAccess }
type Selects struct{ viewAccess }

// Control partitions Flow's direct authored control relations.
type Control struct{ viewAccess }
type Returns struct{ viewAccess }
type Breaks struct{ viewAccess }
type Labels struct{ viewAccess }
type Gotos struct{ viewAccess }
type Branches struct{ viewAccess }
type Loops struct{ viewAccess }

// Claims and TypeValues expose direct scalar authored Flow rows. Static owns
// every target sidecar; Flow retains only their executable occurrence shape.
type Claims struct{ viewAccess }
type TypeValues struct{ viewAccess }

// Cold is the immutable, allocation-free identity snapshot of authored Flow.
// It deliberately retains no component pointer: callers that keep a cold
// identity must not retain the full authored Flow graph through it.
type Cold struct{ contentID keyspace.ContentID }

func (view View) Values() Values         { return Values{viewAccess: view.viewAccess} }
func (view View) Access() Access         { return Access{viewAccess: view.viewAccess} }
func (view View) Storage() Storage       { return Storage{viewAccess: view.viewAccess} }
func (view View) Tables() Tables         { return Tables{viewAccess: view.viewAccess} }
func (view View) Fields() Fields         { return Fields{viewAccess: view.viewAccess} }
func (view View) Functions() Functions   { return Functions{viewAccess: view.viewAccess} }
func (view View) Calls() Calls           { return Calls{viewAccess: view.viewAccess} }
func (view View) Operators() Operators   { return Operators{viewAccess: view.viewAccess} }
func (view View) Control() Control       { return Control{viewAccess: view.viewAccess} }
func (view View) Claims() Claims         { return Claims{viewAccess: view.viewAccess} }
func (view View) TypeValues() TypeValues { return TypeValues{viewAccess: view.viewAccess} }
func (view View) Cold() Cold {
	if !view.active() {
		return Cold{}
	}
	return Cold{contentID: view.component.contentID}
}

func (c Cold) ContentID() keyspace.ContentID {
	return c.contentID
}
