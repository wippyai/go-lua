package body

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type signatureIdentityResolver struct {
	bindings            *bind.Result
	graph               cfg.Graph
	imports             moduleidentity.Projection
	implicitStdlibNames map[string]struct{}
	shadowedGlobalNames map[string]struct{}
	rootWriteQuery      factquery.DominatingOrdinaryRootWriteQuery
}

func newSignatureIdentityResolver(bindings *bind.Result, graph cfg.Graph, body *wir.Body, modules moduleidentity.Projection, signatures signaturelookup.Source, globalTypes map[string]typ.Type, moduleExports importlookup.Source) *signatureIdentityResolver {
	rootWrites := signatureRootWritesFromWIR(bindings, body)
	rootWriteQuery := factquery.NewDominatingOrdinaryRootWriteQuery(graph, func(point cfg.Point, target symbol.ID) bool {
		roots := rootWrites[point]
		if len(roots) == 0 {
			return false
		}
		_, ok := roots[target]
		return ok
	})
	return &signatureIdentityResolver{
		bindings:            bindings,
		graph:               graph,
		imports:             modules,
		implicitStdlibNames: implicitStdlibSignatureNames(signatures),
		shadowedGlobalNames: shadowedGlobalSignatureNames(globalTypes, moduleExports),
		rootWriteQuery:      rootWriteQuery,
	}
}

func shadowedGlobalSignatureNames(globalTypes map[string]typ.Type, moduleExports importlookup.Source) map[string]struct{} {
	var out map[string]struct{}
	for name, globalType := range globalTypes {
		moduleType, ok := moduleExports.LookupExport(name)
		if name == "" || globalType == nil || !ok || moduleType == nil || typ.TypeEquals(globalType, moduleType) {
			continue
		}
		if out == nil {
			out = make(map[string]struct{})
		}
		out[name] = struct{}{}
	}
	return out
}

func implicitStdlibSignatureNames(signatures signaturelookup.Source) map[string]struct{} {
	if !signatures.IncludeStdlib {
		return nil
	}
	names := signaturelookup.StdlibSignatureNames()
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

func (r *signatureIdentityResolver) nameForCall(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool) {
	return r.nameForCallee(ctx, call.CalleeSymbol(), call.CalleePathRef())
}

func (r *signatureIdentityResolver) nameForCallSiteView(ctx transfer.NodeContext, site factflow.CallSiteView) (string, bool) {
	return r.nameForCallee(ctx, site.CalleeSymbol(), site.CalleePathRef())
}

func (r *signatureIdentityResolver) nameForCallee(ctx transfer.NodeContext, callee symbol.ID, calleePath path.Path) (string, bool) {
	if r == nil {
		return "", false
	}
	if r.rootReplacedAt(ctx.Point, callee, calleePath) {
		return "", false
	}
	if name, ok := r.stableCalleeName(callee, calleePath); ok {
		return name, true
	}
	if name, ok := r.imports.SignatureName(ctx.Point, calleePath); ok {
		return name, true
	}
	return r.implicitGlobalCalleeName(callee, calleePath)
}

func (r *signatureIdentityResolver) rootReplacedAt(point cfg.Point, callee symbol.ID, calleePath path.Path) bool {
	if r == nil {
		return false
	}
	root := callee
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	if root == 0 {
		return false
	}
	return r.dominatingOrdinaryRootWrite(point, root)
}

func (r *signatureIdentityResolver) dominatingOrdinaryRootWrite(point cfg.Point, target symbol.ID) bool {
	if r == nil {
		return false
	}
	_, ok := r.rootWriteQuery.DominatingOrdinaryRootWrite(point, target)
	return ok
}

func (r *signatureIdentityResolver) nameForIndexedCallSiteView(site factflow.CallSiteView) (string, bool) {
	point, ok := r.pointForSiteView(site)
	if !ok {
		return "", false
	}
	ctx := transfer.NodeContext{
		Graph: r.graph,
		Point: point,
	}
	if r.graph != nil {
		ctx.Node = r.graph.Node(point)
	}
	return r.nameForCallSiteView(ctx, site)
}

func (r *signatureIdentityResolver) nameForIndexedIteratorCallSiteView(site factflow.CallSiteView) (string, bool) {
	point, ok := r.pointForSiteView(site)
	if !ok {
		return "", false
	}
	ctx := transfer.NodeContext{
		Graph: r.graph,
		Point: point,
	}
	if r.graph != nil {
		ctx.Node = r.graph.Node(point)
	}
	if name, ok := r.nameForCallSiteView(ctx, site); ok {
		return name, true
	}
	return r.implicitBuiltinIteratorName(site.CalleeSymbol(), site.CalleePathRef())
}

func (r *signatureIdentityResolver) pointForSiteView(site factflow.CallSiteView) (cfg.Point, bool) {
	return site.Point()
}

func (r *signatureIdentityResolver) stableCalleeName(callee symbol.ID, calleePath path.Path) (string, bool) {
	if r == nil || r.bindings == nil {
		return "", false
	}
	root := callee
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	if root == 0 {
		return "", false
	}
	kind, ok := r.bindings.Kind(root)
	if !ok || kind != symbol.Global {
		return "", false
	}
	if r.bindings.IsImplicitGlobalSymbol(root) {
		return "", false
	}
	name := r.bindings.Name(root)
	if name == "" {
		return "", false
	}
	if _, shadowed := r.shadowedGlobalNames[name]; shadowed {
		return "", false
	}
	var fullName string
	var b strings.Builder
	b.WriteString(name)
	for _, seg := range calleePath.Segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name == "" {
				return "", false
			}
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case segment.SegmentIndexInt:
			b.WriteString(segment.FormatSegments([]segment.Segment{seg}))
		default:
			return "", false
		}
	}
	fullName = b.String()
	if len(calleePath.Segments) == 0 {
		fullName = name
	}
	return fullName, true
}

