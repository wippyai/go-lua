// Package value owns the canonical value registry and source-level value
// facts. Product and axis packages own the carrier itself; this package maps
// exact Program occurrences into that carrier.
package value

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueregistry "github.com/wippyai/go-lua/analysis/domain/value/registry"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

const occurrenceKey uint64 = 0

// Domain is the installed source-literal semantics tranche. It owns no
// separate source model: Program remains the authority for literal rows and
// Link remains the authority for their shard coordinates.
type Domain struct {
	solver *engine.Solver
	source *link.Link
	factor *engine.Factor[uint64, product.Value]
}

// Install declares the canonical source-literal Factor and rules for exactly
// the Program Nil, Bool, Integer, Float, and String rows. The Factor has no
// widening rank: its first source-literal tranche is valid only in acyclic equations,
// which the engine proves while sealing the demanded composition.
func Install(solver *engine.Solver, source *link.Link) (*Domain, bool) {
	if solver == nil || source == nil {
		return nil, false
	}
	registry := valueregistry.Registry()
	authority, ok := registryAuthority(registry)
	if !ok {
		return nil, false
	}
	factor, ok := engine.DeclareFactor(solver, engine.FactorConfig[uint64, product.Value]{
		Keys:        engine.KeySpace{End: 1},
		Semantic:    semanticFactor(authority),
		Lattice:     product.Domain(registry),
		Default:     product.Bottom(registry),
		Fingerprint: func(value product.Value) uint64 { return product.Hash(registry, value) },
	})
	if !ok || !installSourceRules(solver, source, factor, registry, authority) {
		return nil, false
	}
	return &Domain{solver: solver, source: source, factor: factor}, true
}

// Query observes one exact Entry-activation literal occurrence. It deliberately
// does not create candidate queries or infer values for calls, packs, Cells,
// tables, or static Program forms.
func (domain *Domain) Query(shard link.Shard, term program.Term) (*engine.Query[uint64, product.Value], bool) {
	if domain == nil || domain.solver == nil || domain.source == nil || domain.factor == nil {
		return nil, false
	}
	source, ok := domain.source.Program(shard)
	if !ok || source == nil {
		return nil, false
	}
	if !sourceLiteralTerm(source, term) {
		return nil, false
	}
	entry, ok := source.Entry()
	if !ok {
		return nil, false
	}
	activation, ok := source.Activation(term)
	if !ok || activation != entry {
		return nil, false
	}
	return engine.DeclareQuery(domain.solver, domain.factor, shard, term, occurrenceKey)
}
