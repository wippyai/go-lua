package manifesttarget_test

import (
	"context"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// A module that names its error type states that a trailing optional error is
// the error half of a value/error pair. The pair is a correlation: the value
// is present exactly when the error is absent. Without it a caller that tests
// the error learns nothing about the value it was answered beside, so the
// idiom the whole host surface is written in - call, test the error, use the
// value - carries no proof at all.
//
// These laws read the correlation back out of the sealed Target, through the
// same published query surface the checker consumes.

func correlationErrorType() typ.Type {
	return typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Any).Returns(typ.String).Build()},
	})
}

// TestDeclaredErrorTypeSealsCorrelatedNormalArms is the law itself: a member
// answering (T, Error?) under a declared module error type seals into two
// normal arms, one answering the value with no error and one answering nil
// with the error.
func TestDeclaredErrorTypeSealsCorrelatedNormalArms(t *testing.T) {
	errorType := correlationErrorType()
	handle := typ.NewInterface("Handle", []typ.Method{
		{Name: "close", Type: typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()},
	})
	sealed := hostProbeSealed(t, "probe.correlation", func(declaration *manifestwire.Manifest) {
		declaration.ErrorType = errorType
		declaration.DefineType("Error", errorType)
		declaration.DefineType("Handle", handle)
		declaration.DefineFunctionSignature("open", signature.Function{
			Type: typ.Func().Param("name", typ.String).Returns(handle, typeexpr.Optional(errorType)).Build(),
		})
	})

	arms := correlationNormalArms(t, sealed, hostProbeBinding("probe", "open"))
	if len(arms) != 2 {
		t.Fatalf("open publishes %d normal arms, want the value arm and the error arm", len(arms))
	}
	value, failure := correlationSplit(t, arms)
	if correlationIsNil(value[0]) {
		t.Fatalf("value arm answers nil for its value; the arm exists to say the value is there")
	}
	if !correlationIsNil(failure[0]) {
		t.Fatalf("error arm answers %s for its value, want nil; a member reporting an error has no value beside it", failure[0])
	}
	if !typ.TypeEquals(failure[1], errorType) {
		t.Fatalf("error arm answers %s, want the module error type", failure[1])
	}
}

// correlationSplit names the two correlated arms by what they state rather
// than by the order the Target happens to publish them in: the value arm is
// the arm with no error, the error arm is the arm that carries one.
func correlationSplit(t *testing.T, arms [][]typ.Type) (value []typ.Type, failure []typ.Type) {
	t.Helper()
	for _, arm := range arms {
		if len(arm) != 2 {
			t.Fatalf("correlated arm carries %d values, want a value and an error", len(arm))
		}
		if correlationIsNil(arm[1]) {
			if value != nil {
				t.Fatal("two arms answer no error; the correlation states exactly one")
			}
			value = arm
			continue
		}
		if failure != nil {
			t.Fatal("two arms answer an error; the correlation states exactly one")
		}
		failure = arm
	}
	if value == nil || failure == nil {
		t.Fatal("the published arms are not a value/error pair")
	}
	return value, failure
}

// TestDeclaredOptionalValueKeepsItsNilOnTheValueArm holds the derivation to
// what the correlation states and no more. A member that may answer nil on
// success keeps that nil on the value arm: the correlation rules out a value
// beside an error, never a declared nil beside no error.
func TestDeclaredOptionalValueKeepsItsNilOnTheValueArm(t *testing.T) {
	errorType := correlationErrorType()
	sealed := hostProbeSealed(t, "probe.correlation.optional", func(declaration *manifestwire.Manifest) {
		declaration.ErrorType = errorType
		declaration.DefineType("Error", errorType)
		declaration.DefineFunctionSignature("query", signature.Function{
			Type: typ.Func().Param("key", typ.String).
				Returns(typeexpr.Optional(typ.String), typeexpr.Optional(errorType)).Build(),
		})
	})

	arms := correlationNormalArms(t, sealed, hostProbeBinding("probe", "query"))
	if len(arms) != 2 {
		t.Fatalf("query publishes %d normal arms, want the value arm and the error arm", len(arms))
	}
	value, _ := correlationSplit(t, arms)
	if !hostProbeAdmitsNil(value[0]) {
		t.Fatalf("value arm publishes %s for a declared optional value; the correlation never proved the value present", value[0])
	}
}

