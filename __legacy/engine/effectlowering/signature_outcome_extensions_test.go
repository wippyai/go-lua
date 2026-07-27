package effectlowering

import (
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSignatureExtensionsPrepareExactSiteQueryLanes(t *testing.T) {
	reg := standard.Registry()
	operands, err := SealSignatureOutcomeOperands(state.RegisteredProductDomain(reg), keyspace.New())
	if err != nil {
		t.Fatal(err)
	}
	path, err := operands.WithPathValueQuery()
	if err != nil {
		t.Fatal(err)
	}
	heap, err := operands.WithHeapMemberQuery()
	if err != nil {
		t.Fatal(err)
	}
	argumentTypes, err := SealSignatureArgumentTypeProgram(path, func(SignatureArgumentTypeContext) (typ.Type, bool) {
		return typ.String, true
	})
	if err != nil {
		t.Fatal(err)
	}
	returnValues, err := SealSignatureReturnValueProgram(heap, func(site SignatureOutcomeSite) bool {
		if site.Name != "table.unpack" {
			return false
		}
		usesFirst := false
		site.Site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
			usesFirst = target.ResultIndex() == 0
			return !usesFirst
		})
		return usesFirst
	}, func(SignatureReturnValueInputContext) (product.Value, bool) {
		return product.Bottom(reg), true
	})
	if err != nil {
		t.Fatal(err)
	}

	resultZero := factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{})
	plainSite := factflow.NewCallSite(factflow.CallSiteConfig{}).View()
	unpackSite := factflow.NewCallSite(factflow.CallSiteConfig{ResultTargets: []factflow.CallResultTarget{resultZero}}).View()
	for _, tc := range []struct {
		name string
		site SignatureOutcomeSite
		want state.LaneSet
	}{
		{name: "operand-only", site: SignatureOutcomeSite{Site: plainSite, Name: "type", Signature: signature.Function{Type: typ.Func().Returns(typ.String).Build()}}, want: state.LaneSet{}},
		{name: "generic", site: SignatureOutcomeSite{Site: plainSite, Name: "generic", Signature: signature.Function{Type: typ.Func().TypeParam("T", typ.Any).Returns(typ.Any).Build()}}, want: state.NewLaneSet(state.LanePathEvidence)},
		{name: "table unpack result zero", site: SignatureOutcomeSite{Site: unpackSite, Name: "table.unpack", Signature: signature.Function{Type: typ.Func().Returns(typ.Any).Build()}}, want: state.NewLaneSet(state.LaneHeapTableIdentity)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preparedArguments, err := argumentTypes.PrepareSite(tc.site)
			if err != nil {
				t.Fatal(err)
			}
			preparedReturns, err := returnValues.PrepareSite(tc.site)
			if err != nil {
				t.Fatal(err)
			}
			input, err := UnionSignatureOutcomeInputPrograms(
				mustSignatureInputProgram(preparedArguments.InputProgram()),
				mustSignatureInputProgram(preparedReturns.InputProgram()),
			)
			if err != nil {
				t.Fatal(err)
			}
			got := input.Lanes().IDs()
			want := tc.want.IDs()
			if len(got) != len(want) {
				t.Fatalf("prepared lanes = %v, want %v", got, want)
			}
			for index := range want {
				if got[index] != want[index] {
					t.Fatalf("prepared lanes = %v, want %v", got, want)
				}
			}
		})
	}
}

