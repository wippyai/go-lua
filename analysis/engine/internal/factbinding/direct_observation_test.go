package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

func TestDirectObservationExactCursorEmitsRowsAndRevokesOnClose(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, on, off, _, _ := observationMasks(t, manager)
	binding, state, _, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)

	var scratch DirectObservation[uint64, uint64]
	if !binding.BeginDirectObservation(&scratch, work, state, declared.exact[0], whole) {
		t.Fatal("begin direct observation")
	}
	row, observation, ok := scratch.Next()
	if !ok || !row.Region().Equal(whole) || observation.Count() != 1 {
		t.Fatal("direct cursor did not emit the initial exact row")
	}
	entry, ok := observation.At(0)
	if !ok {
		t.Fatal("initial entry")
	}
	value, present := entry.Read()
	if value != 0 || present {
		t.Fatalf("initial entry = %d/%t, want 0/false", value, present)
	}
	if _, _, ok := scratch.Next(); ok {
		t.Fatal("direct cursor emitted an extra row")
	}
	if !scratch.Close() || scratch.Close() {
		t.Fatal("direct close lifecycle")
	}
	if observation.Valid() || row.Region().Valid() || scratch.Valid() {
		t.Fatal("closed direct observation remained live")
	}

	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.target[0], on, 7) {
		t.Fatal("stored branch")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	state = commit(t, work, state, candidate)
	if !binding.BeginDirectObservation(&scratch, work, state, declared.exact[0], whole) {
		t.Fatal("reuse direct observation")
	}
	rows := 0
	for {
		row, observation, ok = scratch.Next()
		if !ok {
			break
		}
		if observation.Count() != 1 {
			t.Fatal("exact direct observation width")
		}
		entry, ok := observation.At(0)
		if !ok {
			t.Fatal("stored entry")
		}
		value, present := entry.Read()
		switch {
		case row.Region().Equal(on) && value == 7 && present:
		case row.Region().Equal(off) && value == 0 && !present:
		default:
			t.Fatalf("unexpected direct row %v = %d/%t", row.Region(), value, present)
		}
		rows++
	}
	if rows != 2 || !scratch.Close() {
		t.Fatalf("direct rows/close = %d", rows)
	}
}

func TestDirectObservationRejectsForeignUnitStateAndDoubleBegin(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, state, _, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	foreignBinding, foreignState, _, foreignComposition, foreignDeclared := newObservationBinding(t, manager)
	foreignWork := newWork(t, foreignComposition)
	var scratch DirectObservation[uint64, uint64]
	if !binding.BeginDirectObservation(&scratch, work, state, declared.exact[0], whole) {
		t.Fatal("direct begin")
	}
	if binding.BeginDirectObservation(&scratch, work, state, declared.exact[1], whole) {
		t.Fatal("double begin accepted")
	}
	if foreignBinding.BeginDirectObservation(&scratch, foreignWork, foreignState, foreignDeclared.exact[0], whole) {
		t.Fatal("foreign binding accepted live scratch")
	}
	if !scratch.Close() {
		t.Fatal("close")
	}
	if binding.BeginDirectObservation(&scratch, foreignWork, foreignState, declared.exact[0], whole) {
		t.Fatal("foreign work/state accepted")
	}
	if binding.BeginDirectObservation(&scratch, work, foreignState, declared.exact[0], whole) {
		t.Fatal("foreign state accepted")
	}
	if !binding.BeginDirectObservation(&scratch, work, state, declared.summary, whole) {
		t.Fatal("summary unit rejected by direct cursor")
	}
	if _, summary, ok := scratch.Next(); !ok || summary.Count() != 2 {
		t.Fatal("direct summary row")
	}
	if _, _, status := scratch.Step(); status != DirectObservationExhausted {
		t.Fatal("direct summary exhaustion")
	}
	if !scratch.Close() {
		t.Fatal("summary close")
	}
	if scratch.Close() {
		t.Fatal("double close accepted")
	}
}

func TestDirectObservationReusesWarmScratch(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	whole, _, _, _, _ := observationMasks(t, manager)
	binding, state, _, composition, declared := newObservationBinding(t, manager)
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil || !patch.Write(declared.target[0], whole, 7) {
		t.Fatal("warm branch")
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("warm accept")
	}
	state = commit(t, work, state, candidate)
	var scratch DirectObservation[uint64, uint64]
	run := func() bool {
		if !binding.BeginDirectObservation(&scratch, work, state, declared.exact[0], whole) {
			return false
		}
		for {
			if _, _, ok := scratch.Next(); !ok {
				break
			}
		}
		return scratch.Close()
	}
	if !run() || !run() {
		t.Fatal("warmup")
	}
	if allocations := testing.AllocsPerRun(20, func() {
		if !run() {
			t.Fatal("direct run")
		}
	}); allocations != 0 {
		t.Fatalf("warm direct observation allocated %v times", allocations)
	}
}

func TestDirectObservationSummaryFormsShareCanonicalCursor(t *testing.T) {
	binding, state, work, declared := newDistributiveBinding(t, 3)
	var scratch DirectObservation[uint64, uint64]
	if !binding.BeginDirectObservation(&scratch, work, state, declared.correlated, state.Support()) {
		t.Fatal("correlated summary begin")
	}
	rows := 0
	for {
		_, view, status := scratch.Step()
		switch status {
		case DirectObservationAvailable:
			if !view.Valid() || view.Count() != 3 {
				t.Fatal("correlated summary view")
			}
			rows++
		case DirectObservationExhausted:
			if rows == 0 {
				t.Fatal("correlated summary had no rows")
			}
			goto correlatedDone
		default:
			t.Fatal("correlated summary refused")
		}
	}
correlatedDone:
	if !scratch.Close() {
		t.Fatal("correlated summary close")
	}
	if !binding.BeginDirectObservation(&scratch, work, state, declared.distributive, state.Support()) {
		t.Fatal("distributive summary begin")
	}
	if _, view, status := scratch.Step(); status != DirectObservationAvailable || !view.Valid() || view.Count() != 3 {
		t.Fatal("distributive summary view")
	}
	if _, _, status := scratch.Step(); status != DirectObservationExhausted {
		t.Fatal("distributive summary exhaustion")
	}
	if !scratch.Close() || scratch.Close() {
		t.Fatal("distributive summary close lifecycle")
	}
}
