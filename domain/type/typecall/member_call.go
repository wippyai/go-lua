package typecall

import (
	"github.com/wippyai/go-lua/domain/type/access"
	"github.com/wippyai/go-lua/domain/type/ambient"
	graph "github.com/wippyai/go-lua/domain/type/internal/typegraph"
	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/stringlib"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/subtype"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// MemberCallStatus describes whether a receiver member can be called.
type MemberCallStatus uint8

const (
	MemberCallOK MemberCallStatus = iota
	MemberCallMissing
	MemberCallNotCallable
)

// MemberCall resolves a receiver member for call syntax. Unlike Field, this is
// strict for calls: every possible non-nil receiver alternative must provide a
// callable member, and optional or missing member values are not callable.
func MemberCall(t typ.Type, name string) (typ.Type, MemberCallStatus) {
	if name == "" {
		return nil, MemberCallMissing
	}
	res := memberCallDepth(t, name, 0)
	if res.status == MemberCallOK && res.t == nil {
		res.t = typ.Unknown
	}
	return res.t, res.status
}

// IndexedMemberCall resolves a receiver member reached through exact bracket
// syntax for call validation. The key is a static literal type such as
// LiteralInt(1) or LiteralString("run").
func IndexedMemberCall(t typ.Type, key typ.Type) (typ.Type, MemberCallStatus) {
	if key == nil {
		return nil, MemberCallMissing
	}
	res := indexedMemberCallDepth(t, key, 0)
	if res.status == MemberCallOK && res.t == nil {
		res.t = typ.Unknown
	}
	return res.t, res.status
}

// MemberCallable resolves a receiver member for call syntax, extracts a
// concrete callable witness, and binds receiver-relative Self/SelfRef
// references to the concrete receiver type.
func MemberCallable(receiver typ.Type, name string) (*typ.Function, MemberCallStatus, bool) {
	memberType, status := MemberCall(receiver, name)
	if status != MemberCallOK {
		return nil, status, false
	}
	callable, ok := Callable(memberType)
	if !ok || callable == nil {
		return nil, status, false
	}
	return bindMemberCallableReceiver(callable, receiver), status, true
}

// CallableConsumesReceiver reports whether a callable member contract consumes
// the implicit receiver in its first formal parameter when called with colon
// syntax.
func CallableConsumesReceiver(fn *typ.Function, receiver typ.Type) bool {
	if fn == nil || len(fn.Params) == 0 {
		return false
	}
	return ParamConsumesReceiver(fn.Params[0].Receiver, fn.Params[0].Type, receiver)
}

// ParamConsumesReceiver applies the receiver-consumption rule for a single
// formal parameter. An explicit receiver formal is authoritative; otherwise a concrete
// receiver subtype relation can prove that formal 0 is the receiver slot.
func ParamConsumesReceiver(receiverParam bool, param typ.Type, receiver typ.Type) bool {
	if receiverParam {
		return true
	}
	param = unwrap.Annotated(param)
	if typ.TypeEquals(param, typ.Self) {
		return true
	}
	if ref, ok := param.(*typ.Ref); ok && ref.Module == "" && ref.Name == "self" {
		return true
	}
	if param == nil || receiver == nil || typ.IsAny(param) || typ.IsUnknown(param) {
		return false
	}
	if typ.IsBuiltinTableTopMarker(param) {
		return false
	}
	return subtype.IsSubtype(receiver, param)
}

// IndexedMemberCallable resolves an exact bracket member for call syntax,
// extracts a concrete callable witness, and binds receiver-relative Self/SelfRef
// references to the concrete receiver type.
func IndexedMemberCallable(receiver typ.Type, key typ.Type) (*typ.Function, MemberCallStatus, bool) {
	memberType, status := IndexedMemberCall(receiver, key)
	if status != MemberCallOK {
		return nil, status, false
	}
	callable, ok := Callable(memberType)
	if !ok || callable == nil {
		return nil, status, false
	}
	return bindMemberCallableReceiver(callable, receiver), status, true
}

func bindMemberCallableReceiver(callable *typ.Function, receiver typ.Type) *typ.Function {
	if callable == nil {
		return nil
	}
	if substituted, ok := subst.Self(callable, receiver).(*typ.Function); ok {
		callable = substituted
	}
	if substituted, ok := subst.SelfRef(callable, receiver).(*typ.Function); ok {
		callable = substituted
	}
	return callable
}

type memberCallResult struct {
	t      typ.Type
	status MemberCallStatus
}

func memberCallDepth(t typ.Type, name string, depth int) memberCallResult {
	return memberCallSeen(t, name, depth, &graph.Path{})
}

func memberCallSeen(t typ.Type, name string, depth int, active *graph.Path) memberCallResult {
	if stopDepth(t, depth) {
		return memberCallResult{status: MemberCallMissing}
	}
	if !active.Enter(t, 0) {
		return memberCallResult{status: MemberCallMissing}
	}
	defer active.Leave(t, 0)
	if method, ok := ambientChannelMethod(t, name, depth+1); ok {
		return memberCallResult{t: method, status: MemberCallOK}
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return memberCallUnionSeen(v, name, depth+1, active)
	case *typ.Intersection:
		return memberCallIntersectionSeen(v, name, depth+1, active)
	case *typ.Optional:
		return memberCallSeen(v.Inner, name, depth+1, active)
	case *typ.Alias:
		return memberCallSeen(v.UnaliasedTarget(), name, depth+1, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return memberCallResult{status: MemberCallMissing}
		}
		return memberCallSeen(expanded, name, depth+1, active)
	default:
		return memberCallSingle(t, name, depth+1)
	}
}

func memberCallUnionSeen(u *typ.Union, name string, depth int, active *graph.Path) memberCallResult {
	if u == nil || len(u.Members) == 0 {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeUnion(u.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return memberCallSeen(member, name, depth, active)
	})
}

func memberCallIntersectionSeen(in *typ.Intersection, name string, depth int, active *graph.Path) memberCallResult {
	if in == nil {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeIntersection(in.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return memberCallSeen(member, name, depth, active)
	})
}

func memberCallSingle(t typ.Type, name string, depth int) memberCallResult {
	if method, ok := stringMethod(name, t); ok {
		return memberCallResult{t: method, status: MemberCallOK}
	}
	if method, ok := metaMethod(name, t); ok {
		return memberCallResult{t: method, status: MemberCallOK}
	}
	memberType, ok := access.Field(t, name)
	return memberCallableResult(memberType, ok, depth+1)
}

func indexedMemberCallDepth(t typ.Type, key typ.Type, depth int) memberCallResult {
	return indexedMemberCallSeen(t, key, depth, &graph.Path{})
}

func indexedMemberCallSeen(t typ.Type, key typ.Type, depth int, active *graph.Path) memberCallResult {
	if stopDepth(t, depth) {
		return memberCallResult{status: MemberCallMissing}
	}
	if !active.Enter(t, 0) {
		return memberCallResult{status: MemberCallMissing}
	}
	defer active.Leave(t, 0)
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return indexedMemberCallUnionSeen(v, key, depth+1, active)
	case *typ.Intersection:
		return indexedMemberCallIntersectionSeen(v, key, depth+1, active)
	case *typ.Optional:
		return indexedMemberCallSeen(v.Inner, key, depth+1, active)
	case *typ.Alias:
		return indexedMemberCallSeen(v.UnaliasedTarget(), key, depth+1, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return memberCallResult{status: MemberCallMissing}
		}
		return indexedMemberCallSeen(expanded, key, depth+1, active)
	default:
		return indexedMemberCallSingle(t, key, depth+1)
	}
}

