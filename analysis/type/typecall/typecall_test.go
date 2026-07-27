package typecall

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestCallableDirectFunction(t *testing.T) {
	fn := typ.Func().
		Param("input", typ.String).
		Returns(typ.Number).
		Build()

	got, ok := Callable(fn)
	if !ok {
		t.Fatal("Callable(function) failed")
	}
	assertType(t, got, fn)

	got, ok = Callable(typeexpr.Optional(fn))
	if !ok {
		t.Fatal("Callable(optional function) failed")
	}
	assertType(t, got, fn)
}

func TestCallableRecordMetatableCall(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()

	rec := recordWithMetamethod("__call", fn)
	got, ok := Callable(rec)
	if !ok {
		t.Fatal("Callable(record with __call) failed")
	}
	assertType(t, got, fn)

	noCall := recordWithMetamethod("__index", typ.String)
	if _, ok := Callable(noCall); ok {
		t.Fatal("Callable(record without __call) succeeded")
	}

	unconstrained := typetable.NewRecord().
		Metatable(typetable.MetatableUnconstrained).
		Build()
	if _, ok := Callable(unconstrained); ok {
		t.Fatal("Callable(record with unconstrained metatable) succeeded")
	}
	if got, ok := GetMetamethod(unconstrained, "__call"); ok || got != nil {
		t.Fatalf("GetMetamethod(record with unconstrained metatable, __call) = %v/%v, want nil/false", got, ok)
	}
}

func TestGetMetamethodDirectCallAndIndex(t *testing.T) {
	call := typ.Func().Returns(typ.Boolean).Build()
	index := typetable.NewRecord().
		Field("method", typ.Func().Returns(typ.String).Build()).
		Build()
	mt := typetable.NewRecord().
		Field("__call", call).
		Field("__index", index).
		Build()
	rec := typetable.NewRecord().Metatable(mt).Build()

	got, ok := GetMetamethod(rec, "__call")
	if !ok {
		t.Fatal("GetMetamethod(record, __call) failed")
	}
	assertType(t, got, call)

	got, ok = GetMetamethod(rec, "__index")
	if !ok {
		t.Fatal("GetMetamethod(record, __index) failed")
	}
	assertType(t, got, index)
}

func TestGetMetamethodWrappers(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	rec := recordWithMetamethod("__call", fn)

	t.Run("alias", func(t *testing.T) {
		got, ok := GetMetamethod(typ.NewAlias("CallableRecord", rec), "__call")
		if !ok {
			t.Fatal("GetMetamethod(alias record, __call) failed")
		}
		assertType(t, got, fn)
	})

	t.Run("optional", func(t *testing.T) {
		got, ok := GetMetamethod(typeexpr.Optional(rec), "__call")
		if !ok {
			t.Fatal("GetMetamethod(optional record, __call) failed")
		}
		assertType(t, got, fn)
	})

	t.Run("annotated", func(t *testing.T) {
		wrapped := typ.NewAnnotated(rec, []annotation.Annotation{{Name: "tag"}})
		got, ok := GetMetamethod(wrapped, "__call")
		if !ok {
			t.Fatal("GetMetamethod(annotated record, __call) failed")
		}
		assertType(t, got, fn)
	})

	t.Run("instantiated", func(t *testing.T) {
		param := typ.NewTypeParam("T", nil)
		body := typetable.NewRecord().
			Metatable(typetable.NewRecord().
				Field("__call", typ.Func().Returns(param).Build()).
				Build()).
			Build()
		box := typ.NewGeneric("CallableBox", []*typ.TypeParam{param}, body)

		got, ok := GetMetamethod(typ.Instantiate(box, typ.Number), "__call")
		if !ok {
			t.Fatal("GetMetamethod(CallableBox<number>, __call) failed")
		}
		assertType(t, got, typ.Func().Returns(typ.Number).Build())
	})
}

func TestGetMetamethodDoesNotFollowIndexChain(t *testing.T) {
	call := typ.Func().Returns(typ.Number).Build()
	methods := typetable.NewRecord().Field("__call", call).Build()
	rec := recordWithMetamethod("__index", methods)

	got, ok := GetMetamethod(rec, "__index")
	if !ok {
		t.Fatal("GetMetamethod(record, __index) failed")
	}
	assertType(t, got, methods)

	if _, ok := GetMetamethod(rec, "__call"); ok {
		t.Fatal("GetMetamethod followed __index chain for __call")
	}
	if _, ok := Callable(rec); ok {
		t.Fatal("Callable followed __index chain for __call")
	}
}

func TestCallableUnionAndIntersection(t *testing.T) {
	fn := typ.Func().Returns(typ.String).Build()
	rec := recordWithMetamethod("__call", fn)

	got, ok := Callable(typeexpr.Union(fn, rec))
	if !ok {
		t.Fatal("Callable(union of callable members) failed")
	}
	assertType(t, got, fn)

	if _, ok := Callable(typeexpr.Union(fn, typ.String)); ok {
		t.Fatal("Callable(union with non-callable member) succeeded")
	}

	got, ok = Callable(typeexpr.Intersection(typ.String, fn))
	if !ok {
		t.Fatal("Callable(intersection with callable member) failed")
	}
	assertType(t, got, fn)
}

func TestCallableUnionRequiresStableRepresentative(t *testing.T) {
	stringFn := typ.Func().Returns(typ.String).Build()
	numberFn := typ.Func().Returns(typ.Number).Build()

	if _, ok := Callable(typeexpr.Union(stringFn, numberFn)); ok {
		t.Fatal("Callable(union with different function witnesses) succeeded")
	}
}

