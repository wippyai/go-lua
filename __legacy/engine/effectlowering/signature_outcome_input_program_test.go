package effectlowering

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestSignatureOutcomeInputProgramReadsOnlyRegisteredDenseFactors(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := state.RegisteredProductDomain(reg)
	program, err := SealSignatureOutcomeOperands(domain, keys)
	if err != nil {
		t.Fatal(err)
	}
	program, err = program.WithHeapMemberQuery()
	if err != nil {
		t.Fatal(err)
	}
	program, err = program.WithPathValueQuery()
	if err != nil {
		t.Fatal(err)
	}
	if got := program.Lanes(); got.Len() != 2 || !got.Has(state.LaneHeapTableIdentity) || !got.Has(state.LanePathEvidence) {
		t.Fatalf("registered query lanes = %v", got.IDs())
	}

	memberSuffix := []segment.Segment{{Kind: segment.SegmentField, Name: "id"}}
	id := identity.ID{Kind: "test.table", Site: "signature-input", Index: 1}
	owner := identityvalue.WithExact(reg,
		product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Table)),
		id,
	)
	member := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	object := heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: owner})
	object, ok := object.WithStaticMember(reg, keys, memberSuffix, member)
	if !ok {
		t.Fatal("failed to construct heap member")
	}
	path := pathdom.Path{Symbol: symbol.ID(99), Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}}
	pathKey := keys.FromPath(path)
	pathValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	concrete := state.Reachable(state.State{}).
		WriteHeapTableObject(reg, id, object).
		WriteLocalPathKey(reg, pathKey, pathValue)

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
	frame, err := callpayload.BindConcreteExternalCallInputFrame(&carrier, []state.State{concrete}, []callpayload.DiagnosticOutput{{}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := frame.BindCallOutcomeInput(callpayload.CallOutcomeValueOperands{
		Arguments: []callpayload.CallOutcomeArgumentOperand{{Value: owner, Present: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := program.Bind(input)
	if err != nil {
		t.Fatal(err)
	}
	gotMember, present, err := bound.HeapMember(owner, memberSuffix)
	if err != nil || !present {
		t.Fatalf("heap member query: present=%t err=%v", present, err)
	}
	wantMember, wantPresent := sourcevalue.HeapMemberFromValue(reg, keys, concrete, owner, memberSuffix)
	if !wantPresent || !product.Equal(reg, gotMember, wantMember) {
		t.Fatal("factor-native heap member differs from concrete carrier law")
	}
	gotPath, present, err := bound.PathValue(path)
	if err != nil || !present || !product.Equal(reg, gotPath, pathValue) {
		t.Fatalf("factor-native path value = %#v present=%t err=%v", gotPath, present, err)
	}
	if argument, present := bound.Argument(0); !present || !product.Equal(reg, argument, owner) {
		t.Fatal("operand tuple changed while binding registered queries")
	}
}

func TestSignatureOutcomeInputProgramRejectsUndeclaredQueries(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := state.RegisteredProductDomain(reg)
	program, err := SealSignatureOutcomeOperands(domain, keys)
	if err != nil {
		t.Fatal(err)
	}
	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs:  []state.TransferInputAccess{{}},
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
	frame, err := carrier.BindFrame([]callpayload.ExternalCallInputWireOperands{{}})
	if err != nil {
		t.Fatal(err)
	}
	input, err := frame.BindCallOutcomeInput(callpayload.CallOutcomeValueOperands{})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := program.Bind(input)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := bound.HeapMember(product.Bottom(reg), nil); err == nil {
		t.Fatal("operand-only program admitted an undeclared heap query")
	}
	if _, _, err := bound.PathValue(pathdom.Path{Symbol: 1}); err == nil {
		t.Fatal("operand-only program admitted an undeclared path query")
	}
}

func TestSignatureOutcomeExtensionProgramsKeepPerExtensionAuthority(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain := state.RegisteredProductDomain(reg)
	operands, err := SealSignatureOutcomeOperands(domain, keys)
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
	first, err := SealSignatureArgumentTypeProgram(pathProgram, func(ctx SignatureArgumentTypeContext) (typ.Type, bool) {
		// This extension's input must not inherit the sibling heap grant.
		if _, _, heapErr := ctx.Input.HeapMember(ctx.Value, nil); heapErr == nil {
			t.Fatal("path extension inherited sibling heap authority")
		}
		return nil, false
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SealSignatureArgumentTypeProgram(heapProgram, func(ctx SignatureArgumentTypeContext) (typ.Type, bool) {
		if _, _, pathErr := ctx.Input.PathValue(pathdom.Path{Symbol: 1}); pathErr == nil {
			t.Fatal("heap extension inherited sibling path authority")
		}
		return typ.String, true
	})
	if err != nil {
		t.Fatal(err)
	}
	composed := ComposeSignatureArgumentTypePrograms(first, second)
	combined, err := composed.maximumInputProgram()
	if err != nil {
		t.Fatal(err)
	}
	if got := combined.Lanes(); got.Len() != 2 || !got.Has(state.LanePathEvidence) || !got.Has(state.LaneHeapTableIdentity) {
		t.Fatalf("composed extension lanes = %v", got.IDs())
	}

	access, err := state.SealTransferAccess(domain, state.TransferAccessConfig{
		ProviderInputs:  []state.TransferInputAccess{{Lanes: combined.Lanes()}},
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
	got, ok, err := composed.evaluate(input, SignatureArgumentTypeContext{Value: product.Bottom(reg)})
	if err != nil || !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("composed typed extension = %v ok=%t err=%v", got, ok, err)
	}

	reverse, err := UnionSignatureOutcomeInputPrograms(heapProgram, pathProgram)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := reverse.Lanes().IDs(), combined.Lanes().IDs(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("query program union is order-sensitive: %v vs %v", got, want)
	}
}

func TestSignatureOutcomeIntrinsicProgramSelectsHeapOnlyForMemberReturn(t *testing.T) {
	reg := standard.Registry()
	base, err := SealSignatureOutcomeOperands(state.RegisteredProductDomain(reg), keyspace.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		flow signature.ReturnFlowKind
		heap bool
	}{
		{name: "param", flow: signature.ReturnFlowParam},
		{name: "param member", flow: signature.ReturnFlowParamMember, heap: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			program, err := signatureOutcomeIntrinsicInputProgram(base, signature.Function{OperationalEffects: &signature.OperationalEffects{
				ReturnFlows: []signature.ReturnFlow{{Kind: tc.flow}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			if got := program.Lanes().Has(state.LaneHeapTableIdentity); got != tc.heap {
				t.Fatalf("heap query selected = %t, want %t", got, tc.heap)
			}
			if program.Lanes().Has(state.LanePathEvidence) {
				t.Fatal("intrinsic signature program stole operand path/dynamic authority")
			}
		})
	}
}

func TestSignatureOutcomeProviderCapabilityUsesIntrinsicRegisteredProgram(t *testing.T) {
	reg := standard.Registry()
	base, err := SealSignatureOutcomeOperands(state.RegisteredProductDomain(reg), keyspace.New())
	if err != nil {
		t.Fatal(err)
	}
	providers := func(flow signature.ReturnFlowKind) callpayload.CallOutcomeProgram {
		return SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
			Signatures: signatureMap{"test": {
				Type: typ.Func().Returns(typ.Any).Build(),
				OperationalEffects: &signature.OperationalEffects{
					ReturnFlows: []signature.ReturnFlow{{Kind: flow}},
				},
			}},
			NameForSite:  func(transfer.NodeContext, factflow.CallSiteView) (string, bool) { return "test", true },
			InputProgram: base,
		})
	}
	site := factflow.NewCallSite(factflow.CallSiteConfig{}).View()
	plain := testPrepareCallOutcome(t, providers(signature.ReturnFlowParam), transfer.NodeContext{Registry: reg}, site).Capability()
	if plain.PrimaryInputLanes().Len() != 0 {
		t.Fatalf("plain param flow lanes = %v", plain.PrimaryInputLanes().IDs())
	}
	member := testPrepareCallOutcome(t, providers(signature.ReturnFlowParamMember), transfer.NodeContext{Registry: reg}, site).Capability()
	if got := member.PrimaryInputLanes(); got.Len() != 1 || !got.Has(state.LaneHeapTableIdentity) {
		t.Fatalf("member param flow lanes = %v", got.IDs())
	}
}