func indexedMemberCallUnionSeen(u *typ.Union, key typ.Type, depth int, active *graph.Path) memberCallResult {
	if u == nil || len(u.Members) == 0 {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeUnion(u.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return indexedMemberCallSeen(member, key, depth, active)
	})
}

func indexedMemberCallIntersectionSeen(in *typ.Intersection, key typ.Type, depth int, active *graph.Path) memberCallResult {
	if in == nil {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeIntersection(in.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return indexedMemberCallSeen(member, key, depth, active)
	})
}

func indexedMemberCallSingle(t typ.Type, key typ.Type, depth int) memberCallResult {
	memberType, ok := access.Index(t, key)
	return memberCallableResult(memberType, ok, depth+1)
}

func memberCallableResult(memberType typ.Type, ok bool, depth int) memberCallResult {
	if !ok {
		return memberCallResult{status: MemberCallMissing}
	}
	if containsNil(memberType, depth) {
		return memberCallResult{t: memberType, status: MemberCallNotCallable}
	}
	if !callableValue(memberType, depth) {
		return memberCallResult{t: memberType, status: MemberCallNotCallable}
	}
	return memberCallResult{t: memberType, status: MemberCallOK}
}

func stringMethod(name string, receiver typ.Type) (typ.Type, bool) {
	if receiver == nil || !subtype.IsSubtype(receiver, typ.String) {
		return nil, false
	}
	fn, ok := stringlib.Method(name)
	if !ok {
		return nil, false
	}
	return fn, true
}