func TestContextualCallableUsesUniqueCallableUnionArm(t *testing.T) {
	callback := typ.Func().
		Param("capture", typ.String).
		Returns(typeexpr.Optional(typ.String)).
		Build()
	formal := typeexpr.Union(
		typ.String,
		typetable.NewMap(typ.Any, typ.Any),
		callback,
	)

	got, ok := ContextualCallable(formal)
	if !ok {
		t.Fatal("ContextualCallable(union with one callable arm) failed")
	}
	assertType(t, got, callback)

	if _, ok := ContextualCallable(typeexpr.Union(callback, typ.Func().Param("n", typ.Number).Build())); ok {
		t.Fatal("ContextualCallable(ambiguous callable union) succeeded")
	}
}

func TestMetamethodAnyUnknownNeverPolicy(t *testing.T) {
	tests := []struct {
		name string
		in   typ.Type
		want typ.Type
	}{
		{name: "any", in: typ.Any, want: typ.Any},
		{name: "unknown", in: typ.Unknown, want: typ.Unknown},
		{name: "never", in: typ.Never, want: typ.Never},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetMetamethod(tt.in, "__call")
			if !ok {
				t.Fatalf("GetMetamethod(%s, __call) failed", tt.name)
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestGetMetamethodDoesNotSynthesizeBuiltinTableTopFields(t *testing.T) {
	if got, ok := GetMetamethod(typ.BuiltinTableTopMarker(), "__len"); ok || got != nil {
		t.Fatalf("GetMetamethod(table, __len) = %v/%v, want nil/false", got, ok)
	}
}

func TestCallableAnyUnknownNeverStrict(t *testing.T) {
	for _, tt := range []typ.Type{typ.Any, typ.Unknown, typ.Never} {
		if _, ok := Callable(tt); ok {
			t.Fatalf("Callable(%v) succeeded", tt)
		}
	}
}

func TestCallableConsumesReceiver(t *testing.T) {
	receiver := typetable.NewRecord().Field("id", typ.String).Build()

	namedSelf := typ.Func().
		Param("self", typ.Any).
		Param("payload", typ.String).
		Build()
	if !CallableConsumesReceiver(namedSelf, nil) {
		t.Fatal("named self formal should consume the receiver even without receiver type")
	}

	structuralSelf := typ.Func().
		Param("client", typetable.NewRecord().Field("id", typ.String).Build()).
		Param("payload", typ.String).
		Build()
	if !CallableConsumesReceiver(structuralSelf, receiver) {
		t.Fatal("receiver subtype should prove the first formal consumes the receiver")
	}

	plain := typ.Func().
		Param("payload", typ.String).
		Build()
	if CallableConsumesReceiver(plain, receiver) {
		t.Fatal("unrelated first formal should not consume the receiver")
	}
}

func TestGetMetamethodUnionFailsWhenOneBranchLacksMetamethod(t *testing.T) {
	withCall := recordWithMetamethod("__call", typ.Func().Returns(typ.String).Build())
	withoutCall := typetable.NewRecord().Field("name", typ.String).Build()

	if _, ok := GetMetamethod(typeexpr.Union(withCall, withoutCall), "__call"); ok {
		t.Fatal("GetMetamethod(union, __call) succeeded when one branch lacked the metamethod")
	}
}

func TestCallableUnionFailsWhenOneRecordBranchLacksCall(t *testing.T) {
	withCall := recordWithMetamethod("__call", typ.Func().Returns(typ.String).Build())
	withoutCall := typetable.NewRecord().Field("name", typ.String).Build()

	if _, ok := Callable(typeexpr.Union(withCall, withoutCall)); ok {
		t.Fatal("Callable(union with one non-callable record branch) succeeded")
	}
}

func TestMemberCallUnionRequiresCallableMemberOnEveryAlternative(t *testing.T) {
	stringMethod, status := MemberCall(typ.String, "upper")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(string, upper) status = %v, want ok", status)
	}
	if !callableValue(stringMethod, 0) {
		t.Fatalf("MemberCall(string, upper) type = %v, want callable", stringMethod)
	}

	if _, status := MemberCall(typeexpr.Union(typ.String, typ.Number), "upper"); status != MemberCallMissing {
		t.Fatalf("MemberCall(string|number, upper) status = %v, want missing", status)
	}

	left := typetable.NewRecord().
		Field("run", typ.Func().Returns(typ.String).Build()).
		Build()
	right := typetable.NewRecord().
		Field("run", typ.Func().Returns(typ.Number).Build()).
		Build()
	member, status := MemberCall(typeexpr.Union(left, right), "run")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(callable record union, run) status = %v, want ok", status)
	}
	assertType(t, member, typeexpr.Union(
		typ.Func().Returns(typ.String).Build(),
		typ.Func().Returns(typ.Number).Build(),
	))
}

func TestMemberCallTerminatesOnRecursiveUnion(t *testing.T) {
	loop := typ.NewRecursivePlaceholder("CallableLoop")
	loop.SetBody(typeexpr.Union(
		loop,
		typetable.NewRecord().Field("run", typ.Func().Returns(typ.String).Build()).Build(),
	))

	// The recursive arm has the same callable evidence as its record arm;
	// access resolves the regular graph to that fixed point without a depth
	// cutoff.
	member, status := MemberCall(loop, "run")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(recursive union, run) status = %v, want ok", status)
	}
	assertType(t, member, typ.Func().Returns(typ.String).Build())
}

