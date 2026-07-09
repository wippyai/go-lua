package effectlowering

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type signatureMap map[string]signature.Function

func (m signatureMap) Lookup(name string) (signature.Function, bool) {
	sig, ok := m[name]
	return sig, ok
}

type moduleExportMap map[string]typ.Type

func (m moduleExportMap) LookupExport(path string) (typ.Type, bool) {
	t, ok := m[path]
	return t, ok
}

func staticName(name string) SignatureNameFunc {
	name = strings.TrimSpace(name)
	return func(transfer.NodeContext, factflow.CallProducer) (string, bool) {
		return name, name != ""
	}
}

func TestOperationalNormalReturnLanesTrackStorageRegistry(t *testing.T) {
	storage := callboundary.NormalReturnFactLanes()
	if len(operationalNormalReturnLanes) != len(storage) {
		t.Fatalf("operational lanes = %d, storage lanes = %d", len(operationalNormalReturnLanes), len(storage))
	}
	for i, lane := range operationalNormalReturnLanes {
		if lane.ID != storage[i].ID() {
			t.Fatalf("lane %d id = %s, want %s", i, lane.ID, storage[i].ID())
		}
		if lane.Storage.ID() != storage[i].ID() {
			t.Fatalf("lane %d storage id = %s, want %s", i, lane.Storage.ID(), storage[i].ID())
		}
		if lane.Value == nil {
			t.Fatalf("lane %s has nil operational handler", lane.ID)
		}
	}
}

func TestSignatureNormalReturnLanesTrackStorageRegistry(t *testing.T) {
	storage := callboundary.NormalReturnFactLanes()
	if len(signatureNormalReturnLanes) != len(storage) {
		t.Fatalf("signature lanes = %d, storage lanes = %d", len(signatureNormalReturnLanes), len(storage))
	}
	for i, lane := range signatureNormalReturnLanes {
		if lane.ID != storage[i].ID() {
			t.Fatalf("lane %d id = %s, want %s", i, lane.ID, storage[i].ID())
		}
		if lane.Storage.ID() != storage[i].ID() {
			t.Fatalf("lane %d storage id = %s, want %s", i, lane.Storage.ID(), storage[i].ID())
		}
		if lane.Value == nil {
			t.Fatalf("lane %s has nil signature handler", lane.ID)
		}
	}
}

type countingSourceValues struct {
	values map[factflow.ExprRef]product.Value
	calls  map[factflow.ExprRef]int
}

func (s *countingSourceValues) ValueOfSource(_ cfg.Point, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (product.Value, bool) {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return product.Value{}, false
	}
	s.calls[source.ExprRef]++
	value, ok := s.values[source.ExprRef]
	return value, ok
}

func TestTableMutatorRecordTypeTreatsFreshEmptyTableAsArray(t *testing.T) {
	target := typetable.NewRecord().Build()
	got, ok := tableMutatorRecordType(target, typ.String)
	want := typ.NewArray(typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("table mutator target type = %v/%v, want %v", got, ok, want)
	}
}

func TestTableMutatorRecordTypePreservesMixedRecordShape(t *testing.T) {
	target := typetable.NewRecord().Field("name", typ.String).Build()
	got, ok := tableMutatorRecordType(target, typ.String)
	want := typetable.NewRecord().
		Field("name", typ.String).
		MapComponent(typ.Integer, typ.String).
		Build()
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("table mutator target type = %v/%v, want %v", got, ok, want)
	}
}

func TestTableMutatorTargetTypeTransformsSupportedUnionArms(t *testing.T) {
	target := typeexpr.Union(
		typetable.NewRecord().Build(),
		typ.NewArray(typ.String),
	)
	got, ok := tableMutatorTargetType(target, typ.String)
	want := typ.NewArray(typ.String)
	if !ok || !typ.TypeEquals(got, want) {
		t.Fatalf("table mutator union target type = %v/%v, want %v", got, ok, want)
	}
}

func TestTableMutatorTargetTypeRejectsUnsupportedUnionArm(t *testing.T) {
	target := typeexpr.Union(typ.Nil, typ.NewArray(typ.String))
	if got, ok := tableMutatorTargetType(target, typ.String); ok {
		t.Fatalf("table mutator union target type = %v/true, want false for nil arm", got)
	}
}

func TestSignatureOutcomeProviderPrefersCallSiteNameResolver(t *testing.T) {
	reg := standard.Registry()
	callee := symbol.ID(701)
	calleePath := path.NewPath(callee, "f").Field("member")
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context:      factflow.CallSiteContextAssignmentSource,
		CalleeSymbol: callee,
		CalleePath:   calleePath,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, symbol.ID(702), path.NewPath(symbol.ID(702), "out")),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f.member": {Type: typ.Func().Returns(typ.String).Build()},
		},
		NameFor: func(transfer.NodeContext, factflow.CallProducer) (string, bool) {
			t.Fatal("producer fallback should not be used when NameForSite is present")
			return "", false
		},
		NameForSite: func(_ transfer.NodeContext, got factflow.CallSiteView) (string, bool) {
			if got.CalleeSymbol() != callee {
				t.Fatalf("callee symbol = %v, want %v", got.CalleeSymbol(), callee)
			}
			if !got.CalleePathEqual(calleePath) {
				t.Fatalf("callee path = %s, want %s", got.CalleePath().String(), calleePath.String())
			}
			return "f.member", true
		},
	})

	got := provider(transfer.NodeContext{Registry: reg}, site.View(), state.State{}, nil)
	if len(got.Results) != 1 || got.Results[0].Index != 0 {
		t.Fatalf("results = %#v, want one slot-zero result", got.Results)
	}
}

func TestAmbientChannelSendOutcomeProviderEscapesPayloadNotReceiver(t *testing.T) {
	channelType := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	provider := AmbientChannelSendOutcomeProvider(AmbientChannelSendOutcomeProviderConfig{
		ReceiverType: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
			return channelType, true
		},
	})
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		MethodName:        "send",
		ReceiverPath:      path.NewPath(symbol.ID(920), "out"),
		HasReceiverPath:   true,
		ArgumentSources:   []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(921), HasExpr: true}},
		ResultTargets:     nil,
		HasReceiverSource: false,
	})
	got := provider(transfer.NodeContext{}, site.View(), state.State{}, nil)

	if len(got.NormalReturnFacts.EscapeEvents) != 1 {
		t.Fatalf("escape events = %#v, want one channel payload send", got.NormalReturnFacts.EscapeEvents)
	}
	event := got.NormalReturnFacts.EscapeEvents[0]
	if event.Kind != callboundary.EscapeEventSend || !event.Recursive || !event.Target.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("escape event = %#v, want recursive send escape on payload placeholder $1", event)
	}
}

func TestAmbientChannelSendOutcomeProviderIgnoresNonChannelReceiver(t *testing.T) {
	provider := AmbientChannelSendOutcomeProvider(AmbientChannelSendOutcomeProviderConfig{
		ReceiverType: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
			return typetable.NewRecord().Build(), true
		},
	})
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		MethodName:      "send",
		ArgumentSources: []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(922), HasExpr: true}},
	})
	got := provider(transfer.NodeContext{}, site.View(), state.State{}, nil)
	if len(got.NormalReturnFacts.EscapeEvents) != 0 {
		t.Fatalf("escape events = %#v, want none for non-channel receiver", got.NormalReturnFacts.EscapeEvents)
	}
}

func TestAmbientChannelLifecycleOutcomeProviderClosedReceiveReturnsOptionalPayload(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(923)
	receiverSym := symbol.ID(923)
	receiver := path.NewPath(receiverSym, "ch")
	resolver := visibility.NewResolver(visibility.NewTable(map[cfg.Point]map[symbol.ID]ssa.Version{
		point: {
			receiverSym: {Root: "ch", Symbol: receiverSym, ID: 1},
		},
	}))
	receiverKey, ok := visibility.AddressAt(resolver, point, receiver).VisibleStateKey()
	if !ok {
		t.Fatal("receiver state key not resolved")
	}
	resource := state.TypestateResourceFromCanonicalKey(receiverKey, ChannelLifecycleProtocol)
	in := state.State{}.
		AcquireTypestate(resource, ChannelStateOpen, typestate.Obligation{}).
		TransitionTypestate(resource, ChannelStateOpen, ChannelStateClosed)
	channelType := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	provider := AmbientChannelLifecycleOutcomeProvider(AmbientChannelLifecycleOutcomeProviderConfig{
		ReceiverType: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
			return channelType, true
		},
		KeySpace: resolver.KeySpace(),
		Resolver: resolver,
	})
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		MethodName:      "receive",
		ReceiverPath:    receiver,
		HasReceiverPath: true,
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, site.View(), in, nil)

	if !got.PostReturnAuthority {
		t.Fatal("PostReturnAuthority = false, want closed receive result authority")
	}
	if len(got.Results) != 2 {
		t.Fatalf("results = %#v, want two receive result slots", got.Results)
	}
	assertTypeWitness(t, reg, got.Results[0].Value, typeexpr.Optional(typ.String))
	assertTypeWitness(t, reg, got.Results[1].Value, typ.Boolean)
}

func TestSignatureOutcomeProviderLowersLifecycleLabels(t *testing.T) {
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"open": {
				Effect: effect.Empty.With(lifecycle.Acquire{
					Target:   effect.ParamRef{Index: 0},
					Protocol: typestate.Protocol("transaction"),
					State:    typestate.State("open"),
					Obligation: typestate.Obligation{
						Final: typestate.State("closed"),
					},
				}).With(lifecycle.Transition{
					Target:   effect.ParamRef{Index: 0},
					Protocol: typestate.Protocol("transaction"),
					From:     typestate.State("open"),
					To:       typestate.State("closed"),
				}),
			},
		},
		NameFor: staticName("open"),
	})

	got := provider(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextStatement,
		ArgumentSources: []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(930), HasExpr: true},
		},
	}).View(), state.State{}, nil)

	facts := got.NormalReturnFacts.LifecycleFacts
	if len(facts) != 2 {
		t.Fatalf("LifecycleFacts = %#v, want acquire and transition", facts)
	}
	if facts[0].Kind != callboundary.LifecycleAcquire ||
		!facts[0].Target.Equal(path.NewPlaceholder(0)) ||
		facts[0].Protocol != typestate.Protocol("transaction") ||
		facts[0].To != typestate.State("open") ||
		facts[0].Obligation.Final != typestate.State("closed") {
		t.Fatalf("acquire fact = %#v, want transaction open with close obligation", facts[0])
	}
	if facts[1].Kind != callboundary.LifecycleTransition ||
		!facts[1].Target.Equal(path.NewPlaceholder(0)) ||
		facts[1].From != typestate.State("open") ||
		facts[1].To != typestate.State("closed") {
		t.Fatalf("transition fact = %#v, want open -> closed", facts[1])
	}
}

func TestSignatureOutcomeProviderLowersOperationalLifecycleEffects(t *testing.T) {
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"close": {
				OperationalEffects: &signature.OperationalEffects{
					LifecycleEffects: []signature.LifecycleEffect{{
						Target:   path.NewPlaceholder(0),
						Kind:     signature.LifecycleTransition,
						Protocol: typestate.Protocol("transaction"),
						From:     typestate.State("open"),
						To:       typestate.State("closed"),
					}},
				},
			},
		},
		NameFor: staticName("close"),
	})

	got := provider(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextStatement,
		ArgumentSources: []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(931), HasExpr: true},
		},
	}).View(), state.State{}, nil)

	facts := got.NormalReturnFacts.LifecycleFacts
	if len(facts) != 1 ||
		facts[0].Kind != callboundary.LifecycleTransition ||
		!facts[0].Target.Equal(path.NewPlaceholder(0)) ||
		facts[0].Protocol != typestate.Protocol("transaction") ||
		facts[0].From != typestate.State("open") ||
		facts[0].To != typestate.State("closed") {
		t.Fatalf("LifecycleFacts = %#v, want operational close transition", facts)
	}
}

func testReturnTypeOps() ReturnTypeOps {
	return ReturnTypeOps{
		CallableReturn: testCallableReturn,
		ElementOf:      testElementOf,
		TypeProjection: testTypeProjection,
		InstantiateGenericCall: func(fn *typ.Function, args []typ.Type) (GenericCallInstantiation, bool) {
			instantiated, violations, bindings := typecall.InstantiateGenericCallWithBindings(fn, args)
			out := GenericCallInstantiation{Type: instantiated}
			for _, binding := range bindings {
				out.TypeParams = append(out.TypeParams, binding.Param)
				out.TypeArgs = append(out.TypeArgs, binding.Type)
			}
			return out, instantiated != nil && len(violations) == 0
		},
	}
}

func TestContextualFunctionExpressionKeepsConcreteCallbackParams(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	predicate := typ.NewGeneric("Predicate", []*typ.TypeParam{outer},
		typ.Func().Param("item", outer).Returns(typ.Boolean).Build())
	fnParam := typ.NewTypeParam("T", nil)
	user := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build()
	actual := typ.Func().
		Param("user", user).
		Returns(typ.Boolean).
		Build()

	got := contextualFunctionExpressionSignatureType(actual, typ.Instantiate(predicate, fnParam))
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, user) {
		t.Fatalf("contextual callback type = %v, want concrete user param preserved", got)
	}
}

func TestContextualFunctionExpressionFillsUnknownCallbackParams(t *testing.T) {
	outer := typ.NewTypeParam("T", nil)
	predicate := typ.NewGeneric("Predicate", []*typ.TypeParam{outer},
		typ.Func().Param("item", outer).Returns(typ.Boolean).Build())
	fnParam := typ.NewTypeParam("T", nil)
	actual := typ.Func().
		Param("item", typ.Unknown).
		Returns(typ.Boolean).
		Build()

	got := contextualFunctionExpressionSignatureType(actual, typ.Instantiate(predicate, fnParam))
	fn, ok := got.(*typ.Function)
	if !ok || len(fn.Params) != 1 || fn.Params[0].Type != fnParam {
		t.Fatalf("contextual callback type = %v, want formal type parameter fill", got)
	}
}

func testCallableReturn(t typ.Type) (typ.Type, bool) {
	fn, ok := unwrap.Alias(t).(*typ.Function)
	if !ok || fn == nil || len(fn.Returns) == 0 || fn.Returns[0] == nil {
		return nil, false
	}
	return fn.Returns[0], true
}

func testElementOf(t typ.Type) (typ.Type, bool) {
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return tt.Element, tt.Element != nil
	case *typ.Map:
		return tt.Value, tt.Value != nil
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return nil, false
		}
		return typeexpr.Union(tt.Elements...), true
	default:
		return nil, false
	}
}

