// Package channelruntime recognizes the ambient Lua channel runtime ABI, not
// arbitrary user-authored APIs.
package channelruntime

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
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
	if !IsReceiveCaseSyntax(call, bindings) {
		return false
	}
	channelPath, ok := pathexpr.Resolve(call.Receiver, bindings)
	if !ok || channelPath.IsEmpty() {
		return false
	}
	channelType, ok := pathType(bindings, channelPath)
	return ok && isChannelType(channelType)
}

// IsReceiveCaseSyntax reports whether call has the runtime receive-case shape.
// It does not prove the receiver is Channel<T>; callers that have a richer
// module-aware type resolver must perform that proof before publishing payload
// evidence.
func IsReceiveCaseSyntax(call *ast.FuncCallExpr, bindings *bind.Result) bool {
	if call == nil || bindings == nil || call.Receiver == nil || call.Method != caseReceiveMethod || len(call.Args) != 0 || len(call.TypeArgs) != 0 {
		return false
	}
	channelPath, ok := pathexpr.Resolve(call.Receiver, bindings)
	return ok && !channelPath.IsEmpty()
}

// pathType resolves the annotated type for a path, following field and index
// projections.
func pathType(bindings *bind.Result, p pathdom.Path) (typ.Type, bool) {
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
	return luatypeprojection.ApplySegments(current, p.Segments)
}

// isChannelType reports whether t is the ambient Channel<T> instantiation.
func isChannelType(t typ.Type) bool {
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

func symbolKind(bindings *bind.Result, id symbol.ID) symbol.Kind {
	kind, ok := bindings.Kind(id)
	if !ok {
		return symbol.Unknown
	}
	return kind
}