func TestMemberCallUsesTypeParameterConstraintSurface(t *testing.T) {
	method := typ.Func().
		Param("self", typ.Self).
		Returns(typ.String).
		Build()
	constraint := typetable.NewRecord().
		Field("tostring", method).
		Build()
	param := typ.NewTypeParam("T", constraint)

	member, status := MemberCall(param, "tostring")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(constrained T, tostring) status = %v, want ok", status)
	}
	assertType(t, member, method)

	if got, status := MemberCall(typ.NewTypeParam("U", nil), "tostring"); status != MemberCallMissing {
		t.Fatalf("MemberCall(unconstrained U, tostring) = %v/%v, want missing", got, status)
	}
}

func TestMemberCallRejectsOptionalUnionMember(t *testing.T) {
	callable := typ.Func().Build()
	left := typetable.NewRecord().
		Field("run", callable).
		Build()
	right := typetable.NewRecord().
		OptField("run", callable).
		Build()

	member, status := MemberCall(typeexpr.Union(left, right), "run")
	if status != MemberCallNotCallable {
		t.Fatalf("MemberCall(optional member union, run) status = %v, want not-callable", status)
	}
	assertType(t, member, typeexpr.Optional(callable))
}

func TestIndexedMemberCallUsesExactStaticIntMembers(t *testing.T) {
	callable := typ.Func().Returns(typ.String).Build()
	receiver := typetable.NewRecord().
		StaticIntIndex(1, callable).
		StaticIntIndex(2, typ.Number).
		Build()

	member, status := IndexedMemberCall(receiver, typ.LiteralInt(1))
	if status != MemberCallOK {
		t.Fatalf("IndexedMemberCall([1]) status = %v, want ok", status)
	}
	assertType(t, member, callable)

	member, status = IndexedMemberCall(receiver, typ.LiteralInt(2))
	if status != MemberCallNotCallable {
		t.Fatalf("IndexedMemberCall([2]) status = %v, want not-callable", status)
	}
	assertType(t, member, typ.Number)

	if _, status := IndexedMemberCall(receiver, typ.LiteralInt(3)); status != MemberCallMissing {
		t.Fatalf("IndexedMemberCall([3]) status = %v, want missing", status)
	}
}

func TestIndexedMemberCallableBindsReceiverSelf(t *testing.T) {
	method := typ.Func().
		Param("receiver", typ.Self).
		Param("payload", typ.String).
		Returns(typ.Self).
		Build()
	receiver := typetable.NewRecord().
		StaticIntIndex(1, method).
		Build()

	got, status, ok := IndexedMemberCallable(receiver, typ.LiteralInt(1))
	if status != MemberCallOK || !ok {
		t.Fatalf("IndexedMemberCallable(receiver, [1]) = status %v ok %v, want ok", status, ok)
	}
	if len(got.Params) != 2 {
		t.Fatalf("params = %#v, want receiver and payload", got.Params)
	}
	if got.Params[0].Type != receiver {
		t.Fatalf("receiver param = %v, want concrete receiver", got.Params[0].Type)
	}
	if !typ.TypeEquals(got.Params[1].Type, typ.String) {
		t.Fatalf("payload param = %v, want string", got.Params[1].Type)
	}
	if len(got.Returns) != 1 || got.Returns[0] != receiver {
		t.Fatalf("returns = %#v, want concrete receiver", got.Returns)
	}
}

func TestMemberCallAmbientChannelReceive(t *testing.T) {
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	member, status := MemberCall(channel, "receive")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(Channel<string>, receive) status = %v, want ok", status)
	}
	fn, ok := member.(*typ.Function)
	if !ok {
		t.Fatalf("receive member = %T %[1]v, want function", member)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, channel) {
		t.Fatalf("receive params = %#v, want self Channel<string>", fn.Params)
	}
	if len(fn.Returns) != 2 || !typ.TypeEquals(fn.Returns[0], typ.String) || !typ.TypeEquals(fn.Returns[1], typ.Boolean) {
		t.Fatalf("receive returns = %#v, want string, boolean", fn.Returns)
	}
}

func TestAmbientChannelPayloadType(t *testing.T) {
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	got, ok := AmbientChannelPayloadType(channel)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("AmbientChannelPayloadType(Channel<string>) = %v/%v, want string/true", got, ok)
	}
	if got, ok := AmbientChannelPayloadType(typ.String); ok || got != nil {
		t.Fatalf("AmbientChannelPayloadType(string) = %v/%v, want nil/false", got, ok)
	}
}

func TestMemberCallRuntimeModuleChannelReceive(t *testing.T) {
	channel := typ.Instantiate(runtimeModuleChannelGeneric(), typ.String)
	member, status := MemberCall(channel, "receive")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(channel.Channel<string>, receive) status = %v, want ok", status)
	}
	fn, ok := member.(*typ.Function)
	if !ok {
		t.Fatalf("receive member = %T %[1]v, want function", member)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, channel) {
		t.Fatalf("receive params = %#v, want self channel.Channel<string>", fn.Params)
	}
	if len(fn.Returns) != 2 || !typ.TypeEquals(fn.Returns[0], typ.String) || !typ.TypeEquals(fn.Returns[1], typ.Boolean) {
		t.Fatalf("receive returns = %#v, want string, boolean", fn.Returns)
	}
}

