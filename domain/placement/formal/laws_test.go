package formal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

func TestFormalSelectorInjectionKinds(t *testing.T) {
	tests := []struct {
		name    string
		spec    vocabulary.FormalEffectSpec
		start   int
		end     int
		unknown bool
		owns    bool
		valid   bool
	}{
		{name: "borrow", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrow, Param: -1}, valid: true},
		{name: "retain", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: 1}, start: 1, end: 2, owns: true, valid: true},
		{name: "store", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 2, HasInto: true, Into: 0}, start: 2, end: 3, owns: true, valid: true},
		{name: "borrow-all", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrowAll}, valid: true},
		{name: "send-suffix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 1}, start: 1, end: 4, owns: true, valid: true},
		{name: "send-param", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendParam, Param: 0}, start: 0, end: 1, owns: true, valid: true},
		{name: "export", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectExport, Param: 3}, start: 3, end: 4, owns: true, valid: true},
		{name: "opaque", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectOpaque, Param: 0}, start: 0, end: 1, owns: true, valid: true},
		{name: "freeze", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectFreeze, Param: 2}, valid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveFormalSelectorRange(test.spec, 4, false)
			if got.start != test.start || got.end != test.end || got.unknown != test.unknown || got.owns != test.owns || got.valid != test.valid {
				t.Fatalf("selector = %#v, want [%d,%d) unknown=%v owns=%v valid=%v", got, test.start, test.end, test.unknown, test.owns, test.valid)
			}
		})
	}
}

func TestFormalSelectorInjectionRuntimeLastAndSuffix(t *testing.T) {
	last := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendParam, Param: -1}, 3, false)
	if last.start != 2 || last.end != 3 || last.unknown || !last.owns {
		t.Fatalf("closed runtime-last selector = %#v", last)
	}
	openLast := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: -1}, 3, true)
	if openLast.start != 0 || openLast.end != 0 || !openLast.unknown || !openLast.owns {
		t.Fatalf("open runtime-last selector = %#v", openLast)
	}
	// A trailing selector at a zero-actual call names no supplied actual, so
	// it selects nothing.
	zeroLast := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: -1}, 0, false)
	if zeroLast.start != 0 || zeroLast.end != 0 || zeroLast.unknown || !zeroLast.owns || !zeroLast.valid {
		t.Fatalf("empty runtime-last selector = %#v, want a valid empty selector", zeroLast)
	}
	openSuffix := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 2}, 3, true)
	if openSuffix.start != 2 || openSuffix.end != 3 || !openSuffix.unknown || !openSuffix.owns {
		t.Fatalf("open suffix selector = %#v", openSuffix)
	}
	// A suffix that begins past the supplied prefix sends nothing.
	closedEmptySuffix := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 9}, 3, false)
	if closedEmptySuffix.unknown || !closedEmptySuffix.owns || !closedEmptySuffix.valid || closedEmptySuffix.start != 3 || closedEmptySuffix.end != 3 {
		t.Fatalf("closed out-of-range suffix selector = %#v, want a valid empty suffix", closedEmptySuffix)
	}
}

func TestFormalSelectorRangeSemantics(t *testing.T) {
	tests := []struct {
		name        string
		spec        vocabulary.FormalEffectSpec
		actuals     int
		runtimeTail bool
		start       int
		end         int
		unknown     bool
		owns        bool
		valid       bool
	}{
		{name: "fixed", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 1}, actuals: 3, start: 1, end: 2, owns: true, valid: true},
		{name: "runtime-last", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendParam, Param: -1}, actuals: 3, runtimeTail: true, start: 0, end: 0, unknown: true, owns: true, valid: true},
		{name: "suffix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 1}, actuals: 3, start: 1, end: 3, owns: true, valid: true},
		{name: "empty-suffix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 9}, actuals: 3, start: 3, end: 3, owns: true, valid: true},
		{name: "open-suffix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 1}, actuals: 3, runtimeTail: true, start: 1, end: 3, unknown: true, owns: true, valid: true},
		{name: "negative-count", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: 0}, actuals: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rangeResult := resolveFormalSelectorRange(test.spec, test.actuals, test.runtimeTail)
			if rangeResult.start != test.start || rangeResult.end != test.end || rangeResult.unknown != test.unknown || rangeResult.owns != test.owns || rangeResult.valid != test.valid {
				t.Fatalf("range = %#v, want [%d,%d) unknown=%t owns=%t valid=%t", rangeResult, test.start, test.end, test.unknown, test.owns, test.valid)
			}
		})
	}
}

