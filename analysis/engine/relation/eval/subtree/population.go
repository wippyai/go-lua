package subtree

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Population is one authenticated owner population coordinate bound to a
// sealed ApplyReplay.  Binding it once lets several child subtrees reuse the
// exact population reader without re-reading or widening the caller's scope.
// The reader is still a borrowed solve-local capability; Population is
// serial and must not be used concurrently with another read on its Session.
type Population struct {
	session       Session
	replay        arrangement.ApplyReplay
	populationRef model.DenominatorRef
	row           model.RowID
	scope         witness.Scope
	population    binding.DenominatorWitness
	reader        read.Reader
	sealed        bool
}

// ForPopulation authenticates one owner-issued population RowID and its
// exact normalized cofiber against the replay's own driver denominator and
// reader.  In particular, a valid mounted scope belonging to another row is
// not accepted as a substitute.  The lookup is exact and does not scan or
// invert a key.
func (session Session) ForPopulation(replay arrangement.ApplyReplay, row model.RowID, scope witness.Scope) (Population, bool) {
	if !session.Available() || !replay.Available() || !row.Available() || !scope.ValidFor(session.mounted.RuntimeFence()) {
		return Population{}, false
	}
	correlation := replay.Correlation()
	populationRef := replay.Population()
	if !correlation.Available() || !populationRef.Available() || correlation.Population() != populationRef || !correlation.Type().Available() {
		return Population{}, false
	}
	if _, scopeOK := session.mounted.ScopeToken(scope); !scopeOK {
		return Population{}, false
	}
	populationWitness, witnessOK := session.mounted.Denominator(populationRef)
	if !witnessOK || !populationWitness.Available() || !populationWitness.ValidFor(session.mounted.RuntimeFence()) || !populationWitness.Matches(populationRef) || !populationWitness.Contains(row) || row.Relation() != populationRef.Relation() {
		return Population{}, false
	}
	driver, driverOK := replay.Driver()
	coordinate, coordinateOK := replay.Coordinate()
	coordinateOrdinal, coordinateOrdinalOK := replay.CoordinateOrdinal()
	columns := driver.Columns()
	if !driverOK || !driver.Available() || !driver.ValidFor(session.mounted.Fence()) || driver.Access().Relation() != populationRef.Relation() || driver.Access().Key().Available() || driver.CoordinateClass() != arrangement.CoordinateClassNone || !coordinateOK || !coordinate.Available() || !coordinateOrdinalOK || uint64(coordinateOrdinal) >= uint64(len(columns)) || columns[coordinateOrdinal] != coordinate {
		return Population{}, false
	}
	reader, readerOK := read.Bind(session.root, driver, session.geometry, session.scratch)
	if !readerOK || !reader.Available() || !reader.Layout().Equal(driver) {
		return Population{}, false
	}
	// Authenticate the exact population cofiber once. LookupRowID may expose
	// other valid cofibers for the same RowID; only the caller's exact scope is
	// admitted, and a duplicate of that scope is malformed.
	found := false
	malformed := false
	completed, valid := reader.LookupRowID(row, func(candidate read.Row) bool {
		if candidate == nil || !candidate.Available() || !reader.Owns(candidate) || candidate.ID() != row || candidate.ID().Relation() != populationRef.Relation() || !candidate.Scope().ValidFor(session.mounted.RuntimeFence()) {
			malformed = true
			return false
		}
		if !candidate.Scope().Same(scope) {
			return true
		}
		if found {
			malformed = true
			return false
		}
		found = true
		return true
	})
	if malformed || !completed || !valid || !found {
		return Population{}, false
	}
	value := Population{session: session, replay: replay, populationRef: populationRef, row: row, scope: scope, population: populationWitness, reader: reader, sealed: true}
	return value, value.Available()
}

// Available reports whether the owner coordinate, replay driver, reader, and
// exact population witness remain bound to this session's mounted root.
func (value Population) Available() bool {
	if !value.sealed || !value.session.Available() || !value.replay.Available() || !value.populationRef.Available() || !value.row.Available() || !value.scope.ValidFor(value.session.mounted.RuntimeFence()) || !value.population.Available() || !value.population.ValidFor(value.session.mounted.RuntimeFence()) || !value.population.Matches(value.populationRef) || !value.population.Contains(value.row) || value.row.Relation() != value.populationRef.Relation() || !value.reader.Available() {
		return false
	}
	if _, scopeOK := value.session.mounted.ScopeToken(value.scope); !scopeOK {
		return false
	}
	correlation := value.replay.Correlation()
	driver, driverOK := value.replay.Driver()
	coordinate, coordinateOK := value.replay.Coordinate()
	coordinateOrdinal, coordinateOrdinalOK := value.replay.CoordinateOrdinal()
	columns := driver.Columns()
	return correlation.Available() && correlation.Population() == value.populationRef && driverOK && driver.Available() && driver.ValidFor(value.session.mounted.Fence()) && driver.Access().Relation() == value.populationRef.Relation() && driver.Access().Key() == (model.KeyID{}) && driver.CoordinateClass() == arrangement.CoordinateClassNone && coordinateOK && coordinate.Available() && coordinateOrdinalOK && uint64(coordinateOrdinal) < uint64(len(columns)) && columns[coordinateOrdinal] == coordinate && value.reader.Layout().Equal(driver)
}

// Replay returns the exact ApplyReplay that authorized this population.
func (value Population) Replay() arrangement.ApplyReplay {
	if !value.Available() {
		return arrangement.ApplyReplay{}
	}
	return value.replay
}

// Row returns the owner-issued population RowID.
func (value Population) Row() model.RowID {
	if !value.Available() {
		return model.RowID{}
	}
	return value.row
}

// Scope returns the exact normalized cofiber authenticated during binding.
func (value Population) Scope() witness.Scope {
	if !value.Available() {
		return witness.Scope{}
	}
	return value.scope
}

// Denominator returns the exact mounted owner witness used to authenticate
// Row. It is diagnostic only; child evaluation still receives it through the
// sealed Population capability.
func (value Population) Denominator() binding.DenominatorWitness {
	if !value.Available() {
		return binding.DenominatorWitness{}
	}
	return value.population
}

// Evaluate redeems the exact child authorized by this replay. A child copied
// from another replay or another ordinal is refused even if its root digest
// happens to look structurally compatible.
func (value Population) Evaluate(child arrangement.CorrelatedSubtree) (Result, bool) {
	if !value.Available() || !child.Available() {
		return Result{}, false
	}
	canonical, ok := value.replay.ChildAt(int(child.Ordinal()))
	if !ok || !sameSubtree(canonical, child) {
		return Result{}, false
	}
	return value.session.evaluate(child, value.populationRef, value.population, value.row, value.scope, value.reader)
}

func sameSubtree(left, right arrangement.CorrelatedSubtree) bool {
	if !left.Available() || !right.Available() || left.Ordinal() != right.Ordinal() || left.Digest() != right.Digest() || left.Root() != right.Root() {
		return false
	}
	return true
}