func TestMemberCallAmbientChannelCaseReceive(t *testing.T) {
	channel := typeexpr.Optional(typ.Instantiate(ambient.ChannelGeneric(), typ.Number))
	member, status := MemberCall(channel, "case_receive")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(Channel<number>?, case_receive) status = %v, want ok", status)
	}
	fn, ok := member.(*typ.Function)
	if !ok {
		t.Fatalf("case_receive member = %T %[1]v, want function", member)
	}
	if len(fn.Params) != 1 || !typ.TypeEquals(fn.Params[0].Type, typ.Instantiate(ambient.ChannelGeneric(), typ.Number)) {
		t.Fatalf("case_receive params = %#v, want self Channel<number>", fn.Params)
	}
}

func TestMemberCallAmbientChannelSend(t *testing.T) {
	channel := typ.Instantiate(ambient.ChannelGeneric(), typ.String)
	member, status := MemberCall(channel, "send")
	if status != MemberCallOK {
		t.Fatalf("MemberCall(Channel<string>, send) status = %v, want ok", status)
	}
	fn, ok := member.(*typ.Function)
	if !ok {
		t.Fatalf("send member = %T %[1]v, want function", member)
	}
	if len(fn.Params) != 2 ||
		!typ.TypeEquals(fn.Params[0].Type, channel) ||
		!typ.TypeEquals(fn.Params[1].Type, typ.String) {
		t.Fatalf("send params = %#v, want self Channel<string>, string payload", fn.Params)
	}
	if len(fn.Returns) != 1 || !typ.TypeEquals(fn.Returns[0], typ.Boolean) {
		t.Fatalf("send returns = %#v, want boolean", fn.Returns)
	}
}

func runtimeModuleChannelGeneric() *typ.Generic {
	elem := typ.NewTypeParam("T", nil)
	return typ.NewGeneric("channel.Channel", []*typ.TypeParam{elem}, typ.NewInterface("channel.Channel", nil))
}

func TestMemberCallableBindsReceiverSelf(t *testing.T) {
	method := typ.Func().
		Param("receiver", typ.Self).
		Param("next", typ.Self).
		Returns(typ.Self).
		Build()
	receiver := typetable.NewRecord().
		Field("chain", method).
		Build()

	got, status, ok := MemberCallable(receiver, "chain")
	if status != MemberCallOK || !ok {
		t.Fatalf("MemberCallable(receiver, chain) = status %v ok %v, want ok", status, ok)
	}
	if len(got.Params) != 2 {
		t.Fatalf("params = %#v, want two receiver-bound params", got.Params)
	}
	if got.Params[0].Type != receiver || got.Params[1].Type != receiver {
		t.Fatalf("params = %#v, want Self substituted with receiver", got.Params)
	}
	if len(got.Returns) != 1 || got.Returns[0] != receiver {
		t.Fatalf("returns = %#v, want receiver", got.Returns)
	}
}

func TestMemberCallableBindsReceiverSelfRef(t *testing.T) {
	receiverRef := typ.NewRef("", "Builder")
	unrelatedRef := typ.NewRef("", "Builder")
	method := typ.Func().
		Param("receiver", receiverRef).
		Param("payload", typetable.NewRecord().
			Field("target", receiverRef).
			Field("shadow", unrelatedRef).
			Build()).
		Returns(receiverRef, unrelatedRef).
		Build()
	receiver := typetable.NewRecord().
		Field("chain", method).
		Build()

	got, status, ok := MemberCallable(receiver, "chain")
	if status != MemberCallOK || !ok {
		t.Fatalf("MemberCallable(receiver, chain) = status %v ok %v, want ok", status, ok)
	}
	if len(got.Params) != 2 {
		t.Fatalf("params = %#v, want two params", got.Params)
	}
	if got.Params[0].Type != receiver {
		t.Fatalf("receiver param = %v, want concrete receiver", got.Params[0].Type)
	}
	payload, ok := got.Params[1].Type.(*typ.Record)
	if !ok {
		t.Fatalf("payload param = %T %[1]v, want record", got.Params[1].Type)
	}
	target := payload.GetField("target")
	if target == nil || target.Type != receiver {
		t.Fatalf("payload target = %v, want concrete receiver", target)
	}
	shadow := payload.GetField("shadow")
	if shadow == nil || shadow.Type != unrelatedRef {
		t.Fatalf("payload shadow = %v, want unrelated ref", shadow)
	}
	if len(got.Returns) != 2 || got.Returns[0] != receiver || got.Returns[1] != unrelatedRef {
		t.Fatalf("returns = %#v, want receiver and unrelated ref", got.Returns)
	}
}

func TestCallableReturnFirstReturn(t *testing.T) {
	fn := typ.Func().
		Param("input", typ.String).
		Returns(typ.Number, typ.Boolean).
		Build()

	got, ok := CallableReturn(fn)
	if !ok {
		t.Fatal("CallableReturn(function) failed")
	}
	assertType(t, got, typ.Number)
}

func TestCallableReturnUnionProjectionUsesNormalizePackage(t *testing.T) {
	callableReturning := func(t typ.Type) typ.Type {
		return typ.Func().Returns(t).Build()
	}

	returns := []typ.Type{typ.Unknown, typ.String, typ.Never}
	got, ok := CallableReturn(typeexpr.Union(
		callableReturning(returns[0]),
		callableReturning(returns[1]),
		callableReturning(returns[2]),
	))
	if !ok {
		t.Fatal("CallableReturn(union) failed")
	}
	assertType(t, got, normalize.UnionForEvidence(returns...))
}

