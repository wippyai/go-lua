package body

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type signatureIdentityResolver struct {
	bindings   *bind.Result
	graph      cfg.Graph
	imports    moduleidentity.Projection
	callPoints map[factflow.ExprRef]cfg.Point
}

func newSignatureIdentityResolver(bindings *bind.Result, graph cfg.Graph, modules moduleidentity.Projection) *signatureIdentityResolver {
	return &signatureIdentityResolver{
		bindings: bindings,
		graph:    graph,
		imports:  modules,
	}
}

func (r *signatureIdentityResolver) indexCallSites(facts factflow.Facts) {
	if r == nil || r.graph == nil {
		return
	}
	for _, point := range r.graph.RPO() {
		site, ok := facts.CallSiteView(point)
		if !ok {
			continue
		}
		expr, ok := site.Expr()
		if !ok {
			continue
		}
		if r.callPoints == nil {
			r.callPoints = make(map[factflow.ExprRef]cfg.Point)
		}
		r.callPoints[expr] = point
	}
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
	if name, ok := r.stableCalleeName(callee, calleePath); ok {
		return name, true
	}
	return r.imports.SignatureName(ctx.Point, calleePath)
}

func (r *signatureIdentityResolver) nameForSite(site factflow.CallSite) (string, bool) {
	return r.nameForIndexedCallSiteView(site.View())
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

func (r *signatureIdentityResolver) pointForSite(site factflow.CallSite) (cfg.Point, bool) {
	return r.pointForSiteView(site.View())
}

func (r *signatureIdentityResolver) pointForSiteView(site factflow.CallSiteView) (cfg.Point, bool) {
	if r == nil || len(r.callPoints) == 0 {
		return 0, false
	}
	expr, ok := site.Expr()
	if !ok {
		return 0, false
	}
	point, ok := r.callPoints[expr]
	return point, ok
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
	if len(calleePath.Segments) == 0 {
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
	return b.String(), true
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