func testTypeProjection(source typ.Type, p projection.Projection) (typ.Type, bool) {
	current := source
	for _, step := range p.Steps {
		switch step.Kind {
		case projection.StepField:
			record, ok := unwrap.Alias(current).(*typ.Record)
			if !ok || record == nil {
				return nil, false
			}
			field := record.GetField(step.Field)
			if field == nil || field.Type == nil {
				return nil, false
			}
			current = field.Type
		case projection.StepCallableReturn:
			next, ok := testCallableReturn(current)
			if !ok {
				return nil, false
			}
			current = next
		case projection.StepGenericArg:
			if step.Index < 0 {
				return nil, false
			}
			inst, ok := unwrap.Alias(current).(*typ.Instantiated)
			if !ok || inst == nil || step.Index >= len(inst.TypeArgs) || inst.TypeArgs[step.Index] == nil {
				return nil, false
			}
			current = inst.TypeArgs[step.Index]
		default:
			return nil, false
		}
	}
	return current, current != nil
}

func TestSignatureOutcomeProviderMaterializesDeclaredReturns(t *testing.T) {
	reg := standard.Registry()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: symbol.ID(17),
	}).View(),

		state.State{}, nil).
		Results

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureOutcomeProviderMaterializesOptionalDeclaredReturn(t *testing.T) {
	reg := standard.Registry()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typeexpr.Optional(typ.String)).Build()},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: symbol.ID(18),
	}).View(),

		state.State{}, nil).
		Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertPresence(t, reg, got[0].Value, presence.Maybe())
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	assertTypeWitness(t, reg, got[0].Value, typeexpr.Optional(typ.String))
}

func TestSignatureOutcomeProviderMaterializesInterfaceDeclaredReturnAsPresent(t *testing.T) {
	reg := standard.Registry()
	iface := typ.NewInterface("Resource", []typ.Method{
		{
			Name: "close",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Nil).
				Build(),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(iface).Build()},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{
		CalleeSymbol: symbol.ID(19),
	}).View(),

		state.State{}, nil).
		Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertPresence(t, reg, got[0].Value, presence.Present())
	assertTypeWitness(t, reg, got[0].Value, iface)
}

func TestSignatureOutcomeProviderResolvesInterfaceMethodFromReceiverCallSource(t *testing.T) {
	reg := standard.Registry()
	callPoint := cfg.Point(23)
	receiverPoint := cfg.Point(22)
	receiverSource, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, receiverPoint, factflow.ValueSourceShape{})
	if !ok {
		t.Fatal("invalid receiver call source")
	}
	contractType := typ.NewInterface("contract.Contract", []typ.Method{
		{
			Name: "open",
			Type: typ.Func().
				Param("self", typ.Self).
				Param("id", typ.String).
				Returns(typ.Any, typeexpr.Optional(typ.String)).
				Build(),
		},
	})
	prior := state.State{}.WriteReturnSlot(reg, 0, typevalue.WithWitness(reg, typevalue.FromType(reg, contractType), contractType))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"contract.Contract.open": {Type: contractType.Methods[0].Type},
		},
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
	})

	got := provider(
		transfer.NodeContext{Registry: reg, Point: callPoint},
		factflow.NewCallSite(factflow.CallSiteConfig{
			MethodName:        "open",
			ReceiverSource:    receiverSource,
			HasReceiverSource: true,
		}).View(),
		state.State{},
		func(point cfg.Point) state.State {
			if point == receiverPoint {
				return prior
			}
			return state.State{}
		},
	)

	if len(got.Results) != 2 {
		t.Fatalf("got %d results, want receiver-source method signature returns: %#v", len(got.Results), got.Results)
	}
	if got.Results[0].Index != 0 || got.Results[1].Index != 1 {
		t.Fatalf("result indexes = %#v, want slots 0 and 1", got.Results)
	}
	assertPresence(t, reg, got.Results[1].Value, presence.Maybe())
	assertTypeWitness(t, reg, got.Results[1].Value, typeexpr.Optional(typ.String))
}

func TestSignatureOutcomeProviderBindsSelfReturnForFluentReceiverSource(t *testing.T) {
	reg := standard.Registry()
	receiverSymbol := symbol.ID(311)
	withScopePoint := cfg.Point(24)
	openPoint := cfg.Point(25)
	contractType := typ.NewInterface("contract.Contract", nil)
	withScopeType := typ.Func().
		Param("self", typ.Self).
		Param("scope", typ.Any).
		Returns(typ.Self).
		Build()
	openType := typ.Func().
		Param("self", typ.Self).
		Param("id", typ.String).
		Returns(typ.Any, typeexpr.Optional(typ.String)).
		Build()
	contractType.Methods = []typ.Method{
		{Name: "with_scope", Type: withScopeType},
		{Name: "open", Type: openType},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"contract.Contract.with_scope": {Type: withScopeType},
			"contract.Contract.open":       {Type: openType},
		},
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
	})
	receiverValue := typevalue.WithWitness(reg, typevalue.FromType(reg, contractType), contractType)
	withScope := provider(
		transfer.NodeContext{Registry: reg, Point: withScopePoint},
		factflow.NewCallSite(factflow.CallSiteConfig{
			MethodName:        "with_scope",
			ReceiverPath:      path.NewPath(receiverSymbol, "def"),
			HasReceiverPath:   true,
			ArgumentSources:   []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(311), HasExpr: true}},
			HasReceiverSource: false,
		}).View(),
		state.State{}.WriteValue(reg, key.SymbolValue(receiverSymbol), receiverValue),
		nil,
	)
	if len(withScope.Results) != 1 {
		t.Fatalf("with_scope results = %#v, want one Self-bound contract result", withScope.Results)
	}
	assertTypeWitness(t, reg, withScope.Results[0].Value, contractType)
	withScopeState := state.State{}.WriteReturnSlot(reg, 0, withScope.Results[0].Value)
	openReceiver, ok := factflow.NewCallValueSource(0, factflow.NoValueSourceIndex, factflow.NoValueSourceIndex, 0, withScopePoint, factflow.ValueSourceShape{})
	if !ok {
		t.Fatal("invalid open receiver source")
	}

	open := provider(
		transfer.NodeContext{Registry: reg, Point: openPoint},
		factflow.NewCallSite(factflow.CallSiteConfig{
			MethodName:        "open",
			ReceiverSource:    openReceiver,
			HasReceiverSource: true,
		}).View(),
		state.State{},
		func(point cfg.Point) state.State {
			if point == withScopePoint {
				return withScopeState
			}
			return state.State{}
		},
	)

	if len(open.Results) != 2 {
		t.Fatalf("open results = %#v, want receiver-source method signature after Self return", open.Results)
	}
	if open.Results[0].Index != 0 || open.Results[1].Index != 1 {
		t.Fatalf("open result indexes = %#v, want slots 0 and 1", open.Results)
	}
	assertPresence(t, reg, open.Results[1].Value, presence.Maybe())
}

func TestSignatureOutcomeProviderBindsSelfReturnWhenNameForSiteResolvesMethod(t *testing.T) {
	reg := standard.Registry()
	receiverSymbol := symbol.ID(411)
	point := cfg.Point(35)
	contractType := typ.NewInterface("contract.Contract", nil)
	withScopeType := typ.Func().
		Param("self", typ.Self).
		Param("scope", typ.Any).
		Returns(typ.Self).
		Build()
	contractType.Methods = []typ.Method{
		{Name: "with_scope", Type: withScopeType},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"contract.Contract.with_scope": {Type: withScopeType},
		},
		NameForSite: func(transfer.NodeContext, factflow.CallSiteView) (string, bool) {
			return "contract.Contract.with_scope", true
		},
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
	})
	receiverValue := typevalue.WithWitness(reg, typevalue.FromType(reg, contractType), contractType)
	got := provider(
		transfer.NodeContext{Registry: reg, Point: point},
		factflow.NewCallSite(factflow.CallSiteConfig{
			MethodName:      "with_scope",
			ReceiverPath:    path.NewPath(receiverSymbol, "contract"),
			HasReceiverPath: true,
			ArgumentSources: []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(411), HasExpr: true}},
		}).View(),
		state.State{}.WriteValue(reg, key.SymbolValue(receiverSymbol), receiverValue),
		nil,
	)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one Self-bound method result", got.Results)
	}
	assertTypeWitness(t, reg, got.Results[0].Value, contractType)
}

func TestSignatureOutcomeProviderLowersErrorReturnToReturnPresenceRelations(t *testing.T) {
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want true for matched signature")
	}
	if len(got.ReturnPresenceRelations) != 3 {
		t.Fatalf("return presence relations = %d, want 3: %#v", len(got.ReturnPresenceRelations), got.ReturnPresenceRelations)
	}
	assertCallReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	assertCallReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
	assertCallReturnPresenceRelation(t, got.ReturnPresenceRelations, 0, presence.Absent(), 1, presence.Present())
}

func TestActiveReturnLabelsUsesCentralLabelNormalization(t *testing.T) {
	label := returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	var nilLabel *returns.ErrorReturn
	got := activeErrorReturnLabels(signature.Function{
		Effect: effect.Row{Labels: []effect.Label{&label, nilLabel}},
	})
	if len(got) != 1 {
		t.Fatalf("activeErrorReturnLabels = %#v, want one normalized label", got)
	}
	if got[0] != label {
		t.Fatalf("activeErrorReturnLabels[0] = %#v, want %#v", got[0], label)
	}
}

func TestSignatureOutcomeProviderWeakAnyReturnIsNotPostReturnAuthority(t *testing.T) {
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"require": {Type: typ.Func().Returns(typ.Any).Build()},
		},
		NameFor: staticName("require"),
	})

	got := provider(transfer.NodeContext{Registry: standard.Registry()}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = true for weak any return, want false")
	}
	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one weak fallback result", got.Results)
	}
}

func TestModuleLoadOutcomeProviderIsRequireNameBound(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(243)
	argRef := factflow.ExprRef(244)
	modulePath := typ.LiteralString("pkg.mod")
	modulePathValue := typevalue.WithWitness(reg, typevalue.FromType(reg, modulePath), modulePath)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}},
	}).View()

	providerFor := func(name string) callpayload.CallOutcomeProvider {
		return ModuleLoadOutcomeProvider(ModuleLoadOutcomeProviderConfig{
			Exports: moduleExportMap{
				"pkg.mod": typ.Number,
			},
			NameFor: staticName(name),
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
				Registry: reg,
				ExpressionValues: map[factflow.ExprRef]product.Value{
					argRef: modulePathValue,
				},
			}),
		})
	}

	nonRequire := providerFor("loadModule")(transfer.NodeContext{Registry: reg, Point: point}, site, state.State{}, nil)
	if len(nonRequire.Results) != 0 || nonRequire.PostReturnAuthority {
		t.Fatalf("non-require module load outcome = %#v, want no operational rehydration", nonRequire)
	}

	got := providerFor("require")(transfer.NodeContext{Registry: reg, Point: point}, site, state.State{}, nil)
	if len(got.Results) != 1 {
		t.Fatalf("require module load results = %#v, want one export result", got.Results)
	}
	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want true for exact module export")
	}
	assertTypeWitness(t, reg, got.Results[0].Value, typ.Number)
}

func TestSignatureOutcomeProviderLowersNormalReturnRefinementToParamPathRefinementAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(801)
	argExpr := factflow.ExprRef(802)
	argSymbol := symbol.ID(803)
	argPath := path.NewPath(argSymbol, "x")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"assertLike": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Present{},
				}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "assertLike", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 {
		t.Fatalf("param path refinements = %d, want 1: %#v", len(got.ParamPathRefinements), got.ParamPathRefinements)
	}
	if !got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("target path = %s, want $0", got.ParamPathRefinements[0].Path.String())
	}
	assertPresence(t, reg, got.ParamPathRefinements[0].Value, presence.Present())

	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: provider,
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Present())
}

func TestSignatureOutcomeProviderLowersAbsentNormalReturnRefinementAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(804)
	argExpr := factflow.ExprRef(805)
	argSymbol := symbol.ID(806)
	argPath := path.NewPath(argSymbol, "err")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"isNil": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
					Target:     effect.ParamRef{Index: 0},
					Refinement: postcondition.Absent{},
				}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "isNil", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 {
		t.Fatalf("param path refinements = %d, want 1: %#v", len(got.ParamPathRefinements), got.ParamPathRefinements)
	}
	if !got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("target path = %s, want $0", got.ParamPathRefinements[0].Path.String())
	}
	assertPresence(t, reg, got.ParamPathRefinements[0].Value, presence.Absent())

	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: provider,
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Absent())
}

func TestSignatureOutcomeProviderNormalReturnRefinementDoesNotApplyWithoutExpressionPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(811)
	argSymbol := symbol.ID(812)
	existing := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(813), HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"assertLike": {
				Type: typ.Func().Param("v", typ.Any).Build(),
				Effect: effect.Empty.With(
					postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{}},
					postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 1}, Refinement: postcondition.Present{}},
				),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "assertLike", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathRefinements) != 1 || !got.ParamPathRefinements[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path refinements = %#v, want one unresolved $0 refinement", got.ParamPathRefinements)
	}
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(argSymbol), existing),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			Sources:     sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
			CallOutcome: provider,
		}),
	})
	assertStatePresence(t, reg, flow[graph.Exit()], key.SymbolValue(argSymbol), presence.Maybe())
}

func TestSignatureOutcomeProviderLowersTableMutatorToParamPathInvalidationAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	argExpr := factflow.ExprRef(901)
	argSymbol := symbol.ID(902)
	argPath := path.NewPath(argSymbol, "items")
	containerKey := path.PathKey("sym902@1")
	childKey := path.PathKey("sym902@1.child")
	unrelatedKey := path.PathKey("sym903@1.child")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
					mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 {
		t.Fatalf("param path invalidations = %d, want 1: %#v", len(got.ParamPathInvalidations), got.ParamPathInvalidations)
	}
	if !got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("invalidation path = %s, want $0", got.ParamPathInvalidations[0].Path.String())
	}
	if !got.ParamPathInvalidations[0].PreserveStructuralWitness {
		t.Fatalf("table-mutator invalidation should preserve the target structural witness: %#v", got.ParamPathInvalidations[0])
	}
	if len(got.NormalReturnFacts.PathInvalidations) != 1 ||
		!got.NormalReturnFacts.PathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("normal-return invalidations = %#v, want $0", got.NormalReturnFacts.PathInvalidations)
	}
	if !got.NormalReturnFacts.PathInvalidations[0].PreserveStructuralWitness {
		t.Fatalf("table-mutator normal-return invalidation should preserve the target structural witness: %#v", got.NormalReturnFacts.PathInvalidations[0])
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(call, argSymbol, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WritePathKey(reg, ks, containerKey, present).
			WritePathKey(reg, ks, childKey, present).
			WritePathKey(reg, ks, unrelatedKey, present),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  resolver,
		}),
	})
	assertPathValue(t, reg, ks, flow[graph.Exit()], containerKey, present)
	assertPathValue(t, reg, ks, flow[graph.Exit()], childKey, product.Bottom(reg))
	assertPathValue(t, reg, ks, flow[graph.Exit()], unrelatedKey, present)
}

