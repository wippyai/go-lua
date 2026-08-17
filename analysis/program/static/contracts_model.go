package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// FunctionContract and CallContract are authored static sidecars for Flow's
// Function and Call terms. They retain only static syntax: Flow remains the
// sole owner of callable evaluation, formals, bodies, callee selection, and
// runtime argument occurrences.
type FunctionContract struct {
	TypeParams   []keyspace.Term
	ReturnsKnown bool
	Returns      []keyspace.Term
}

// CallContract contains only authored static type arguments. Runtime actual
// occurrences remain owned by Flow's Values relation.
type CallContract struct{ TypeArguments []keyspace.Term }

// ContractsInput is dense by canonical Function and Call ordinal. Empty rows
// are meaningful: they distinguish an omitted return clause from an explicit
// known-empty one, without inventing a second Function or Call identity.
type ContractsInput struct {
	Function []FunctionContract
	Call     []CallContract
}

type contractsStore struct {
	functions []functionContractRow
	calls     []poolRange
	// callTypeArgumentIDs are immutable per-call type-argument column
	// identities. Authored terms remain in terms; this is only their stable
	// content identity.
	callTypeArgumentIDs []identity.ContentID
	terms               []keyspace.Term
}

type functionContractRow struct {
	typeParams   poolRange
	returnsKnown bool
	returns      poolRange
}

// Contracts exposes the two dense sidecars without becoming a second Flow
// graph. Functions and Calls are exact canonical-family views.
type Contracts struct {
	component *Component
	state     *draftState
}
type Functions struct {
	component *Component
	state     *draftState
}
type Calls struct {
	component *Component
	state     *draftState
}
