// Package typefactor owns the sole runtime Type Factor over canonical Program
// subjects. Program supplies identities and evaluation order; typedomain owns
// labels and Packs; engine owns only generic fixed-point composition.
package typefactor

import (
	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/literal"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/values"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

// Domain owns one sealed type-label Table, one finite subject table, and one
// Type Factor. It exposes no product registry, typ graph, or mutable admission
// path after installation.
type Domain struct {
	solver *engine.Solver
	source *link.Link
	table  *typedomain.Table
	// universe is the Link-owned finite provenance authority of this exact
	// Factor installation.  It is closed before any rule can execute.
	universe *origin.Universe
	factor   *engine.Factor[link.Value, carrier.Value]
}

// Install compiles the Type domain in one direction: Link static authority →
// finite labels/closed Link values → sealed carrier Factor → domain Rules. No prior
// reduced-value domain is installed or consulted.
func Install(solver *engine.Solver, source *link.Link) (*Domain, bool) {
	if solver == nil || source == nil || !solver.OwnsLink(source) {
		return nil, false
	}
	static, ok := typeauthority.Seal(source)
	if !ok {
		return nil, false
	}
	table, err := typedomain.NewTable(static)
	if err != nil {
		return nil, false
	}
	literals, err := literal.Admit(source, table)
	if err != nil {
		return nil, false
	}
	table.Seal()
	universe, ok := origin.Build(source)
	if !ok {
		return nil, false
	}
	factorConfig, ok := config(source, table, universe)
	if !ok {
		return nil, false
	}
	factor, ok := engine.DeclareFactor(solver, factorConfig)
	if !ok || !literal.Declare(solver, source, table, universe, factor, literals) ||
		!values.Declare(solver, source, table, universe, factor) {
		return nil, false
	}
	return &Domain{solver: solver, source: source, table: table, universe: universe, factor: factor}, true
}

// Query observes subject at one exact root-activation Program coordinate.
// The two Terms are intentionally distinct: at is execution state; subject is
// the value, Values pack, or Cell whose type is requested there.
func (domain *Domain) Query(shard link.Shard, at, subjectTerm program.Term) (*engine.Query[link.Value, carrier.Value], bool) {
	if domain == nil || domain.solver == nil || domain.source == nil || domain.factor == nil {
		return nil, false
	}
	key, ok := domain.source.ValueOf(shard, subjectTerm)
	if !ok {
		return nil, false
	}
	return engine.DeclareQuery(domain.solver, domain.factor, shard, at, key)
}

// CandidateQuery is Query for one exact existing Link body Candidate. It does
// not infer a callee or construct an invocation identity.
func (domain *Domain) CandidateQuery(candidate link.Candidate, shard link.Shard, at, subjectTerm program.Term) (*engine.Query[link.Value, carrier.Value], bool) {
	if domain == nil || domain.solver == nil || domain.source == nil || domain.factor == nil {
		return nil, false
	}
	key, ok := domain.source.ValueOf(shard, subjectTerm)
	if !ok {
		return nil, false
	}
	return engine.DeclareCandidateQuery(domain.solver, domain.factor, candidate, shard, at, key)
}

// Table returns the immutable label authority needed to interpret query Pack
// handles. It has no admission authority after Install.
func (domain *Domain) Table() *typedomain.Table {
	if domain == nil {
		return nil
	}
	return domain.table
}