func TestSignatureOutcomeProviderLowersTableMutatorValueToElementRefinement(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(911)
	valueExpr := factflow.ExprRef(912)
	listSymbol := symbol.ID(913)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typetable.NewRecord().Build()),
			valueExpr: returnValueFromType(reg, typ.String),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(typ.String)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	dynamic := got.NormalReturnFacts.DynamicIndexFacts[0]
	if !dynamic.Table.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("dynamic table = %s, want $0", dynamic.Table)
	}
	if keyType, ok := typevalue.TypeOf(reg, dynamic.Value.KeyValue); !ok || !typ.TypeEquals(keyType, typ.Integer) {
		t.Fatalf("dynamic key type = %v/%v, want integer", keyType, ok)
	}
	if valueType, ok := typevalue.TypeOf(reg, dynamic.Value.Value); !ok || !typ.TypeEquals(valueType, typ.String) {
		t.Fatalf("dynamic value type = %v/%v, want string", valueType, ok)
	}
	if dynamic.Value.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic admission = %v, want admitted", dynamic.Value.Admission)
	}
}

func TestSignatureOutcomeProviderLowersTableMutatorFromPathSources(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listSymbol := symbol.ID(9121)
	valueSymbol := symbol.ID(9122)
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(call, listSymbol, "items")
	visibilityBuilder.Define(call, valueSymbol, "id")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	listKey, ok := visibility.AddressAt(resolver, call, path.Path{Symbol: listSymbol}).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("missing list key")
	}
	valueKey, ok := visibility.AddressAt(resolver, call, path.Path{Symbol: valueSymbol}).RootOrVisibleStateKey()
	if !ok {
		t.Fatal("missing value key")
	}
	listSource, ok := factflow.NewPathValueSource(listKey.PathKey(), 0, 0, 0, factflow.ValueSourceShape{Final: false})
	if !ok {
		t.Fatal("list path source")
	}
	valueSource, ok := factflow.NewPathValueSource(valueKey.PathKey(), 1, 1, 0, factflow.ValueSourceShape{Final: true})
	if !ok {
		t.Fatal("value path source")
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:         factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{listSource, valueSource},
			}),
		},
	})
	listValue := returnValueFromType(reg, typetable.NewRecord().Build())
	valueValue := returnValueFromType(reg, typ.String)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(listSymbol), listValue).
		WriteValue(reg, key.SymbolValue(valueSymbol), valueValue)
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		KeySpace: ks,
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor:  staticName("table.insert"),
		Facts:    facts,
		Sources:  sources,
		KeySpace: ks,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, in, nil)

	if len(got.ParamPathWrites) != 1 || !got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	dynamic := got.NormalReturnFacts.DynamicIndexFacts[0]
	if !dynamic.ValuePath.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("dynamic value path = %s, want $1", dynamic.ValuePath)
	}
	if valueType, ok := typevalue.TypeOf(reg, dynamic.Value.Value); !ok || !typ.TypeEquals(valueType, typ.String) {
		t.Fatalf("dynamic value type = %v/%v, want string", valueType, ok)
	}
}

func TestSignatureOutcomeProviderReadsTableMutatorEvidenceOncePerLabel(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	listExpr := factflow.ExprRef(9071)
	valueExpr := factflow.ExprRef(9072)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
	})
	sources := &countingSourceValues{
		values: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typ.NewArray(typ.String)),
			valueExpr: returnValueFromType(reg, typ.String),
		},
		calls: map[factflow.ExprRef]int{},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 {
		t.Fatalf("param path writes = %#v, want one", got.ParamPathWrites)
	}
	if len(got.ParamObligations) != 1 {
		t.Fatalf("param obligations = %#v, want one", got.ParamObligations)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic-index facts = %#v, want one", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if sources.calls[listExpr] != 1 || sources.calls[valueExpr] != 1 {
		t.Fatalf("source reads = list:%d value:%d, want one read each", sources.calls[listExpr], sources.calls[valueExpr])
	}
}

func TestSignatureOutcomeProviderLowersTableMutatorElementTypeToValueObligation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(931)
	valueExpr := factflow.ExprRef(932)
	listSymbol := symbol.ID(933)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typ.NewArray(typ.String)),
			valueExpr: returnValueFromType(reg, typ.String),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamObligations) != 1 || got.ParamObligations[0].ParamIndex != 1 {
		t.Fatalf("param obligations = %#v, want one obligation for inserted value argument", got.ParamObligations)
	}
	if origin := got.ParamObligations[0].Origin; !origin.HasOrigin ||
		origin.SubjectLabel != "argument 2" || origin.ProviderLabel != "argument 1 element" {
		t.Fatalf("param obligation origin = %#v, want inserted value constrained by table element", origin)
	}
	obligationType, ok := typevalue.TypeOf(reg, got.ParamObligations[0].Value)
	if !ok || !typ.TypeEquals(obligationType, typ.String) {
		t.Fatalf("obligation type = %v/%v, want string", obligationType, ok)
	}
}

func TestSignatureOutcomeProviderKeepsAnyArrayMutatorWhenInsertedValueIsAny(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(914)
	valueExpr := factflow.ExprRef(915)
	listSymbol := symbol.ID(916)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typ.NewArray(typ.Any)),
			valueExpr: returnValueFromType(reg, typ.Any),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(typ.Any)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if want := returnValueFromType(reg, typ.Any); !product.Equal(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value, want) {
		t.Fatalf("dynamic value = %#v, want explicit any product", got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value)
	}
}

func TestSignatureOutcomeProviderKeepsAnyArrayMutatorWhenInsertedValueIsUnknown(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(9161)
	valueExpr := factflow.ExprRef(9162)
	listSymbol := symbol.ID(9163)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typ.NewArray(typ.Any)),
			valueExpr: returnValueFromType(reg, typ.Unknown),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(typ.Any)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if want := returnValueFromType(reg, typ.Any); !product.Equal(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value, want) {
		t.Fatalf("dynamic value = %#v, want explicit any product", got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value)
	}
}

func TestSignatureOutcomeProviderKeepsAnyArrayMutatorWhenInsertedValueIsConcreteRecord(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(91621)
	valueExpr := factflow.ExprRef(91622)
	listSymbol := symbol.ID(91623)
	entryType := typetable.NewRecord().
		Field("id", typ.String).
		Field("meta", typ.NewMap(typ.String, typ.Any)).
		Build()
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typ.NewArray(typ.Any)),
			valueExpr: returnValueFromType(reg, entryType),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(typ.Any)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if want := returnValueFromType(reg, entryType); !product.Equal(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value, want) {
		t.Fatalf("dynamic value = %#v, want inserted record product", got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value)
	}
}

func TestSignatureOutcomeProviderSkipsAccumulatorObligationForExactInferredTable(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(91631)
	valueExpr := factflow.ExprRef(91632)
	listSymbol := symbol.ID(91633)
	accumulatorID := identity.ID{Kind: "test.table", Site: "accumulator", Index: 1}
	targetType := typ.NewArray(typ.LiteralString("system"))
	insertedType := typetable.NewRecord().
		Field("role", typ.LiteralString("cache_marker")).
		Field("marker_id", typ.String).
		Build()
	targetValue := product.Set(reg, returnValueFromType(reg, targetType), identity.Key, identity.Singleton(accumulatorID))
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  targetValue,
			valueExpr: returnValueFromType(reg, insertedType),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamObligations) != 0 {
		t.Fatalf("param obligations = %#v, want no inserted-value obligation for inferred exact accumulator", got.ParamObligations)
	}
	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one widening write for accumulator", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	if !ok || !strings.Contains(refinedType.String(), "system") || !strings.Contains(refinedType.String(), "cache_marker") {
		t.Fatalf("refined accumulator type = %v/%v, want widened element union containing system and cache_marker", refinedType, ok)
	}
}

func TestSignatureOutcomeProviderTableMutatorUsesSignatureArgumentTypeForInsertedValue(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(91634)
	valueExpr := factflow.ExprRef(91635)
	listSymbol := symbol.ID(91636)
	insertedType := typetable.NewRecord().
		Field("run", typ.Boolean).
		Build()
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr: returnValueFromType(reg, typetable.NewRecord().Build()),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
		ArgumentType: func(_ transfer.NodeContext, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (typ.Type, bool) {
			if source.Kind == factflow.ValueSourceExpression && source.HasExpr && source.ExprRef == valueExpr {
				return insertedType, true
			}
			return nil, false
		},
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(insertedType)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	dynamicValueType, ok := typevalue.TypeOf(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value)
	if !ok || !typ.TypeEquals(dynamicValueType, insertedType) {
		t.Fatalf("dynamic value type = %v/%v, want %v", dynamicValueType, ok, insertedType)
	}
}

func TestSignatureOutcomeProviderTableMutatorRefinesBuiltinTableTargetFromArgumentType(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(91644)
	valueExpr := factflow.ExprRef(91645)
	listSymbol := symbol.ID(91646)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  returnValueFromType(reg, typ.BuiltinTableTopMarker()),
			valueExpr: returnValueFromType(reg, typ.String),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
		ArgumentType: func(_ transfer.NodeContext, source factflow.ValueSource, _ state.State, _ func(cfg.Point) state.State) (typ.Type, bool) {
			switch source.ExprRef {
			case listExpr:
				return typetable.NewRecord().Build(), true
			case valueExpr:
				return typ.String, true
			default:
				return nil, false
			}
		},
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(typ.String)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
}

func TestTableMutatorWritePreservesInferredAccumulatorIdentity(t *testing.T) {
	reg := standard.Registry()
	accumulatorID := identity.ID{Kind: "test.table", Site: "write-accumulator", Index: 1}
	targetType := typ.NewArray(typ.LiteralString("system"))
	insertedType := typetable.NewRecord().
		Field("role", typ.LiteralString("cache_marker")).
		Field("marker_id", typ.String).
		Build()
	targetValue := product.Set(reg, returnValueFromType(reg, targetType), identity.Key, identity.Singleton(accumulatorID))

	write, ok := tableMutatorParamWrite(transfer.NodeContext{Registry: reg}, nil, tableMutatorEvidence{
		targetIndex: 0,
		valueIndex:  1,
		target:      targetValue,
		value:       returnValueFromType(reg, insertedType),
		targetType:  targetType,
		valueType:   insertedType,
	})
	if !ok {
		t.Fatalf("table mutator write missing for inferred accumulator")
	}
	if gotID, ok := identityvalue.ExactID(reg, write.Value); !ok || gotID != accumulatorID {
		t.Fatalf("write value identity = %v/%v, want preserved accumulator identity %v", gotID, ok, accumulatorID)
	}
	refinedType, ok := typevalue.TypeOf(reg, write.Value)
	if !ok {
		t.Fatalf("write value has no type witness")
	}
	obligation, ok := tableMutatorParamObligation(transfer.NodeContext{Registry: reg}, nil, tableMutatorEvidence{
		targetIndex: 0,
		valueIndex:  1,
		target:      write.Value,
		value:       returnValueFromType(reg, insertedType),
		targetType:  refinedType,
		valueType:   insertedType,
	})
	if ok {
		t.Fatalf("second inferred accumulator obligation = %#v, want none after identity-preserving write", obligation)
	}
}

func TestTableMutatorWritePreservesDeclaredAccumulatorClaim(t *testing.T) {
	reg := standard.Registry()
	accumulatorID := identity.ID{Kind: "test.table", Site: "write-declared-accumulator", Index: 1}
	targetType := typ.NewArray(typ.LiteralString("system"))
	insertedType := typetable.NewRecord().
		Field("role", typ.LiteralString("cache_marker")).
		Build()
	targetValue := product.Set(reg, returnValueFromType(reg, targetType), identity.Key, identity.Singleton(accumulatorID))
	targetValue = product.Set(reg, targetValue, assertion.Key, assertion.Type())

	write, ok := tableMutatorParamWrite(transfer.NodeContext{Registry: reg}, nil, tableMutatorEvidence{
		targetIndex: 0,
		valueIndex:  1,
		target:      targetValue,
		value:       returnValueFromType(reg, insertedType),
		targetType:  targetType,
		valueType:   insertedType,
	})
	if !ok {
		t.Fatalf("table mutator write missing for declared accumulator")
	}
	if gotID, ok := identityvalue.ExactID(reg, write.Value); !ok || gotID != accumulatorID {
		t.Fatalf("write value identity = %v/%v, want preserved declared accumulator identity %v", gotID, ok, accumulatorID)
	}
	if got := product.Get(reg, write.Value, assertion.Key); !got.Has(assertion.TypeClaim) {
		t.Fatalf("write value assertion = %s, want preserved declared type claim", got.String())
	}
}

func TestSignatureOutcomeProviderKeepsDeclaredExactAccumulatorObligation(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(91641)
	valueExpr := factflow.ExprRef(91642)
	listSymbol := symbol.ID(91643)
	accumulatorID := identity.ID{Kind: "test.table", Site: "declared-accumulator", Index: 1}
	targetType := typ.NewArray(typ.LiteralString("system"))
	targetValue := product.Set(reg, returnValueFromType(reg, targetType), identity.Key, identity.Singleton(accumulatorID))
	targetValue = product.Set(reg, targetValue, assertion.Key, assertion.Type())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr:  targetValue,
			valueExpr: returnValueFromType(reg, typetable.NewRecord().Field("role", typ.LiteralString("cache_marker")).Build()),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamObligations) != 1 || got.ParamObligations[0].ParamIndex != 1 {
		t.Fatalf("param obligations = %#v, want declared accumulator to enforce inserted value", got.ParamObligations)
	}
	obligationType, ok := typevalue.TypeOf(reg, got.ParamObligations[0].Value)
	if !ok || !typ.TypeEquals(obligationType, typ.String) {
		t.Fatalf("obligation type = %v/%v, want string family base for declared literal element", obligationType, ok)
	}
}

