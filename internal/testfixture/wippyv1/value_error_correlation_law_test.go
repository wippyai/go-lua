package wippyv1_test

import (
	"context"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
)

// The v1 runtime answers nil for the value of every fallible member that
// reports an error: store/store.go pushes nil beside invalidError, process/
// module.go answers pushProcessError(l, lua.LNil, ...), and the json, expr and
// http modules each push lua.LNil before their error. The whole application
// surface is written around that fact - call, test the error, use the value -
// and the module declares it once, by naming its error type.
//
// This law reads the fact back out of the sealed production Target. Without
// it, a caller that tests the error learns nothing about the value it was
// answered beside, and a caller that skips the test reads a value the checker
// believes cannot be nil.
func TestProductionFallibleMembersPublishCorrelatedArms(t *testing.T) {
	target, err := wippyv1.Target()
	if err != nil {
		t.Fatalf("seal wippy v1 host target: %v", err)
	}
	for _, probe := range []struct {
		module string
		member string
	}{
		{module: "store", member: "get"},
		{module: "store", member: "Store.has"},
		{module: "process", member: "send"},
		{module: "json", member: "decode"},
		{module: "expr", member: "eval"},
		{module: "http", member: "request"},
	} {
		t.Run(probe.module+"."+probe.member, func(t *testing.T) {
			arms := correlatedArms(t, target, memberBinding(probe.module, probe.member))
			if len(arms) != 2 {
				t.Fatalf("%s.%s publishes %d normal arms; a fallible member answers its value or its error",
					probe.module, probe.member, len(arms))
			}
			value, failure := arms[0], arms[1]
			if isNil(value[1]) == isNil(failure[1]) {
				t.Fatalf("%s.%s publishes two arms that agree about the error; the arms exist to disagree",
					probe.module, probe.member)
			}
			if isNil(failure[1]) {
				value, failure = failure, value
			}
			if !isNil(failure[0]) {
				t.Fatalf("%s.%s answers %s for its value on the arm that reports an error, want nil",
					probe.module, probe.member, failure[0])
			}
			if isNil(value[0]) {
				t.Fatalf("%s.%s answers nil for its value on the arm that reports no error; the arm exists to answer the value",
					probe.module, probe.member)
			}
		})
	}
}

// correlatedArms decodes every published normal arm of one sealed operation.
func correlatedArms(t *testing.T, sealed *contract.Contract, binding vocabulary.BindingSpec) [][]typ.Type {
	t.Helper()
	operation, ok := sealed.Operations.Lookup(binding)
	if !ok {
		t.Fatalf("sealed target has no operation for %+v", binding)
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
		if len(arm) != 2 {
			t.Fatalf("operation %+v normal arm %d carries %d values, want a value and an error", binding, index, len(arm))
		}
		arms = append(arms, arm)
	}
	return arms
}

// isNil is the exact nil one arm answers for the slot the other arm owns. A
// declared optional still admits nil, so a probe that asked whether a slot may
// be nil could not tell the two arms apart at all.
func isNil(value typ.Type) bool {
	return typ.TypeEquals(value, typ.Nil)
}
