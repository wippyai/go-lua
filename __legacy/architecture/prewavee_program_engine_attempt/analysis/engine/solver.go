package engine

import (
	"crypto/sha256"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// Solver owns one sealed Link analysis composition.  Program and Link remain
// the only source/control authorities; Solver owns only Factor schema, Rules,
// private scheduling, and the disposable fixed-point carrier.
type Solver struct {
	link       *link.Link
	coordinate *coordinate.Table
	owner      *solverOwner

	factors []factorDeclaration
	rules   []ruleDeclaration
	queries []queryDeclaration

	bodies           map[bodyOrigin]compiledBody
	activationBodies map[activationOrigin]compiledActivation
	equationCaches   []artifact.EquationCache
	cacheBodies      map[bodyOrigin]compiledBody

	actions           []compiledAction
	schedule          *schedule.Schedule
	regions           []compiledRegion
	presenceFollowers [][]int
	roots             []coordinate.Coordinate
	queryRoots        []coordinate.Coordinate
	queryResults      [][]compiledQueryResult
	entrySeeds        map[coordinate.Coordinate]struct{}

	// active is the finite monotone set of accepted resolved Relation
	// applications. It is never a State payload or second graph; each carrier
	// epoch compiles a disposable derived view from this set.
	active         []activeRelation
	supportCatalog []supportBinding
	supportTargets []int

	guards        *guard.Manager
	decisionAtoms map[decisionOrigin]guard.Atom
	epoch         *factEpoch
	facts         *facts.Schema
	zero          facts.Facts
	entry         facts.Facts

	sealed     bool
	evaluating bool
}

var equationCacheEngineKey = SemanticKey{
	ID:      program.ContentID(sha256.Sum256([]byte("go-lua/engine/body-equation-cache"))),
	Version: 1,
}

type solverOwner struct{ marker byte }

// decisionOrigin is a private lookup key for one existing Program decision
// under one exact activation provenance.
type decisionOrigin struct {
	candidate link.Candidate
	shard     link.Shard
	term      program.Term
}

type edgeOrigin struct {
	candidate link.Candidate
	shard     link.Shard
	edge      program.Edge
}

// compiledRegion is private WTO metadata.  It owns no semantic transport;
// Program's exact Mu edges remain the sole recurrence vocabulary.
type compiledRegion struct {
	head     int
	outer    bool
	narrow   bool
	slots    []int
	supports []int
	members  []int
}

// New binds an empty demanded-coordinate table to one sealed Link.  It does
// not enumerate Candidates or infer an application family.
func New(source *link.Link, caches ...artifact.EquationCache) (*Solver, error) {
	if source == nil || !source.ContentID().Available() {
		return nil, errors.New("engine: unavailable Link")
	}
	coordinates, ok := coordinate.New(source)
	if !ok {
		return nil, errors.New("engine: unavailable Link coordinate table")
	}
	if !artifact.EquationCachesFit(caches) {
		return nil, errors.New("engine: equation cache resource limit")
	}
	owned := make([]artifact.EquationCache, 0, len(caches))
	for _, cache := range caches {
		owned = append(owned, cloneEquationCache(cache))
	}
	return &Solver{link: source, coordinate: coordinates, owner: &solverOwner{}, equationCaches: owned}, nil
}

// OwnsLink reports whether source names the sealed Link authority this Solver
// was constructed for. It compares content identity rather than Go pointer
// identity, so an independently decoded equal Link is accepted while a
// different project cannot accidentally register Rules by colliding dense
// shard or Term ordinals.
//
// This is an admission fence only. It exposes neither the Solver's Link nor
// its coordinates, and it remains valid before and after Solver.Seal.
func (solver *Solver) OwnsLink(source *link.Link) bool {
	if solver == nil || solver.link == nil || source == nil {
		return false
	}
	owned, supplied := solver.link.ContentID(), source.ContentID()
	return owned.Available() && supplied.Available() && owned == supplied
}

// Seal fixes schema and Rule declarations.  Relation endpoints remain lazy:
// only a live selector can resolve them during Solve.
func (solver *Solver) Seal() bool {
	if solver == nil || solver.sealed || solver.evaluating || len(solver.factors) == 0 {
		return false
	}
	if !solver.refreshCarrier(nil) || !solver.validateRules() || !solver.validateQueries() || !solver.compileBodies() {
		return false
	}
	if !solver.compileQueries(nil) {
		return false
	}
	if solver.epoch == nil || !solver.epoch.commit() {
		return false
	}
	solver.sealed = true
	return true
}

// refreshCarrier constructs a fresh finite Guard/Factor generation from the
// exact demanded activations and active relations.  There is no cap or budget:
// a later discovered finite relation causes another finite epoch.
func (solver *Solver) refreshCarrier(active []activeRelation) bool {
	if solver == nil {
		return false
	}
	atoms, decisions, ok := solver.rootDecisionAtoms(active)
	if !ok {
		return false
	}
	guards, err := guard.New(atoms)
	if err != nil {
		return false
	}
	epoch, ok := newFactEpoch(guards, solver.factors)
	if !ok || epoch == nil || epoch.schema == nil || !epoch.valid(epoch.zero) || !epoch.valid(epoch.entry) {
		return false
	}
	solver.guards, solver.decisionAtoms = guards, decisions
	solver.epoch = epoch
	solver.facts, solver.zero, solver.entry = epoch.schema, epoch.zero, epoch.entry
	return true
}

// rootDecisionAtoms scales with demanded activation instances, not with every
// project shard.  A Seed has no CandidateBody and contributes no phantom
// decision activation; relations using a Seed can still execute at the
// caller's real Program term.
func (solver *Solver) rootDecisionAtoms(active []activeRelation) ([]guard.Atom, map[decisionOrigin]guard.Atom, bool) {
	origins, ok := solver.decisionBatch(active, true)
	if !ok {
		return nil, nil, false
	}
	atoms := make([]guard.Atom, len(origins))
	byOrigin := make(map[decisionOrigin]guard.Atom, len(origins))
	for index, origin := range origins {
		atom := guard.Atom(index + 1)
		atoms[index], byOrigin[origin] = atom, atom
	}
	return atoms, byOrigin, true
}

// decisionBatch derives the complete fixed decision universe for one compiled
// epoch. It is the structural boundary between Link provenance and the Guard
// carrier: active Relations supply existing term origins, and their owning
// Program activations supply the decision occurrences. A later topology
// reformation builds a fresh universe rather than extending this one.
func (solver *Solver) decisionBatch(relations []activeRelation, includeQueries bool) ([]decisionOrigin, bool) {
	if solver == nil || solver.link == nil {
		return nil, false
	}
	origins := make([]decisionOrigin, 0)
	appendOrigin := func(origin termOrigin) bool {
		if !solver.validTermOrigin(origin) {
			return false
		}
		programValue, ok := solver.link.Program(origin.shard)
		if !ok || programValue == nil {
			return false
		}
		activation, ok := programValue.Activation(origin.term)
		if !ok || activation == 0 {
			return false
		}
		count, ok := programValue.ActivationDecisionCount(activation)
		if !ok || count < 0 {
			return false
		}
		for index := 0; index < count; index++ {
			term, ok := programValue.ActivationDecisionAt(activation, index)
			if !ok {
				return false
			}
			if origin.candidate == (link.Candidate{}) {
				if !solver.validEntryAnchor(origin.shard, term) {
					return false
				}
			} else if !solver.validCandidateAnchor(origin.candidate, origin.shard, term) {
				return false
			}
			origins = append(origins, decisionOrigin{candidate: origin.candidate, shard: origin.shard, term: term})
		}
		return true
	}
	if includeQueries {
		for _, query := range solver.queries {
			if !appendOrigin(termOrigin{candidate: query.candidate, shard: query.shard, term: query.term}) {
				return nil, false
			}
		}
	}
	for _, relation := range relations {
		if !solver.validActiveRelation(relation) || !appendOrigin(relation.output) {
			return nil, false
		}
		for _, input := range relation.inputs {
			if !appendOrigin(input) {
				return nil, false
			}
		}
		candidate, shard, term, ok := solver.coordinate.Semantic(relation.source.caller)
		if !ok || !appendOrigin(termOrigin{candidate: candidate, shard: shard, term: term}) {
			return nil, false
		}
	}
	sort.Slice(origins, func(left, right int) bool {
		leftRoot := origins[left].candidate == (link.Candidate{})
		rightRoot := origins[right].candidate == (link.Candidate{})
		if leftRoot != rightRoot {
			return leftRoot
		}
		if !leftRoot {
			order, ok := solver.link.CompareCandidate(origins[left].candidate, origins[right].candidate)
			if !ok {
				return false
			}
			if order != 0 {
				return order < 0
			}
		}
		if origins[left].shard != origins[right].shard {
			return origins[left].shard < origins[right].shard
		}
		return origins[left].term < origins[right].term
	})
	origins = compactDecisionOrigins(origins)
	return origins, true
}

func compactDecisionOrigins(origins []decisionOrigin) []decisionOrigin {
	write := 0
	for _, origin := range origins {
		if write != 0 && origin == origins[write-1] {
			continue
		}
		origins[write] = origin
		write++
	}
	return origins[:write]
}

func (solver *Solver) decisionAtom(candidate link.Candidate, shard link.Shard, term program.Term) (guard.Atom, bool) {
	if solver == nil || shard == 0 || term == 0 {
		return 0, false
	}
	atom, ok := solver.decisionAtoms[decisionOrigin{candidate: candidate, shard: shard, term: term}]
	return atom, ok && atom != 0
}

func (solver *Solver) valid() bool {
	return solver != nil && solver.owner != nil && solver.sealed && solver.link != nil && solver.coordinate != nil && solver.guards != nil && solver.decisionAtoms != nil && solver.epoch != nil && solver.epoch.valid(solver.zero) && solver.epoch.valid(solver.entry) && solver.facts != nil && solver.facts.Valid(solver.zero) && solver.facts.Valid(solver.entry)
}