func TestSignatureOutcomeProviderKeepsAnyArrayMutatorWhenInsertedValueIsUnresolved(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(9164)
	valueExpr := factflow.ExprRef(9165)
	listSymbol := symbol.ID(9166)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr: returnValueFromType(reg, typ.NewArray(typ.Any)),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 1 ||
		!got.ParamPathWrites[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path writes = %#v, want one $0 write", got.ParamPathWrites)
	}
	refinedType, ok := typevalue.TypeOf(reg, got.ParamPathWrites[0].Value)
	want := typ.NewArray(typ.Any)
	if !ok || !typ.TypeEquals(refinedType, want) {
		t.Fatalf("refined list type = %v/%v, want %v", refinedType, ok, want)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one table-mutator element fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if want := returnValueFromType(reg, typ.Any); !product.Equal(reg, got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value, want) {
		t.Fatalf("dynamic value = %#v, want explicit any product", got.NormalReturnFacts.DynamicIndexFacts[0].Value.Value)
	}
}

func TestSignatureOutcomeProviderKeepsConcreteArrayMutatorProvenanceWhenInsertedValueIsUnresolved(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	listExpr := factflow.ExprRef(9170)
	valueExpr := factflow.ExprRef(9171)
	listSymbol := symbol.ID(9172)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: listExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: valueExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			listExpr: path.NewPath(listSymbol, "items"),
		},
	})
	sources := sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[factflow.ExprRef]product.Value{
			listExpr: returnValueFromType(reg, typ.NewArray(typ.String)),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(
					mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
				),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
		Sources: sources,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathWrites) != 0 {
		t.Fatalf("param path writes = %#v, want none without inserted value type proof", got.ParamPathWrites)
	}
	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic index facts = %#v, want one provenance-preserving table-mutator fact", got.NormalReturnFacts.DynamicIndexFacts)
	}
	dynamic := got.NormalReturnFacts.DynamicIndexFacts[0]
	if !dynamic.ValuePath.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("dynamic value path = %s, want $1", dynamic.ValuePath)
	}
	if keyType, ok := typevalue.TypeOf(reg, dynamic.Value.KeyValue); !ok || !typ.TypeEquals(keyType, typ.Integer) {
		t.Fatalf("dynamic key type = %v/%v, want integer", keyType, ok)
	}
	if !product.Domain(reg).Equal(dynamic.Value.Value, product.Bottom(reg)) {
		t.Fatalf("dynamic value = %#v, want bottom/no invented value type", dynamic.Value.Value)
	}
	if dynamic.Value.Admission != dynamicindex.AdmissionAdmitted {
		t.Fatalf("dynamic admission = %v, want admitted", dynamic.Value.Admission)
	}
}

func TestSignatureOutcomeProviderLowersMutateToParamPathInvalidationAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	argExpr := factflow.ExprRef(905)
	argSymbol := symbol.ID(906)
	argPath := path.NewPath(argSymbol, "items")
	containerKey := path.PathKey("sym906@1")
	childKey := path.PathKey("sym906@1.child")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: argExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			argExpr: argPath,
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.sort": {
				Effect: effect.Empty.With(mutation.Mutate{
					Target:    effect.ParamRef{Index: 0},
					Transform: mutation.Unchanged{},
				}),
			},
		},
		NameFor: staticName("table.sort"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 ||
		!got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path invalidations = %#v, want target $0", got.ParamPathInvalidations)
	}
	if !got.ParamPathInvalidations[0].PreserveStructuralWitness {
		t.Fatalf("table.sort invalidation should preserve the root structural witness: %#v", got.ParamPathInvalidations[0])
	}
	if len(got.NormalReturnFacts.PathInvalidations) != 1 ||
		!got.NormalReturnFacts.PathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("normal-return invalidations = %#v, want target $0", got.NormalReturnFacts.PathInvalidations)
	}
	if !got.NormalReturnFacts.PathInvalidations[0].PreserveStructuralWitness {
		t.Fatalf("table.sort normal-return invalidation should preserve the root structural witness: %#v", got.NormalReturnFacts.PathInvalidations[0])
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(call, argSymbol, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WritePathKey(reg, ks, containerKey, present).
			WritePathKey(reg, ks, childKey, present),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  resolver,
		}),
	})
	assertPathValue(t, reg, ks, flow[graph.Exit()], containerKey, present)
	assertPathValue(t, reg, ks, flow[graph.Exit()], childKey, product.Bottom(reg))
}

func TestSignatureParamPathInvalidationTreatsMutationPayloadsAsMetadata(t *testing.T) {
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(930), HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(931), HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(932), HasExpr: true},
		},
	})
	sig := signature.Function{
		Effect: effect.Empty.With(
			mutation.Mutate{
				Target: effect.ParamRef{Index: 0},
				Transform: mutation.ContainerElementUnion{
					Container: effect.ParamRef{Index: 1},
					Value:     effect.ParamRef{Index: 2},
				},
			},
			mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 99},
			mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: 2}},
		),
	}

	got := signatureParamPathInvalidationsForReader(sig, signatureArgumentsFromView(site.View()))

	if len(got) != 1 || !got[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path invalidations = %#v, want only target $0", got)
	}
}

func TestSignatureParamPathInvalidationProjectsEachDistinctMutatedArgument(t *testing.T) {
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(936), HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(937), HasExpr: true},
		},
	})
	sig := signature.Function{
		Effect: effect.Empty.With(
			mutation.TableMutator{Target: effect.ParamRef{Index: 0}, Value: effect.ParamRef{Index: -1}},
			mutation.TableMutator{Target: effect.ParamRef{Index: 1}, Value: effect.ParamRef{Index: -1}},
			mutation.Mutate{Target: effect.ParamRef{Index: 1}, Transform: mutation.Unchanged{}},
		),
	}

	got := signatureParamPathInvalidationsForReader(sig, signatureArgumentsFromView(site.View()))

	if len(got) != 2 {
		t.Fatalf("param path invalidations = %#v, want targets $0 and $1", got)
	}
	if !got[0].Path.Equal(path.NewPlaceholder(0)) || !got[1].Path.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("param path invalidations = %#v, want targets $0 and $1 in signature order", got)
	}
	if !got[1].PreserveStructuralWitness {
		t.Fatalf("target $1 has only structural-preserving table-mutator/unchanged evidence, so the merged invalidation must preserve structural witness: %#v", got[1])
	}
}

func TestSignatureParamLengthFloorsProjectOnlyPositiveLengthChange(t *testing.T) {
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(940), HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(941), HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(942), HasExpr: true},
		},
	})
	sig := signature.Function{
		Effect: effect.Empty.With(
			mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 2},
			mutation.LengthChange{Target: effect.ParamRef{Index: 1}, Delta: 0},
			mutation.LengthChange{Target: effect.ParamRef{Index: 2}, Delta: -1},
			mutation.Mutate{
				Target:      effect.ParamRef{Index: 1},
				Transform:   mutation.Unchanged{},
				LengthDelta: expr.C(99),
			},
		),
	}

	got := signatureParamLengthFloorsForReader(sig, signatureArgumentsFromView(site.View()))

	if len(got) != 1 {
		t.Fatalf("param length floors = %#v, want one positive LengthChange floor", got)
	}
	if !got[0].Path.Equal(path.NewPlaceholder(0)) || got[0].Floor != 2 {
		t.Fatalf("param length floor = %#v, want $0 >= 2", got[0])
	}
}

func TestSignatureOutcomeProviderLowersStoreIntoContainerArgument(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	containerExpr := factflow.ExprRef(911)
	insertedExpr := factflow.ExprRef(912)
	containerPath := path.NewPath(symbol.ID(913), "container")
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: containerExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: insertedExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			containerExpr: containerPath,
			insertedExpr:  path.NewPath(symbol.ID(914), "inserted"),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"store": {
				Effect: effect.Empty.With(ownership.Store{
					Param: effect.ParamRef{Index: -1},
					Into:  effect.ParamRef{Index: 0},
				}),
			},
		},
		NameFor: staticName("store"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 || !got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path invalidations = %#v, want container argument $0", got.ParamPathInvalidations)
	}
	if !got.ParamPathInvalidations[0].PreserveStructuralWitness {
		t.Fatalf("store invalidation should preserve the destination structural witness: %#v", got.ParamPathInvalidations[0])
	}
	if len(got.NormalReturnFacts.PathInvalidations) != 1 ||
		!got.NormalReturnFacts.PathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("normal-return invalidations = %#v, want container argument $0", got.NormalReturnFacts.PathInvalidations)
	}
	if !got.NormalReturnFacts.PathInvalidations[0].PreserveStructuralWitness {
		t.Fatalf("store normal-return invalidation should preserve the destination structural witness: %#v", got.NormalReturnFacts.PathInvalidations[0])
	}
	if len(got.NormalReturnFacts.StoreRelations) != 1 ||
		!got.NormalReturnFacts.StoreRelations[0].Source.Equal(path.NewPlaceholder(1)) ||
		!got.NormalReturnFacts.StoreRelations[0].Into.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("store relations = %#v, want source $1 into $0", got.NormalReturnFacts.StoreRelations)
	}
}

func TestSignatureOutcomeProviderSkipsExactStoreRelationWithoutKnownDestination(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	firstExpr := factflow.ExprRef(916)
	lastExpr := factflow.ExprRef(917)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: firstExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: lastExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			firstExpr: path.NewPath(symbol.ID(918), "first"),
			lastExpr:  path.NewPath(symbol.ID(919), "last"),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"store": {
				Effect: effect.Empty.With(ownership.Store{
					Param: effect.ParamRef{Index: 0},
					Into:  effect.ParamRef{Index: -1},
				}),
			},
		},
		NameFor: staticName("store"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathInvalidations) != 0 {
		t.Fatalf("param path invalidations = %#v, want none", got.ParamPathInvalidations)
	}
	if len(got.NormalReturnFacts.PathInvalidations) != 0 {
		t.Fatalf("normal-return path invalidations = %#v, want none", got.NormalReturnFacts.PathInvalidations)
	}
	if len(got.NormalReturnFacts.StoreRelations) != 0 {
		t.Fatalf("store relations = %#v, want none for unknown store destination", got.NormalReturnFacts.StoreRelations)
	}
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventStore, true)
}

func TestSignatureOutcomeProviderLowersOwnershipSendAndStoreEscapeEvents(t *testing.T) {
	point := cfg.Point(912)
	arg0 := factflow.ExprRef(912)
	arg1 := factflow.ExprRef(913)
	arg2 := factflow.ExprRef(914)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: arg0, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg1, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg2, HasExpr: true},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"send": {
				Effect: effect.Empty.
					With(ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 2}}).
					With(ownership.Send{FromParam: 1}),
			},
		},
		NameFor: staticName("send"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventStore, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(1), callboundary.EscapeEventSend, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(2), callboundary.EscapeEventSend, true)
}

func TestSignatureOutcomeProviderLowersOwnershipSendParamToSingleEscapeEvent(t *testing.T) {
	point := cfg.Point(915)
	arg0 := factflow.ExprRef(915)
	arg1 := factflow.ExprRef(916)
	arg2 := factflow.ExprRef(917)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: arg0, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg1, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg2, HasExpr: true},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"sendOne": {
				Effect: effect.Empty.With(ownership.SendParam{Param: effect.ParamRef{Index: 1}}),
			},
		},
		NameFor: staticName("sendOne"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	if len(got.NormalReturnFacts.EscapeEvents) != 1 {
		t.Fatalf("escape events = %#v, want exactly one send event", got.NormalReturnFacts.EscapeEvents)
	}
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(1), callboundary.EscapeEventSend, true)
}

func TestSignatureOutcomeProviderLowersExactOwnershipEscapeLabels(t *testing.T) {
	point := cfg.Point(918)
	arg0 := factflow.ExprRef(918)
	arg1 := factflow.ExprRef(919)
	arg2 := factflow.ExprRef(920)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: arg0, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg1, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg2, HasExpr: true},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"escapeKinds": {
				Effect: effect.Empty.
					With(ownership.Retain{Param: effect.ParamRef{Index: 0}}).
					With(ownership.Export{Param: effect.ParamRef{Index: 1}}).
					With(ownership.Opaque{Param: effect.ParamRef{Index: 2}}),
			},
		},
		NameFor: staticName("escapeKinds"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventRetain, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(1), callboundary.EscapeEventExport, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(2), callboundary.EscapeEventOpaque, true)
}

func TestSignatureOutcomeProviderLowersOperationalEffectsNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	operational := &signature.OperationalEffects{
		ReturnPresenceRelations: []signature.ReturnPresenceRelation{{
			TriggerIndex:    1,
			TriggerPresence: presence.Present(),
			TargetIndex:     0,
			TargetPresence:  presence.Absent(),
		}},
		NormalReturnPresenceRefinements: []signature.PathPresenceRefinement{{
			Path:     path.NewPlaceholder(0).Field("ready"),
			Presence: presence.Present(),
		}},
		NormalReturnTypeRefinements: []signature.PathTypeRefinement{{
			Path: path.NewPlaceholder(0),
			Type: typ.String,
		}},
		PathStaticMembers: []signature.PathStaticMemberFact{{
			Path: path.NewPlaceholder(1).Field("kind"),
			Type: typ.String,
		}},
		PathInvalidations: []signature.PathInvalidation{{
			Path: path.NewPlaceholder(1).Field("items"),
		}},
		BranchProofs: []signature.BranchProof{{
			Kind:  signature.BranchProofPathNotEqual,
			Path:  path.NewPlaceholder(0).Field("channel"),
			Other: path.NewPlaceholder(1),
		}},
		FrozenTables: []signature.FrozenTable{{
			Target: path.NewPlaceholder(0).Field("sealed"),
		}},
		EscapeEvents: []signature.EscapeEvent{{
			Target:    path.NewPlaceholder(0).Field("payload"),
			Kind:      signature.EscapeSend,
			Recursive: true,
		}},
		StoreRelations: []signature.StoreRelation{{
			Source: path.NewPlaceholder(0).Field("payload"),
			Into:   path.NewPlaceholder(1).Field("items"),
		}},
		ParamRelations: []signature.ParamRelation{
			{
				Param:                2,
				EscapeClass:          signature.EscapeNone,
				PlacementConsequence: signature.PlacementConsequenceKeep,
			},
			{
				Param:                3,
				EscapeClass:          signature.EscapeBorrow,
				PlacementConsequence: signature.PlacementConsequenceKeep,
			},
			{
				Param:                4,
				EscapeClass:          signature.EscapeStore,
				PlacementConsequence: signature.PlacementConsequenceOwnedHeap,
				StoredInto:           5,
				HasStoredInto:        true,
			},
		},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {OperationalEffects: operational},
		},
		NameFor: staticName("f"),
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want true for operational effects")
	}
	assertCallReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	if len(got.NormalReturnFacts.PathRefinements) != 2 ||
		!got.NormalReturnFacts.PathRefinements[0].Path.Equal(path.NewPlaceholder(0).Field("ready")) {
		t.Fatalf("path refinements = %#v", got.NormalReturnFacts.PathRefinements)
	}
	assertPresence(t, reg, got.NormalReturnFacts.PathRefinements[0].Value, presence.Present())
	if !got.NormalReturnFacts.PathRefinements[1].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("type refinement path = %s, want $0", got.NormalReturnFacts.PathRefinements[1].Path.String())
	}
	assertTypeWitness(t, reg, got.NormalReturnFacts.PathRefinements[1].Value, typ.String)
	if len(got.NormalReturnFacts.PathStaticMembers) != 1 ||
		!got.NormalReturnFacts.PathStaticMembers[0].Path.Equal(path.NewPlaceholder(1).Field("kind")) {
		t.Fatalf("path static members = %#v", got.NormalReturnFacts.PathStaticMembers)
	}
	assertTypeWitness(t, reg, got.NormalReturnFacts.PathStaticMembers[0].Value, typ.String)
	if len(got.NormalReturnFacts.PathInvalidations) != 1 ||
		!got.NormalReturnFacts.PathInvalidations[0].Path.Equal(path.NewPlaceholder(1).Field("items")) {
		t.Fatalf("path invalidations = %#v", got.NormalReturnFacts.PathInvalidations)
	}
	if len(got.NormalReturnFacts.BranchProofs) != 1 ||
		got.NormalReturnFacts.BranchProofs[0].Kind != pathevidence.BranchProofPathNotEqual ||
		!got.NormalReturnFacts.BranchProofs[0].Path.Equal(path.NewPlaceholder(0).Field("channel")) ||
		!got.NormalReturnFacts.BranchProofs[0].Other.Equal(path.NewPlaceholder(1)) {
		t.Fatalf("branch proofs = %#v", got.NormalReturnFacts.BranchProofs)
	}
	if len(got.NormalReturnFacts.FrozenTables) != 1 ||
		!got.NormalReturnFacts.FrozenTables[0].Target.Equal(path.NewPlaceholder(0).Field("sealed")) {
		t.Fatalf("frozen tables = %#v", got.NormalReturnFacts.FrozenTables)
	}
	if len(got.NormalReturnFacts.EscapeEvents) != 2 {
		t.Fatalf("escape events = %#v, want explicit escape plus relational store escape", got.NormalReturnFacts.EscapeEvents)
	}
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0).Field("payload"), callboundary.EscapeEventSend, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(4), callboundary.EscapeEventStore, true)
	if len(got.NormalReturnFacts.StoreRelations) != 2 ||
		!got.NormalReturnFacts.StoreRelations[0].Source.Equal(path.NewPlaceholder(0).Field("payload")) ||
		!got.NormalReturnFacts.StoreRelations[0].Into.Equal(path.NewPlaceholder(1).Field("items")) ||
		!got.NormalReturnFacts.StoreRelations[1].Source.Equal(path.NewPlaceholder(4)) ||
		!got.NormalReturnFacts.StoreRelations[1].Into.Equal(path.NewPlaceholder(5)) {
		t.Fatalf("store relations = %#v", got.NormalReturnFacts.StoreRelations)
	}
}

