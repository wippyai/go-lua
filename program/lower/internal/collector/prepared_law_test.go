package collector

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestPreparedNilFlowDraftTerminalizesMovedSiblings(t *testing.T) {
	prepared := minimalPreparedForTest(t, "prepared-nil-flow.lua")
	sourceFinalizer, staticFinalizer, moduleFinalizer, orphanFlow := preparedOwnersForTest(t, prepared)
	preimage := sourceFinalizer.Preimage()
	staticView, moduleView := staticFinalizer.View(), moduleFinalizer.View()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	state.flow = nil
	state.mu.Unlock()

	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("Assemble with nil Flow Draft = %v/%v, want terminal failure", assembly, err)
	}
	assertPreparedSlotsMoved(t, prepared)
	assertPreparedSiblingsTerminal(t, preimage, staticView, moduleView, sourceFinalizer, staticFinalizer, moduleFinalizer)
	if assembly, err := flow.Assemble(source.Finalizer{}, static.Finalizer{}, module.Finalizer{}, orphanFlow, 0); err == nil || assembly != nil {
		t.Fatalf("orphan Flow cleanup = %v/%v, want terminal failure", assembly, err)
	}
}

func TestPreparedConsumedFlowDraftTerminalizesMovedSiblings(t *testing.T) {
	prepared := minimalPreparedForTest(t, "prepared-consumed-flow.lua")
	sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft := preparedOwnersForTest(t, prepared)
	preimage := sourceFinalizer.Preimage()
	staticView, moduleView := staticFinalizer.View(), moduleFinalizer.View()
	if assembly, err := flow.Assemble(source.Finalizer{}, static.Finalizer{}, module.Finalizer{}, flowDraft, 0); err == nil || assembly != nil {
		t.Fatalf("Flow Draft consumption = %v/%v, want post-claim failure", assembly, err)
	}
	if !preimage.Identity().ContentID().Available() || !staticView.Available() || !moduleView.ContentID().Available() {
		t.Fatal("consuming only Flow touched a Prepared sibling")
	}

	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("Assemble with consumed Flow Draft = %v/%v, want terminal failure", assembly, err)
	}
	assertPreparedSlotsMoved(t, prepared)
	assertPreparedSiblingsTerminal(t, preimage, staticView, moduleView, sourceFinalizer, staticFinalizer, moduleFinalizer)
}

func TestPreparedPostClaimFallbackIsInertAfterFlowCleanup(t *testing.T) {
	prepared := minimalPreparedForTest(t, "prepared-post-claim.lua")
	sourceFinalizer, staticFinalizer, moduleFinalizer, flowDraft := preparedOwnersForTest(t, prepared)
	preimage := sourceFinalizer.Preimage()
	staticView, moduleView := staticFinalizer.View(), moduleFinalizer.View()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	state.entry = 0
	state.mu.Unlock()

	if assembly, err := prepared.Assemble(); err == nil || assembly != nil {
		t.Fatalf("post-claim failure Assemble = %v/%v, want terminal failure", assembly, err)
	}
	assertPreparedSlotsMoved(t, prepared)
	assertPreparedSiblingsTerminal(t, preimage, staticView, moduleView, sourceFinalizer, staticFinalizer, moduleFinalizer)
	if assembly, err := flow.Assemble(source.Finalizer{}, static.Finalizer{}, module.Finalizer{}, flowDraft, 0); err == nil || assembly != nil {
		t.Fatalf("post-failure Flow Draft reopened: %v/%v", assembly, err)
	}
}

func TestPreparedCopiedLoserCannotReachMovedOwnerSlots(t *testing.T) {
	prepared := minimalPreparedForTest(t, "prepared-copy.lua")
	loser := prepared
	preimage := preparedSourcePreimage(t, prepared)
	staticView, moduleView := preparedSiblingViewsForTest(t, prepared)

	assembly, err := prepared.Assemble()
	if err != nil || assembly == nil {
		t.Fatalf("winner Assemble = %v/%v", assembly, err)
	}
	assertPreparedSlotsMoved(t, prepared)
	if preimage.Identity().ContentID().Available() || staticView.Available() || moduleView.ContentID().Available() {
		t.Fatal("winner left a construction owner live")
	}

	if repeated, repeatedErr := loser.Assemble(); repeatedErr == nil || repeated != nil {
		t.Fatalf("copied loser Assemble = %v/%v, want terminal rejection", repeated, repeatedErr)
	}
	assertPreparedSlotsMoved(t, loser)

	sourceComponent, flowComponent, staticComponent, moduleComponent, err := assembly.Take()
	if err != nil || sourceComponent == nil || flowComponent == nil || staticComponent == nil || moduleComponent == nil {
		t.Fatalf("winner Assembly.Take = %p/%p/%p/%p, %v", sourceComponent, flowComponent, staticComponent, moduleComponent, err)
	}
}

