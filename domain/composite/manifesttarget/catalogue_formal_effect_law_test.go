package manifesttarget_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

func TestManifestFormalEffectsProjectSupportedOwnershipKinds(t *testing.T) {
	labels := []effect.Label{
		ownership.SendParam{Param: effect.ParamRef{Index: -1}},
		ownership.Send{FromParam: 5},
		ownership.BorrowAll{},
		ownership.Store{Param: effect.ParamRef{Index: -1}, Into: effect.ParamRef{Index: -1}},
		ownership.Retain{Param: effect.ParamRef{Index: 4}},
		ownership.Borrow{Param: effect.ParamRef{Index: -1}},
	}
	contract := sealFormalCatalogue(t, signature.Function{
		Type:   typ.Func().Param("value", typ.Any).Returns(typ.Any).Build(),
		Effect: effect.Row{Labels: labels, Tail: &effect.Var{Name: "tail"}},
	})
	op, ok := contract.Operations.Lookup(formalBinding())
	if !ok {
		t.Fatal("formal operation missing")
	}

	want := []vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectBorrow, Param: -1},
		{Kind: vocabulary.FormalEffectRetain, Param: 4},
		{Kind: vocabulary.FormalEffectStore, Param: -1, Into: -1},
		{Kind: vocabulary.FormalEffectBorrowAll},
		{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 5},
		{Kind: vocabulary.FormalEffectSendParam, Param: -1},
	}
	if got := contract.Operations.FormalEffectCount(op); got != len(want) {
		t.Fatalf("formal effect count = %d, want %d", got, len(want))
	}
	for index, expected := range want {
		got, ok := contract.Operations.FormalEffectAt(op, index)
		if !ok || got != expected {
			t.Fatalf("formal effect %d = %#v/%v, want %#v/true", index, got, ok, expected)
		}
	}
	if tail, ok := contract.Operations.FormalEffectTail(op); !ok || tail != vocabulary.RowUnknownOpen {
		t.Fatalf("formal effect tail = %d/%v, want unknown-open/true", tail, ok)
	}
	// Formal ownership metadata is not an invocation row and cannot acquire a
	// publication descriptor as a side effect of crossing the manifest. The
	// open signature tail still retains the ordinary opaque invocation effect
	// envelope, independently of the formal ownership occurrences.
	if got := contract.Operations.EffectCount(op); got != 1 {
		t.Fatalf("open ownership declaration emitted %d invocation effects, want 1 opaque row", got)
	}
	if _, ok := contract.Operations.EffectPublication(op, 0); ok {
		t.Fatal("ownership metadata unexpectedly acquired publication metadata")
	}
}

func TestManifestFormalEffectsKeepKnownNonOwnershipAndOpenTailSeparate(t *testing.T) {
	known := sealFormalCatalogue(t, signature.Function{
		Type:   typ.Func().Returns(typ.Any).Build(),
		Effect: effect.Empty.With(dispatch.ModuleLoad{}),
	})
	knownOp, ok := known.Operations.Lookup(formalBinding())
	if !ok {
		t.Fatal("known-label operation missing")
	}
	if got := known.Operations.FormalEffectCount(knownOp); got != 0 {
		t.Fatalf("known non-ownership label produced %d formal effects", got)
	}
	if tail, ok := known.Operations.FormalEffectTail(knownOp); !ok || tail != vocabulary.RowClosed {
		t.Fatalf("known non-ownership formal tail = %d/%v, want closed/true", tail, ok)
	}
	if got := known.Operations.EffectCount(knownOp); got != 1 {
		t.Fatalf("known non-ownership label emitted %d invocation effects, want 1", got)
	}
	if _, ok := known.Operations.EffectPublication(knownOp, 0); ok {
		t.Fatal("known non-ownership invocation effect unexpectedly acquired publication metadata")
	}

	open := sealFormalCatalogue(t, signature.Function{
		Type:   typ.Func().Returns(typ.Any).Build(),
		Effect: effect.Row{Tail: &effect.Var{Name: "?"}},
	})
	openOp, ok := open.Operations.Lookup(formalBinding())
	if !ok {
		t.Fatal("open-label operation missing")
	}
	if tail, ok := open.Operations.FormalEffectTail(openOp); !ok || tail != vocabulary.RowUnknownOpen {
		t.Fatalf("open formal tail = %d/%v, want unknown-open/true", tail, ok)
	}
}

func TestManifestFormalEffectsSurviveReplacement(t *testing.T) {
	provider := manifest.Provider{
		Identity: "formal", Mount: manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("formal")
			declaration.DefineFunctionSignature("formal", signature.Function{
				Type:   typ.Func().Param("value", typ.Any).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(ownership.Borrow{Param: effect.ParamRef{Index: -1}}),
			})
			declaration.DefineFunctionOperation("formal", manifestwire.Operation{
				Replace: true,
				Input:   manifestwire.Values{Tail: manifestwire.ValuesClosed},
				Outcomes: []manifestwire.Outcome{
					{Kind: manifestwire.OutcomeNormal, Values: manifestwire.Values{Tail: manifestwire.ValuesClosed}},
					{Kind: manifestwire.OutcomeThrow, Values: manifestwire.Values{Fixed: []typ.Type{typ.Any}, Tail: manifestwire.ValuesClosed}},
				},
				Effects: manifestwire.RowSpec{Tail: manifestwire.RowClosed},
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
	op, ok := contract.Operations.Lookup(formalBinding())
	if !ok {
		t.Fatal("replacement operation missing")
	}
	if got := contract.Operations.FormalEffectCount(op); got != 1 {
		t.Fatalf("replacement formal effect count = %d, want 1", got)
	}
	if got, ok := contract.Operations.FormalEffectAt(op, 0); !ok || got != (vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrow, Param: -1}) {
		t.Fatalf("replacement formal effect = %#v/%v", got, ok)
	}
	if got := contract.Operations.EffectCount(op); got != 0 {
		t.Fatalf("replacement emitted %d invocation effects from ownership metadata", got)
	}
}

func sealFormalCatalogue(t *testing.T, function signature.Function) *contract.Contract {
	t.Helper()
	provider := manifest.Provider{
		Identity: "formal", Mount: manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("formal")
			declaration.DefineFunctionSignature("formal", function)
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

func formalBinding() vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"formal"}, Member: []string{"formal"}}
}