func TestFormalSelectorInjectionOpenTailBroadcastContract(t *testing.T) {
	// A RowUnknownOpen tail is applied to every fixed actual by planFor. The
	// selector itself remains finite: runtime uncertainty is represented by
	// the separate tail pass, and a runtime actual is never fabricated as a
	// Value coordinate.
	fixed := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: 0}, 2, true)
	if fixed.start != 0 || fixed.end != 1 || fixed.unknown || !fixed.owns {
		t.Fatalf("open fixed selector = %#v", fixed)
	}
}

func TestFormalEscapeInjectionAndNonownership(t *testing.T) {
	tests := []struct {
		kind vocabulary.FormalEffectKind
		want placement.Escape
		own  bool
	}{
		{vocabulary.FormalEffectInvalid, placement.None, false},
		{vocabulary.FormalEffectBorrow, placement.None, false},
		{vocabulary.FormalEffectRetain, placement.Retain, true},
		{vocabulary.FormalEffectStore, placement.Store, true},
		{vocabulary.FormalEffectBorrowAll, placement.None, false},
		{vocabulary.FormalEffectSendSuffix, placement.Send, true},
		{vocabulary.FormalEffectSendParam, placement.Send, true},
		{vocabulary.FormalEffectExport, placement.Export, true},
		{vocabulary.FormalEffectOpaque, placement.Opaque, true},
		{vocabulary.FormalEffectFreeze, placement.None, false},
	}
	for _, test := range tests {
		got, owns := FormalEscape(test.kind)
		if got != test.want || owns != test.own {
			t.Errorf("FormalEscape(%v) = %v/%v, want %v/%v", test.kind, got, owns, test.want, test.own)
		}
	}
	// Into is provenance only: changing it cannot alter source selection.
	left := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 1, HasInto: true, Into: 0}, 3, false)
	right := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 1, HasInto: true, Into: 2}, 3, false)
	if left != right {
		t.Fatalf("Store Into changed source selection: left=%#v right=%#v", left, right)
	}
}

func TestFormalAlternateTargetJoinInjection(t *testing.T) {
	if got := strongerEscape(placement.Retain, placement.Store); got != placement.Store {
		t.Fatalf("owned alternate join = %v, want store provenance", got)
	}
	if got := strongerEscape(placement.Send, placement.Export); got != placement.Export {
		t.Fatalf("shared alternate join = %v, want export provenance", got)
	}
	if got := strongerEscape(placement.Retain, placement.Send); got != placement.Send {
		t.Fatalf("owned/shared alternate join = %v, want send", got)
	}
}