func TestSignatureOutcomeProviderLowersOperationalAllocationTemplates(t *testing.T) {
	reg := standard.Registry()
	rootType := typetable.NewRecord().Field("child", typetable.NewRecord().Build()).Build()
	childType := typetable.NewRecord().Field("name", typ.String).Build()
	entryType := typetable.NewRecord().Field("route", typ.String).Build()
	operational := &signature.OperationalEffects{
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        "builder.build:return:0:root",
			Objects: []signature.AllocationObjectTemplate{
				{
					ID:   "builder.build:return:0:root",
					Type: rootType,
					StaticMembers: []signature.AllocationStaticMemberTemplate{{
						Suffix: []segment.Segment{{Kind: segment.SegmentField, Name: "child"}},
						Value:  "builder.build:return:0:root.child",
					}},
					DynamicEntries: []signature.AllocationDynamicEntryTemplate{{
						KeyType: typ.String,
						Value:   "builder.build:return:0:root:dynamic:0:value",
					}},
				},
				{ID: "builder.build:return:0:root.child", Type: childType},
				{ID: "builder.build:return:0:root:dynamic:0:value", Type: entryType},
			},
		}},
	}
	ks := keyspace.New()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"builder.build": {
				Type:               typ.Func().Returns(rootType).Build(),
				OperationalEffects: operational,
			},
		},
		NameFor:  staticName("builder.build"),
		KeySpace: ks,
	})

	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one return", got.Results)
	}
	rootID := allocationTemplateIdentity("builder.build:return:0:root")
	childID := allocationTemplateIdentity("builder.build:return:0:root.child")
	entryID := allocationTemplateIdentity("builder.build:return:0:root:dynamic:0:value")
	if id, ok := product.Get(reg, got.Results[0].Value, identity.Key).ID(); !ok || id != rootID {
		t.Fatalf("return identity = %v/%v, want %v", id, ok, rootID)
	}
	if gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value); !ok || !typ.TypeEquals(gotType, rootType) {
		t.Fatalf("return type = %v/%v, want %v", gotType, ok, rootType)
	}
	if gotPlacement := got.Placements[rootID]; gotPlacement != placement.OwnedHeap {
		t.Fatalf("root placement = %s, want %s", gotPlacement, placement.OwnedHeap)
	}
	if gotPlacement := got.Placements[childID]; gotPlacement != placement.OwnedHeap {
		t.Fatalf("child placement = %s, want %s", gotPlacement, placement.OwnedHeap)
	}
	if gotPlacement := got.Placements[entryID]; gotPlacement != placement.OwnedHeap {
		t.Fatalf("dynamic value placement = %s, want %s", gotPlacement, placement.OwnedHeap)
	}
	rootObject, ok := got.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("heap objects missing root %v: %#v", rootID, got.HeapTableObjects)
	}
	childKey, ok := heapidentity.StaticMemberSuffixKey(ks, []segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("child suffix key failed")
	}
	childValue, ok := rootObject.StaticMember(childKey)
	if !ok {
		t.Fatalf("root static members = %#v", rootObject.StaticMembers())
	}
	if id, ok := product.Get(reg, childValue, identity.Key).ID(); !ok || id != childID {
		t.Fatalf("child identity = %v/%v, want %v", id, ok, childID)
	}
	if gotType, ok := typevalue.TypeOf(reg, childValue); !ok || !typ.TypeEquals(gotType, childType) {
		t.Fatalf("child type = %v/%v, want %v", gotType, ok, childType)
	}
	dynamic := rootObject.DynamicIndexFacts()
	if len(dynamic) != 1 {
		t.Fatalf("dynamic entries = %#v, want one", dynamic)
	}
	for _, fact := range dynamic {
		if keyType, ok := typevalue.TypeOf(reg, fact.KeyValue); !ok || !typ.TypeEquals(keyType, typ.String) {
			t.Fatalf("dynamic key type = %v/%v, want string", keyType, ok)
		}
		if id, ok := product.Get(reg, fact.Value, identity.Key).ID(); !ok || id != entryID {
			t.Fatalf("dynamic value identity = %v/%v, want %v", id, ok, entryID)
		}
		if gotType, ok := typevalue.TypeOf(reg, fact.Value); !ok || !typ.TypeEquals(gotType, entryType) {
			t.Fatalf("dynamic value type = %v/%v, want %v", gotType, ok, entryType)
		}
	}
}

func TestSignatureOutcomeProviderSpecializesAllocationTemplateIdentityByCallPoint(t *testing.T) {
	reg := standard.Registry()
	rootType := typetable.NewRecord().Build()
	rootTemplate := signature.AllocationTemplateID("stdlib.table.create:return:0")
	operational := &signature.OperationalEffects{
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        rootTemplate,
			Objects: []signature.AllocationObjectTemplate{{
				ID:   rootTemplate,
				Type: rootType,
			}},
		}},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.create": {
				Type:               typ.Func().Param("narray", typ.Integer).OptParam("nhash", typ.Integer).Returns(rootType).Build(),
				OperationalEffects: operational,
			},
		},
		NameFor:  staticName("table.create"),
		KeySpace: keyspace.New(),
	})

	left := provider(transfer.NodeContext{Registry: reg, Point: cfg.Point(4101)}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	right := provider(transfer.NodeContext{Registry: reg, Point: cfg.Point(4102)}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	leftID := callResultIdentityAt(reg, left.Results, 0)
	rightID := callResultIdentityAt(reg, right.Results, 0)
	if leftID == (identity.ID{}) || rightID == (identity.ID{}) {
		t.Fatalf("return identities = %v/%v, want concrete identities", leftID, rightID)
	}
	if leftID == rightID {
		t.Fatalf("allocation identities collapsed across call sites: %v", leftID)
	}
	if leftID != allocationTemplateIdentityAt(cfg.Point(4101), rootTemplate) {
		t.Fatalf("left identity = %v, want call-site-specialized template identity", leftID)
	}
	if rightID != allocationTemplateIdentityAt(cfg.Point(4102), rootTemplate) {
		t.Fatalf("right identity = %v, want call-site-specialized template identity", rightID)
	}
}

func TestSignatureOutcomeProviderClosesUninferredGenericDeclaredReturn(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(28)
	boxParam := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typetable.NewRecord().Field("value", boxParam).Build())
	fnParam := typ.NewTypeParam("T", nil)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"make": {
				Type: typ.Func().
					TypeParamRef(fnParam).
					Returns(typ.Instantiate(box, fnParam)).
					Build(),
			},
		},
		NameFor: staticName("make"),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	want := typ.Instantiate(box, typ.Unknown)
	if gotType, ok := typevalue.TypeOf(reg, got[0].Value); !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("result type = %v/%v, want %v", gotType, ok, want)
	}
}

func TestSignatureOutcomeProviderClosesUninferredGenericAllocationTemplateType(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(29)
	boxParam := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typetable.NewRecord().Field("value", boxParam).Build())
	fnParam := typ.NewTypeParam("T", nil)
	openReturn := typ.Instantiate(box, fnParam)
	closedReturn := typ.Instantiate(box, typ.Unknown)
	rootTemplate := signature.AllocationTemplateID("box.make:return:0:root")
	operational := &signature.OperationalEffects{
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        rootTemplate,
			Objects: []signature.AllocationObjectTemplate{{
				ID:   rootTemplate,
				Type: openReturn,
			}},
		}},
	}
	ks := keyspace.New()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"box.make": {
				Type: typ.Func().
					TypeParamRef(fnParam).
					Returns(openReturn).
					Build(),
				OperationalEffects: operational,
			},
		},
		NameFor:  staticName("box.make"),
		KeySpace: ks,
	})

	outcome := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if len(outcome.Results) != 1 {
		t.Fatalf("results = %#v, want one return", outcome.Results)
	}
	rootID := allocationTemplateIdentityAt(point, rootTemplate)
	if id, ok := product.Get(reg, outcome.Results[0].Value, identity.Key).ID(); !ok || id != rootID {
		t.Fatalf("return identity = %v/%v, want %v", id, ok, rootID)
	}
	if gotType, ok := typevalue.TypeOf(reg, outcome.Results[0].Value); !ok || !typ.TypeEquals(gotType, closedReturn) {
		t.Fatalf("return type = %v/%v, want %v", gotType, ok, closedReturn)
	}
	object, ok := outcome.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("heap objects missing %v: %#v", rootID, outcome.HeapTableObjects)
	}
	if gotType, ok := typevalue.TypeOf(reg, object.Root()); !ok || !typ.TypeEquals(gotType, closedReturn) {
		t.Fatalf("heap root type = %v/%v, want %v", gotType, ok, closedReturn)
	}
}

func TestSignatureOutcomeProviderAllocationTemplateRootTypeRefinesClosedGenericReturn(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(30)
	resultParam := typ.NewTypeParam("T", nil)
	errType := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", resultParam).
			Build(),
		errType,
	))
	fnParam := typ.NewTypeParam("T", nil)
	openReturn := typ.Instantiate(result, fnParam)
	rootTemplate := signature.AllocationTemplateID("result.err:return:0:root")
	operational := &signature.OperationalEffects{
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        rootTemplate,
			Objects: []signature.AllocationObjectTemplate{{
				ID:   rootTemplate,
				Type: errType,
			}},
		}},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"result.err": {
				Type: typ.Func().
					TypeParamRef(fnParam).
					Param("error", typ.String).
					Returns(openReturn).
					Build(),
				OperationalEffects: operational,
			},
		},
		NameFor:  staticName("result.err"),
		KeySpace: keyspace.New(),
	})

	outcome := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if len(outcome.Results) != 1 {
		t.Fatalf("results = %#v, want one return", outcome.Results)
	}
	if gotType, ok := typevalue.TypeOf(reg, outcome.Results[0].Value); !ok || !typ.TypeEquals(gotType, errType) {
		t.Fatalf("return type = %v/%v, want exact error variant %v", gotType, ok, errType)
	}
	rootID := allocationTemplateIdentityAt(point, rootTemplate)
	if id, ok := product.Get(reg, outcome.Results[0].Value, identity.Key).ID(); !ok || id != rootID {
		t.Fatalf("return identity = %v/%v, want %v", id, ok, rootID)
	}
}

func TestSignatureOutcomeProviderAllocationTemplatePreservesPreciseSignatureReturn(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(30)
	returnType := typetable.NewRecord().
		Field("data_func", typeexpr.Optional(typ.String)).
		Build()
	rootTemplate := signature.AllocationTemplateID("page:return:0:root")
	operational := &signature.OperationalEffects{
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        rootTemplate,
			Objects: []signature.AllocationObjectTemplate{{
				ID:   rootTemplate,
				Type: typetable.NewRecord().Build(),
			}},
		}},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"page.build": {
				Type: typ.Func().
					Returns(returnType).
					Build(),
				OperationalEffects: operational,
			},
		},
		NameFor:  staticName("page.build"),
		KeySpace: keyspace.New(),
	})

	outcome := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	if len(outcome.Results) != 1 {
		t.Fatalf("results = %#v, want one return", outcome.Results)
	}
	if gotType, ok := typevalue.TypeOf(reg, outcome.Results[0].Value); !ok || !typ.TypeEquals(gotType, returnType) {
		t.Fatalf("return type = %v/%v, want precise signature return %v", gotType, ok, returnType)
	}
	rootID := allocationTemplateIdentityAt(point, rootTemplate)
	if id, ok := product.Get(reg, outcome.Results[0].Value, identity.Key).ID(); !ok || id != rootID {
		t.Fatalf("return identity = %v/%v, want %v", id, ok, rootID)
	}
}

