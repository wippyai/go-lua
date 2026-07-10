package label_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

func TestIDForClassifiesAuditedLabels(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
		want  string
	}{
		{"return same as", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{}}, capability.ReturnsReturnSameAs},
		{"return element of", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{}}, capability.ReturnsReturnElementOf},
		{"return optional element of", returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{}}, capability.ReturnsReturnOptionalElementOf},
		{"return callback", returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{}}, capability.ReturnsReturnCallbackReturn},
		{"return array callback", returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{}}, capability.ReturnsReturnArrayOfCallbackReturn},
		{"return type projection", returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{}}, capability.ReturnsReturnTypeProjection},
		{"return deep element", returns.Return{ReturnIndex: 0, Transform: returns.DeepElementOf{}}, capability.ReturnsReturnDeepElementOf},
		{"return string unpack", returns.Return{ReturnIndex: 0, Transform: returns.StringUnpackValue{}}, capability.ReturnsReturnStringUnpackValue},
		{"return select case", returns.Return{ReturnIndex: 0, Transform: returns.SelectCaseOfParam{}}, capability.ReturnsReturnSelectCaseOfParam},
		{"return select result", returns.Return{ReturnIndex: 0, Transform: returns.SelectResultOfCases{}}, capability.ReturnsReturnSelectResultOfCases},
		{"error return", returns.ErrorReturn{}, capability.ReturnsErrorReturn},
		{"return length", returns.ReturnLength{}, capability.ReturnsReturnLength},
		{"correlated return", returns.CorrelatedReturn{}, capability.ReturnsCorrelatedReturn},
		{"postcondition normal return", postcondition.NormalReturnRefinement{}, capability.PostconditionNormalReturnRefinement},
		{"borrow", ownership.Borrow{}, capability.OwnershipBorrow},
		{"retain", ownership.Retain{}, capability.OwnershipRetain},
		{"store", ownership.Store{}, capability.OwnershipStore},
		{"send", ownership.Send{}, capability.OwnershipSend},
		{"send param", ownership.SendParam{}, capability.OwnershipSendParam},
		{"export", ownership.Export{}, capability.OwnershipExport},
		{"opaque", ownership.Opaque{}, capability.OwnershipOpaque},
		{"freeze", ownership.Freeze{}, capability.OwnershipFreeze},
		{"borrow all", ownership.BorrowAll{}, capability.OwnershipBorrowAll},
		{"iterator", iteration.Iterator{}, capability.IterationIterator},
		{"module load", dispatch.ModuleLoad{}, capability.DispatchModuleLoad},
		{"type predicate", dispatch.TypePredicate{}, capability.DispatchTypePredicate},
		{"variadic transform", dispatch.VariadicTransform{}, capability.DispatchVariadicTransform},
		{"mutate", mutation.Mutate{}, capability.MutationMutate},
		{"length change", mutation.LengthChange{}, capability.MutationLengthChange},
		{"table mutator", mutation.TableMutator{}, capability.MutationTableMutator},
		{"lifecycle acquire", lifecycle.Acquire{Protocol: typestate.Protocol("transaction")}, capability.LifecycleAcquire},
		{"lifecycle transition", lifecycle.Transition{Protocol: typestate.Protocol("transaction")}, capability.LifecycleTransition},
		{"lifecycle escape", lifecycle.Escape{Protocol: typestate.Protocol("transaction")}, capability.LifecycleEscape},
		{"throw", control.Throw{}, capability.ControlThrow},
		{"io", control.IO{}, capability.ControlIO},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := caplabel.IDFor(tt.label)
			if !ok || got != tt.want {
				t.Fatalf("IDFor(%T) = %q/%v, want %q/true", tt.label, got, ok, tt.want)
			}
			desc, ok := caplabel.DescriptorFor(tt.label)
			if !ok || desc.ID != tt.want {
				t.Fatalf("DescriptorFor(%T) = %#v/%v, want %q/true", tt.label, desc, ok, tt.want)
			}
		})
	}
}

func TestIDForNormalizesPointerLabelsAndReturnTransforms(t *testing.T) {
	label := &returns.Return{ReturnIndex: 0, Transform: &returns.ElementOf{}}
	got, ok := caplabel.IDFor(label)
	if !ok || got != capability.ReturnsReturnElementOf {
		t.Fatalf("IDFor(pointer return) = %q/%v, want %q/true", got, ok, capability.ReturnsReturnElementOf)
	}

	tests := []struct {
		name      string
		transform returns.ReturnType
		want      string
	}{
		{"same as", &returns.SameAs{}, capability.ReturnsReturnSameAs},
		{"element of", &returns.ElementOf{}, capability.ReturnsReturnElementOf},
		{"optional element of", &returns.OptionalElementOf{}, capability.ReturnsReturnOptionalElementOf},
		{"callback", &returns.CallbackReturn{}, capability.ReturnsReturnCallbackReturn},
		{"array callback", &returns.ArrayOfCallbackReturn{}, capability.ReturnsReturnArrayOfCallbackReturn},
		{"type projection", &returns.TypeProjection{}, capability.ReturnsReturnTypeProjection},
		{"deep element", &returns.DeepElementOf{}, capability.ReturnsReturnDeepElementOf},
		{"string unpack", &returns.StringUnpackValue{}, capability.ReturnsReturnStringUnpackValue},
		{"select case", &returns.SelectCaseOfParam{}, capability.ReturnsReturnSelectCaseOfParam},
		{"select result", &returns.SelectResultOfCases{}, capability.ReturnsReturnSelectResultOfCases},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := caplabel.IDForReturnTransform(tt.transform)
			if !ok || got != tt.want {
				t.Fatalf("IDForReturnTransform(%T) = %q/%v, want %q/true", tt.transform, got, ok, tt.want)
			}
		})
	}
}

func TestIDForRejectsUnknownReturnTransform(t *testing.T) {
	if got, ok := caplabel.IDFor(returns.Return{}); ok || got != "" {
		t.Fatalf("IDFor(empty return transform) = %q/%v, want rejected", got, ok)
	}
}
