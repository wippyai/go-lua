package boundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// boundaryStorageReadIdentityAt is the narrow pre-Artifact admission needed
// while Boundary seals its semantic value directory. Flow/Source prove the
// authored Read and its exact evaluation endpoints; schema/program owns the
// scalar equation.
func boundaryStorageReadIdentityAt(input *program.Program, index int) (identity.ContentID, keyspace.Term, bool) {
	if input == nil || !input.Available() || index < 0 {
		return identity.ContentID{}, 0, false
	}
	view := input.Flow()
	reads := view.Authored().Storage().Reads()
	term, present := reads.At(index)
	owner, source, _, related := reads.Get(term)
	if !present || !related || term == 0 || owner == 0 || source == 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, 0, false
	}
	if _, _, _, cellOK := view.Authored().Storage().Cells().Get(source); !cellOK {
		return identity.ContentID{}, 0, false
	}
	bodyPath, bodyID, bodyOK := view.BodyContextIDs(owner)
	readPath, readPathOK := view.SemanticTermPath(term)
	_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
	entry, entryOK := view.Causal().Sites().ForTerm(entryTerm)
	finish, finishOK := view.Causal().Sites().ForTerm(finishTerm)
	if !bodyOK || !readPathOK || !readPath.Available() || !spanOK || !entryOK || !finishOK || !entry.Available() || !finish.Available() {
		return identity.ContentID{}, 0, false
	}
	id, idOK := programschema.StorageReadIdentity(input.ContentID(), bodyPath, bodyID, readPath, entry.ContextID(), finish.ContextID())
	return id, term, idOK && id.Available()
}