func TestSignatureOutcomeProviderSubstitutesGenericOperationalAllocationTemplate(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(31)
	resultParam := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", resultParam).
		Build())
	fnParam := typ.NewTypeParam("T", nil)
	userType := typetable.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Build()
	openReturn := typ.Instantiate(result, fnParam)
	wantReturn := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", userType).
		Build()
	rootTemplate := signature.AllocationTemplateID("result.ok:return:0:root")
	operational := &signature.OperationalEffects{
		ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0,
			Root:        rootTemplate,
			Objects: []signature.AllocationObjectTemplate{{
				ID: rootTemplate,
				Type: typetable.NewRecord().
					Field("ok", typ.LiteralBool(true)).
					Field("value", fnParam).
					Build(),
			}},
		}},
	}
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"result.ok": {
				Type: typ.Func().
					TypeParamRef(fnParam).
					Param("value", fnParam).
					Returns(openReturn).
					Build(),
				OperationalEffects: operational,
			},
		},
		NameFor:       staticName("result.ok"),
		ReturnTypeOps: testReturnTypeOps(),
		ArgumentType: func(transfer.NodeContext, factflow.ValueSource, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
			return userType, true
		},
		KeySpace: keyspace.New(),
	})

	outcome := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(31), HasExpr: true}},
	}).View(), state.State{}, nil)

	if len(outcome.Results) != 1 {
		t.Fatalf("results = %#v, want one return", outcome.Results)
	}
	if gotType, ok := typevalue.TypeOf(reg, outcome.Results[0].Value); !ok || !typ.TypeEquals(gotType, wantReturn) {
		t.Fatalf("return type = %v/%v, want %v", gotType, ok, wantReturn)
	}
	rootID := allocationTemplateIdentityAt(point, rootTemplate)
	object, ok := outcome.HeapTableObjects[rootID]
	if !ok {
		t.Fatalf("heap objects missing %v: %#v", rootID, outcome.HeapTableObjects)
	}
	if gotType, ok := typevalue.TypeOf(reg, object.Root()); !ok || !typ.TypeEquals(gotType, wantReturn) {
		t.Fatalf("heap root type = %v/%v, want %v", gotType, ok, wantReturn)
	}
}

func TestCallableValueOutcomeProviderPreservesFreeOuterTypeParamReturn(t *testing.T) {
	reg := standard.Registry()
	resultParam := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", resultParam).
		Build())
	outerParam := typ.NewTypeParam("U", nil)
	returnType := typ.Instantiate(result, outerParam)
	wantReturn := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", outerParam).
		Build()
	calleeType := typ.Func().
		Param("value", typ.String).
		Returns(returnType).
		Build()
	calleeValue := typevalue.WithWitness(reg, typevalue.FromType(reg, calleeType), calleeType)
	provider := CallableValueOutcomeProvider(CallableValueOutcomeProviderConfig{
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			return calleeValue, true
		},
		Callable: typecall.Callable,
	})

	outcome := provider(transfer.NodeContext{Registry: reg}, factflow.CallSiteView{}, state.State{}, nil)
	if len(outcome.Results) != 1 {
		t.Fatalf("results = %#v, want free-outer-param return preserved", outcome.Results)
	}
	if gotType, ok := typevalue.TypeOf(reg, outcome.Results[0].Value); !ok || !typ.TypeEquals(gotType, wantReturn) {
		t.Fatalf("return type = %v/%v, want %v", gotType, ok, wantReturn)
	}
}

func TestCallableValueOutcomeProviderDoesNotAllocateWeakReturnPayload(t *testing.T) {
	reg := standard.Registry()
	calleeType := typ.Func().
		Returns(typ.Any, typ.Unknown, typ.Never).
		Build()
	calleeValue := typevalue.WithWitness(reg, typevalue.FromType(reg, calleeType), calleeType)
	provider := CallableValueOutcomeProvider(CallableValueOutcomeProviderConfig{
		CalleeValue: func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) (product.Value, bool) {
			return calleeValue, true
		},
		Callable: typecall.Callable,
	})

	outcome := provider(transfer.NodeContext{Registry: reg}, factflow.CallSiteView{}, state.State{}, nil)
	if outcome.Results != nil {
		t.Fatalf("results = %#v, want nil result payload for weak returns", outcome.Results)
	}
	if outcome.PostReturnAuthority {
		t.Fatal("PostReturnAuthority = true, want false without concrete return payload")
	}
}

func TestSignatureOutcomeProviderOperationalEffectsSuppressRowOperationalFallback(t *testing.T) {
	point := cfg.Point(9018)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(9019), HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(9020), HasExpr: true},
				},
			}),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Effect: effect.Empty.
					With(ownership.Freeze{Param: effect.ParamRef{Index: 0}}).
					With(ownership.SendParam{Param: effect.ParamRef{Index: 0}}).
					With(ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}}).
					With(mutation.TableMutator{Target: effect.ParamRef{Index: 1}, Value: effect.ParamRef{Index: -1}}).
					With(lifecycle.Transition{
						Target:   effect.ParamRef{Index: 0},
						Protocol: typestate.Protocol("transaction"),
						From:     typestate.State("open"),
						To:       typestate.State("closed"),
					}),
				OperationalEffects: &signature.OperationalEffects{
					FrozenTables: []signature.FrozenTable{{
						Target: path.NewPlaceholder(0).Field("child"),
					}},
					EscapeEvents: []signature.EscapeEvent{{
						Target:    path.NewPlaceholder(0).Field("child"),
						Kind:      signature.EscapeSend,
						Recursive: true,
					}},
					PathInvalidations: []signature.PathInvalidation{{
						Path: path.NewPlaceholder(1).Field("items"),
					}},
					StoreRelations: []signature.StoreRelation{{
						Source: path.NewPlaceholder(0).Field("child"),
						Into:   path.NewPlaceholder(1).Field("items"),
					}},
					LifecycleEffects: []signature.LifecycleEffect{{
						Target:   path.NewPlaceholder(0).Field("child"),
						Kind:     signature.LifecycleTransition,
						Protocol: typestate.Protocol("transaction"),
						From:     typestate.State("open"),
						To:       typestate.State("closed"),
					}},
				},
			},
		},
		NameFor: staticName("f"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}

	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	if len(got.ParamPathInvalidations) != 0 || len(got.ParamPathRefinements) != 0 || len(got.ParamPathWrites) != 0 {
		t.Fatalf("row-derived param facts leaked: refinements=%#v writes=%#v invalidations=%#v", got.ParamPathRefinements, got.ParamPathWrites, got.ParamPathInvalidations)
	}
	if len(got.NormalReturnFacts.FrozenTables) != 1 ||
		!got.NormalReturnFacts.FrozenTables[0].Target.Equal(path.NewPlaceholder(0).Field("child")) {
		t.Fatalf("frozen tables = %#v, want only descendant DTO fact", got.NormalReturnFacts.FrozenTables)
	}
	if len(got.NormalReturnFacts.EscapeEvents) != 1 ||
		!got.NormalReturnFacts.EscapeEvents[0].Target.Equal(path.NewPlaceholder(0).Field("child")) {
		t.Fatalf("escape events = %#v, want only descendant DTO fact", got.NormalReturnFacts.EscapeEvents)
	}
	if len(got.NormalReturnFacts.PathInvalidations) != 1 ||
		!got.NormalReturnFacts.PathInvalidations[0].Path.Equal(path.NewPlaceholder(1).Field("items")) {
		t.Fatalf("path invalidations = %#v, want only descendant DTO fact", got.NormalReturnFacts.PathInvalidations)
	}
	if len(got.NormalReturnFacts.StoreRelations) != 1 ||
		!got.NormalReturnFacts.StoreRelations[0].Source.Equal(path.NewPlaceholder(0).Field("child")) ||
		!got.NormalReturnFacts.StoreRelations[0].Into.Equal(path.NewPlaceholder(1).Field("items")) {
		t.Fatalf("store relations = %#v, want only descendant DTO fact", got.NormalReturnFacts.StoreRelations)
	}
	if len(got.NormalReturnFacts.LifecycleFacts) != 1 ||
		!got.NormalReturnFacts.LifecycleFacts[0].Target.Equal(path.NewPlaceholder(0).Field("child")) ||
		got.NormalReturnFacts.LifecycleFacts[0].Kind != callboundary.LifecycleTransition ||
		got.NormalReturnFacts.LifecycleFacts[0].Protocol != typestate.Protocol("transaction") {
		t.Fatalf("lifecycle facts = %#v, want only descendant DTO fact", got.NormalReturnFacts.LifecycleFacts)
	}
}

func TestSignatureOutcomeProviderOperationalDynamicIndexFactUsesPlaceholderOperands(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9024)
	providerSym := symbol.ID(9025)
	providerExpr := factflow.ExprRef(9026)
	keyExpr := factflow.ExprRef(9027)
	keyType := typ.LiteralString("send")
	valueType := typ.Func().Param("v", typ.String).Build()
	keyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, keyType), keyType)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: providerExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: keyExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			providerExpr: path.NewPath(providerSym, "p"),
		},
		ExpressionValues: map[factflow.ExprRef]product.Value{
			keyExpr: keyValue,
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"install": {
				OperationalEffects: &signature.OperationalEffects{
					DynamicIndexFacts: []signature.DynamicIndexFact{{
						Table:       path.NewPlaceholder(0),
						Site:        "ops.install.dynamic",
						KeyPresence: presence.Maybe(),
						Key: signature.DynamicIndexOperand{
							Path: path.NewPlaceholder(1),
						},
						Value: signature.DynamicIndexOperand{
							Type: valueType,
						},
						Admission: signature.DynamicIndexAdmissionUnknown,
					}},
				},
			},
		},
		NameFor: staticName("install"),
		Facts:   facts,
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				keyExpr: keyValue,
			},
		}),
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, site, state.State{}, nil)

	if len(got.NormalReturnFacts.DynamicIndexFacts) != 1 {
		t.Fatalf("dynamic-index facts = %#v, want one", got.NormalReturnFacts.DynamicIndexFacts)
	}
	fact := got.NormalReturnFacts.DynamicIndexFacts[0]
	if !fact.Table.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("dynamic table = %s, want $0", fact.Table)
	}
	if keyGot, ok := typevalue.TypeOf(reg, fact.Value.KeyValue); !ok || !typ.TypeEquals(keyGot, keyType) {
		t.Fatalf("dynamic key type = %v/%v, want %v", keyGot, ok, keyType)
	}
	if valueGot, ok := typevalue.TypeOf(reg, fact.Value.Value); !ok || !typ.TypeEquals(valueGot, valueType) {
		t.Fatalf("dynamic value type = %v/%v, want %v", valueGot, ok, valueType)
	}
}

func TestSignatureOutcomeProviderEmptyOperationalEffectsUsesRowFallback(t *testing.T) {
	point := cfg.Point(9021)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(9022), HasExpr: true},
				},
			}),
		},
	})
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Effect: effect.Empty.With(ownership.SendParam{
					Param: effect.ParamRef{Index: 0},
				}),
				OperationalEffects: &signature.OperationalEffects{},
			},
		},
		NameFor: staticName("f"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}

	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventSend, true)
}

func TestSignatureOutcomeProviderOwnershipBorrowEffectsRecordBorrowWithoutPlacementOrFreeze(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(925)
	target := symbol.ID(926)
	targetExpr := factflow.ExprRef(926)
	targetPath := path.NewPath(target, "obj")
	tableID := identity.ID{Kind: "lua.table", Site: "ownership-borrow-noop", Index: 1}
	tableValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(tableID))
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: targetExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			targetExpr: targetPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(call, target, "obj")
	resolver := visibility.NewResolver(builder.Build())

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"borrow": {
				Effect: effect.Empty.
					With(ownership.Borrow{Param: effect.ParamRef{Index: 0}}).
					With(ownership.BorrowAll{}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "borrow", true
		},
		Facts: facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	outcome := provider(transfer.NodeContext{Graph: graph, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)
	assertEscapeEvent(t, outcome.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventBorrow, true)
	if len(outcome.NormalReturnFacts.FrozenTables) != 0 {
		t.Fatalf("FrozenTables = %#v, want none", outcome.NormalReturnFacts.FrozenTables)
	}
	if len(outcome.ParamPathInvalidations) != 0 {
		t.Fatalf("ParamPathInvalidations = %#v, want none", outcome.ParamPathInvalidations)
	}
	if len(outcome.NormalReturnFacts.EffectDeltas) != 0 {
		t.Fatalf("EffectDeltas = %#v, want none", outcome.NormalReturnFacts.EffectDeltas)
	}

	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(target), tableValue),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  resolver,
		}),
	})
	got := flow[graph.Exit()]
	if gotPlacement := got.ReadPlacement(tableID); gotPlacement != placement.Bottom {
		t.Fatalf("placement[%v] = %s, want %s", tableID, gotPlacement, placement.Bottom)
	}
	if got.IsTableFrozen(tableID) {
		t.Fatalf("table %v was frozen by borrow-only signature", tableID)
	}
}

func TestSignatureOutcomeProviderBorrowAllBorrowsEveryBindableArgument(t *testing.T) {
	point := cfg.Point(927)
	arg0 := factflow.ExprRef(927)
	arg1 := factflow.ExprRef(928)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: arg0, HasExpr: true},
					{Kind: factflow.ValueSourceNil},
					{Kind: factflow.ValueSourceExpression, ExprRef: arg1, HasExpr: true},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"borrowAll": {
				Effect: effect.Empty.With(ownership.BorrowAll{}),
			},
		},
		NameFor: staticName("borrowAll"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	if len(got.NormalReturnFacts.EscapeEvents) != 2 {
		t.Fatalf("escape events = %#v, want two bindable-argument borrows", got.NormalReturnFacts.EscapeEvents)
	}
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(0), callboundary.EscapeEventBorrow, true)
	assertEscapeEvent(t, got.NormalReturnFacts.EscapeEvents, path.NewPlaceholder(2), callboundary.EscapeEventBorrow, true)
}

