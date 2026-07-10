package typecall

import (
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/stringlib"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	return ParamConsumesReceiver(fn.Params[0].Name, fn.Params[0].Type, receiver)
}

// ParamConsumesReceiver applies the receiver-consumption rule for a single
// formal parameter. A named `self` formal is authoritative; otherwise a concrete
// receiver subtype relation can prove that formal 0 is the receiver slot.
func ParamConsumesReceiver(name string, param typ.Type, receiver typ.Type) bool {
	if name == "self" {
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
	if typetable.IsBuiltinTopMarker(param) {
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
	if stopDepth(t, depth) {
		return memberCallResult{status: MemberCallMissing}
	}
	if method, ok := ambientChannelMethod(t, name, depth+1); ok {
		return memberCallResult{t: method, status: MemberCallOK}
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return memberCallUnion(v, name, depth+1)
	case *typ.Intersection:
		return memberCallIntersection(v, name, depth+1)
	case *typ.Optional:
		return memberCallDepth(v.Inner, name, depth+1)
	case *typ.Alias:
		return memberCallDepth(v.UnaliasedTarget(), name, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return memberCallResult{status: MemberCallMissing}
		}
		return memberCallDepth(expanded, name, depth+1)
	default:
		return memberCallSingle(t, name, depth+1)
	}
}

func memberCallUnion(u *typ.Union, name string, depth int) memberCallResult {
	if u == nil || len(u.Members) == 0 {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeUnion(u.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return memberCallDepth(member, name, depth)
	})
}

func memberCallIntersection(in *typ.Intersection, name string, depth int) memberCallResult {
	if in == nil {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeIntersection(in.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return memberCallDepth(member, name, depth)
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
	if stopDepth(t, depth) {
		return memberCallResult{status: MemberCallMissing}
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Union:
		return indexedMemberCallUnion(v, key, depth+1)
	case *typ.Intersection:
		return indexedMemberCallIntersection(v, key, depth+1)
	case *typ.Optional:
		return indexedMemberCallDepth(v.Inner, key, depth+1)
	case *typ.Alias:
		return indexedMemberCallDepth(v.UnaliasedTarget(), key, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return memberCallResult{status: MemberCallMissing}
		}
		return indexedMemberCallDepth(expanded, key, depth+1)
	default:
		return indexedMemberCallSingle(t, key, depth+1)
	}
}

func indexedMemberCallUnion(u *typ.Union, key typ.Type, depth int) memberCallResult {
	if u == nil || len(u.Members) == 0 {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeUnion(u.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return indexedMemberCallDepth(member, key, depth)
	})
}

func indexedMemberCallIntersection(in *typ.Intersection, key typ.Type, depth int) memberCallResult {
	if in == nil {
		return memberCallResult{status: MemberCallMissing}
	}
	return memberCallDistributeIntersection(in.Members, depth, func(member typ.Type, depth int) memberCallResult {
		return indexedMemberCallDepth(member, key, depth)
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
	if stopDepth(t, depth) {
		return nil, nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return channelPayloadType(v.Inner, depth+1)
	case *typ.Alias:
		return channelPayloadType(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		if payload, ok := ambient.ChannelPayloadType(v); ok {
			return v, payload, true
		}
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, nil, false
		}
		return channelPayloadType(expanded, depth+1)
	default:
		return nil, nil, false
	}
}

func containsNil(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Optional:
		return true
	case *typ.Union:
		for _, member := range v.Members {
			if containsNil(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return containsNil(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && containsNil(expanded, depth+1)
	default:
		return isNilType(t)
	}
}

func callableValue(t typ.Type, depth int) bool {
	if stopDepth(t, depth) {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Function:
		return true
	case *typ.Record:
		call, ok := metamethodInRecord(v, "__call", depth+1)
		return ok && !containsNil(call, depth+1) && callableValue(call, depth+1)
	case *typ.Union:
		checked := false
		for _, member := range v.Members {
			if typ.IsNever(member) {
				continue
			}
			if isNilType(member) || !callableValue(member, depth+1) {
				return false
			}
			checked = true
		}
		return checked
	case *typ.Intersection:
		for _, member := range v.Members {
			if callableValue(member, depth+1) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return callableValue(v.UnaliasedTarget(), depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		return expanded != nil && expanded != t && callableValue(expanded, depth+1)
	default:
		return false
	}
}
