package exportmanifest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromProgramResultExportsReturnedTableMemberErrorReturnEffect(t *testing.T) {
	result := checkProgram(t, `
		local client = {}
		function client.fetch(id: string): (number?, string?)
			if id == "" then
				return nil, "missing"
			end
			return 1, nil
		end
		return client
	`)

	m := FromProgramResult("client", result)
	sig, ok := m.FunctionSignatures["client.fetch"]
	if !ok {
		t.Fatalf("missing client.fetch function signature: %#v", m.FunctionSignatures)
	}
	if len(sig.Type.Returns) != 2 {
		t.Fatalf("client.fetch returns = %d, want 2", len(sig.Type.Returns))
	}
	if !typ.TypeEquals(sig.Type.Returns[0], typeexpr.Optional(typ.Number)) {
		t.Fatalf("client.fetch return 1 = %v, want number?", sig.Type.Returns[0])
	}
	if !typ.TypeEquals(sig.Type.Returns[1], typeexpr.Optional(typ.String)) {
		t.Fatalf("client.fetch return 2 = %v, want string?", sig.Type.Returns[1])
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("client.fetch effect = %v, want ErrorReturn(0, 1)", sig.Effect)
	}
	if sig.OperationalEffects == nil {
		t.Fatalf("client.fetch operational effects = nil")
	}
	assertSignatureReturnPresenceRelation(t, sig.OperationalEffects.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	assertSignatureReturnPresenceRelation(t, sig.OperationalEffects.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
}

func TestFromProgramResultExportsIsNilNormalReturnRefinementEffect(t *testing.T) {
	result := checkProgram(t, `
		local test = {}
		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "expected nil", 2)
			end
		end
		return test
	`)

	m := FromProgramResult("test", result)
	sig, ok := m.FunctionSignatures["test.is_nil"]
	if !ok {
		t.Fatalf("missing test.is_nil function signature: %#v", m.FunctionSignatures)
	}
	if !hasNormalReturnAbsentRefinement(sig.Effect, 0) {
		t.Fatalf("test.is_nil effect = %v, want normal return absent refinement for param 0", sig.Effect)
	}
	if hasNormalReturnAbsentRefinement(sig.Effect, 1) {
		t.Fatalf("test.is_nil effect = %v, did not expect absent refinement for msg param", sig.Effect)
	}
}

func TestFunctionSummaryEffectDoesNotSerializeParamObligationsToManifestEffects(t *testing.T) {
	reg := standard.Registry()
	got := functionSummaryEffect(summary.Summary{
		ParamObligations: []product.Value{
			typevalue.FromType(reg, typ.Number),
		},
	}, typ.Func().Param("tokens", typ.Any).Returns(typ.Number).Build())
	if !got.Pure() {
		t.Fatalf("effect = %v, want no manifest effect labels for pre-call ParamObligations", got)
	}
}

func TestFunctionSummaryEffectExportsExactRootOwnershipBoundaryFacts(t *testing.T) {
	got := functionSummaryEffect(summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			EscapeEvents: []callboundary.EscapeEventFact{
				{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventSend, Recursive: true},
				{Target: pathdom.NewPlaceholder(1), Kind: callboundary.EscapeEventStore, Recursive: true},
				{Target: pathdom.NewPlaceholder(2), Kind: callboundary.EscapeEventRetain, Recursive: true},
				{Target: pathdom.NewPlaceholder(3), Kind: callboundary.EscapeEventExport, Recursive: true},
				{Target: pathdom.NewPlaceholder(4), Kind: callboundary.EscapeEventOpaque, Recursive: true},
				{Target: pathdom.NewPlaceholder(5).Field("child"), Kind: callboundary.EscapeEventSend, Recursive: true},
				{Target: pathdom.NewPlaceholder(5), Kind: callboundary.EscapeEventSend},
				{Target: pathdom.NewPlaceholder(6), Kind: callboundary.EscapeEventBorrow, Recursive: true},
			},
			FrozenTables: []callboundary.FrozenTableFact{
				{Target: pathdom.NewPlaceholder(5)},
				{Target: pathdom.NewPlaceholder(1).Field("child")},
			},
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: pathdom.NewPlaceholder(1)},
				{Path: pathdom.NewPlaceholder(2).Field("child")},
			},
		},
	}, typ.Func().
		Param("sent", typ.Any).
		Param("stored", typ.Any).
		Param("retained", typ.Any).
		Param("exported", typ.Any).
		Param("opaque", typ.Any).
		Param("frozen", typ.Any).
		Param("borrowed", typ.Any).
		Build())

	if !hasOwnershipSendParam(got, 0) {
		t.Fatalf("effect = %v, want exact SendParam for param 0", got)
	}
	if !hasOwnershipStoreUnknown(got, 1) {
		t.Fatalf("effect = %v, want root Store for param 1", got)
	}
	if !hasMutationTableMutator(got, 1) {
		t.Fatalf("effect = %v, want root TableMutator for param 1", got)
	}
	if !hasOwnershipRetain(got, 2) {
		t.Fatalf("effect = %v, want root Retain for param 2", got)
	}
	if !hasOwnershipExport(got, 3) {
		t.Fatalf("effect = %v, want root Export for param 3", got)
	}
	if !hasOwnershipOpaque(got, 4) {
		t.Fatalf("effect = %v, want root Opaque for param 4", got)
	}
	if !hasOwnershipFreeze(got, 5) {
		t.Fatalf("effect = %v, want root Freeze for param 5", got)
	}
	if hasOwnershipSendParam(got, 5) {
		t.Fatalf("effect = %v, did not expect descendant/non-recursive send export for param 5", got)
	}
	if hasOwnershipFreeze(got, 1) {
		t.Fatalf("effect = %v, did not expect descendant freeze export for param 1", got)
	}
	if !hasOwnershipBorrow(got, 6) {
		t.Fatalf("effect = %v, want root Borrow for param 6", got)
	}
}

