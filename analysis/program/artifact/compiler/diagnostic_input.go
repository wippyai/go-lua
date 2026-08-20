package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

func declaredStaticTypeID(programID identity.ContentID, view staticquery.View, cell keyspace.Term) (identity.ContentID, bool) {
	if !programID.Available() || !view.Available() || cell == 0 {
		return identity.ContentID{}, false
	}
	declarations := view.Declarations().DeclaredTypes()
	declaration, declarationOK := declarations.ForCell(cell)
	declaredCell, target, rowOK := declarations.Get(declaration)
	ref, refOK := view.StaticTypes().Ref(target)
	id, idOK := staticquery.TypeReferenceID(programID, ref)
	if !declarationOK || !rowOK || declaredCell != cell || !refOK || ref.Term() != target || !idOK {
		return identity.ContentID{}, false
	}
	return id, true
}

func (compiler *compiler) diagnosticStorageBindIdentityAt(index int) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() || !compiler.key.ProgramID().Available() || index < 0 {
		return identity.ContentID{}, false
	}
	input, view := compiler.input, compiler.input.Flow()
	binds := view.Authored().Storage().Binds()
	term, present := binds.At(index)
	owner, values, related := binds.Get(term)
	width, widthOK := input.Source().Binds().Len(term)
	if !present || !related || !widthOK || width < 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return identity.ContentID{}, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
	entry, entryOK := view.Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	if !bodyOK || !bodyPath.Available() || !bodyID.Available() || !spanOK || !entryOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, false
	}
	return programschema.StorageBindIdentity(compiler.key.ProgramID(), bodyPath, width, bodyID, entry.ContextID(), finish.ContextID())
}
