// Package channelruntime owns recognition of Lua channel runtime constructs.
package channelruntime

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeaccess"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

const (
	moduleName        = "channel"
	selectName        = "select"
	caseReceiveMethod = "case_receive"
)

// IsSelectCall reports whether call is the ambient Lua channel.select runtime
// call. Arbitrary values with a trailing .select field are deliberately not
// recognized.
func IsSelectCall(call *ast.FuncCallExpr, bindings *bind.Result) bool {
	if call == nil || bindings == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return false
	}
	callee, ok := pathexpr.Resolve(call.Func, bindings)
	if !ok {
		return false
	}
	field, ok := callee.DirectFieldName()
	if !ok || field != selectName || callee.Symbol == 0 {
		return false
	}
	return bindings.Name(callee.Symbol) == moduleName && symbolKind(bindings, callee.Symbol) == symbol.Global
}

// IsReceiveCaseCall reports whether call is a Channel<T>:case_receive() runtime
// case. The receiver must resolve to a path whose static annotation is Channel.
func IsReceiveCaseCall(call *ast.FuncCallExpr, bindings *bind.Result) bool {
	if call == nil || call.Receiver == nil || call.Method != caseReceiveMethod || len(call.Args) != 0 || len(call.TypeArgs) != 0 {
		return false
	}
	channelPath, ok := pathexpr.Resolve(call.Receiver, bindings)
	if !ok || channelPath.IsEmpty() {
		return false
	}
	channelType, ok := PathType(bindings, channelPath)
	return ok && IsChannelType(channelType)
}

// PathType resolves the annotated type for a path, following record fields.
func PathType(bindings *bind.Result, p pathdom.Path) (typ.Type, bool) {
	if bindings == nil || p.Symbol == 0 {
		return nil, false
	}
	expr, ok := bindings.SymbolTypeAnnotation(p.Symbol)
	if !ok {
		return nil, false
	}
	current, ok := typeresolve.New(bindings).Type(expr)
	if !ok {
		return nil, false
	}
	for _, seg := range p.Segments {
		next, ok := segmentType(current, seg)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

// IsChannelType reports whether t is the ambient Channel<T> instantiation.
func IsChannelType(t typ.Type) bool {
	inst, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Instantiated)
	return ok && inst.Generic != nil && inst.Generic.Name == ambient.Channel && len(inst.TypeArgs) == 1 && inst.TypeArgs[0] != nil
}

// ChannelPayloadType returns the T carried by an ambient Channel<T>.
func ChannelPayloadType(t typ.Type) (typ.Type, bool) {
	inst, ok := unwrap.Alias(unwrap.Annotations(t)).(*typ.Instantiated)
	if !ok || inst.Generic == nil || inst.Generic.Name != ambient.Channel || len(inst.TypeArgs) != 1 || inst.TypeArgs[0] == nil {
		return nil, false
	}
	return inst.TypeArgs[0], true
}

func segmentType(container typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		return typeaccess.Field(container, seg.Name)
	case segment.SegmentIndexString:
		return typeaccess.RuntimeIndex(container, typ.LiteralString(seg.Name))
	case segment.SegmentIndexInt:
		return typeaccess.RuntimeIndex(container, typ.LiteralInt(int64(seg.Index)))
	default:
		return nil, false
	}
}

func symbolKind(bindings *bind.Result, id symbol.ID) symbol.Kind {
	kind, ok := bindings.Kind(id)
	if !ok {
		return symbol.Unknown
	}
	return kind
}
