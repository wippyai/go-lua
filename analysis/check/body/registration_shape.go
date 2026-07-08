package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// StaticStringAssignmentTarget describes an assignment to container[key] where
// key is known statically. It is a syntax-owned shape query; higher readmodels
// decide what the assignment means.
type StaticStringAssignmentTarget struct {
	Container pathdom.Path
	Key       string
	Span      SourceSpan
}

// LoweredAssignmentWrite is the factflow-owned shape of one assignment-like
// write. It carries only canonical target/source evidence and optional display
// metadata, so readmodels do not need semantic assignment sidecars.
type LoweredAssignmentWrite struct {
	Target  pathdom.Path
	Source  factflow.ValueSource
	Span    SourceSpan
	HasSpan bool
}

// LoweredAssignmentWrite returns the factflow assignment write at point.
func (r *Result) LoweredAssignmentWrite(point cfg.Point) (LoweredAssignmentWrite, bool) {
	if r == nil {
		return LoweredAssignmentWrite{}, false
	}
	if fact, ok := r.RootAssignment(point); ok && fact.Kind() == factflow.RootAssignmentOrdinaryRootWrite {
		return loweredAssignmentWriteFromRoot(fact), true
	}
	if fact, ok := r.PathAssignment(point); ok {
		return loweredAssignmentWriteFromPath(fact), true
	}
	if write, ok := r.DynamicIndexWrite(point); ok {
		target := write.TablePathRef()
		if key, keyOK := r.StaticStringValueSourceAtBoundary(point, write.KeySource()); keyOK {
			target = target.Append(segment.Segment{Kind: segment.SegmentField, Name: key})
		}
		return LoweredAssignmentWrite{Target: target, Source: write.Source()}, true
	}
	return LoweredAssignmentWrite{}, false
}

func loweredAssignmentWriteFromRoot(fact factflow.RootAssignment) LoweredAssignmentWrite {
	out := LoweredAssignmentWrite{
		Target: fact.TargetPathRef(),
		Source: fact.Source(),
	}
	if span, ok := fact.TargetSpan(); ok {
		out.Span = sourceSpanFromFactflow(span)
		out.HasSpan = true
	}
	return out
}

func loweredAssignmentWriteFromPath(fact factflow.PathAssignment) LoweredAssignmentWrite {
	out := LoweredAssignmentWrite{
		Target: fact.TargetPathRef(),
		Source: fact.Source(),
	}
	if span, ok := fact.TargetSpan(); ok {
		out.Span = sourceSpanFromFactflow(span)
		out.HasSpan = true
	}
	return out
}

func (w LoweredAssignmentWrite) StaticStringTarget() (StaticStringAssignmentTarget, bool) {
	if w.Target.Symbol == 0 || len(w.Target.Segments) == 0 {
		return StaticStringAssignmentTarget{}, false
	}
	seg, ok := w.Target.LastSegment()
	if !ok {
		return StaticStringAssignmentTarget{}, false
	}
	key, ok := staticStringSegmentKey(seg)
	if !ok {
		return StaticStringAssignmentTarget{}, false
	}
	return StaticStringAssignmentTarget{
		Container: w.Target.Parent(),
		Key:       key,
		Span:      w.Span,
	}, true
}

// AssignmentSourceMayBeFunctionBeforeBoundary reports whether a lowered source
// could be a function before point's node-local assignment effect runs. Unknown
// sources remain maybe-function so open registration mutations stay
// conservative.
func (r *Result) AssignmentSourceMayBeFunctionBeforeBoundary(point cfg.Point, source factflow.ValueSource) bool {
	if r == nil || r.registry == nil {
		return true
	}
	value, ok := r.SourceValueBeforeBoundary(point, source)
	if !ok {
		return true
	}
	if productKind := productRuntimeKind(r, value); productKind.Contains(runtimekind.Function) {
		return true
	}
	t, ok := r.ValueTypeWithPresence(value)
	return !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t)
}

// AssignmentSourceProvenFunctionAtBoundary reports whether a lowered source is
// proven to evaluate to a function at point.
func (r *Result) AssignmentSourceProvenFunctionAtBoundary(point cfg.Point, source factflow.ValueSource) bool {
	if r == nil || r.registry == nil {
		return false
	}
	value, ok := r.SourceValueAtBoundary(point, source)
	if !ok {
		value, ok = r.SourceValueBeforeBoundary(point, source)
	}
	if !ok {
		return false
	}
	if _, ok := r.FunctionValueTypeForValueAtBoundary(point, value); ok {
		return true
	}
	return productRuntimeKind(r, value).Contains(runtimekind.Function)
}

func productRuntimeKind(r *Result, value product.Value) runtimekind.Value {
	return product.Get(r.registry, value, runtimekind.Key)
}

