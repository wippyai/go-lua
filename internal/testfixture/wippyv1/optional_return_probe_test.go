package wippyv1_test

import (
	"context"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
)

// This probe is the production instance of the host-boundary optional-return
// question. The synthetic and standard-library probes in manifesttarget already
// state the law; this one states the cost, on declarations a shipped runtime
// makes to every application running on it.
//
// http.Request:query answers nil for a query parameter the request does not
// carry, and v1 declares exactly that: string? alongside its error result. Every
// Wippy HTTP handler that reads an optional parameter depends on the checker
// knowing it, because the handler's own nil test is the only thing standing
// between a missing parameter and a concatenation of nil. If the sealed Target
// publishes string instead, the checker holds a proof of non-nilness that the
// module never gave, on the single most-called surface in the deployment.
//
// The probe asserts what the boundary owes.
func TestProductionOptionalReturnsKeepTheirNilability(t *testing.T) {
	target, err := wippyv1.Target()
	if err != nil {
		t.Fatalf("seal wippy v1 host target: %v", err)
	}
	for _, probe := range []struct {
		module string
		member string
		index  int
	}{
		{module: "http", member: "Request.query", index: 0},
		{module: "http", member: "Request.header", index: 0},
		{module: "http", member: "Request.content_type", index: 0},
		{module: "http", member: "Request.param", index: 0},
		{module: "http", member: "MultipartFile.header", index: 0},
	} {
		t.Run(probe.module+"."+probe.member, func(t *testing.T) {
			for arm, declared := range normalReturns(t, target, memberBinding(probe.module, probe.member), probe.index) {
				if typ.MayRuntimeKinds(declared)&runtimekind.Bit(runtimekind.Nil) == 0 {
					t.Fatalf("%s.%s is declared to answer an optional; the sealed Target publishes %s on normal arm %d, so every caller reads a nil-free proof the module never gave",
						probe.module, probe.member, declared, arm)
				}
			}
		})
	}
}

// normalReturns projects one sealed operation's normal outcome values back
// into static types through the published Target query surface only. A
// fallible member answers two correlated normal arms, and the declaration owes
// its nilability on each of them.
func normalReturns(t *testing.T, sealed *contract.Contract, binding vocabulary.BindingSpec, index int) []typ.Type {
	t.Helper()
	operation, ok := sealed.Operations.Lookup(binding)
	if !ok {
		t.Fatalf("sealed target has no operation for %+v", binding)
	}
	var out []typ.Type
	for outcome := 0; outcome < sealed.Operations.OutcomeCount(operation); outcome++ {
		kind, values, ok := sealed.Operations.OutcomeAt(operation, outcome)
		if !ok {
			t.Fatalf("operation %+v outcome %d unavailable", binding, outcome)
		}
		if kind != flowkind.OutcomeNormal {
			continue
		}
		valueType, ok := sealed.Operations.ValuesAt(values, index)
		if !ok {
			t.Fatalf("operation %+v normal value %d unavailable", binding, index)
		}
		declaration, ok := sealed.Operations.TypeDeclaration(valueType)
		if !ok {
			t.Fatalf("operation %+v normal value %d publishes no type declaration", binding, index)
		}
		decoded, err := domaincontract.Decode(context.Background(), declaration, nil)
		if err != nil || decoded == nil {
			t.Fatalf("decode operation %+v normal value %d: %v", binding, index, err)
		}
		out = append(out, decoded)
	}
	if len(out) == 0 {
		t.Fatalf("operation %+v publishes no normal outcome", binding)
	}
	return out
}
