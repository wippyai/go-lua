package program

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (program *Program) scalarIdentityAvailable() bool {
	if program == nil || program.source == nil || program.flow == nil || program.static == nil || program.module == nil || !program.id.Available() {
		return false
	}
	sourceID := program.source.Cold().ContentID()
	flowID := program.flow.ContentID()
	staticID := program.static.Cold().ContentID()
	moduleID := program.module.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return false
	}
	provenance := program.flow.View().Provenance()
	return provenance.Source == sourceID && provenance.Flow == flowID && provenance.Static == staticID && provenance.Module == moduleID
}

func (program *Program) scalarBody(owner keyspace.Term) (identity.ContentID, identity.ContentID, bool) {
	if !program.scalarIdentityAvailable() || owner == 0 {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	view := program.Flow()
	body, ok := view.FunctionBoundaries().ForBody(owner)
	if !ok || !body.Available() {
		return identity.ContentID{}, identity.ContentID{}, false
	}
	path, pathOK := view.BodyPath(owner)
	context := body.ContextID()
	return path, context, pathOK && path.Available() && context.Available()
}

func writeProgramTerm(writer *framing.Writer, term keyspace.Term) bool {
	return writer != nil && keyspace.TermFamily(term) != keyspace.FamilyInvalid && keyspace.TermOrdinal(term) != 0 &&
		writer.Uint(uint64(keyspace.TermFamily(term))) == nil && writer.Uint(uint64(keyspace.TermOrdinal(term))) == nil
}
