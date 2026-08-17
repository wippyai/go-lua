package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	"github.com/wippyai/go-lua/internal/framing"
)

// Callable identities are scalar queries over the sealed Flow/Static/Source
// owners.  They deliberately return IDs and sealed boundary data instead of
// constructing Function/Formal/Cell wrapper objects for Artifact compilation.
// The equations are the former transformer equations, kept here at the
// Program owner so every cold compiler consumes one canonical result.
const (
	callableCellFormal       = uint64(1)
	callableCellVararg       = uint64(2)
	callableCellCaptureInner = uint64(3)
	callableCellCaptureOuter = uint64(4)
)

// writeProgramTerm frames one existing Program term for the formal identity
// queries that consume it. The term remains owned by keyspace; this helper
// only emits its canonical family and ordinal.
func writeProgramTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0 &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}

// FunctionID returns the Program-owned identity of one exact sealed Function
// boundary.  The boundary remains a Flow handle; only its scalar identity is
// allowed to cross into Artifact compilation.
func (input *Program) FunctionID(boundary flow.FunctionBoundary) (identity.ContentID, bool) {
	if !input.Available() || !boundary.Available() || !input.Flow().FunctionBoundaries().OwnsFunction(boundary) {
		return identity.ContentID{}, false
	}
	bodyTerm, bodyOK := boundary.Body()
	body, bodyOK := input.Body(bodyTerm)
	context := boundary.ContextID()
	if !bodyOK || !input.OwnsBody(body) || !context.Available() {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/function", input.ContentID(), func(writer *framing.Writer) bool {
		return writer.Bytes(context[:]) == nil
	})
	return id, id.Available()
}

// FunctionFormalAt returns the exact scalar identities for one fixed formal:
// formal role, lexical Cell role, storage Cell role, and optional declared
// Static type. A missing declared type is a valid unannotated formal and is
// represented by a false declared-type result while the row remains valid.
func (input *Program) FunctionFormalAt(boundary flow.FunctionBoundary, index int) (formalID, cellID, storageID, declaredTypeID identity.ContentID, ok bool) {
	if !input.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	functionID, functionOK := input.FunctionID(boundary)
	if !functionOK || !functionID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	term, termOK := boundary.FormalAt(index)
	bodyTerm, bodyOK := boundary.Body()
	body, bodyViewOK := input.Body(bodyTerm)
	pathID := body.PathID()
	if !termOK || term == 0 || !bodyOK || !bodyViewOK || !input.OwnsBody(body) || !pathID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	cellID = callableCellID(pathID, callableCellFormal, uint64(index))
	formalID = programSemanticID("program/transformer/formal", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Uint(uint64(index)) == nil && writer.Bytes(cellID[:]) == nil
	})
	storageID, storageOK := input.StorageCellID(term)
	if !cellID.Available() || !formalID.Available() || !storageOK || !storageID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	declaredTypeID, _ = input.declaredStaticTypeID(term)
	return formalID, cellID, storageID, declaredTypeID, true
}

// StorageCellID returns the canonical storage-role identity for an authored
// Cell term. This is shared by callable formals and the Flow storage columns;
// keeping the equation on Program prevents the two consumers from drifting.
func (input *Program) StorageCellID(term keyspace.Term) (identity.ContentID, bool) {
	if !input.Available() || term == 0 {
		return identity.ContentID{}, false
	}
	if _, _, _, cellOK := input.Flow().Authored().Storage().Cells().Get(term); !cellOK {
		return identity.ContentID{}, false
	}
	id := programRoleID("program/transformer/storage-cell", input.ContentID(), func(writer *framing.Writer) bool {
		return writeProgramTerm(writer, term)
	})
	return id, id.Available()
}

// FunctionVararg returns the exact optional open-input identities. No storage
// identity is invented because vararg has no fixed storage Cell row.
func (input *Program) FunctionVararg(boundary flow.FunctionBoundary) (id, cellID identity.ContentID, ok bool) {
	if !input.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	_, functionOK := input.FunctionID(boundary)
	term, termOK := boundary.Vararg()
	bodyTerm, bodyOK := boundary.Body()
	body, bodyViewOK := input.Body(bodyTerm)
	pathID := body.PathID()
	if !functionOK || !termOK || term == 0 || !bodyOK || !bodyViewOK || !input.OwnsBody(body) || !pathID.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	cellID = callableCellID(pathID, callableCellVararg, 0)
	id = programSemanticID("program/transformer/vararg", func(writer *framing.Writer) bool {
		return writer.Bytes(pathID[:]) == nil && writer.Bytes(cellID[:]) == nil
	})
	return id, cellID, id.Available() && cellID.Available()
}

// FunctionCaptureAt returns one ordered inner/outer capture edge and its
// scalar Cell and Body-path identities. The inner Body must be the callable's
// own Body; the outer Body must be a distinct existing sealed Body.
func (input *Program) FunctionCaptureAt(boundary flow.FunctionBoundary, index int) (id, innerID, outerID, innerBodyID, outerBodyID identity.ContentID, ok bool) {
	if !input.Available() || index < 0 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	functionID, functionOK := input.FunctionID(boundary)
	if !functionOK || !functionID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	pair, pairOK := boundary.CaptureAt(index)
	bodyTerm, bodyOK := boundary.Body()
	body, bodyViewOK := input.Body(bodyTerm)
	innerBody, innerBodyOK := input.Body(pair.InnerBody)
	outerBody, outerBodyOK := input.Body(pair.OuterBody)
	if !pairOK || pair.Inner == 0 || pair.Outer == 0 || !bodyOK || !bodyViewOK || !innerBodyOK || !outerBodyOK ||
		!input.OwnsBody(body) || !input.OwnsBody(innerBody) || !input.OwnsBody(outerBody) || !innerBody.Equal(body) || outerBody.Equal(innerBody) {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	innerBodyID, outerBodyID = innerBody.PathID(), outerBody.PathID()
	if !innerBodyID.Available() || !outerBodyID.Available() {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
	}
	innerID = callableCellID(innerBodyID, callableCellCaptureInner, uint64(index))
	outerID = callableCellID(outerBodyID, callableCellCaptureOuter, uint64(index))
	id = programSemanticID("program/transformer/capture", func(writer *framing.Writer) bool {
		return writer.Bytes(innerBodyID[:]) == nil && writer.Bytes(outerBodyID[:]) == nil &&
			writer.Uint(uint64(index)) == nil && writer.Bytes(innerID[:]) == nil && writer.Bytes(outerID[:]) == nil
	})
	return id, innerID, outerID, innerBodyID, outerBodyID, id.Available() && innerID.Available() && outerID.Available()
}

func callableCellID(bodyPath identity.ContentID, role, index uint64) identity.ContentID {
	return programSemanticID("program/transformer/cell-semantic", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Uint(role) == nil && writer.Uint(index) == nil
	})
}

func (input *Program) declaredStaticTypeID(cell keyspace.Term) (identity.ContentID, bool) {
	if !input.Available() || cell == 0 {
		return identity.ContentID{}, false
	}
	static := input.Static()
	declaration, declarationOK := static.Declarations().DeclaredTypes().ForCell(cell)
	declaredCell, target, rowOK := static.Declarations().DeclaredTypes().Get(declaration)
	ref, refOK := static.StaticTypes().Ref(target)
	id, idOK := programstatic.TypeReferenceID(input.ContentID(), ref)
	if !declarationOK || !rowOK || declaredCell != cell || !refOK || ref.Term() != target || !idOK {
		return identity.ContentID{}, false
	}
	return id, true
}