func TestFunctionSummaryEffectExportsExactStoreRelationWithoutDegradedPair(t *testing.T) {
	got := functionSummaryEffect(summary.Summary{
		NormalReturnFacts: callboundary.NormalReturnFacts{
			EscapeEvents: []callboundary.EscapeEventFact{
				{Target: pathdom.NewPlaceholder(0), Kind: callboundary.EscapeEventStore, Recursive: true},
			},
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: pathdom.NewPlaceholder(1)},
			},
			StoreRelations: []callboundary.StoreRelationFact{
				{Source: pathdom.NewPlaceholder(0), Into: pathdom.NewPlaceholder(1)},
			},
		},
	}, typ.Func().
		Param("value", typ.Any).
		Param("container", typ.Any).
		Build())

	if !hasOwnershipStoreExact(got, 0, 1) {
		t.Fatalf("effect = %v, want exact Store{Param:0, Into:1}", got)
	}
	if hasOwnershipStoreUnknown(got, 0) {
		t.Fatalf("effect = %v, did not expect degraded Store{Param:0, Into:-1}", got)
	}
	if hasMutationTableMutator(got, 1) {
		t.Fatalf("effect = %v, did not expect redundant TableMutator{Target:1, Value:-1}", got)
	}
}

func TestFunctionSummaryOperationalEffectsPreservesDescendantBoundaryFacts(t *testing.T) {
	reg := standard.Registry()
	got := functionSummaryOperationalEffects(summary.Summary{
		ReturnPresenceRelations: []summary.ReturnPresenceRelation{
			{
				TriggerIndex:    1,
				TriggerPresence: presence.Present(),
				TargetIndex:     0,
				TargetPresence:  presence.Absent(),
			},
		},
		NormalReturnParams: []product.Value{
			typevalue.FromType(reg, typ.Number),
			product.Absent(reg),
		},
		NormalReturnFacts: callboundary.NormalReturnFacts{
			PathInvalidations: []callboundary.PathInvalidationFact{
				{Path: pathdom.NewPlaceholder(0).Field("items")},
			},
			FrozenTables: []callboundary.FrozenTableFact{
				{Target: pathdom.NewPlaceholder(1).Field("sealed")},
			},
			EscapeEvents: []callboundary.EscapeEventFact{
				{Target: pathdom.NewPlaceholder(0).Field("payload"), Kind: callboundary.EscapeEventSend, Recursive: true},
				{Target: pathdom.NewPlaceholder(1).Field("borrowed"), Kind: callboundary.EscapeEventBorrow},
			},
			StoreRelations: []callboundary.StoreRelationFact{
				{Source: pathdom.NewPlaceholder(0).Field("payload"), Into: pathdom.NewPlaceholder(1).Field("bucket")},
			},
		},
	}, typ.Func().
		Param("source", typ.Any).
		Param("target", typ.Any).
		Returns(typeexpr.Optional(typ.Number), typeexpr.Optional(typ.String)).
		Build())

	if got == nil {
		t.Fatalf("operational effects = nil")
	}
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v", got.ReturnPresenceRelations)
	}
	if len(got.NormalReturnPresenceRefinements) != 2 ||
		!got.NormalReturnPresenceRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) ||
		!presence.Equal(got.NormalReturnPresenceRefinements[0].Presence, presence.Present()) ||
		!got.NormalReturnPresenceRefinements[1].Path.Equal(pathdom.NewPlaceholder(1)) ||
		!presence.Equal(got.NormalReturnPresenceRefinements[1].Presence, presence.Absent()) {
		t.Fatalf("normal-return presence refinements = %#v", got.NormalReturnPresenceRefinements)
	}
	if len(got.PathInvalidations) != 1 || !got.PathInvalidations[0].Path.Equal(pathdom.NewPlaceholder(0).Field("items")) {
		t.Fatalf("path invalidations = %#v", got.PathInvalidations)
	}
	if len(got.FrozenTables) != 1 || !got.FrozenTables[0].Target.Equal(pathdom.NewPlaceholder(1).Field("sealed")) {
		t.Fatalf("frozen tables = %#v", got.FrozenTables)
	}
	if len(got.EscapeEvents) != 2 ||
		!got.EscapeEvents[0].Target.Equal(pathdom.NewPlaceholder(0).Field("payload")) ||
		got.EscapeEvents[0].Kind != signature.EscapeSend ||
		!got.EscapeEvents[0].Recursive ||
		!got.EscapeEvents[1].Target.Equal(pathdom.NewPlaceholder(1).Field("borrowed")) ||
		got.EscapeEvents[1].Kind != signature.EscapeBorrow ||
		got.EscapeEvents[1].Recursive {
		t.Fatalf("escape events = %#v", got.EscapeEvents)
	}
	if len(got.StoreRelations) != 1 ||
		!got.StoreRelations[0].Source.Equal(pathdom.NewPlaceholder(0).Field("payload")) ||
		!got.StoreRelations[0].Into.Equal(pathdom.NewPlaceholder(1).Field("bucket")) {
		t.Fatalf("store relations = %#v", got.StoreRelations)
	}
}