func TestSignatureReturnValueProgramsKeepPerExtensionAuthorityAndOrder(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	operands, err := SealSignatureOutcomeOperands(domain, keyspace.New())
	if err != nil {
		t.Fatal(err)
	}
	pathProgram, err := operands.WithPathValueQuery()
	if err != nil {
		t.Fatal(err)
	}
	heapProgram, err := operands.WithHeapMemberQuery()
	if err != nil {
		t.Fatal(err)
	}

	visited := make([]string, 0, 2)
	first, err := SealSignatureReturnValueProgram(pathProgram, func(SignatureOutcomeSite) bool { return true }, func(ctx SignatureReturnValueInputContext) (product.Value, bool) {
		visited = append(visited, "path")
		if _, _, heapErr := ctx.Input.HeapMember(product.Bottom(reg), nil); heapErr == nil {
			t.Fatal("path return extension inherited sibling heap authority")
		}
		return product.Value{}, false
	})
	if err != nil {
		t.Fatal(err)
	}
	want := product.Bottom(reg)
	second, err := SealSignatureReturnValueProgram(heapProgram, func(SignatureOutcomeSite) bool { return true }, func(ctx SignatureReturnValueInputContext) (product.Value, bool) {
		visited = append(visited, "heap")
		if _, _, pathErr := ctx.Input.PathValue(pathdom.Path{Symbol: 1}); pathErr == nil {
			t.Fatal("heap return extension inherited sibling path authority")
		}
		return want, true
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := SealSignatureReturnValueProgram(operands, func(SignatureOutcomeSite) bool { return true }, func(SignatureReturnValueInputContext) (product.Value, bool) {
		visited = append(visited, "unreachable")
		return product.Top(), true
	})
	if err != nil {
		t.Fatal(err)
	}

	program := ComposeSignatureReturnValuePrograms(first, second, third)
	combined, err := program.maximumInputProgram()
	if err != nil {
		t.Fatal(err)
	}
	input := bindSignatureExtensionTestInput(t, domain, combined)
	got, ok, err := program.evaluate(input, SignatureReturnValueInputContext{Name: "test.return", Index: 1})
	if err != nil || !ok || !product.Equal(reg, got, want) {
		t.Fatalf("composed return extension = %#v ok=%t err=%v", got, ok, err)
	}
	if len(visited) != 2 || visited[0] != "path" || visited[1] != "heap" {
		t.Fatalf("extension order/short-circuit = %v", visited)
	}
}

func TestSignatureExtensionInputUnionRejectsDifferentOwners(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	left, err := SealSignatureOutcomeOperands(domain, keyspace.New())
	if err != nil {
		t.Fatal(err)
	}
	right, err := SealSignatureOutcomeOperands(domain, keyspace.New())
	if err != nil {
		t.Fatal(err)
	}
	leftProgram, err := SealSignatureArgumentTypeProgram(left, func(SignatureArgumentTypeContext) (typ.Type, bool) {
		return nil, false
	})
	if err != nil {
		t.Fatal(err)
	}
	rightProgram, err := SealSignatureArgumentTypeProgram(right, func(SignatureArgumentTypeContext) (typ.Type, bool) {
		return nil, false
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ComposeSignatureArgumentTypePrograms(leftProgram, rightProgram).maximumInputProgram(); err == nil {
		t.Fatal("extension union admitted different keyspace owners")
	}
}

func bindSignatureExtensionTestInput(t *testing.T, domain state.ProductDomain, program SignatureOutcomeInputProgram) callpayload.CallOutcomeInput {
	t.Helper()
	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs:  []state.TransferInputAccess{{Lanes: program.Lanes()}},
		ValueCarry:      0,
		LaneCarry:       0,
		DiagnosticCarry: 0,
		ReachableCarry:  0,
	})
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := callpayload.PrepareExternalCallInputProgram(domain, access, []cfg.Point{1}, 0, func(slot statekey.Value) (statekey.Value, bool) { return slot, true })
	if err != nil {
		t.Fatal(err)
	}
	frame, err := callpayload.BindConcreteExternalCallInputFrame(&carrier, []state.State{state.Reachable(state.State{})}, []callpayload.DiagnosticOutput{{}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := frame.BindCallOutcomeInput(callpayload.CallOutcomeValueOperands{})
	if err != nil {
		t.Fatal(err)
	}
	return input
}