// TestAuthoredArmsSurviveTheDerivation keeps one authority over one boundary.
// A provider that states its own normal arms has answered for the
// correlation, and the derivation may not restate it.
func TestAuthoredArmsSurviveTheDerivation(t *testing.T) {
	errorType := correlationErrorType()
	sealed := hostProbeSealed(t, "probe.correlation.authored", func(declaration *manifestwire.Manifest) {
		declaration.ErrorType = errorType
		declaration.DefineType("Error", errorType)
		declaration.DefineFunctionSignature("parse", signature.Function{
			Type: typ.Func().Param("text", typ.String).
				Returns(typeexpr.Optional(typ.Integer), typeexpr.Optional(errorType)).Build(),
		})
		declaration.DefineFunctionOperation("parse", manifestwire.Operation{
			ReplaceNormalSet: true,
			ReplaceNormal: []manifestwire.Values{
				{Fixed: []typ.Type{typ.Integer, typ.Nil}, Tail: manifestwire.ValuesClosed},
				{Fixed: []typ.Type{typ.Nil, errorType}, Tail: manifestwire.ValuesClosed},
			},
		})
	})

	arms := correlationNormalArms(t, sealed, hostProbeBinding("probe", "parse"))
	if len(arms) != 2 {
		t.Fatalf("parse publishes %d normal arms, want the two the provider authored", len(arms))
	}
	value, _ := correlationSplit(t, arms)
	if hostProbeAdmitsNil(value[0]) {
		t.Fatalf("value arm publishes %s; the provider authored a value arm that rules the nil out", value[0])
	}
}

// TestMembersWithoutTheErrorPairKeepOneNormalArm fences the derivation. A
// member whose trailing optional is not the module error, and a member whose
// only result is the error, state no value/error pair and gain no arm.
func TestMembersWithoutTheErrorPairKeepOneNormalArm(t *testing.T) {
	errorType := correlationErrorType()
	sealed := hostProbeSealed(t, "probe.correlation.unpaired", func(declaration *manifestwire.Manifest) {
		declaration.ErrorType = errorType
		declaration.DefineType("Error", errorType)
		declaration.DefineFunctionSignature("lookup", signature.Function{
			Type: typ.Func().Param("key", typ.String).
				Returns(typ.String, typeexpr.Optional(typ.Integer)).Build(),
		})
		declaration.DefineFunctionSignature("flush", signature.Function{
			Type: typ.Func().Returns(typeexpr.Optional(errorType)).Build(),
		})
		declaration.DefineFunctionSignature("name", signature.Function{
			Type: typ.Func().Returns(typ.String).Build(),
		})
	})

	for _, member := range []string{"lookup", "flush", "name"} {
		t.Run(member, func(t *testing.T) {
			if arms := correlationNormalArms(t, sealed, hostProbeBinding("probe", member)); len(arms) != 1 {
				t.Fatalf("%s publishes %d normal arms, want the one its signature states", member, len(arms))
			}
		})
	}
}

// hostProbeBinding addresses one member of a mounted probe module.
func hostProbeBinding(module string, member string) vocabulary.BindingSpec {
	return vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{module}, Member: []string{member}}
}

// correlationNormalArms decodes every published normal arm of one sealed
// operation into its fixed value types, in published order.
func correlationNormalArms(t *testing.T, sealed *contract.Contract, binding vocabulary.BindingSpec) [][]typ.Type {
	t.Helper()
	operation, ok := sealed.Operations.Lookup(binding)
	if !ok {
		t.Fatalf("sealed target has no operation for binding %+v", binding)
	}
	var arms [][]typ.Type
	for index := 0; index < sealed.Operations.OutcomeCount(operation); index++ {
		kind, values, ok := sealed.Operations.OutcomeAt(operation, index)
		if !ok {
			t.Fatalf("operation %+v outcome %d unavailable", binding, index)
		}
		if kind != flowkind.OutcomeNormal {
			continue
		}
		arm := make([]typ.Type, 0, sealed.Operations.ValuesCount(values))
		for slot := 0; slot < sealed.Operations.ValuesCount(values); slot++ {
			valueType, ok := sealed.Operations.ValuesAt(values, slot)
			if !ok {
				t.Fatalf("operation %+v outcome %d value %d unavailable", binding, index, slot)
			}
			declaration, ok := sealed.Operations.TypeDeclaration(valueType)
			if !ok {
				t.Fatalf("operation %+v outcome %d value %d publishes no type declaration", binding, index, slot)
			}
			decoded, err := domaincontract.Decode(context.Background(), declaration, nil)
			if err != nil || decoded == nil {
				t.Fatalf("decode operation %+v outcome %d value %d: %v", binding, index, slot, err)
			}
			arm = append(arm, decoded)
		}
		arms = append(arms, arm)
	}
	return arms
}

// correlationIsNil reports the exact nil answer an arm states for a slot the
// other arm owns.
func correlationIsNil(value typ.Type) bool {
	return typ.TypeEquals(value, typ.Nil)
}