func TestCallableReturnUnionProjectionPolicy(t *testing.T) {
	callableReturning := func(t typ.Type) typ.Type {
		return typ.Func().Returns(t).Build()
	}

	tests := []struct {
		name     string
		receiver typ.Type
		want     typ.Type
	}{
		{
			name: "any preserves concrete projection",
			receiver: typeexpr.Union(
				callableReturning(typ.Any),
				callableReturning(typ.String),
			),
			want: typeexpr.Union(typ.Any, typ.String),
		},
		{
			name: "optional return preserves nilability",
			receiver: typeexpr.Union(
				callableReturning(typeexpr.Optional(typ.Number)),
				callableReturning(typ.String),
			),
			want: typeexpr.Union(typ.Nil, typ.String, typ.Number),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CallableReturn(tt.receiver)
			if !ok {
				t.Fatal("CallableReturn(union) failed")
			}
			assertType(t, got, tt.want)
		})
	}
}

func TestCallableReturnAnyUnknownPolicy(t *testing.T) {
	got, ok := CallableReturn(typ.Any)
	if !ok {
		t.Fatal("CallableReturn(any) failed")
	}
	assertType(t, got, typ.Any)

	got, ok = CallableReturn(typ.Unknown)
	if !ok {
		t.Fatal("CallableReturn(unknown) failed")
	}
	assertType(t, got, typ.Unknown)
}

func TestInstantiateGenericCallInfersIdentityReturn(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(param).
		Param("x", param).
		Returns(param).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.LiteralString("hello")})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("x", typ.LiteralString("hello")).
		Returns(typ.LiteralString("hello")).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallWithBindingsReturnsCanonicalParams(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(param).
		Param("value", param).
		Returns(param).
		Build()

	got, violations, bindings := InstantiateGenericCallWithBindings(fn, []typ.Type{typ.String})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("value", typ.String).
		Returns(typ.String).
		Build()
	assertType(t, got, want)
	if len(bindings) != 1 || bindings[0].Param != param || !typ.TypeEquals(bindings[0].Type, typ.String) || bindings[0].Index != 0 {
		t.Fatalf("bindings = %#v, want T -> string from argument 0", bindings)
	}
}

func TestInstantiateGenericCallReportsConstraintViolation(t *testing.T) {
	constraint := typetable.NewRecord().Field("name", typ.String).Build()
	param := typ.NewTypeParam("T", constraint)
	fn := typ.Func().
		TypeParamRef(param).
		Param("x", param).
		Returns(param).
		Build()

	_, violations := InstantiateGenericCall(fn, []typ.Type{typ.LiteralInt(42)})
	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one", violations)
	}
	if violations[0].Index != 0 {
		t.Fatalf("violation index = %d, want 0", violations[0].Index)
	}
	assertType(t, violations[0].Got, typ.LiteralInt(42))
	assertType(t, violations[0].Constraint, constraint)
}

func TestInstantiateGenericCallRejectsNestedRecordMismatchBeyondRecursionDepth(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	formal := nestedRecordType(257, typ.String)
	actual := nestedRecordType(257, typ.Number)
	fn := typ.Func().
		TypeParamRef(param).
		Param("seed", param).
		Param("value", formal).
		Build()

	_, violations := InstantiateGenericCall(fn, []typ.Type{typ.String, actual})
	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one deeply nested record violation", violations)
	}
	if violations[0].Index != 1 {
		t.Fatalf("violation index = %d, want 1", violations[0].Index)
	}
}

func TestInstantiatedArgumentAssignableRejectsNestedRecordMismatchBeyondRecursionDepth(t *testing.T) {
	formal := nestedRecordType(257, typ.String)
	actual := nestedRecordType(257, typ.Number)

	if InstantiatedArgumentAssignable(actual, formal) {
		t.Fatal("deeply nested mismatched record was assignable")
	}
}

func TestGenericCallValidationIgnoresIncidentalDepthArgument(t *testing.T) {
	record := typetable.NewRecord().Field("value", typ.String).Build()
	deep := 257

	if !instantiatedArgumentAssignable(typ.String, typ.String, deep) {
		t.Fatal("equal arguments should validate at any traversal depth")
	}
	if !providedRecordFieldsAssignable(record, record, deep) {
		t.Fatal("equal records should validate at any traversal depth")
	}

	if !InstantiatedArgumentAssignable(nil, typ.String) {
		t.Fatal("nil actual type should remain unspecified")
	}
	if !InstantiatedArgumentAssignable(typ.String, nil) {
		t.Fatal("nil formal type should remain unspecified")
	}
}

func TestGenericCallSubstitutionUsesDeepInferenceEvidence(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	formal := nestedRecordType(257, param)
	actual := nestedRecordType(257, typ.Number)
	fn := typ.Func().
		TypeParamRef(param).
		Param("seed", param).
		Param("value", formal).
		Build()

	_, violations := InstantiateGenericCall(fn, []typ.Type{typ.String, actual})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want deep evidence included in inferred union", violations)
	}
}

func nestedRecordType(depth int, leaf typ.Type) typ.Type {
	result := leaf
	for range depth {
		result = typetable.NewRecord().Field("next", result).Build()
	}
	return result
}

