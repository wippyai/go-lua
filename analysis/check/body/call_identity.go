package body

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type signatureIdentityResolver struct {
	bindings            *bind.Result
	graph               cfg.Graph
	facts               factflow.Facts
	imports             moduleidentity.Projection
	implicitStdlibNames map[string]struct{}
	callPoints          map[factflow.ExprRef]cfg.Point
	rootQuery           factquery.RootDeclarationQuery
	rootQueryReady      bool
}

func newSignatureIdentityResolver(bindings *bind.Result, graph cfg.Graph, facts factflow.Facts, modules moduleidentity.Projection, signatures signaturelookup.Source) *signatureIdentityResolver {
	return &signatureIdentityResolver{
		bindings:            bindings,
		graph:               graph,
		facts:               facts,
		imports:             modules,
		implicitStdlibNames: implicitStdlibSignatureNames(signatures),
	}
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
		if expr == 0 {
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
	_, ok := r.rootDeclarationQuery().DominatingOrdinaryRootWrite(point, root)
	return ok
}

func (r *signatureIdentityResolver) rootDeclarationQuery() factquery.RootDeclarationQuery {
	if r == nil {
		return factquery.RootDeclarationQuery{}
	}
	if !r.rootQueryReady {
		r.rootQuery = factquery.NewRootDeclarationQuery(r.facts, r.graph)
		r.rootQueryReady = true
	}
	return r.rootQuery
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
