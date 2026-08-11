package analysis

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// mountedBody is a transient Project-issued denominator row. It is not a
// compiler arm: it exists only to bind one detached result row and its Effect
// root after the common occurrence transaction completes.
type mountedBody struct {
	linked  *link.Link
	shard   linkproject.Shard
	program *program.Program
	module  string
	term    keyspace.Term
}

func mountedProgramBodies(source *link.Link) ([]mountedBody, bool) {
	if source == nil || source.Project() == nil || !source.ContentID().Available() {
		return nil, false
	}
	mounts := source.Project().Mounts()
	result := make([]mountedBody, 0)
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		p, programOK := mounts.Program(shard)
		module, moduleOK := mounts.Name(shard)
		if !shardOK || !programOK || !moduleOK || p == nil || module == "" || !p.ContentID().Available() {
			return nil, false
		}
		count := p.Flow().Executable().FamilyCount(keyspace.FamilyBody)
		for ordinal := 1; ordinal <= count; ordinal++ {
			term := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
			if term == 0 || !p.Flow().Executable().Contains(term) {
				return nil, false
			}
			result = append(result, mountedBody{linked: source, shard: shard, program: p, module: module, term: term})
		}
	}
	return result, len(result) != 0
}

func (body mountedBody) valid(source *link.Link) bool {
	if source == nil || body.linked != source || source.Project() == nil || body.program == nil || body.module == "" || keyspace.TermFamily(body.term) != keyspace.FamilyBody || !body.program.Flow().Executable().Contains(body.term) {
		return false
	}
	p, programOK := source.Project().Mounts().Program(body.shard)
	module, moduleOK := source.Project().Mounts().Name(body.shard)
	return programOK && moduleOK && p == body.program && module == body.module
}
