package engine

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// testSnapshotAnswer is the test-only read path for a solved result. Tests
// name the published family and stable row identity directly; no receipt
// result wrapper participates in the read.
func testSnapshotAnswer(solver *Solver, state *State, family, key identity.ContentID) (Answer, bool) {
	if state == nil || !family.Available() || !key.Available() {
		return Answer{}, false
	}
	sealed, owned := solver.PublishedSnapshot(state)
	if !owned {
		return Answer{}, false
	}
	published := sealed.Snapshot()
	plan, opened := snapshot.OpenQuery[identity.ContentID, Answer](&published, family)
	if !opened {
		return Answer{}, false
	}
	answer, status := snapshot.Query(&published, plan, key)
	if status != snapshot.ReadHit || !answer.Available() {
		return Answer{}, false
	}
	return answer, true
}

func testSnapshotQueryValue[R any](solver *Solver, state *State, key identity.ContentID) (R, bool) {
	if state == nil {
		var zero R
		return zero, false
	}
	sealed, owned := solver.PublishedSnapshot(state)
	if !owned {
		var zero R
		return zero, false
	}
	return testSnapshotSealedValue[R](sealed, sealed.QueryFamily(), key)
}

func testSnapshotObservationValue[R any](solver *Solver, state *State, key identity.ContentID) (R, bool) {
	if state == nil {
		var zero R
		return zero, false
	}
	sealed, owned := solver.PublishedSnapshot(state)
	if !owned {
		var zero R
		return zero, false
	}
	return testSnapshotSealedValue[R](sealed, sealed.ObservationFamily(), key)
}

func testSnapshotSealedValue[R any](sealed SolvedSnapshot, family, key identity.ContentID) (R, bool) {
	published := sealed.Snapshot()
	if !sealed.Available() || !family.Available() || !key.Available() {
		var zero R
		return zero, false
	}
	plan, opened := snapshot.OpenQuery[identity.ContentID, Answer](&published, family)
	if !opened {
		var zero R
		return zero, false
	}
	answer, status := snapshot.Query(&published, plan, key)
	if status != snapshot.ReadHit || !answer.Available() {
		var zero R
		return zero, false
	}
	return AnswerValue[R](answer)
}