func TestSignatureOutcomeProviderOwnershipEffectsApplyPlacementAndFreeze(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	callee := symbol.ID(930)
	storeSym := symbol.ID(931)
	freezeSym := symbol.ID(932)
	sendSym := symbol.ID(933)
	containerSym := symbol.ID(934)
	storeExpr := factflow.ExprRef(931)
	freezeExpr := factflow.ExprRef(932)
	sendExpr := factflow.ExprRef(933)
	containerExpr := factflow.ExprRef(934)
	storePath := path.NewPath(storeSym, "storeObj")
	freezePath := path.NewPath(freezeSym, "freezeObj")
	sendPath := path.NewPath(sendSym, "sendObj")
	containerPath := path.NewPath(containerSym, "container")
	storeID := identity.ID{Kind: "lua.table", Site: "ownership-apply", Index: 1}
	freezeID := identity.ID{Kind: "lua.table", Site: "ownership-apply", Index: 2}
	sendID := identity.ID{Kind: "lua.table", Site: "ownership-apply", Index: 3}
	containerID := identity.ID{Kind: "lua.table", Site: "ownership-apply", Index: 4}
	storeValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(storeID))
	freezeValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(freezeID))
	sendValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(sendID))
	containerValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(containerID))
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextStatement,
				CalleeSymbol: callee,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: storeExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: freezeExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: sendExpr, HasExpr: true},
					{Kind: factflow.ValueSourceExpression, ExprRef: containerExpr, HasExpr: true},
				},
			}),
		},
		ExpressionPaths: map[factflow.ExprRef]path.Path{
			storeExpr:     storePath,
			freezeExpr:    freezePath,
			sendExpr:      sendPath,
			containerExpr: containerPath,
		},
	})
	builder := visibility.NewBuilder()
	builder.Define(call, storeSym, "storeObj")
	builder.Define(call, freezeSym, "freezeObj")
	builder.Define(call, sendSym, "sendObj")
	builder.Define(call, containerSym, "container")
	resolver := visibility.NewResolver(builder.Build())

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"ownershipEffects": {
				Effect: effect.Empty.
					With(ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 3}}).
					With(ownership.Send{FromParam: 2}).
					With(ownership.Freeze{Param: effect.ParamRef{Index: 1}}),
			},
		},
		NameFor: func(_ transfer.NodeContext, call factflow.CallProducer) (string, bool) {
			if call.CalleeSymbol() != callee {
				return "", false
			}
			return "ownershipEffects", true
		},
		Facts: facts,
	})

	flow := transfer.Run(transfer.Config{
		Graph:    graph,
		Registry: reg,
		EntryState: state.State{}.
			WriteValue(reg, key.SymbolValue(storeSym), storeValue).
			WriteValue(reg, key.SymbolValue(freezeSym), freezeValue).
			WriteValue(reg, key.SymbolValue(sendSym), sendValue).
			WriteValue(reg, key.SymbolValue(containerSym), containerValue),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  resolver,
		}),
	})
	got := flow[graph.Exit()]

	if gotPlacement := got.ReadPlacement(storeID); gotPlacement != placement.OwnedHeap {
		t.Fatalf("placement[%v] = %s, want %s", storeID, gotPlacement, placement.OwnedHeap)
	}
	if gotPlacement := got.ReadPlacement(sendID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", sendID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(containerID); gotPlacement != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want %s", containerID, gotPlacement, placement.SharedHeap)
	}
	if gotPlacement := got.ReadPlacement(freezeID); gotPlacement != placement.Bottom {
		t.Fatalf("placement[%v] = %s, want freeze without escape placement", freezeID, gotPlacement)
	}
	if !got.IsTableFrozen(freezeID) {
		t.Fatalf("freeze target %v was not frozen", freezeID)
	}
}

func TestSignatureOutcomeProviderLowersOwnershipFreezeFrozenTableFact(t *testing.T) {
	point := cfg.Point(920)
	arg0 := factflow.ExprRef(920)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: arg0, HasExpr: true},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"freeze": {
				Effect: effect.Empty.With(ownership.Freeze{Param: effect.ParamRef{Index: 0}}),
			},
		},
		NameFor: staticName("freeze"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(point)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Point: point}, site, state.State{}, nil)

	if len(got.NormalReturnFacts.FrozenTables) != 1 ||
		!got.NormalReturnFacts.FrozenTables[0].Target.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("FrozenTables = %#v, want freeze on $0", got.NormalReturnFacts.FrozenTables)
	}
	if len(got.NormalReturnFacts.EscapeEvents) != 0 {
		t.Fatalf("EscapeEvents = %#v, want freeze not encoded as escape", got.NormalReturnFacts.EscapeEvents)
	}
}

func TestSignatureOutcomeProviderParamPathInvalidationDoesNotApplyWithoutExpressionPath(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	argSymbol := symbol.ID(921)
	childKey := path.PathKey("sym921@1.child")
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{
				Context: factflow.CallSiteContextStatement,
				ArgumentSources: []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(921), HasExpr: true},
					{Kind: factflow.ValueSourceNil},
				},
			}),
		},
	})

	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"table.insert": {
				Effect: effect.Empty.With(mutation.TableMutator{
					Target: effect.ParamRef{Index: 0},
					Value:  effect.ParamRef{Index: -1},
				}),
			},
		},
		NameFor: staticName("table.insert"),
		Facts:   facts,
	})
	site, ok := facts.CallSiteView(call)
	if !ok {
		t.Fatalf("missing call site")
	}
	got := provider(transfer.NodeContext{Graph: graph, Registry: reg, Point: call, Node: graph.Node(call)}, site, state.State{}, nil)

	if len(got.ParamPathInvalidations) != 1 || !got.ParamPathInvalidations[0].Path.Equal(path.NewPlaceholder(0)) {
		t.Fatalf("param path invalidations = %#v, want unresolved $0", got.ParamPathInvalidations)
	}
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(call, argSymbol, "items")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	ks := resolver.KeySpace()
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WritePathKey(reg, ks, childKey, present),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:       facts,
			CallOutcome: provider,
			Visibility:  resolver,
		}),
	})
	assertPathValue(t, reg, ks, flow[graph.Exit()], childKey, present)
}

func TestSignatureNoNormalReturnPredicateMarksNeverReturnCallAndApplies(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	target := symbol.ID(820)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextStatement,
	})
	predicate := SignatureNoNormalReturnPredicate(SignatureNoNormalReturnConfig{
		Graph:    graph,
		Registry: reg,
		Signatures: signatureMap{
			"error": {Type: typ.Func().Param("message", typ.Any).Returns(typ.Never).Build()},
		},
		NameFor: staticName("error"),
	})
	if predicate == nil || !predicate(call, site.View()) {
		t.Fatalf("signature no-normal-return predicate did not mark call")
	}
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			call: site,
		},
		NoNormalReturns: map[cfg.Point]struct{}{
			call: {},
		},
	})
	if !facts.NoNormalReturn(call) {
		t.Fatalf("NoNormalReturn(%d) = false, want true", call)
	}
	flow := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: state.State{}.WriteValue(reg, key.SymbolValue(target), product.Top()),
		NodeTransfer: factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
			Facts:   facts,
			Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
		}),
	})
	assertValue(t, reg, flow[graph.Exit()], key.SymbolValue(target), product.Bottom(reg))
}

func TestSignatureOutcomeProviderSameAsReturnsArgumentValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(4)
	argRef := factflow.ExprRef(7)
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("value", typ.Any).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	assertCallOutcomeResults(t, reg, got, []product.Value{argValue})
}

func TestSignatureOutcomeProviderSameAsResolvesNegativeParamRef(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(5)
	firstRef := factflow.ExprRef(8)
	lastRef := factflow.ExprRef(9)
	firstValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	lastValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("first", typ.Any).Param("last", typ.Any).Returns(typ.String).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: firstRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: lastRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				firstRef: firstValue,
				lastRef:  lastValue,
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	assertCallOutcomeResults(t, reg, got, []product.Value{lastValue})
}

func TestSignatureOutcomeProviderSameAsUsesDeclaredReturnTypeWhenArgumentProjectionFails(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(10), HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{Registry: reg}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderElementOfArrayReturnsElementRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	assertTypeWitness(t, reg, got[0].Value, typ.String)
}

func TestSignatureOutcomeProviderElementOfMapReturnsValueRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(9)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewMap(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderElementOfTupleReturnsElementUnionRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewTuple(typ.String, typ.Number)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Join(
		runtimekind.Singleton(runtimekind.String),
		runtimekind.Singleton(runtimekind.Number),
	))
}

func TestSignatureOutcomeProviderOptionalElementOfArrayKeepsMaybePresence(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(11)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Any).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	if gotPresence := product.PresenceOf(got[0].Value); !presence.Equal(gotPresence, presence.Top()) {
		t.Fatalf("presence = %s, want maybe/top", gotPresence)
	}
}

func TestSignatureOutcomeProviderInstantiatesGenericOptionalElementReturn(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(111)
	itemParam := typ.NewTypeParam("T", nil)
	predicate := typ.NewGeneric("Predicate", []*typ.TypeParam{itemParam},
		typ.Func().Param("item", itemParam).Returns(typ.Boolean).Build())
	user := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build()
	usersRef := factflow.ExprRef(1111)
	predRef := factflow.ExprRef(1112)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"find": {
				Type: typ.Func().
					TypeParamRef(itemParam).
					Param("arr", typ.NewArray(itemParam)).
					Param("pred", typ.Instantiate(predicate, itemParam)).
					Returns(typeexpr.Optional(itemParam)).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("find"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: usersRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: predRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				usersRef: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.NewArray(user)), typ.NewArray(user)),
				predRef: typevalue.WithWitness(reg, typevalue.FromType(reg,
					typ.Func().Param("item", user).Returns(typ.Boolean).Build()),
					typ.Func().Param("item", user).Returns(typ.Boolean).Build()),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertTypeWitness(t, reg, got[0].Value, user)
	if gotPresence := product.PresenceOf(got[0].Value); !presence.Equal(gotPresence, presence.Top()) {
		t.Fatalf("presence = %s, want maybe/top", gotPresence)
	}
}

func TestSignatureOutcomeProviderInstantiatesGenericSameAsAccumulatorReturn(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(112)
	itemParam := typ.NewTypeParam("T", nil)
	accParam := typ.NewTypeParam("A", nil)
	reducer := typ.NewGeneric("Reducer", []*typ.TypeParam{itemParam, accParam},
		typ.Func().Param("acc", accParam).Param("item", itemParam).Returns(accParam).Build())
	itemsRef := factflow.ExprRef(1121)
	fnRef := factflow.ExprRef(1122)
	initialRef := factflow.ExprRef(1123)
	numberReducer := typ.Func().Param("acc", typ.Number).Param("item", typ.String).Returns(typ.Number).Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"reduce": {
				Type: typ.Func().
					TypeParamRef(itemParam).
					TypeParamRef(accParam).
					Param("arr", typ.NewArray(itemParam)).
					Param("fn", typ.Instantiate(reducer, itemParam, accParam)).
					Param("initial", accParam).
					Returns(accParam).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 2}}}),
			},
		},
		NameFor:       staticName("reduce"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: itemsRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: fnRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: initialRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				itemsRef:   typevalue.WithWitness(reg, typevalue.FromType(reg, typ.NewArray(typ.String)), typ.NewArray(typ.String)),
				fnRef:      typevalue.WithWitness(reg, typevalue.FromType(reg, numberReducer), numberReducer),
				initialRef: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertTypeWitness(t, reg, got[0].Value, typ.Number)
	if gotPresence := product.PresenceOf(got[0].Value); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present", gotPresence)
	}
}

func TestSignatureOutcomeProviderElementOfUsesDeclaredReturnTypeWhenParamProjectionFails(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(12)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Param("items", typ.NewArray(typ.String)).Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 1}}}),
			},
		},
		NameFor: staticName("f"),
		Facts:   signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderCallbackReturnProjectsFirstReturnRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(13)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Integer).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestSignatureOutcomeProviderCallbackReturnResolvesNegativeParamRef(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(14)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", typ.String).
					Param("callback", typ.Func().Returns(typ.Boolean).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: -1}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression},
			{Kind: factflow.ValueSourceExpression},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureOutcomeProviderArrayOfCallbackReturnProjectsTableRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(15)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Table))
}

func TestSignatureOutcomeProviderCallbackReturnUsesDeclaredReturnTypeWhenProjectionFails(t *testing.T) {
	reg := standard.Registry()

	tests := []struct {
		name      string
		point     cfg.Point
		paramType typ.Type
		ref       effect.ParamRef
		args      []factflow.ValueSource
		want      runtimekind.Value
	}{
		{
			name:      "non-callable callback parameter",
			point:     cfg.Point(16),
			paramType: typ.String,
			ref:       effect.ParamRef{Index: 0},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
		{
			name:      "out-of-range callback parameter",
			point:     cfg.Point(17),
			paramType: typ.Func().Returns(typ.Number).Build(),
			ref:       effect.ParamRef{Index: 1},
			args:      []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}},
			want:      runtimekind.Singleton(runtimekind.Boolean),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("callback", tc.paramType).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: tc.ref}}),
					},
				},
				NameFor:       staticName("f"),
				ReturnTypeOps: testReturnTypeOps(),
				Facts:         signatureOutcomeProviderFacts(tc.point, tc.args),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: tc.point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, tc.want)
		})
	}
}

func TestSignatureOutcomeProviderTypeProjectionFieldReturnsFieldRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(18)
	record := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Integer).
		Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", record).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.Field("name")}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureOutcomeProviderTypeProjectionCallableReturnReturnsFirstReturnRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("callback", typ.Func().Returns(typ.Boolean, typ.String).Build()).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.CallableReturn()}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
}

func TestSignatureOutcomeProviderTypeProjectionGenericArgReturnsArgRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(20)
	param := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{param}, param)
	stringBox := typ.NewAlias("StringBox", typ.Instantiate(box, typ.String))
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", stringBox).
					Returns(typ.Any).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.GenericArg(0)}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSignatureOutcomeProviderConditionalTypeUsesSpecializedReturnWhenProjectedArgumentProvesCase(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(21)
	optionsRef := factflow.ExprRef(2101)
	optionsType := typetable.NewRecord().
		Field("message", typ.LiteralBool(true)).
		Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"listen": {
				Type: typ.Func().
					Param("topic", typ.String).
					Param("options", typ.Any).
					Returns(typ.Number).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
					Source: effect.ParamRef{Index: 1},
					Projection: projection.Projection{Steps: []projection.Step{
						projection.Field("message"),
					}},
					When: typ.LiteralBool(true),
					Then: typ.String,
				}}),
			},
		},
		NameFor:       staticName("listen"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression},
			{Kind: factflow.ValueSourceExpression, ExprRef: optionsRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				optionsRef: typevalue.WithWitness(reg, typevalue.FromType(reg, optionsType), optionsType),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.String))
	assertTypeWitness(t, reg, got[0].Value, typ.String)
}

func TestSignatureOutcomeProviderConditionalTypeTriesLaterMatchingCase(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(2102)
	choiceRef := factflow.ExprRef(2102)
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"select_mode": {
				Type: typ.Func().
					Param("choice", typ.String).
					Returns(typeexpr.Union(typ.String, typ.Number, typ.Boolean)).
					Build(),
				Effect: effect.Empty.
					With(returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
						Source: effect.ParamRef{Index: 0},
						When:   typ.LiteralString("auto"),
						Then:   typ.String,
					}}).
					With(returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
						Source: effect.ParamRef{Index: 0},
						When:   typ.LiteralString("none"),
						Then:   typ.Boolean,
					}}),
			},
		},
		NameFor:       staticName("select_mode"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: choiceRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				choiceRef: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.LiteralString("none")), typ.LiteralString("none")),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
	assertTypeWitness(t, reg, got[0].Value, typ.Boolean)
}

func TestSignatureOutcomeProviderConditionalTypeUsesDeclaredReturnWhenCaseIsNotProven(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	optionsRef := factflow.ExprRef(2201)
	optionsType := typetable.NewRecord().
		Field("message", typ.Boolean).
		Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"listen": {
				Type: typ.Func().
					Param("topic", typ.String).
					Param("options", typ.Any).
					Returns(typ.Number).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
					Source: effect.ParamRef{Index: 1},
					Projection: projection.Projection{Steps: []projection.Step{
						projection.Field("message"),
					}},
					When: typ.LiteralBool(true),
					Then: typ.String,
				}}),
			},
		},
		NameFor:       staticName("listen"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression},
			{Kind: factflow.ValueSourceExpression, ExprRef: optionsRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				optionsRef: typevalue.WithWitness(reg, typevalue.FromType(reg, optionsType), optionsType),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
	assertTypeWitness(t, reg, got[0].Value, typ.Number)
}

