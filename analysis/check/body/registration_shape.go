package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// StaticStringAssignmentTarget describes an assignment to container[key] where
// key is known statically. It is a syntax-owned shape query; higher readmodels
// decide what the assignment means.
type StaticStringAssignmentTarget struct {
	Container pathdom.Path
	Key       string
	Span      SourceSpan
}

// StaticStringAssignmentTarget returns the container and key for an assignment
// target known to write one static string member.
func (r *Result) StaticStringAssignmentTarget(point cfg.Point, fact OrdinaryAssignmentFact) (StaticStringAssignmentTarget, bool) {
	if fact.HasPath && fact.Path.Symbol != 0 {
		if seg, ok := fact.Path.LastSegment(); ok {
			if key, keyOK := staticStringSegmentKey(seg); keyOK {
				return StaticStringAssignmentTarget{
					Container: fact.Path.Parent(),
					Key:       key,
					Span:      sourceSpanFromAST(ast.SpanOf(fact.Target)),
				}, true
			}
		}
	}
	if r == nil || fact.Target == nil {
		return StaticStringAssignmentTarget{}, false
	}
	attr, ok := fact.Target.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return StaticStringAssignmentTarget{}, false
	}
	container, ok := r.ExpressionPath(attr.Object)
	if !ok || container.Symbol == 0 {
		return StaticStringAssignmentTarget{}, false
	}
	key, ok := r.StaticStringExprValueAtBoundary(point, attr.Key)
	if !ok {
		return StaticStringAssignmentTarget{}, false
	}
	return StaticStringAssignmentTarget{
		Container: container,
		Key:       key,
		Span:      sourceSpanFromAST(ast.SpanOf(fact.Target)),
	}, true
}

// AssignmentValueMayBeFunctionBeforeBoundary reports whether the assignment
// value could be a function before the boundary state at point.
func (r *Result) AssignmentValueMayBeFunctionBeforeBoundary(point cfg.Point, fact OrdinaryAssignmentFact) bool {
	return r != nil && r.ExpressionMayBeFunctionBeforeBoundary(point, fact.Value)
}

// AssignmentValueProvenFunctionAtBoundary reports whether the assignment value
// is proven to be a function at the boundary state at point.
func (r *Result) AssignmentValueProvenFunctionAtBoundary(point cfg.Point, fact OrdinaryAssignmentFact) bool {
	return r != nil && r.ExpressionProvenFunctionAtBoundary(point, fact.Value)
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
