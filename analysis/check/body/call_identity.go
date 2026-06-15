package body

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
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
		site, ok := facts.CallSite(point)
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
	if r == nil {
		return "", false
	}
	if name, ok := r.stableCalleeName(call.CalleeSymbol(), call.CalleePath()); ok {
		return name, true
	}
	return r.imports.SignatureName(ctx.Point, call.CalleePath())
}

func (r *signatureIdentityResolver) nameForSite(site factflow.CallSite) (string, bool) {
	point, ok := r.pointForSite(site)
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
	return r.nameForCall(ctx, callproducer.FromSite(site))
}

func (r *signatureIdentityResolver) pointForSite(site factflow.CallSite) (cfg.Point, bool) {
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
		default:
			return "", false
		}
	}
	return b.String(), true
}
