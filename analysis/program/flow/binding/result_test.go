package binding

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/flowtest"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

type bindingOwnerFixture struct {
	preimage source.Preimage
	flow     authored.View
	body     *body.Result
	sourceID identity.ContentID
	flowID   identity.ContentID
	close    func()
}

func openBindingOwnerFixture(t *testing.T, name string, input authored.Input) bindingOwnerFixture {
	t.Helper()
	bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	flowDraft, err := authored.Build(input)
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinish, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		_ = flowFinish.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinish, err := staticDraft.Finalizer()
	if err != nil {
		_ = flowFinish.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}

	sourceInput := source.Input{
		Name:       name,
		ExactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "global"}},
		Bodies:     []source.BodySource{{Body: bodyTerm}},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := int(input.Counts[family])
		spans := make([]source.Span, count)
		for index := range spans {
			line := uint32(index + 1)
			spans[index] = source.Span{File: name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		sourceInput.Families = append(sourceInput.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		_ = flowFinish.Abort()
		_ = staticFinish.Abort()
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinish, err := sourceDraft.Finalizer()
	if err != nil {
		_ = flowFinish.Abort()
		_ = staticFinish.Abort()
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinish.Preimage()
	bodyResult, err := body.Seal(preimage, flowFinish.View(), staticFinish.View(), bodyTerm)
	if err != nil {
		_ = flowFinish.Abort()
		_ = staticFinish.Abort()
		_ = sourceFinish.Abort()
		t.Fatalf("body.Seal: %v", err)
	}
	flow := flowFinish.View()
	return bindingOwnerFixture{
		preimage: preimage,
		flow:     flow,
		body:     bodyResult,
		sourceID: preimage.Identity().ContentID(),
		flowID:   flow.Cold().ContentID(),
		close: func() {
			_ = flowFinish.Abort()
			_ = staticFinish.Abort()
			_ = sourceFinish.Abort()
		},
	}
}

func bindingProvenanceInput() authored.Input {
	return authored.Input{
		Counts: [keyspace.FamilyCount]uint32{
			keyspace.FamilyBody: 1,
			keyspace.FamilyCell: 1,
		},
		Storage: authored.StorageInput{
			Cells: []authored.Cell{{Kind: authored.CellGlobal, Key: 1}},
		},
	}
}

func TestSealAndMatchesRejectEqualDenominatorForeignOwners(t *testing.T) {
	first := openBindingOwnerFixture(t, "binding-provenance-a.lua", bindingProvenanceInput())
	defer first.close()
	foreignSource := openBindingOwnerFixture(t, "binding-provenance-b.lua", bindingProvenanceInput())
	defer foreignSource.close()
	foreignFlowInput := bindingProvenanceInput()
	// Keep every authored denominator equal while changing the canonical Flow
	// digest. The global Cell remains structurally identical, so a cardinality
	// check alone cannot distinguish this owner.
	foreignFlowInput.Storage.Cells[0].Key = 2
	foreignFlow := openBindingOwnerFixture(t, "binding-provenance-a.lua", foreignFlowInput)
	defer foreignFlow.close()

	if first.sourceID == foreignSource.sourceID || first.flowID == foreignFlow.flowID {
		t.Fatal("foreign equal-denominator fixture did not change its owner identity")
	}
	if first.body.BodyCount() != foreignSource.body.BodyCount() || first.body.BodyCount() != foreignFlow.body.BodyCount() {
		t.Fatal("foreign fixtures changed the Body denominator")
	}

	result, err := Seal(first.preimage, first.flow, first.body, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	if !Matches(&result, first.sourceID, first.flowID) {
		t.Fatal("Binding result did not retain exact Source/Flow identities")
	}
	foreignSourceID := first.sourceID
	foreignSourceID[0]++
	foreignFlowID := first.flowID
	foreignFlowID[0]++
	if Matches(&result, foreignSourceID, first.flowID) || Matches(&result, first.sourceID, foreignFlowID) ||
		Matches(&result, identity.ContentID{}, first.flowID) || Matches(&result, first.sourceID, identity.ContentID{}) {
		t.Fatal("Binding provenance accepted a foreign or unavailable identity")
	}

	if _, err := Seal(first.preimage, first.flow, foreignSource.body, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil ||
		!strings.Contains(err.Error(), "Body provenance") {
		t.Fatalf("foreign equal-denominator Body splice was accepted or failed outside provenance fence: %v", err)
	}
	if _, err := Seal(first.preimage, foreignFlow.flow, first.body, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil ||
		!strings.Contains(err.Error(), "Body provenance") {
		t.Fatalf("foreign equal-denominator Flow splice was accepted or failed outside provenance fence: %v", err)
	}
	if _, err := Seal(foreignSource.preimage, first.flow, first.body, keyspace.MakeTerm(keyspace.FamilyBody, 1)); err == nil ||
		!strings.Contains(err.Error(), "Body provenance") {
		t.Fatalf("foreign equal-denominator Source splice was accepted or failed outside provenance fence: %v", err)
	}
}

func TestBindingResultQueriesFailClosedWithoutOwnerIDs(t *testing.T) {
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	zero := Result{
		roles: []kind.CellRole{0, kind.CellChunkVararg},
		hosts: []keyspace.Term{0, keyspace.MakeTerm(keyspace.FamilyBind, 1)},
		chunk: cell,
	}
	if Matches(&zero, flowtest.ContentIDAt(0x11), flowtest.ContentIDAt(0x22)) || zero.CellCount() != 0 {
		t.Fatal("zero-ID Binding result bypassed provenance fail-closed law")
	}
	if role, ok := zero.Role(cell); ok || role != 0 {
		t.Fatalf("zero-ID Role = %v/%v, want 0/false", role, ok)
	}
	if host, ok := zero.Host(cell); ok || host != 0 {
		t.Fatalf("zero-ID Host = %v/%v, want 0/false", host, ok)
	}
	if chunk, ok := zero.ChunkVararg(); ok || chunk != 0 {
		t.Fatalf("zero-ID ChunkVararg = %v/%v, want 0/false", chunk, ok)
	}
	if function, ok := zero.FunctionCell(keyspace.MakeTerm(keyspace.FamilyFunction, 1)); ok || function != 0 {
		t.Fatalf("zero-ID FunctionCell = %v/%v, want 0/false", function, ok)
	}
}
