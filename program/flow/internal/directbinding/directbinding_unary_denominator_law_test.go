package directbinding

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbody "github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestSealRejectsUnaryDenominatorMismatch(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	var sourceCounts [keyspace.FamilyCount]uint32
	sourceCounts[keyspace.FamilyBody] = 1
	sourceCounts[keyspace.FamilyUnary] = 1

	sourceInput := source.Input{
		Name:   "directbinding-unary-denominator.lua",
		Bodies: []source.BodySource{{Body: body}},
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, sourceCounts[family])
		for index := range spans {
			spans[index] = source.Span{
				File: sourceInput.Name, StartLine: uint32(index + 1), StartCol: 1,
				EndLine: uint32(index + 1), EndCol: 1,
			}
		}
		sourceInput.Families = append(sourceInput.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	sourceDraft, err := source.Build(sourceInput)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalizer, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	defer sourceFinalizer.Abort()

	flowCounts := sourceCounts
	flowCounts[keyspace.FamilyUnary] = 0
	flowDraft, err := authored.Build(authored.Input{Counts: flowCounts})
	if err != nil {
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalizer, err := flowDraft.Finalizer()
	if err != nil {
		t.Fatalf("authored.Finalizer: %v", err)
	}
	defer flowFinalizer.Abort()

	moduleDraft, err := module.Build(module.Input{})
	if err != nil {
		t.Fatalf("module.Build: %v", err)
	}
	moduleFinalizer, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("module.Finalizer: %v", err)
	}
	defer moduleFinalizer.Abort()

	staticDraft, err := static.Build(static.Input{})
	if err != nil {
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalizer, err := staticDraft.Finalizer()
	if err != nil {
		t.Fatalf("static.Finalizer: %v", err)
	}
	defer staticFinalizer.Abort()

	flowView := flowFinalizer.View()
	bindings := directBindingProof(t, sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	bodyResult, bodyErr := flowbody.Seal(sourceFinalizer.Preimage(), flowView, staticFinalizer.View(), body)
	if bodyErr != nil {
		t.Fatalf("body.Seal: %v", bodyErr)
	}
	_, err = Seal(sourceFinalizer.Preimage(), flowView, bodyResult, bindings, staticFinalizer.View(), moduleFinalizer.View())
	if err == nil || !strings.Contains(err.Error(), "authored family denominator mismatch") {
		t.Fatalf("Seal error = %v, want Unary denominator mismatch rejection", err)
	}
}