func minimalPreparedForTest(t testing.TB, name string) Prepared {
	t.Helper()
	c := New(name, 0, bind.GlobalCensus{})
	order := c.Source().Order()
	body := order.Body(source.Span{File: name})
	if body == 0 || !order.SetBody(body) || !order.SetEntry(body) {
		t.Fatalf("minimal Prepared Source setup failed: %v", failure(c))
	}
	prepared, err := c.Prepare()
	if err != nil {
		t.Fatalf("minimal Prepare: %v", err)
	}
	return prepared
}

func assertPreparedSlotsMoved(t testing.TB, prepared Prepared) {
	t.Helper()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.terminal || state.source != (source.Finalizer{}) || state.flow != nil ||
		state.static != (static.Finalizer{}) || state.module != (module.Finalizer{}) || state.entry != 0 {
		t.Fatalf("Prepared retained owner slots after transfer: terminal=%v source=%#v flow=%p static=%#v module=%#v entry=%v",
			state.terminal, state.source, state.flow, state.static, state.module, state.entry)
	}
}

func preparedOwnersForTest(
	t testing.TB,
	prepared Prepared,
) (source.Finalizer, static.Finalizer, module.Finalizer, *flow.Draft) {
	t.Helper()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		t.Fatal("Prepared is already terminal")
	}
	return state.source, state.static, state.module, state.flow
}

func assertPreparedSiblingsTerminal(
	t testing.TB,
	preimage source.Preimage,
	staticView static.View,
	moduleView module.View,
	sourceFinalizer source.Finalizer,
	staticFinalizer static.Finalizer,
	moduleFinalizer module.Finalizer,
) {
	t.Helper()
	if preimage.Identity().ContentID().Available() || staticView.Available() || moduleView.ContentID().Available() {
		t.Fatal("Prepared failure left a moved sibling owner live")
	}
	sourceReusable := sourceFinalizer.Abort() == nil
	staticReusable := staticFinalizer.Abort() == nil
	moduleReusable := moduleFinalizer.Abort()
	if sourceReusable || staticReusable || moduleReusable {
		t.Fatal("Prepared failure left a moved sibling finalizer reusable")
	}
}

// preparedSourcePreimage is deliberately test-only and package-private. The
// production Prepared surface never exposes a Source finalizer or Preimage;
// these laws inspect expiry through the same-package state that owns it.
func preparedSourcePreimage(t testing.TB, prepared Prepared) source.Preimage {
	t.Helper()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		t.Fatal("Prepared is already terminal")
	}
	return state.source.Preimage()
}

func preparedEntryForTest(t testing.TB, prepared Prepared) Term {
	t.Helper()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		t.Fatal("Prepared is already terminal")
	}
	return state.entry
}

func preparedSiblingViewsForTest(t testing.TB, prepared Prepared) (static.View, module.View) {
	t.Helper()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.terminal {
		t.Fatal("Prepared is already terminal")
	}
	return state.static.View(), state.module.View()
}

// abortPreparedSourceForTest creates the one failure shape that external
// callers cannot construct: a claimed Source owner becomes terminal before
// Prepared transfers its opaque quartet. Prepared.Assemble must then let Flow
// clean the remaining claimed owners, while the preimage expires.
func abortPreparedSourceForTest(t testing.TB, prepared Prepared) error {
	t.Helper()
	state := preparedStateForTest(t, prepared)
	state.mu.Lock()
	if state.terminal {
		state.mu.Unlock()
		return errors.New("Prepared is already terminal")
	}
	sourceFinalizer := state.source
	state.mu.Unlock()
	return sourceFinalizer.Abort()
}

func preparedStateForTest(t testing.TB, prepared Prepared) *preparedState {
	t.Helper()
	if prepared.state == nil {
		t.Fatal("Prepared has no shared state")
	}
	return prepared.state
}
