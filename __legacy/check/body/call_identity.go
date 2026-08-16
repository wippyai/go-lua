package body

import (
	"strings"

	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/moduleidentity"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type signatureIdentityResolver struct {
	bindings                     *bind.Result
	graph                        cfg.Graph
	imports                      moduleidentity.Projection
	implicitStdlibNames          map[string]struct{}
	shadowedGlobalNames          map[string]struct{}
	intrinsicShadowedGlobalNames map[string]struct{}
	rootWriteQuery               factquery.DominatingOrdinaryRootWriteQuery
}

// intrinsicForCallSiteView assigns semantic intrinsic identity only to the
// canonical Lua global installed by the prepared base environment. A matching
// spelling on a local, typed environment override, import, member, or replaced
// root is deliberately not sufficient.
func (r *signatureIdentityResolver) intrinsicForCallSiteView(ctx transfer.NodeContext, site factflow.CallSiteView) (signature.Intrinsic, bool) {
	if r == nil || r.bindings == nil || site.MethodName() != "" {
		return signature.IntrinsicNone, false
	}
	calleePath := site.CalleePathRef()
	if len(calleePath.Segments) != 0 || r.rootReplacedAt(ctx.Point, site.CalleeSymbol(), calleePath) {
		return signature.IntrinsicNone, false
	}
	root := site.CalleeSymbol()
	if calleePath.Symbol != 0 {
		root = calleePath.Symbol
	}
	kind, known := r.bindings.Kind(root)
	if root == 0 || !known || kind != symbol.Global || r.bindings.Name(root) != "type" {
		return signature.IntrinsicNone, false
	}
	if !r.luaTypeIntrinsicEnvironmentSealed(root) {
		return signature.IntrinsicNone, false
	}
	return signature.IntrinsicLuaType, true
}

// canonicalStdlibGlobalCall reports exact authority for one root global from
// the prepared base environment. This is the shared binding/environment seam
// for semantic operations which must not be rediscovered from source text by
// downstream consumers.
func (r *signatureIdentityResolver) canonicalStdlibGlobalCall(ctx transfer.NodeContext, site factflow.CallSiteView, name string) bool {
	if r == nil || r.bindings == nil || name == "" || site.MethodName() != "" {
		return false
	}
	p := site.CalleePathRef()
	if len(p.Segments) != 0 || r.rootReplacedAt(ctx.Point, site.CalleeSymbol(), p) {
		return false
	}
	root := site.CalleeSymbol()
	if p.Symbol != 0 {
		root = p.Symbol
	}
	kind, known := r.bindings.Kind(root)
	if root == 0 || !known || kind != symbol.Global || r.bindings.Name(root) != name || r.bindings.HasWrite(root) {
		return false
	}
	globalTable, hasGlobalTable := r.bindings.GlobalSymbol("_G")
	if hasGlobalTable && r.bindings.HasRead(globalTable) {
		return false
	}
	if _, shadowed := r.intrinsicShadowedGlobalNames[name]; shadowed {
		return false
	}
	_, present := r.implicitStdlibNames[name]
	return present
}

// luaTypeIntrinsicEnvironmentSealed is the unit-wide half of intrinsic
// authority shared by call sites and normalized type predicates. The latter
// have no call site after WIR normalization, so this proof is transported to
// transferfacts explicitly rather than reconstructed from the spelling
// "type" or from CheckTypeEqual alone.
func (r *signatureIdentityResolver) luaTypeIntrinsicEnvironmentSealed(root symbol.ID) bool {
	if r == nil || r.bindings == nil || root == 0 {
		return false
	}
	globalTable, hasGlobalTable := r.bindings.GlobalSymbol("_G")
	if r.bindings.HasWrite(root) || hasGlobalTable && r.bindings.HasRead(globalTable) {
		return false
	}
	if _, shadowed := r.intrinsicShadowedGlobalNames["type"]; shadowed {
		return false
	}
	_, present := r.implicitStdlibNames["type"]
	return present
}

func (r *signatureIdentityResolver) luaTypePredicateChecksSealed() bool {
	if r == nil || r.bindings == nil {
		return false
	}
	root, ok := r.bindings.GlobalSymbol("type")
	if !ok {
		return false
	}
	kind, known := r.bindings.Kind(root)
	return known && kind == symbol.Global && r.bindings.Name(root) == "type" && r.luaTypeIntrinsicEnvironmentSealed(root)
}

// luaTypePredicateChecksSealedForLowering computes the unit-owned authority
// before CFG/WIR construction. It intentionally uses only the same lexical and
// environment facts consumed by luaTypeIntrinsicEnvironmentSealed: lowering
// must know whether it may erase the runtime call, while point-local call
// identity remains the later resolver's responsibility.
func luaTypePredicateChecksSealedForLowering(bindings *bind.Result, signatures signaturelookup.Source, globalTypes map[string]typ.Type) bool {
	if bindings == nil {
		return false
	}
	root, ok := bindings.GlobalSymbol("type")
	if !ok || root == 0 {
		return false
	}
	kind, known := bindings.Kind(root)
	if !known || kind != symbol.Global || bindings.Name(root) != "type" || bindings.HasWrite(root) {
		return false
	}
	if _, overridden := globalTypes["type"]; overridden {
		return false
	}
	globalTable, hasGlobalTable := bindings.GlobalSymbol("_G")
	return !hasGlobalTable || !bindings.HasRead(globalTable)
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
	intrinsicShadowed := make(map[string]struct{})
	if _, configured := globalTypes["type"]; configured {
		intrinsicShadowed["type"] = struct{}{}
	}
	if _, configured := globalTypes["setmetatable"]; configured {
		intrinsicShadowed["setmetatable"] = struct{}{}
	}
	return &signatureIdentityResolver{
		bindings:                     bindings,
		graph:                        graph,
		imports:                      modules,
		implicitStdlibNames:          implicitStdlibSignatureNames(signatures),
		shadowedGlobalNames:          shadowedGlobalSignatureNames(globalTypes, moduleExports),
		intrinsicShadowedGlobalNames: intrinsicShadowed,
		rootWriteQuery:               rootWriteQuery,
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
	// Lua's global type is a language intrinsic, not an opt-in library
	// signature.  Keep its binding/environment seal available even when callers
	// deliberately omit the rest of the stdlib signature surface.
	names := []string{"type"}
	if signatures.IncludeStdlib {
		names = signaturelookup.StdlibSignatureNames()
	}
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