func TestInstantiateGenericCallSubstitutesGenericAliasReturn(t *testing.T) {
	boxParam := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam},
		typetable.NewRecord().Field("value", boxParam).Build())
	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("value", fnParam).
		Returns(typ.Instantiate(box, fnParam)).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.LiteralBool(true)})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("value", typ.LiteralBool(true)).
		Returns(typ.Instantiate(box, typ.LiteralBool(true))).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersThroughRegularRecursiveGeneric(t *testing.T) {
	boxParam := typ.NewTypeParam("BoxT", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{boxParam}, nil)
	box.SetBody(typetable.NewRecord().
		Field("value", boxParam).
		Field("next", typeexpr.Optional(typ.Instantiate(box, boxParam))).
		Build())

	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("box", typ.Instantiate(box, fnParam)).
		Returns(fnParam).
		Build()
	actual := subst.ExpandInstantiated(typ.Instantiate(box, typ.Number))

	got, violations := InstantiateGenericCall(fn, []typ.Type{actual})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	if len(got.Returns) != 1 {
		t.Fatalf("returns = %v, want one inferred result", got.Returns)
	}
	assertType(t, got.Returns[0], typ.Number)
}

func TestInstantiateGenericCallInfersArrayElementOptionalReturn(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(param).
		Param("list", typ.NewArray(param)).
		Returns(typeexpr.Optional(param)).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.NewArray(typ.String)})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("list", typ.NewArray(typ.String)).
		Returns(typeexpr.Optional(typ.String)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersTupleLiteralArrayElementFamily(t *testing.T) {
	tests := []struct {
		name   string
		actual typ.Type
		want   typ.Type
	}{
		{
			name:   "string literals",
			actual: typ.NewTuple(typ.LiteralString("a"), typ.LiteralString("bb"), typ.LiteralString("ccc")),
			want:   typ.String,
		},
		{
			name:   "integer literals",
			actual: typ.NewTuple(typ.LiteralInt(1), typ.LiteralInt(2), typ.LiteralInt(3)),
			want:   typ.Integer,
		},
		{
			name:   "boolean literals",
			actual: typ.NewTuple(typ.LiteralBool(true), typ.LiteralBool(false)),
			want:   typ.Boolean,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := typ.NewTypeParam("T", nil)
			fn := typ.Func().
				TypeParamRef(param).
				Param("list", typ.NewArray(param)).
				Returns(typeexpr.Optional(param)).
				Build()

			got, violations := InstantiateGenericCall(fn, []typ.Type{tt.actual})
			if len(violations) != 0 {
				t.Fatalf("violations = %#v, want none", violations)
			}
			want := typ.Func().
				Param("list", typ.NewArray(tt.want)).
				Returns(typeexpr.Optional(tt.want)).
				Build()
			assertType(t, got, want)
		})
	}
}

func TestInstantiateGenericCallInfersChannelCallbackOptionalReturn(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	output := typ.NewTypeParam("U", nil)
	payload := typetable.NewRecord().Field("id", typ.String).Build()
	fn := typ.Func().
		TypeParamRef(input).
		TypeParamRef(output).
		Param("channel", typ.Instantiate(ambient.ChannelGeneric(), input)).
		Param("fn", typ.Func().Param("value", input).Returns(output).Build()).
		Returns(typeexpr.Optional(output)).
		Build()
	callback := typ.Func().
		Param("value", payload).
		Returns(typ.String).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{
		typ.Instantiate(ambient.ChannelGeneric(), payload),
		callback,
	})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("channel", typ.Instantiate(ambient.ChannelGeneric(), payload)).
		Param("fn", typ.Func().Param("value", payload).Returns(typ.String).Build()).
		Returns(typeexpr.Optional(typ.String)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersChannelFromNestedOptionsObject(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	witness := typetable.NewRecord().
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(input).Build()).
		Build()
	options := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), input)).
		Field("schema", typetable.NewRecord().Field("witness", witness).Build()).
		Build()
	payload := typetable.NewRecord().Field("id", typ.String).Build()
	actual := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), payload)).
		Field("schema", typetable.NewRecord().Field("witness", typetable.NewRecord().
			Field("decode", typ.Func().Param("raw", typ.Any).Returns(payload).Build()).
			Build()).Build()).
		Build()
	fn := typ.Func().
		TypeParamRef(input).
		Param("topic", typ.String).
		Param("options", options).
		Returns(typ.Instantiate(ambient.ChannelGeneric(), input)).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.String, actual})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	assertType(t, got.Returns[0], typ.Instantiate(ambient.ChannelGeneric(), payload))
}

func TestInstantiateGenericCallInfersChannelFromPartialInstantiatedOptionsObject(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	witness := typetable.NewRecord().
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(input).Build()).
		Build()
	optionsBody := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), input)).
		Field("schema", typetable.NewRecord().Field("witness", witness).Build()).
		Build()
	options := typ.NewGeneric("ListenOptions", []*typ.TypeParam{input}, optionsBody)
	payload := typetable.NewRecord().Field("id", typ.String).Build()
	actual := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), payload)).
		Build()
	fn := typ.Func().
		TypeParamRef(input).
		Param("topic", typ.String).
		Param("options", typ.Instantiate(options, input)).
		Returns(typ.Instantiate(ambient.ChannelGeneric(), input)).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.String, actual})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	assertType(t, got.Returns[0], typ.Instantiate(ambient.ChannelGeneric(), payload))
}

func TestInstantiateGenericCallRejectsConflictingInstantiatedOptionsObject(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	optionsBody := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), input)).
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(input).Build()).
		Build()
	options := typ.NewGeneric("ListenOptions", []*typ.TypeParam{input}, optionsBody)
	event := typetable.NewRecord().Field("id", typ.String).Build()
	timer := typetable.NewRecord().Field("elapsed", typ.Number).Build()
	actual := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), event)).
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(timer).Build()).
		Build()
	fn := typ.Func().
		TypeParamRef(input).
		Param("topic", typ.String).
		Param("options", typ.Instantiate(options, input)).
		Returns(typ.Instantiate(ambient.ChannelGeneric(), input)).
		Build()

	_, violations := InstantiateGenericCall(fn, []typ.Type{typ.String, actual})
	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one conflicting options violation", violations)
	}
	if violations[0].Index != 1 {
		t.Fatalf("violation index = %d, want 1", violations[0].Index)
	}
	assertType(t, violations[0].Got, actual)
}