func TestFunctionSummaryOperationalEffectsEmptyIsAbsent(t *testing.T) {
	got := functionSummaryOperationalEffects(summary.Summary{}, typ.Func().
		Param("value", typ.Any).
		Build())
	if got != nil {
		t.Fatalf("operational effects = %#v, want nil for empty summary facts", got)
	}
}

func checkProgram(t *testing.T, src string) program.Result {
	t.Helper()
	stmts, err := parse.ParseString(src, "exportmanifest_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := program.RunChunk(stmts, program.Config{
		Check: body.Config{
			Registry: standard.Registry(),
			Signatures: signaturelookup.Source{
				IncludeStdlib: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}
	if diags := diagnostics.Produce(result.RootResult()); len(diags) != 0 {
		t.Fatalf("diagnostics = %#v, want none", diags)
	}
	return result
}

func hasErrorReturn(row effect.Row, valueIndex, errorIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		err, ok := effect.NormalizeLabel(label).(returns.ErrorReturn)
		return ok && err.ValueIndex == valueIndex && err.ErrorIndex == errorIndex
	})
}

func assertSignatureReturnPresenceRelation(
	t *testing.T,
	relations []signature.ReturnPresenceRelation,
	triggerIndex int,
	triggerPresence presence.Value,
	targetIndex int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex == triggerIndex &&
			presence.Equal(relation.TriggerPresence, triggerPresence) &&
			relation.TargetIndex == targetIndex &&
			presence.Equal(relation.TargetPresence, targetPresence) {
			return
		}
	}
	t.Fatalf("return presence relations = %#v, missing %d/%s -> %d/%s", relations, triggerIndex, triggerPresence, targetIndex, targetPresence)
}

func hasNormalReturnAbsentRefinement(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		refinement, ok := effect.NormalizeLabel(label).(postcondition.NormalReturnRefinement)
		if !ok || refinement.Target.Index != paramIndex {
			return false
		}
		return postcondition.Absent{}.Equals(refinement.Refinement)
	})
}

func hasOwnershipSendParam(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		send, ok := effect.NormalizeLabel(label).(ownership.SendParam)
		return ok && send.Param.Index == paramIndex
	})
}

func hasMutationTableMutator(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		mutator, ok := effect.NormalizeLabel(label).(mutation.TableMutator)
		return ok && mutator.Target.Index == paramIndex && mutator.Value.Index == -1
	})
}

func hasOwnershipStoreUnknown(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		return ok && store.Param.Index == paramIndex && store.Into.Index == -1
	})
}

func hasOwnershipStoreExact(row effect.Row, paramIndex, intoIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		return ok && store.Param.Index == paramIndex && store.Into.Index == intoIndex
	})
}

func hasOwnershipRetain(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		retain, ok := effect.NormalizeLabel(label).(ownership.Retain)
		return ok && retain.Param.Index == paramIndex
	})
}

func hasOwnershipBorrow(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		borrow, ok := effect.NormalizeLabel(label).(ownership.Borrow)
		return ok && borrow.Param.Index == paramIndex
	})
}

func hasOwnershipExport(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		export, ok := effect.NormalizeLabel(label).(ownership.Export)
		return ok && export.Param.Index == paramIndex
	})
}

func hasOwnershipOpaque(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		opaque, ok := effect.NormalizeLabel(label).(ownership.Opaque)
		return ok && opaque.Param.Index == paramIndex
	})
}

func hasOwnershipFreeze(row effect.Row, paramIndex int) bool {
	return row.Has(func(label effect.Label) bool {
		freeze, ok := effect.NormalizeLabel(label).(ownership.Freeze)
		return ok && freeze.Param.Index == paramIndex
	})
}
