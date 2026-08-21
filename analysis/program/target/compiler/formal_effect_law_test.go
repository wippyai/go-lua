package compiler

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func formalEffectOperation(occurrences []vocabulary.FormalEffectSpec, tail vocabulary.RowTail) vocabulary.OperationSpec {
	return vocabulary.OperationSpec{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"formal"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		FormalEffects: vocabulary.FormalEffectRow{
			Occurrences: occurrences,
			Tail:        tail,
		},
	}
}

func TestFormalEffectsCanonicalSetAndIdentity(t *testing.T) {
	left := formalEffectOperation([]vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectFreeze, Param: -1},
		{Kind: vocabulary.FormalEffectStore, Param: -1},
		{Kind: vocabulary.FormalEffectBorrow, Param: -1},
		{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 2},
	}, vocabulary.RowClosed)
	right := formalEffectOperation([]vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 2},
		{Kind: vocabulary.FormalEffectBorrow, Param: -1},
		{Kind: vocabulary.FormalEffectFreeze, Param: -1},
		{Kind: vocabulary.FormalEffectStore, Param: -1, Into: -1},
	}, vocabulary.RowClosed)
	leftContract := mustSeal(t, declaration.Spec{Operations: []vocabulary.OperationSpec{left}})
	rightContract := mustSeal(t, declaration.Spec{Operations: []vocabulary.OperationSpec{right}})
	leftOp, leftOK := leftContract.Operations.Lookup(left.Bindings[0])
	rightOp, rightOK := rightContract.Operations.Lookup(right.Bindings[0])
	if !leftOK || !rightOK {
		t.Fatal("formal operation lookup failed")
	}
	leftID, leftIDOK := leftContract.OperationContentID(leftOp)
	rightID, rightIDOK := rightContract.OperationContentID(rightOp)
	if !leftIDOK || !rightIDOK || leftID != rightID {
		t.Fatal("formal-effect permutation changed portable operation identity")
	}
	if leftContract.ContentID() != rightContract.ContentID() {
		t.Fatal("formal-effect permutation changed contract identity")
	}
	if got := leftContract.Operations.FormalEffectCount(leftOp); got != 4 {
		t.Fatalf("formal effect count = %d, want 4", got)
	}
	want := []vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectBorrow, Param: -1},
		{Kind: vocabulary.FormalEffectStore, Param: -1, Into: -1},
		{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 2},
		{Kind: vocabulary.FormalEffectFreeze, Param: -1},
	}
	for index, expected := range want {
		got, ok := leftContract.Operations.FormalEffectAt(leftOp, index)
		if !ok || got != expected {
			t.Fatalf("formal effect %d = %#v/%v, want %#v/true", index, got, ok, expected)
		}
	}
	if tail, ok := leftContract.Operations.FormalEffectTail(leftOp); !ok || tail != vocabulary.RowClosed {
		t.Fatalf("formal effect tail = %d/%v, want closed/true", tail, ok)
	}
}

func TestFormalEffectsValidation(t *testing.T) {
	valid := func(effect vocabulary.FormalEffectSpec) declaration.Spec {
		return declaration.Spec{Operations: []vocabulary.OperationSpec{formalEffectOperation([]vocabulary.FormalEffectSpec{effect}, vocabulary.RowClosed)}}
	}
	cases := []struct {
		name   string
		effect vocabulary.FormalEffectSpec
		tail   vocabulary.RowTail
	}{
		{"param below sentinel", vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrow, Param: -2}, vocabulary.RowClosed},
		{"unrelated operand", vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrow, Param: 0, FromParam: 1}, vocabulary.RowClosed},
		{"store into below zero", vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 0, HasInto: true, Into: -1}, vocabulary.RowClosed},
		{"negative suffix boundary", vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: -1}, vocabulary.RowClosed},
		{"row variable tail", vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrow, Param: 0}, vocabulary.RowVariable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testSeal(func() *declaration.Spec {
				spec := valid(test.effect)
				spec.Operations[0].FormalEffects.Tail = test.tail
				return &spec
			}()); err == nil {
				t.Fatal("invalid formal effect was accepted")
			}
		})
	}
	duplicate := formalEffectOperation([]vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectBorrow, Param: 0},
		{Kind: vocabulary.FormalEffectBorrow, Param: 0},
	}, vocabulary.RowClosed)
	if _, err := testSeal(&declaration.Spec{Operations: []vocabulary.OperationSpec{duplicate}}); err == nil {
		t.Fatal("duplicate formal effect was accepted")
	}
}
