package reachability

import (
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

const key uint64 = 0

// Domain is the installed reachability query route. It owns no Program
// topology: Install reads it from the sealed Link.
type Domain struct {
	solver *engine.Solver
	factor *engine.Factor[uint64, Value]
}

// Install declares the reachability Factor and transfers it over every
// existing activation Edge in source. Only a shard's actual Entry is seeded;
// selected function bodies need their own future Link boundary Rule to become
// reachable.
func Install(solver *engine.Solver, source *link.Link) (*Domain, bool) {
	if solver == nil || source == nil {
		return nil, false
	}
	factor, ok := engine.DeclareFactor(solver, PlaneConfig())
	if !ok || !installProgramRules(solver, source, factor) {
		return nil, false
	}
	return &Domain{solver: solver, factor: factor}, true
}

// Query observes may reachability at one exact root-activation Program term.
// The engine owns demand and State publication; this domain adds no query
// semantics of its own.
func (domain *Domain) Query(shard link.Shard, term program.Term) (*engine.Query[uint64, Value], bool) {
	if domain == nil || domain.solver == nil || domain.factor == nil {
		return nil, false
	}
	return engine.DeclareQuery(domain.solver, domain.factor, shard, term, key)
}