func metaMethod(name string, receiver typ.Type) (typ.Type, bool) {
	if name != "is" {
		return nil, false
	}
	meta, ok := unwrap.Alias(receiver).(*typ.Meta)
	if !ok || meta == nil || meta.Of == nil {
		return nil, false
	}
	return typ.Func().
		Param("self", receiver).
		Param("value", typ.Any).
		Returns(typeexpr.Optional(meta.Of), typ.Unknown).
		Build(), true
}

func ambientChannelMethod(receiver typ.Type, name string, depth int) (typ.Type, bool) {
	channel, payload, ok := channelPayloadType(receiver, depth+1)
	if !ok {
		return nil, false
	}
	switch name {
	case "receive":
		return typ.Func().
			Param("self", channel).
			Returns(payload, typ.Boolean).
			Build(), true
	case "case_receive":
		return typ.Func().
			Param("self", channel).
			Returns(typ.Unknown).
			Build(), true
	case "send":
		return typ.Func().
			Param("self", channel).
			Param("payload", payload).
			Returns(typ.Boolean).
			Build(), true
	case "case_send":
		return typ.Func().
			Param("self", channel).
			Param("payload", payload).
			Returns(typ.Unknown).
			Build(), true
	case "close":
		return typ.Func().
			Param("self", channel).
			Build(), true
	default:
		return nil, false
	}
}

// AmbientChannelPayloadType reports the payload type for receiver values that
// use the runtime channel ABI accepted by ambient channel member calls.
func AmbientChannelPayloadType(receiver typ.Type) (typ.Type, bool) {
	_, payload, ok := channelPayloadType(receiver, 0)
	return payload, ok
}

func channelPayloadType(t typ.Type, depth int) (typ.Type, typ.Type, bool) {
	return channelPayloadTypeSeen(t, depth, &graph.Path{})
}

func channelPayloadTypeSeen(t typ.Type, depth int, active *graph.Path) (typ.Type, typ.Type, bool) {
	if stopDepth(t, depth) {
		return nil, nil, false
	}
	if !active.Enter(t, 0) {
		return nil, nil, false
	}
	defer active.Leave(t, 0)
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		payloads := make([]typ.Type, 0, len(v.Members))
		for _, member := range v.Members {
			if isNilType(member) || typ.IsNever(member) {
				continue
			}
			_, payload, ok := channelPayloadTypeSeen(member, depth+1, active)
			if !ok {
				return nil, nil, false
			}
			payloads = append(payloads, payload)
		}
		if len(payloads) == 0 {
			return nil, nil, false
		}
		return v, normalize.UnionForEvidence(payloads...), true
	case *typ.Optional:
		return channelPayloadTypeSeen(v.Inner, depth+1, active)
	case *typ.Alias:
		return channelPayloadTypeSeen(v.UnaliasedTarget(), depth+1, active)
	case *typ.Instantiated:
		if payload, ok := ambient.ChannelPayloadType(v); ok {
			return v, payload, true
		}
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, nil, false
		}
		return channelPayloadTypeSeen(expanded, depth+1, active)
	default:
		return nil, nil, false
	}
}

func containsNil(t typ.Type, depth int) bool {
	return containsNilSeen(t, depth, &graph.Path{})
}

func containsNilSeen(t typ.Type, depth int, active *graph.Path) bool {
	if stopDepth(t, depth) {
		return true
	}
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if containsNilSeen(member, depth+1, active) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return containsNilSeen(v.UnaliasedTarget(), depth+1, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && containsNilSeen(expanded, depth+1, active)
	default:
		return isNilType(t)
	}
}

func callableValue(t typ.Type, depth int) bool {
	return callableValueSeen(t, depth, &graph.Path{})
}

func callableValueSeen(t typ.Type, depth int, active *graph.Path) bool {
	if stopDepth(t, depth) {
		return false
	}
	if !active.Enter(t, 0) {
		return false
	}
	defer active.Leave(t, 0)
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return true
	case *typ.Record:
		call, ok := metamethodInRecord(v, "__call", depth+1)
		return ok && !containsNil(call, depth+1) && callableValueSeen(call, depth+1, active)
	case *typ.Union:
		checked := false
		for _, member := range v.Members {
			if typ.IsNever(member) {
				continue
			}
			if isNilType(member) || !callableValueSeen(member, depth+1, active) {
				return false
			}
			checked = true
		}
		return checked
	case *typ.Intersection:
		for _, member := range v.Members {
			if callableValueSeen(member, depth+1, active) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return callableValueSeen(v.UnaliasedTarget(), depth+1, active)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && callableValueSeen(expanded, depth+1, active)
	default:
		return false
	}
}
