package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/position"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestSealPendingCommittedSourceRootsAndProvenance(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 7, keyspace.FamilyInteger: 1,
		keyspace.FamilyValues: 3, keyspace.FamilyLensExact: 1, keyspace.FamilyRead: 1, keyspace.FamilyUnary: 3, keyspace.FamilyBinary: 1,
		keyspace.FamilyCall: 2, keyspace.FamilyReturn: 1,
	}
	bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	nilA, nilB, argA, argB, nilC, nilD, nilG := keyspace.MakeTerm(keyspace.FamilyNil, 1), keyspace.MakeTerm(keyspace.FamilyNil, 2), keyspace.MakeTerm(keyspace.FamilyNil, 3), keyspace.MakeTerm(keyspace.FamilyNil, 4), keyspace.MakeTerm(keyspace.FamilyNil, 5), keyspace.MakeTerm(keyspace.FamilyNil, 6), keyspace.MakeTerm(keyspace.FamilyNil, 7)
	integerDead := keyspace.MakeTerm(keyspace.FamilyInteger, 1)
	valuesRoot, valuesA, valuesB := keyspace.MakeTerm(keyspace.FamilyValues, 1), keyspace.MakeTerm(keyspace.FamilyValues, 2), keyspace.MakeTerm(keyspace.FamilyValues, 3)
	unaryA, unaryB, unaryDead := keyspace.MakeTerm(keyspace.FamilyUnary, 1), keyspace.MakeTerm(keyspace.FamilyUnary, 2), keyspace.MakeTerm(keyspace.FamilyUnary, 3)
	deadExact := keyspace.MakeTerm(keyspace.FamilyLensExact, 1)
	binaryTerm, callA, callB := keyspace.MakeTerm(keyspace.FamilyBinary, 1), keyspace.MakeTerm(keyspace.FamilyCall, 1), keyspace.MakeTerm(keyspace.FamilyCall, 2)
	readDead := keyspace.MakeTerm(keyspace.FamilyRead, 1)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)

	input := source.Input{Name: "pending.lua"}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		spans := make([]source.Span, counts[family])
		for ordinal := range spans {
			line := uint32(ordinal + 1)
			spans[ordinal] = source.Span{File: input.Name, StartLine: line, StartCol: 1, EndLine: line, EndCol: 1}
		}
		input.Families = append(input.Families, source.FamilySpans{Family: family, Spans: spans})
	}
	input.Bodies = []source.BodySource{{Body: bodyTerm, Terms: []keyspace.Term{callA, returnTerm, callB}}}
	input.Nil = []source.NilLiteral{{Owner: bodyTerm}, {Owner: bodyTerm}, {Owner: bodyTerm}, {Owner: bodyTerm}, {Owner: bodyTerm}, {Owner: bodyTerm}, {Owner: bodyTerm}}
	input.Integer = []source.IntegerLiteral{{Owner: bodyTerm, Value: 7}}
	sourceDraft, err := source.Build(input)
	if err != nil {
		t.Fatalf("source.Build: %v", err)
	}
	sourceFinalize, err := sourceDraft.Finalizer()
	if err != nil {
		t.Fatalf("source.Finalizer: %v", err)
	}
	preimage := sourceFinalize.Preimage()

	staticDraft, err := static.Build(static.Input{
		Counts:    counts,
		Contracts: static.ContractsInput{Call: make([]static.CallContract, counts[keyspace.FamilyCall])},
	})
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Build: %v", err)
	}
	staticFinalize, err := staticDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("static.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = staticFinalize.Abort() })

	flowDraft, err := authored.Build(authored.Input{
		Counts: counts,
		Values: authored.ValuesInput{
			Rows: []authored.Value{
				{Owner: bodyTerm, Fixed: authored.Range{End: 1}},
				{Owner: bodyTerm, Fixed: authored.Range{Start: 1, End: 3}},
				{Owner: bodyTerm, Fixed: authored.Range{Start: 3, End: 4}},
			},
			Terms: []keyspace.Term{binaryTerm, argA, nilB, argB},
		},
		Access:  authored.AccessInput{Exact: []authored.ExactLens{{Owner: bodyTerm, Base: nilG, Source: unaryDead, Kind: kind.FieldExact}}},
		Storage: authored.StorageInput{Reads: []authored.Read{{Owner: bodyTerm, Source: deadExact}}},
		Calls: []authored.Call{
			{Owner: bodyTerm, Callee: nilA, Actuals: valuesA},
			{Owner: bodyTerm, Callee: readDead, Actuals: valuesB},
		},
		Operators: authored.OperatorsInput{
			Unaries:  []authored.Unary{{Owner: bodyTerm, Op: kind.UnaryNeg, Operand: nilC}, {Owner: bodyTerm, Op: kind.UnaryNeg, Operand: nilD}, {Owner: bodyTerm, Op: kind.UnaryNeg, Operand: integerDead}},
			Binaries: []authored.Binary{{Owner: bodyTerm, Op: kind.BinaryAdd, Left: unaryA, Right: unaryB}},
		},
		Control: authored.ControlInput{Returns: []authored.Return{{Owner: bodyTerm, Values: valuesRoot}}},
	})
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Build: %v", err)
	}
	flowFinalize, err := flowDraft.Finalizer()
	if err != nil {
		_ = sourceFinalize.Abort()
		t.Fatalf("authored.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = flowFinalize.Abort() })
	flowView := flowFinalize.View()
	staticView := staticFinalize.View()

	bodies, err := body.Seal(preimage, flowView, staticView, bodyTerm)
	if err != nil {
		t.Fatalf("body.Seal: %v", err)
	}
	bindingResult, err := binding.Seal(preimage, flowView, bodies, bodyTerm)
	if err != nil {
		t.Fatalf("binding.Seal: %v", err)
	}
	moduleDraft, err := imports.Build(imports.Input{})
	if err != nil {
		t.Fatalf("imports.Build: %v", err)
	}
	moduleFinalize, err := moduleDraft.Finalizer()
	if err != nil {
		t.Fatalf("imports.Finalizer: %v", err)
	}
	t.Cleanup(func() { _ = moduleFinalize.Abort() })
	forest, _, err := containment.Prove(preimage, staticView, flowView, bodies, bindingResult, moduleFinalize.View(), bodyTerm)
	if err != nil {
		t.Fatalf("containment.Prove: %v", err)
	}
	shape, err := control.Seal(preimage, flowView, bodies, bindingResult, forest, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("control.Seal: %v", err)
	}
	outcomes, err := outcome.Seal(preimage.Identity(), flowView, bodies, shape, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("outcome.Seal: %v", err)
	}
	index, err := position.Seal(preimage, flowView, bodies, forest, outcomes, bodyTerm, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("position.Seal: %v", err)
	}
	sourceComponent, err := sourceFinalize.Commit(index)
	if err != nil {
		t.Fatalf("source.Commit: %v", err)
	}
	sourceView := sourceComponent.View()
	controlResult, err := sourcecontrol.Seal(sourceView, flowView, bodies, forest, shape, bodyTerm, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("sourcecontrol.Seal: %v", err)
	}
	executableResult, err := executable.Seal(sourceView, flowView, forest, controlResult, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("executable.Seal: %v", err)
	}
	candidateResult, err := candidates.Seal(sourceView.Identity(), flowView, executableResult, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("candidates.Seal: %v", err)
	}
	pending, err := SealPending(sourceView, flowView, executableResult, candidateResult, staticView.ContentID(), moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("SealPending: %v", err)
	}
	if !MatchesPending(pending, sourceView.Identity().ContentID(), flowView.Cold().ContentID(), staticView.ContentID(), moduleFinalize.View().ContentID()) {
		t.Fatal("sealed Pending provenance did not match its owners")
	}
	if count, ok := pending.Count(callA); !ok || count != 0 {
		t.Fatalf("Call A pending = %d/%v, want 0/true", count, ok)
	}
	if _, ok := pending.Count(callB); ok {
		t.Fatal("dead Call B unexpectedly received a pending subject root")
	}
	if executableResult.Executable(unaryDead) || candidateResult.UnaryNumeric().Contains(unaryDead) {
		t.Fatal("dead FieldExact UnaryNeg entered the live executable/candidate denominator")
	}
	if _, ok := pending.Count(unaryDead); ok {
		t.Fatal("dead FieldExact UnaryNeg received a pending subject root")
	}
	if _, ok := pending.At(callB, 0); ok {
		t.Fatal("empty Call B pending set exposed an operand")
	}
	if count, ok := pending.Count(unaryA); !ok || count != 0 {
		t.Fatalf("Unary A pending = %d/%v, want 0/true", count, ok)
	}
	if count, ok := pending.Count(unaryB); !ok || count != 1 {
		t.Fatalf("Unary B pending = %d/%v, want 1/true", count, ok)
	}
	if got, ok := pending.At(unaryB, 0); !ok || got != unaryA {
		t.Fatalf("Unary B pending[0] = %v/%v, want Unary A %v/true", got, ok, unaryA)
	}
	if count, ok := pending.Count(binaryTerm); !ok || count != 0 {
		t.Fatalf("Binary pending = %d/%v, want 0/true", count, ok)
	}
}