func TestSignatureOutcomeProviderInstantiatesGenericDeclaredReturnFromArgumentWitnesses(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(23)
	resultParam := typ.NewTypeParam("T", nil)
	resultGeneric := typ.NewGeneric("Result", []*typ.TypeParam{resultParam},
		typetable.NewRecord().Field("value", resultParam).Build())
	tParam := typ.NewTypeParam("T", nil)
	uParam := typ.NewTypeParam("U", nil)
	mapType := typ.Func().
		TypeParamRef(tParam).
		TypeParamRef(uParam).
		Param("result", typ.Instantiate(resultGeneric, tParam)).
		Param("fn", typ.Func().Param("value", tParam).Returns(uParam).Build()).
		Returns(typ.Instantiate(resultGeneric, uParam)).
		Build()
	decodedRef := factflow.ExprRef(22)
	callbackRef := factflow.ExprRef(23)
	decodedType := typ.Instantiate(resultGeneric, typ.String)
	callbackType := typ.Func().Param("value", typ.String).Returns(typ.Number).Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"map": {Type: mapType},
		},
		NameFor:       staticName("map"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
			{Kind: factflow.ValueSourceExpression, ExprRef: decodedRef, HasExpr: true},
			{Kind: factflow.ValueSourceExpression, ExprRef: callbackRef, HasExpr: true},
		}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				decodedRef:  typevalue.WithWitness(reg, typevalue.FromType(reg, decodedType), decodedType),
				callbackRef: typevalue.WithWitness(reg, typevalue.FromType(reg, callbackType), callbackType),
			},
		}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	gotType, ok := typevalue.TypeOf(reg, got[0].Value)
	if !ok {
		t.Fatalf("result type missing from value: %#v", got[0].Value)
	}
	wantType := typ.Instantiate(resultGeneric, typ.Number)
	if !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("result type = %v, want %v", gotType, wantType)
	}
}

func TestSignatureOutcomeProviderTypeProjectionUsesDeclaredReturnTypeWhenProjectionFails(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(25)
	record := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type: typ.Func().
					Param("value", record).
					Returns(typ.Number).
					Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
					Source:     effect.ParamRef{Index: 0},
					Projection: projection.Projection{Steps: []projection.Step{projection.Field("missing")}},
				}}),
			},
		},
		NameFor:       staticName("f"),
		ReturnTypeOps: testReturnTypeOps(),
		Facts:         signatureOutcomeProviderFacts(point, []factflow.ValueSource{{Kind: factflow.ValueSourceExpression}}),
	})

	got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got), got)
	}
	assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Number))
}

func TestActiveReturnTransformClassificationMatrix(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name:  "same as",
			label: returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}},
		},
		{
			name:  "element of",
			label: returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 0}}},
		},
		{
			name:  "optional element of",
			label: returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}}},
		},
		{
			name:  "callback return",
			label: returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}},
		},
		{
			name:  "array of callback return",
			label: returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}}},
		},
		{
			name: "type projection",
			label: returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
				Source: effect.ParamRef{Index: 0},
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("payload"),
					projection.CallableReturn(),
				}},
			}},
		},
		{
			name: "conditional type",
			label: returns.Return{ReturnIndex: 0, Transform: returns.ConditionalType{
				Source: effect.ParamRef{Index: 0},
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("message"),
				}},
				When: typ.LiteralBool(true),
				Then: typ.String,
			}},
		},
		{
			name:  "deep element",
			label: returns.Return{ReturnIndex: 0, Transform: returns.DeepElementOf{Source: effect.ParamRef{Index: 0}}},
		},
		{
			name:  "string unpack",
			label: returns.Return{ReturnIndex: 0, Transform: returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}}},
		},
		{
			name:  "select case",
			label: returns.Return{ReturnIndex: 0, Transform: returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}}},
		},
		{
			name: "select result",
			label: returns.Return{ReturnIndex: 0, Transform: returns.SelectResultOfCases{
				Cases:   effect.ParamRef{Index: 0},
				Default: effect.ParamRef{Index: 1},
			}},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantActive := capabilityStatusForEffectLabel(t, tc.label) == capability.StatusOperational
			transform, ok := activeReturnTransform(signature.Function{Effect: effect.Empty.With(tc.label)}, 0)
			if ok != wantActive {
				t.Fatalf("active = %v, want %v", ok, wantActive)
			}
			if wantActive && transform == nil {
				t.Fatal("active transform = nil, want concrete return transform")
			}
			if !wantActive && transform != nil {
				t.Fatalf("active transform = %#v, want none", transform)
			}
		})
	}
}

func TestActiveReturnTransformPointerTransformsFollowCapabilityStatus(t *testing.T) {
	tests := []struct {
		name      string
		transform returns.ReturnType
	}{
		{
			name:      "same as pointer",
			transform: &returns.SameAs{Source: effect.ParamRef{Index: 0}},
		},
		{
			name:      "element of pointer",
			transform: &returns.ElementOf{Source: effect.ParamRef{Index: 0}},
		},
		{
			name:      "optional element of pointer",
			transform: &returns.OptionalElementOf{Source: effect.ParamRef{Index: 0}},
		},
		{
			name:      "callback return pointer",
			transform: &returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		},
		{
			name:      "array of callback return pointer",
			transform: &returns.ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		},
		{
			name: "type projection pointer",
			transform: &returns.TypeProjection{
				Source:     effect.ParamRef{Index: 0},
				Projection: projection.Projection{Steps: []projection.Step{projection.CallableReturn()}},
			},
		},
		{
			name: "conditional type pointer",
			transform: &returns.ConditionalType{
				Source: effect.ParamRef{Index: 0},
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("message"),
				}},
				When: typ.LiteralBool(true),
				Then: typ.String,
			},
		},
		{
			name:      "reserved deep element pointer",
			transform: &returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
		},
		{
			name:      "reserved string unpack pointer",
			transform: &returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			desc, ok := caplabel.DescriptorForReturnTransform(tc.transform)
			if !ok {
				t.Fatalf("return transform %T has no capability descriptor", tc.transform)
			}
			wantActive := desc.Status == capability.StatusOperational
			transform, ok := activeReturnTransform(signature.Function{
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: tc.transform}),
			}, 0)
			if ok != wantActive {
				t.Fatalf("active = %v, want %v for capability status %q", ok, wantActive, desc.Status)
			}
			if wantActive && transform != tc.transform {
				t.Fatalf("active transform = %#v, want original pointer %#v", transform, tc.transform)
			}
		})
	}
}

func TestActiveReturnTransformRejectsTypedNilOperationalPointer(t *testing.T) {
	var sameAs *returns.SameAs
	desc, ok := caplabel.DescriptorForReturnTransform(sameAs)
	if !ok || desc.Status != capability.StatusOperational {
		t.Fatalf("typed nil SameAs descriptor = %v/%v, want operational", desc.Status, ok)
	}
	transform, ok := activeReturnTransform(signature.Function{
		Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: sameAs}),
	}, 0)
	if ok || transform != nil {
		t.Fatalf("active transform = %#v/%v, want none for typed nil pointer", transform, ok)
	}
}

func capabilityStatusForEffectLabel(t *testing.T, label effect.Label) capability.Status {
	t.Helper()
	if ret, ok := label.(returns.Return); ok {
		desc, ok := caplabel.DescriptorForReturnTransform(ret.Transform)
		if !ok {
			t.Fatalf("return transform %T has no capability descriptor", ret.Transform)
		}
		return desc.Status
	}
	desc, ok := caplabel.DescriptorFor(label)
	if !ok {
		t.Fatalf("label %T has no capability descriptor", label)
	}
	return desc.Status
}

func TestSignatureOutcomeProviderReservedReturnTransformsUseOnlyDeclaredReturnType(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(22)
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name: "deep element",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "string unpack",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select case",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select result",
			label: returns.Return{
				ReturnIndex: 0,
				Transform: returns.SelectResultOfCases{
					Cases:   effect.ParamRef{Index: 0},
					Default: effect.ParamRef{Index: 1},
				},
			},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
				Signatures: signatureMap{
					"f": {
						Type: typ.Func().
							Param("items", typ.NewArray(typ.String)).
							Param("default", typ.Number).
							Returns(typ.Boolean).
							Build(),
						Effect: effect.Empty.With(tc.label),
					},
				},
				NameFor: staticName("f"),
				Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{
					{Kind: factflow.ValueSourceExpression},
					{Kind: factflow.ValueSourceExpression},
				}),
			})

			got := provider(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

			if len(got) != 1 {
				t.Fatalf("got %d results, want 1 declared result: %#v", len(got), got)
			}
			assertRuntimeKind(t, reg, got[0].Value, runtimekind.Singleton(runtimekind.Boolean))
		})
	}
}

func TestActiveReturnTransformIgnoresReservedReturnTransforms(t *testing.T) {
	tests := []struct {
		name  string
		label effect.Label
	}{
		{
			name: "deep element",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.DeepElementOf{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "string unpack",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.StringUnpackValue{Format: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select case",
			label: returns.Return{
				ReturnIndex: 0,
				Transform:   returns.SelectCaseOfParam{Source: effect.ParamRef{Index: 0}},
			},
		},
		{
			name: "select result",
			label: returns.Return{
				ReturnIndex: 0,
				Transform: returns.SelectResultOfCases{
					Cases:   effect.ParamRef{Index: 0},
					Default: effect.ParamRef{Index: 1},
				},
			},
		},
		{
			name:  "return length",
			label: returns.ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if transform, ok := activeReturnTransform(signature.Function{Effect: effect.Empty.With(tc.label)}, 0); ok {
				t.Fatalf("active transform = %#v, want none", transform)
			}
		})
	}
}

func TestSupplementalResultsKeepsPrimarySlotsAndFillsMissingSignatureSlots(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: primaryValue}}}
	}
	signatures := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {Type: typ.Func().Returns(typ.Number, typ.String).Build()},
		},
		NameFor: staticName("f"),
	})

	got := calloutcome.ComposeSupplemental(primary, signatures)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	if len(got) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got), got)
	}
	if got[0].Index != 0 || !product.Equal(reg, got[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got[0])
	}
	if got[1].Index != 1 {
		t.Fatalf("supplemental slot index = %d, want 1", got[1].Index)
	}
	assertRuntimeKind(t, reg, got[1].Value, runtimekind.Singleton(runtimekind.String))
}

func TestSupplementalResultsKeepsPrimarySlotOverSignatureSameAs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7)
	argRef := factflow.ExprRef(11)
	primaryValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Boolean))
	argValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: primaryValue}}}
	}
	signatures := SignatureOutcomeProvider(SignatureOutcomeProviderConfig{
		Signatures: signatureMap{
			"f": {
				Type:   typ.Func().Returns(typ.Number).Build(),
				Effect: effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}}),
			},
		},
		NameFor: staticName("f"),
		Facts: signatureOutcomeProviderFacts(point, []factflow.ValueSource{{
			Kind:    factflow.ValueSourceExpression,
			ExprRef: argRef,
			HasExpr: true,
		}}),
		Sources: sourcevalue.NewSourceValues(sourcevalue.SourceValuesConfig{
			Registry: reg,
			ExpressionValues: map[factflow.ExprRef]product.Value{
				argRef: argValue,
			},
		}),
	})

	got := calloutcome.ComposeSupplemental(primary, signatures)(transfer.NodeContext{Registry: reg, Point: point}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil).Results

	assertCallOutcomeResults(t, reg, got, []product.Value{primaryValue})
}

func assertCallOutcomeResults(t *testing.T, reg *axis.Registry, got []callpayload.CallResult, want []product.Value) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d", len(got), len(want))
	}
	for i, value := range want {
		if got[i].Index != i {
			t.Fatalf("got result[%d].Index = %d, want %d", i, got[i].Index, i)
		}
		if !product.Equal(reg, got[i].Value, value) {
			t.Fatalf("got result[%d].Value = %v, want %v", i, got[i].Value, value)
		}
	}
}

func assertEscapeEvent(
	t *testing.T,
	events []callboundary.EscapeEventFact,
	target path.Path,
	kind callboundary.EscapeEventKind,
	recursive bool,
) {
	t.Helper()
	for _, event := range events {
		if event.Target.Equal(target) && event.Kind == kind && event.Recursive == recursive {
			return
		}
	}
	t.Fatalf("escape events = %#v, want target %s kind %d recursive=%v", events, target, kind, recursive)
}

func signatureOutcomeProviderFacts(point cfg.Point, args []factflow.ValueSource) factflow.Facts {
	return factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{ArgumentSources: args}),
		},
	})
}

func assertValue(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := st.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("state[%v] = %v, want %v", slot, got, want)
	}
}

func assertStatePresence(t *testing.T, reg *axis.Registry, st state.State, slot key.Value, want presence.Value) {
	t.Helper()
	if got := product.PresenceOf(st.ReadValue(reg, slot)); !presence.Equal(got, want) {
		t.Fatalf("state[%v] presence = %s, want %s", slot, got, want)
	}
}

func assertPathValue(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, st state.State, pathKey path.PathKey, want product.Value) {
	t.Helper()
	if got := st.ReadPathKey(reg, ks, pathKey); !product.Equal(reg, got, want) {
		t.Fatalf("state path[%s] = %v, want %v", pathKey, got, want)
	}
}

func assertRuntimeKind(t *testing.T, reg *axis.Registry, got product.Value, want runtimekind.Value) {
	t.Helper()
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("runtimekind = %s, want %s", kind, want)
	}
}

func assertPresence(t *testing.T, _ *axis.Registry, got product.Value, want presence.Value) {
	t.Helper()
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, want) {
		t.Fatalf("presence = %s, want %s", gotPresence, want)
	}
}

func assertTypeWitness(t *testing.T, reg *axis.Registry, got product.Value, want typ.Type) {
	t.Helper()
	witness := product.Get(reg, got, typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || !typ.TypeEquals(gotType, want) {
		t.Fatalf("type witness = %v/%v, want %v", gotType, ok, want)
	}
}

func callResultIdentityAt(reg *axis.Registry, results []callpayload.CallResult, index int) identity.ID {
	for _, result := range results {
		if result.Index != index {
			continue
		}
		id, _ := product.Get(reg, result.Value, identity.Key).ID()
		return id
	}
	return identity.ID{}
}

func assertCallReturnPresenceRelation(
	t *testing.T,
	relations []callpayload.CallReturnPresenceRelation,
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
	t.Fatalf("missing relation %d/%s -> %d/%s in %#v", triggerIndex, triggerPresence, targetIndex, targetPresence, relations)
}