func signatureRootWritesFromWIR(bindings *bind.Result, body *wir.Body) map[cfg.Point]map[symbol.ID]struct{} {
	if body == nil {
		return nil
	}
	out := make(map[cfg.Point]map[symbol.ID]struct{})
	add := func(point cfg.Point, id symbol.ID) {
		if id == 0 {
			return
		}
		if out[point] == nil {
			out[point] = make(map[symbol.ID]struct{}, 1)
		}
		out[point][id] = struct{}{}
	}
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Assign == wir.AssignOrdinaryRootWrite {
			if inst.Dst.Kind == wir.OperandPath {
				p := body.Path(wir.PathRef(inst.Dst.Ref))
				if p.Symbol != 0 && len(p.Segments) == 0 {
					add(inst.Point, p.Symbol)
				}
			}
		}
		switch inst.Op {
		case wir.OpStaticMemberWrite:
			p := body.Path(wir.PathRef(inst.Dst.Ref))
			if global, ok := globalTableFieldRootSymbol(bindings, p); ok {
				add(inst.Point, global)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func globalTableFieldRootSymbol(bindings *bind.Result, p path.Path) (symbol.ID, bool) {
	if bindings == nil || p.Symbol == 0 || bindings.Name(p.Symbol) != "_G" {
		return 0, false
	}
	kind, ok := bindings.Kind(p.Symbol)
	if !ok || kind != symbol.Global {
		return 0, false
	}
	name, ok := p.DirectFieldName()
	if !ok {
		return 0, false
	}
	global, ok := bindings.GlobalSymbol(name)
	return global, ok && global != 0
}

func (r *signatureIdentityResolver) implicitBuiltinIteratorName(callee symbol.ID, calleePath path.Path) (string, bool) {
	if r == nil || r.bindings == nil || len(calleePath.Segments) != 0 {
		return "", false
	}
	root := callee
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	if root == 0 || !r.bindings.IsImplicitGlobalSymbol(root) {
		return "", false
	}
	name := r.bindings.Name(root)
	switch name {
	case "pairs", "ipairs":
		return name, true
	default:
		return "", false
	}
}

func (r *signatureIdentityResolver) implicitGlobalCalleeName(callee symbol.ID, calleePath path.Path) (string, bool) {
	if r == nil || r.bindings == nil {
		return "", false
	}
	root := callee
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	if root == 0 || !r.bindings.IsImplicitGlobalSymbol(root) {
		return "", false
	}
	name := r.bindings.Name(root)
	if name == "" {
		return "", false
	}
	fullName := name
	if len(calleePath.Segments) == 0 {
		if _, ok := r.implicitStdlibNames[fullName]; !ok {
			return "", false
		}
		return name, true
	}
	var b strings.Builder
	b.WriteString(name)
	for _, seg := range calleePath.Segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name == "" {
				return "", false
			}
			b.WriteByte('.')
			b.WriteString(seg.Name)
		case segment.SegmentIndexInt:
			b.WriteString(segment.FormatSegments([]segment.Segment{seg}))
		default:
			return "", false
		}
	}
	fullName = b.String()
	if _, ok := r.implicitStdlibNames[fullName]; !ok {
		return "", false
	}
	return fullName, true
}
