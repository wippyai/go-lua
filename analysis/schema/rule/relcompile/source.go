package relcompile

import (
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// Declaration is the small, resolved input accepted by the first relational
// lowering pass.  The owning declaration surfaces resolve their names to
// nominal relation/column/key identities before handing this value to the
// compiler; relcompile does not keep a second name registry or a physical
// address table.
//
// Relations, columns, keys, scopes, and semantic signatures are copied into
// the resulting plan. Rules contain only relational facts: a base relation,
// ordered equijoins, an optional scope, optional completion, one typed Apply,
// and one publication. Parent, predicate, correspondence, route, activation
// and transport are all represented by the same JoinSpec/column vectors.
type Declaration struct {
	SchemaID  model.SchemaID
	Relations []model.RelationSchema
	Columns   []model.ColumnSchema
	// TypeCapabilities are the owner-declared policies carried by the
	// nominal TypeIDs in this schema. Relcompile transports them unchanged;
	// it never infers a policy from a Go value or a Presence contract.
	TypeCapabilities []model.TypeCapability
	Keys             []model.KeySchema
	Scopes           []model.ScopeSchema
	Signatures       []signature.Signature
	// Initials are the complete owner-authored zero-input invocation rows.
	// They are sealed into the logical schema and never supplied as runtime
	// admission side data.
	Initials []plan.Initial
	Rules    []Rule
}

// Rule is one declaration-to-expression specimen. The compiler does not
// interpret a domain role or choose a runtime form; it lowers these generic
// relational facts into the closed algebra.
type Rule struct {
	ID         model.DependencyID
	Expression model.ExpressionID
	Candidate  model.RelationID
	Joins      []JoinSpec
	// ApplySlots names the exact declared read occurrence that supplies each
	// semantic operation slot. It is resolved from Program.Fold.Inputs (or
	// authored explicitly for a non-Program specimen such as RawGet/RawSet),
	// before Compile sees any tuple layout. No relation/column name lookup is
	// allowed to choose an occurrence later.
	ApplySlots []ReadOccurrence
	Scope      model.ScopeID
	Complete   *model.DenominatorRef
	Apply      signature.Identity
	// Output is the one sealed destination geometry for the terminal Apply.
	// It is authored by the owning plan and transported unchanged; Compile
	// never infers a slot, cardinality mode, or owner-named route.
	Output algebra.OutputAddress
	// ApplyShape is the explicit multi-child terminal shape.  Its children
	// are independently lowered relational expressions; Slots is the exact
	// sealed operation-input to child/cell map.  It exists for operations whose
	// inputs retain distinct range authorities and therefore cannot be
	// flattened into one joined tuple without losing ownership.  The legacy
	// Candidate/Joins/ApplySlots fields remain the single-child authoring form.
	ApplyShape *ApplyShape
	Carry      *CarrySpec
	Publish    *Publication
}

// ApplyShape is the schema-owned declaration of one terminal multi-child
// operation.  Each child is lowered from its own candidate/joins/scope/
// completion declaration.  Slots are positional and exact: one entry per
// signature input, with no relation/column search or runtime correlation
// inference. Correlation is the owner-issued query-site coordinate and exact
// per-child projection map used to replay heterogeneous ranges; it is
// required for this shape and passed unchanged to the sealed algebra
// contract.
//
// This is intentionally a declaration shape, not a runtime cache or an
// alternate relation representation.  The compiler emits the ordinary
// algebra.Apply node and its existing immutable ApplyContract.
type ApplyShape struct {
	Children    []ApplyChild
	Slots       []algebra.SlotSource
	Correlation algebra.ApplyCorrelation
	// Output is the sealed destination geometry of this heterogeneous Apply.
	Output algebra.OutputAddress
}

// ApplyChild is one independently lowered child expression.  Its geometry is
// the same generic relational vocabulary used by Rule: a candidate followed
// by ordered joins, optional scope filtering, and optional complete
// denominator materialization.
type ApplyChild struct {
	Candidate model.RelationID
	Joins     []JoinSpec
	Scope     model.ScopeID
	Complete  *model.DenominatorRef
}

type readOccurrenceKind uint8

const (
	readOccurrenceInvalid readOccurrenceKind = iota
	readOccurrenceCandidate
	readOccurrenceJoin
)

// ReadOccurrence names one authored row occurrence in a Rule's relational
// input. CandidateOccurrence is the base candidate row; JoinOccurrence names
// the right-hand relation input of one ordered Rule.Joins entry. The relation
// may repeat: the ordinal is the authority that keeps the occurrences apart.
type ReadOccurrence struct {
	kind readOccurrenceKind
	join uint32
}

func CandidateOccurrence() ReadOccurrence { return ReadOccurrence{kind: readOccurrenceCandidate} }

func JoinOccurrence(index uint32) ReadOccurrence {
	return ReadOccurrence{kind: readOccurrenceJoin, join: index}
}

func (occurrence ReadOccurrence) Candidate() bool {
	return occurrence.kind == readOccurrenceCandidate
}

func (occurrence ReadOccurrence) Join() (uint32, bool) {
	return occurrence.join, occurrence.kind == readOccurrenceJoin
}

// resolveOccurrenceSource is the one cold lowering map from an authored
// occurrence to the retained tuple source. CandidateOccurrence is resolved
// by the owner-issued candidate relation in the sealed layout; it is never
// addressed by a slot/ordinal convention. A joined occurrence carries the
// source identity assigned when its authored join was lowered.
func resolveOccurrenceSource(occurrence ReadOccurrence, candidate model.RelationID, layout tupleLayout) (uint32, bool) {
	switch occurrence.kind {
	case readOccurrenceCandidate:
		for index, source := range layout.sources {
			if source == candidate {
				return uint32(index), true
			}
		}
		return 0, false
	case readOccurrenceJoin:
		ordinal := occurrence.join + 1
		if int(ordinal) >= len(layout.sources) {
			return 0, false
		}
		return ordinal, true
	default:
		return 0, false
	}
}

func (occurrence ReadOccurrence) available(joinCount int) bool {
	if occurrence.kind == readOccurrenceCandidate {
		return occurrence.join == 0
	}
	return occurrence.kind == readOccurrenceJoin && int(occurrence.join) < joinCount
}

// CarrySpec is the alternative derivation a rule publishes for the rows its
// semantic operation did not produce: the destination relation observed at the
// carried input port, optionally transformed by one further typed operation.
// It is combined with the operation's own rows by the destination key's
// declared algebra, which is Merge and never a form of its own.
type CarrySpec struct {
	Relation model.RelationID
	Scope    model.ScopeID
	// Columns is the exact semantic writable output layout this carried fact
	// contributes to its destination Merge. The destination row/key remains
	// authenticated by the tuple source; address cells are never fabricated as
	// semantic carry values.
	Columns   []model.ColumnID
	Transform *signature.Identity
	// Output is the sealed destination geometry of the optional transform
	// Apply. It is distinct from the enclosing rule's output because the
	// transform reads the carried expression and publishes its own row.
	Output algebra.OutputAddress
}

// JoinSpec is one oriented equijoin. The vectors are typed by the relation
// and column registries in Declaration. A parent relation, predicate/tag
// relation, correspondence, route source, activation branch, or transport
// axis is not a distinct engine mechanism: it is just a declared relation and
// a matching column vector.
type JoinSpec struct {
	Relation     model.RelationID
	LeftColumns  []model.ColumnID
	RightColumns []model.ColumnID
	// Expand is the model-owned dependent join contract. When present this
	// JoinSpec is not an equijoin: the compiler emits algebra.Expand over the
	// current C-left expression and appends the R row columns. The ordinary
	// column vectors remain empty in this form.
	Expand *model.ExpandContract
	// Scope is the decision scope the joined rows are observed at. It is the
	// relational statement of the input port a read was declared on: two reads
	// naming one port observe one scope.
	Scope model.ScopeID
	// Complete is the authenticated denominator this join closes over. It is
	// present exactly when the authored read materializes an absent coordinate
	// through a denominator, and absent when the read stays sparse.
	Complete *model.DenominatorRef
}

// Publication names the sole logical write destination. The engine later
// decides how to arrange or commit it; relcompile emits only Publish.
type Publication struct {
	Relation model.RelationID
	Key      model.KeyID
	// Result is the sealed output-axis signature key.  It is carried from the
	// owner writer binding so a destination Projection.Result can be checked
	// against the exact output signature rather than accepted by role alone.
	Result carrier.Key
	// Columns is the exact writable semantic subset committed by this output
	// declaration. One Program Fold output normally yields one column, even
	// when its destination row has additional address/key cells.
	Columns []model.ColumnID
}