func TestInstantiateGenericCallTraceExplainsConflictingOptionsObject(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	optionsBody := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), input)).
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(input).Build()).
		Build()
	options := typ.NewGeneric("ListenOptions", []*typ.TypeParam{input}, optionsBody)
	event := typetable.NewRecord().Field("id", typ.String).Build()
	timer := typetable.NewRecord().Field("elapsed", typ.Number).Build()
	actual := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), event)).
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(timer).Build()).
		Build()
	fn := typ.Func().
		TypeParamRef(input).
		Param("topic", typ.String).
		Param("options", typ.Instantiate(options, input)).
		Returns(typ.Instantiate(ambient.ChannelGeneric(), input)).
		Build()

	_, violations, trace := InstantiateGenericCallWithTrace(fn, []typ.Type{typ.String, actual})
	if len(violations) != 1 {
		t.Fatalf("violations = %#v, want one conflicting options violation", violations)
	}
	if len(trace.Contributions) != 2 {
		t.Fatalf("trace contributions = %#v, want channel and decode contributions", trace.Contributions)
	}
	channel := trace.Contributions[0]
	if channel.Index != 1 || channel.Param == nil || channel.Param.Name != "T" || len(channel.Path) != 2 ||
		channel.Path[0].Kind != InferencePathField || channel.Path[0].Name != "channel" ||
		channel.Path[1].Kind != InferencePathTypeArgument || channel.Path[1].Index != 1 {
		t.Fatalf("channel contribution = %#v, want argument 2.channel type argument", channel)
	}
	assertType(t, channel.Type, event)
	decode := trace.Contributions[1]
	if decode.Index != 1 || decode.Param == nil || decode.Param.Name != "T" || len(decode.Path) != 2 ||
		decode.Path[0].Kind != InferencePathField || decode.Path[0].Name != "decode" ||
		decode.Path[1].Kind != InferencePathFunctionReturn || decode.Path[1].Index != 1 {
		t.Fatalf("decode contribution = %#v, want argument 2.decode return 1", decode)
	}
	assertType(t, decode.Type, timer)
}

func TestInstantiateGenericCallInfersRecursiveWitnessType(t *testing.T) {
	input := typ.NewTypeParam("T", nil)
	witness := typ.NewGeneric("Type", []*typ.TypeParam{input},
		typetable.NewRecord().
			Field("decode", typ.Func().Param("raw", typ.Any).Returns(input).Build()).
			Build())
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("id", typ.String).
			Field("children", typ.NewArray(self)).
			Build()
	})
	fn := typ.Func().
		TypeParamRef(input).
		Param("data", typ.String).
		Param("witness", typ.Instantiate(witness, input)).
		Returns(input).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{
		typ.String,
		typ.Instantiate(witness, node),
	})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	assertType(t, got.Returns[0], node)
}

