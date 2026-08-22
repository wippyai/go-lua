// Package rowidentity owns the construction identities shared by the
// compiler's callable, storage, call, module, and diagnostic projections.
// They consume only canonical Flow/Static views and publish no compiler state.
package rowidentity

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	programstorage "github.com/wippyai/go-lua/analysis/program/storage"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
)

// StorageCellID is the root-fenced identity of one authored storage Cell.
func StorageCellID(programID identity.ContentID, view flow.View, term keyspace.Term) (identity.ContentID, bool) {
	if !programID.Available() || term == 0 {
		return identity.ContentID{}, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(term); !cellOK {
		return identity.ContentID{}, false
	}
	return lifecycle.StorageCellIdentity(programID, term)
}

// StorageBindID is the single Artifact construction admission for one
// authored Bind identity. Storage and diagnostics consume this same proof;
// neither owns a private reconstruction of the row identity.
func StorageBindID(input *program.Program, programID identity.ContentID, index int) (identity.ContentID, bool) {
	if input == nil || !input.Available() || !programID.Available() || index < 0 {
		return identity.ContentID{}, false
	}
	view := input.Flow()
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
	return programstorage.StorageBindIdentity(programID, bodyPath, width, bodyID, entry.ContextID(), finish.ContextID())
}

// DeclaredStaticType resolves an authored Cell declaration through Static's
// canonical DeclaredTypes relation. It returns the authored target term and
// its published node identity together, so consumers never have to recover a
// term by inverting an identity or maintain a second index.
func DeclaredStaticType(programID identity.ContentID, view staticquery.View, cell keyspace.Term) (keyspace.Term, identity.ContentID, bool) {
	if !programID.Available() || !view.Available() || cell == 0 {
		return 0, identity.ContentID{}, false
	}
	declarations := view.Declarations().DeclaredTypes()
	declaration, declarationOK := declarations.ForCell(cell)
	declaredCell, target, rowOK := declarations.Get(declaration)
	ref, refOK := view.StaticTypes().Ref(target)
	id, idOK := staticquery.TypeReferenceID(programID, ref)
	if !declarationOK || !rowOK || declaredCell != cell || !refOK || ref.Term() != target || !idOK {
		return 0, identity.ContentID{}, false
	}
	return target, id, id.Available()
}

// DeclaredStaticTypeID resolves the published identity of an authored Cell
// declaration. DeclaredStaticType is the canonical query; this narrow helper
// remains for callers that only need the identity.
func DeclaredStaticTypeID(programID identity.ContentID, view staticquery.View, cell keyspace.Term) (identity.ContentID, bool) {
	_, id, ok := DeclaredStaticType(programID, view, cell)
	return id, ok
}