// TestManifestFormalEffectDisplacementContract is the smallest law that
// crosses the manifest boundary without depending on an Artifact occurrence
// walk. It seals one provider declaration into Target's immutable formal row,
// then applies the same selector/escape reduction used by the mounted formal
// Rule. Owned escapes (retain/store) stay at OwnedHeap, shared escapes
// (send/export/opaque) reach SharedHeap, and the independent return escape is
// still an OwnedHeap requirement. An unavailable formal coordinate refuses
// the plan rather than manufacturing an all-root Unknown route.
func TestManifestFormalEffectDisplacementContract(t *testing.T) {
	contract := sealManifestFormalEffectContract(t)
	op, opOK := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"formal-displacement"},
		Member:    []string{"formal"},
	})
	if !opOK {
		t.Fatal("sealed manifest formal operation is unavailable")
	}
	want := []struct {
		row       vocabulary.FormalEffectSpec
		placement placement.Placement
	}{
		{row: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: 0}, placement: placement.OwnedHeap},
		{row: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 1, Into: 2, HasInto: true}, placement: placement.OwnedHeap},
		{row: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendParam, Param: 0}, placement: placement.SharedHeap},
		{row: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectExport, Param: 1}, placement: placement.SharedHeap},
		{row: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectOpaque, Param: 2}, placement: placement.SharedHeap},
	}
	if got := contract.Operations.FormalEffectCount(op); got != len(want) {
		t.Fatalf("sealed manifest formal row count = %d, want %d", got, len(want))
	}
	for index, expected := range want {
		got, rowOK := contract.Operations.FormalEffectAt(op, index)
		if !rowOK || got != expected.row {
			t.Fatalf("formal row %d = %#v/%v, want %#v/true", index, got, rowOK, expected.row)
		}
		selection := resolveFormalSelectorRange(got, 3, false)
		if selection.unknown || !selection.owns || selection.start != int(expected.row.Param) || selection.end != int(expected.row.Param)+1 {
			t.Fatalf("formal row %d selected %#v, want one owned fixed actual %d", index, selection, expected.row.Param)
		}
		escape, owns := FormalEscape(got.Kind)
		if !owns || !escape.ValidManifest() {
			t.Fatalf("formal row %d escaped as %v/%v, want a manifest escape", index, escape, owns)
		}
		if gotPlacement, displacementOK := placement.DisplaceChecked(placement.OwnedHeap, escape); !displacementOK || gotPlacement != expected.placement {
			t.Fatalf("formal row %d displacement = %v/%t, want %v/true", index, gotPlacement, displacementOK, expected.placement)
		}
	}
	if got, ok := placement.DisplaceChecked(placement.OwnedHeap, placement.Return); !ok || got != placement.OwnedHeap {
		t.Fatalf("independent return displacement = %v/%t, want owned-heap/true", got, ok)
	}

	// An authored coordinate this call site does not supply is bound to nil by
	// Lua, so it selects nothing. The selector stays valid and owned: it is
	// neither widened to every allocation root nor refused.
	unsupplied := vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 7}
	selection := resolveFormalSelectorRange(unsupplied, 3, false)
	if !selection.valid || selection.unknown || !selection.owns || selection.start != selection.end {
		t.Fatalf("unsupplied formal coordinate selected %#v, want a valid empty owned selector", selection)
	}
	openTail := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendParam, Param: -1}, 2, true)
	if !openTail.unknown || !openTail.owns || openTail.start != 0 || openTail.end != 0 {
		t.Fatalf("open runtime-last formal selected %#v, want conservative unknown", openTail)
	}
}

func sealManifestFormalEffectContract(t *testing.T) *contract.Contract {
	t.Helper()
	provider := manifest.Provider{
		Identity: "formal-displacement",
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("formal-displacement")
			declaration.DefineFunctionSignature("formal", signature.Function{
				Type: typ.Func().Param("first", typ.Any).Param("second", typ.Any).Param("third", typ.Any).Returns(typ.Any).Build(),
				Effect: effect.Row{Labels: []effect.Label{
					ownership.Retain{Param: effect.ParamRef{Index: 0}},
					ownership.Store{Param: effect.ParamRef{Index: 1}, Into: effect.ParamRef{Index: 2}},
					ownership.SendParam{Param: effect.ParamRef{Index: 0}},
					ownership.Export{Param: effect.ParamRef{Index: 1}},
					ownership.Opaque{Param: effect.ParamRef{Index: 2}},
				}},
			})
			return declaration
		},
	}
	catalogue, err := manifest.Seal(append(stdlib.Providers(), provider)...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}