func TestInstantiateGenericCallInfersCallbackReturn(t *testing.T) {
	result := resultGeneric()
	profile := typetable.NewRecord().Field("id", typ.String).Build()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(fnParamU).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()

	callback := typ.Func().Param("item", profile).Returns(typ.String).Build()
	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.Instantiate(result, profile), callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("result", typ.Instantiate(result, profile)).
		Param("fn", typ.Func().Param("item", profile).Returns(typ.String).Build()).
		Returns(typ.Instantiate(result, typ.String)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersCallbackParamWhenResultArgUnavailable(t *testing.T) {
	result := resultGeneric()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(fnParamU).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()

	callback := typ.Func().Param("item", typ.Number).Returns(typ.String).Build()
	got, violations := InstantiateGenericCall(fn, []typ.Type{nil, callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("result", typ.Instantiate(result, typ.Number)).
		Param("fn", typ.Func().Param("item", typ.Number).Returns(typ.String).Build()).
		Returns(typ.Instantiate(result, typ.String)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersCallbackResultReturn(t *testing.T) {
	result := resultGeneric()
	profile := typetable.NewRecord().Field("id", typ.String).Build()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(typ.Instantiate(result, fnParamU)).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()

	callback := typ.Func().
		Param("item", profile).
		Returns(typ.Instantiate(result, typ.Number)).
		Build()
	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.Instantiate(result, profile), callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("result", typ.Instantiate(result, profile)).
		Param("fn", typ.Func().Param("item", profile).Returns(typ.Instantiate(result, typ.Number)).Build()).
		Returns(typ.Instantiate(result, typ.Number)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersCallbackResultReturnFromStructuralUnionAlias(t *testing.T) {
	errorType := typetable.NewRecord().Field("message", typ.String).Build()
	resultParam := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", resultParam).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", errorType).
			Build(),
	))
	stringResult := typ.NewAlias("StringResult", typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", typ.String).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", errorType).
			Build(),
	))
	profile := typetable.NewRecord().Field("id", typ.String).Build()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(typ.Instantiate(result, fnParamU)).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()

	callback := typ.Func().
		Param("item", profile).
		Returns(stringResult).
		Build()
	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.Instantiate(result, profile), callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("result", typ.Instantiate(result, profile)).
		Param("fn", typ.Func().Param("item", profile).Returns(typ.Instantiate(result, typ.String)).Build()).
		Returns(typ.Instantiate(result, typ.String)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallInfersResultInputFromOkArmOnly(t *testing.T) {
	resultParam := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam}, typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", resultParam).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	user := typetable.NewRecord().
		Field("id", typ.String).
		Field("retries", typ.Number).
		Build()
	userResult := typ.NewAlias("UserResult", typeexpr.Union(
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", user).
			Build(),
		typetable.NewRecord().
			Field("ok", typ.LiteralBool(false)).
			Field("error", typ.String).
			Build(),
	))
	audit := typetable.NewRecord().
		Field("user_id", typ.String).
		Field("event", typ.LiteralString("never")).
		Build()
	fnParamT := typ.NewTypeParam("T", nil)
	fnParamU := typ.NewTypeParam("U", nil)
	fn := typ.Func().
		TypeParamRef(fnParamT).
		TypeParamRef(fnParamU).
		Param("result", typ.Instantiate(result, fnParamT)).
		Param("fn", typ.Func().Param("item", fnParamT).Returns(typ.Instantiate(result, fnParamU)).Build()).
		Returns(typ.Instantiate(result, fnParamU)).
		Build()

	callback := typ.Func().
		Param("item", user).
		Returns(typetable.NewRecord().
			Field("ok", typ.LiteralBool(true)).
			Field("value", audit).
			Build()).
		Build()
	got, violations := InstantiateGenericCall(fn, []typ.Type{userResult, callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	want := typ.Func().
		Param("result", typ.Instantiate(result, user)).
		Param("fn", typ.Func().Param("item", user).Returns(typ.Instantiate(result, audit)).Build()).
		Returns(typ.Instantiate(result, audit)).
		Build()
	assertType(t, got, want)
}

func TestInstantiateGenericCallPreservesUninferredTypeParam(t *testing.T) {
	resultParam := typ.NewTypeParam("T", nil)
	result := typ.NewGeneric("Result", []*typ.TypeParam{resultParam},
		typetable.NewRecord().Field("value", resultParam).Build())
	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("message", typ.String).
		Returns(typ.Instantiate(result, fnParam)).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.LiteralString("missing")})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	if len(got.TypeParams) != 1 {
		t.Fatalf("type params = %d, want uninferred param preserved", len(got.TypeParams))
	}
	assertType(t, got.Returns[0], typ.Instantiate(result, fnParam))
}

func TestInstantiateGenericCallInfersThroughGenericCallbackAlias(t *testing.T) {
	aliasParam := typ.NewTypeParam("T", nil)
	predicate := typ.NewGeneric("Predicate", []*typ.TypeParam{aliasParam},
		typ.Func().Param("item", aliasParam).Returns(typ.Boolean).Build())
	fnParam := typ.NewTypeParam("T", nil)
	user := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build()
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("items", typ.NewArray(fnParam)).
		Param("pred", typ.Instantiate(predicate, fnParam)).
		Returns(typeexpr.Optional(fnParam)).
		Build()
	callback := typ.Func().
		Param("user", user).
		Returns(typ.Boolean).
		Build()

	got, violations, bindings := InstantiateGenericCallWithBindings(fn, []typ.Type{typ.NewArray(user), callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	if len(bindings) != 1 || bindings[0].Param != fnParam || !typ.TypeEquals(bindings[0].Type, user) {
		t.Fatalf("bindings = %#v, want T=%v", bindings, user)
	}
	assertType(t, got.Returns[0], typeexpr.Optional(user))
}

// TestContainsTypeParamOutsideShadowDeepFiniteChain verifies that a finite
// record chain reaches its exact leaf instead of being conservatively cut off.
func TestContainsTypeParamOutsideShadowDeepFiniteChain(t *testing.T) {
	target := typ.NewTypeParam("T", nil)
	var deep typ.Type = typ.String
	for i := 0; i < 10000; i++ {
		deep = typetable.NewRecord().Field("next", deep).Build()
	}
	if containsTypeParamOutsideShadow(deep, target, false, nil, 0) {
		t.Fatal("deep finite search reported an absent type parameter")
	}
}

func TestInstantiateGenericCallInfersThroughRecursiveUnionWithoutDepthLimit(t *testing.T) {
	param := typ.NewTypeParam("T", nil)
	formal := typ.NewRecursivePlaceholder("RecursiveChoice")
	formal.SetBody(&typ.Union{Members: []typ.Type{formal, param}})
	fn := typ.Func().
		TypeParamRef(param).
		Param("value", formal).
		Returns(param).
		Build()

	got, violations := InstantiateGenericCall(fn, []typ.Type{typ.String})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	if len(got.TypeParams) != 0 {
		t.Fatalf("type params = %d, want fully inferred", len(got.TypeParams))
	}
	assertType(t, got.Returns[0], typ.String)
}

func resultGeneric() *typ.Generic {
	resultParam := typ.NewTypeParam("T", nil)
	return typ.NewGeneric("Result", []*typ.TypeParam{resultParam},
		typetable.NewRecord().Field("value", resultParam).Build())
}

func assertType(t *testing.T, got typ.Type, want typ.Type) {
	t.Helper()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("type = %v, want %v", got, want)
	}
}

func recordWithMetamethod(name string, mt typ.Type) *typ.Record {
	return typetable.NewRecord().
		Metatable(typetable.NewRecord().Field(name, mt).Build()).
		Build()
}