// StaticStringValueSourceAtBoundary returns a concrete string literal carried by
// a lowered source.
func (r *Result) StaticStringValueSourceAtBoundary(point cfg.Point, source factflow.ValueSource) (string, bool) {
	if source.Kind == factflow.ValueSourceLiteral && source.LiteralKind == factflow.ValueSourceLiteralString {
		return source.String, true
	}
	if r == nil {
		return "", false
	}
	value, ok := r.SourceValueAtBoundary(point, source)
	if !ok {
		return "", false
	}
	seg, ok := staticStringSegmentFromValue(r.registry, r.typeValues, value)
	if !ok {
		return "", false
	}
	return seg.Name, seg.Name != ""
}

// CallArgumentInfo is the syntax-free readmodel view of one call argument.
type CallArgumentInfo struct {
	Index           int
	Path            pathdom.Path
	HasPath         bool
	StaticString    string
	HasStaticString bool
	ProvenFunction  bool
}

// RegistryKeyCallShape describes a call shape that uses a registry path and one
// argument as a key. Member calls use parameter 0 as the key; function calls use
// argument 1 after argument 0 supplies the registry.
type RegistryKeyCallShape struct {
	Registry pathdom.Path
	KeyIndex int
	Args     []CallArgumentInfo
	Span     SourceSpan
}

// RegistryKeyCallShape returns the generic registry/key call layout used by
// registration-style APIs.
func (r *Result) RegistryKeyCallShape(point cfg.Point, site factflow.CallSite, fact CallFact) (RegistryKeyCallShape, bool) {
	if fact.Call == nil {
		return RegistryKeyCallShape{}, false
	}
	args := r.callArgumentInfos(point, fact)
	if registry, ok := CallSiteMemberReceiverPath(site); ok && len(args) >= 2 {
		return RegistryKeyCallShape{Registry: registry, KeyIndex: 0, Args: args, Span: fact.CallSpan}, true
	}
	if len(args) >= 3 {
		first := args[0]
		if first.HasPath && first.Path.Symbol != 0 {
			return RegistryKeyCallShape{Registry: first.Path, KeyIndex: 1, Args: args, Span: fact.CallSpan}, true
		}
	}
	return RegistryKeyCallShape{}, false
}

// DispatchCallShape describes a call that dispatches over one or more argument
// paths using a registry-like first receiver/argument.
type DispatchCallShape struct {
	Registry pathdom.Path
	Args     []CallArgumentInfo
	Span     SourceSpan
}

// DispatchCallShape returns a registry path and the arguments dispatched
// through it. Member calls use all args; function calls use args after the
// registry argument.
func (r *Result) DispatchCallShape(point cfg.Point, site factflow.CallSite, fact CallFact) (DispatchCallShape, bool) {
	args := r.callArgumentInfos(point, fact)
	if registry, ok := CallSiteMemberReceiverPath(site); ok && len(args) > 0 {
		return DispatchCallShape{Registry: registry, Args: args, Span: fact.CallSpan}, true
	}
	if len(args) >= 2 {
		first := args[0]
		if first.HasPath && first.Path.Symbol != 0 {
			return DispatchCallShape{Registry: first.Path, Args: append([]CallArgumentInfo(nil), args[1:]...), Span: fact.CallSpan}, true
		}
	}
	return DispatchCallShape{}, false
}

// CallArgumentInfos returns syntax-free facts for every call argument.
func (r *Result) CallArgumentInfos(point cfg.Point, fact CallFact) []CallArgumentInfo {
	return r.callArgumentInfos(point, fact)
}

func (r *Result) callArgumentInfos(point cfg.Point, fact CallFact) []CallArgumentInfo {
	out := make([]CallArgumentInfo, 0, len(fact.Args))
	for i, arg := range fact.Args {
		info := CallArgumentInfo{Index: i}
		if r != nil {
			if p, ok := r.ExpressionPath(arg); ok {
				info.Path = p
				info.HasPath = true
			}
			if key, ok := r.StaticStringExprValueAtBoundary(point, arg); ok {
				info.StaticString = key
				info.HasStaticString = true
			}
			info.ProvenFunction = r.ExpressionProvenFunctionAtBoundary(point, arg)
		}
		out = append(out, info)
	}
	return out
}

// CallSiteMemberReceiverPath returns the receiver path for a member-call site.
func CallSiteMemberReceiverPath(site factflow.CallSite) (pathdom.Path, bool) {
	receiver, _, ok := site.CalleeMemberAccessPath()
	if !ok || receiver.IsEmpty() {
		return pathdom.Path{}, false
	}
	return receiver, true
}

func staticStringSegmentKey(seg segment.Segment) (string, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}
