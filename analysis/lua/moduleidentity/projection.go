package moduleidentity

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

type moduleRoot struct {
	modulePath string
	point      cfg.Point
	inherited  bool
}

type moduleAlias struct {
	target     path.Path
	modulePath string
	point      cfg.Point
	inherited  bool
}

type signatureAlias struct {
	target    path.Path
	name      string
	point     cfg.Point
	inherited bool
}

type pathWrite struct {
	target  path.Path
	point   cfg.Point
	dynamic bool
}

// Projection is the Lua-layer view of require/module identity in a body.
type Projection struct {
	bindings    *bind.Result
	roots       map[symbol.ID]moduleRoot
	aliases     []moduleAlias
	signatures  []signatureAlias
	aliasNames  map[string]string
	reassigned  map[symbol.ID][]cfg.Point
	pathWrites  []pathWrite
	pointOrders map[cfg.Point]int
}

// New builds a require/module identity projection for a function or chunk body.
func New(bindings *bind.Result, graph cfg.Graph, sem *semantics.Result) Projection {
	out := Projection{bindings: bindings}
	if bindings == nil || sem == nil {
		return out
	}
	if graph != nil {
		points := graph.RPO()
		out.pointOrders = make(map[cfg.Point]int, len(points))
		for i, point := range points {
			out.pointOrders[point] = i
			if fact, ok := sem.LocalAssignment(point); ok {
				if modulePath, ok := ExactRequireCall(bindings, fact.Expr); ok {
					out.addAliasName(fact.Name, modulePath)
					if fact.HasSymbol && fact.Symbol != 0 {
						out.addRoot(fact.Symbol, moduleRoot{modulePath: modulePath, point: point})
					}
				}
			}
			if fact, ok := sem.OrdinaryAssignment(point); ok {
				if fact.HasSymbol && fact.Symbol != 0 && (!fact.HasPath || len(fact.Path.Segments) == 0) {
					if out.reassigned == nil {
						out.reassigned = make(map[symbol.ID][]cfg.Point)
					}
					out.reassigned[fact.Symbol] = append(out.reassigned[fact.Symbol], point)
				}
				if fact.HasPath && len(fact.Path.Segments) != 0 {
					out.pathWrites = append(out.pathWrites, pathWrite{target: fact.Path.Clone(), point: point})
				} else if fact.HasContainerPath && len(fact.ContainerPath.Segments) != 0 {
					out.pathWrites = append(out.pathWrites, pathWrite{target: fact.ContainerPath.Clone(), point: point, dynamic: true})
				}
			}
		}
		out.addCapturedImportRoots(sem.Function())
		for _, point := range points {
			if fact, ok := sem.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
				out.addRootAlias(fact.Symbol, fact.Name, fact.Expr, point, false)
				out.addSignatureAlias(path.NewPath(fact.Symbol, fact.Name), fact.Expr, point, false)
				out.addObjectLiteralAliases(path.NewPath(fact.Symbol, fact.Name), fact.Expr, point, false)
			}
			if fact, ok := sem.OrdinaryAssignment(point); ok && fact.HasPath && len(fact.Path.Segments) != 0 {
				out.addSignatureAlias(fact.Path, fact.Value, point, false)
				out.addAssignmentAlias(fact.Path, fact.Value, point, false)
			}
		}
	}
	out.addCapturedAliasNames(sem.Function())
	out.addCapturedSignatureAliases(sem.Function())
	out.addCapturedObjectAliases(sem.Function())
	return out
}

// ExactRequireCall recognizes exactly require("module") calls.
func ExactRequireCall(bindings *bind.Result, expr ast.Expr) (string, bool) {
	if bindings == nil {
		return "", false
	}
	call, ok := expr.(*ast.FuncCallExpr)
	if !ok || call == nil || call.Receiver != nil || call.Method != "" || len(call.Args) != 1 || len(call.TypeArgs) != 0 {
		return "", false
	}
	fn, ok := call.Func.(*ast.IdentExpr)
	if !ok || !bindings.ResolvesToGlobal(fn, "require") {
		return "", false
	}
	lit, ok := call.Args[0].(*ast.StringExpr)
	if !ok || lit.Value == "" {
		return "", false
	}
	return lit.Value, true
}

// LocalRequireModulePath resolves a local symbol introduced by require("module").
func LocalRequireModulePath(bindings *bind.Result, id symbol.ID) (string, bool) {
	if bindings == nil || id == 0 {
		return "", false
	}
	origin, ok := bindings.LocalOrigin(id)
	if !ok || origin.Stmt == nil {
		return "", false
	}
	return ExactRequireCall(bindings, exprAt(origin.Stmt.Exprs, origin.Index))
}

// ModulePathForAlias resolves a lexical require alias name to its module path.
func (p Projection) ModulePathForAlias(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	modulePath := p.aliasNames[name]
	return modulePath, modulePath != ""
}

// ModuleAliases returns lexical require alias names keyed by alias.
func (p Projection) ModuleAliases() map[string]string {
	if len(p.aliasNames) == 0 {
		return nil
	}
	out := make(map[string]string, len(p.aliasNames))
	for name, modulePath := range p.aliasNames {
		out[name] = modulePath
	}
	return out
}

// SignatureName resolves an imported-module call path to a manifest signature name.
func (p Projection) SignatureName(point cfg.Point, calleePath path.Path) (string, bool) {
	return p.signatureNameForPath(point, calleePath, false)
}

func (p Projection) signatureNameForPath(point cfg.Point, calleePath path.Path, includeStableGlobals bool) (string, bool) {
	if calleePath.Symbol == 0 {
		return "", false
	}
	if includeStableGlobals {
		if name, ok := p.stableGlobalSignatureName(calleePath); ok {
			return name, true
		}
	}
	if len(calleePath.Segments) != 0 {
		if root, ok := p.rootIdentity(calleePath.Symbol); ok && p.rootActiveAt(root, calleePath, point) {
			suffix, ok := staticSignatureSuffix(calleePath.Segments)
			if !ok {
				return "", false
			}
			return root.modulePath + suffix, true
		}
	}
	if alias, ok := p.activeSignatureAliasFor(point, calleePath); ok {
		return alias.name, true
	}
	if len(calleePath.Segments) == 0 {
		return "", false
	}
	alias, remaining, ok := p.activeAliasFor(point, calleePath)
	if !ok {
		return "", false
	}
	suffix, ok := staticSignatureSuffix(remaining)
	if !ok {
		return "", false
	}
	return alias.modulePath + suffix, true
}

func (p Projection) stableGlobalSignatureName(calleePath path.Path) (string, bool) {
	if p.bindings == nil || calleePath.Symbol == 0 {
		return "", false
	}
	kind, ok := p.bindings.Kind(calleePath.Symbol)
	if !ok || kind != symbol.Global {
		return "", false
	}
	if p.bindings.IsImplicitGlobalSymbol(calleePath.Symbol) {
		return "", false
	}
	name := p.bindings.Name(calleePath.Symbol)
	if name == "" {
		return "", false
	}
	if len(calleePath.Segments) == 0 {
		return name, true
	}
	suffix, ok := staticSignatureSuffix(calleePath.Segments)
	if !ok {
		return "", false
	}
	return name + suffix, true
}

func (p *Projection) addRoot(id symbol.ID, root moduleRoot) {
	if id == 0 || root.modulePath == "" {
		return
	}
	if p.roots == nil {
		p.roots = make(map[symbol.ID]moduleRoot)
	}
	p.roots[id] = root
}

func (p *Projection) addAliasName(name, modulePath string) {
	if name == "" || modulePath == "" {
		return
	}
	if p.aliasNames == nil {
		p.aliasNames = make(map[string]string)
	}
	p.aliasNames[name] = modulePath
}

func (p *Projection) addRootAlias(id symbol.ID, name string, expr ast.Expr, point cfg.Point, inherited bool) {
	if p == nil || id == 0 {
		return
	}
	modulePath, ok := p.moduleIdentityForExpr(expr)
	if !ok {
		return
	}
	p.addAliasName(name, modulePath)
	p.addRoot(id, moduleRoot{modulePath: modulePath, point: point, inherited: inherited})
}

func (p Projection) rootIdentity(id symbol.ID) (moduleRoot, bool) {
	if id == 0 {
		return moduleRoot{}, false
	}
	if root, ok := p.roots[id]; ok {
		return root, true
	}
	if modulePath, ok := LocalRequireModulePath(p.bindings, id); ok {
		return moduleRoot{modulePath: modulePath, inherited: true}, true
	}
	return moduleRoot{}, false
}

func (p Projection) rootActiveAt(root moduleRoot, calleePath path.Path, point cfg.Point) bool {
	startOrder, callOrder, ok := p.activeWindow(root.point, root.inherited, point)
	if !ok {
		return false
	}
	for _, reassigned := range p.reassigned[calleePath.Symbol] {
		reassignedOrder, ok := p.pointOrders[reassigned]
		if ok && reassignedOrder > startOrder && reassignedOrder < callOrder {
			return false
		}
	}
	return !p.hasInvalidatingPathWrite(startOrder, callOrder, calleePath, calleePath)
}

func (p Projection) activeAliasFor(point cfg.Point, calleePath path.Path) (moduleAlias, []segment.Segment, bool) {
	var best moduleAlias
	var bestRemaining []segment.Segment
	bestPrefixLen := -1
	bestOrder := -1
	for _, alias := range p.aliases {
		remaining, ok := calleePath.SuffixAfter(alias.target)
		if !ok || len(remaining) == 0 {
			continue
		}
		startOrder, _, ok := p.activeWindow(alias.point, alias.inherited, point)
		if !ok {
			continue
		}
		if !p.aliasActiveAt(alias, calleePath, point) {
			continue
		}
		prefixLen := len(alias.target.Segments)
		if prefixLen < bestPrefixLen || (prefixLen == bestPrefixLen && startOrder <= bestOrder) {
			continue
		}
		best = alias
		bestRemaining = remaining
		bestPrefixLen = prefixLen
		bestOrder = startOrder
	}
	if bestPrefixLen < 0 {
		return moduleAlias{}, nil, false
	}
	return best, bestRemaining, true
}

func (p Projection) activeSignatureAliasFor(point cfg.Point, calleePath path.Path) (signatureAlias, bool) {
	var best signatureAlias
	bestPrefixLen := -1
	bestOrder := -1
	for _, alias := range p.signatures {
		remaining, ok := calleePath.SuffixAfter(alias.target)
		if !ok || len(remaining) != 0 {
			continue
		}
		startOrder, _, ok := p.activeWindow(alias.point, alias.inherited, point)
		if !ok {
			continue
		}
		if !p.signatureAliasActiveAt(alias, calleePath, point) {
			continue
		}
		prefixLen := len(alias.target.Segments)
		if prefixLen < bestPrefixLen || (prefixLen == bestPrefixLen && startOrder <= bestOrder) {
			continue
		}
		best = alias
		bestPrefixLen = prefixLen
		bestOrder = startOrder
	}
	if bestPrefixLen < 0 {
		return signatureAlias{}, false
	}
	return best, true
}

func (p Projection) aliasActiveAt(alias moduleAlias, calleePath path.Path, point cfg.Point) bool {
	return p.aliasTargetActiveAt(alias.point, alias.inherited, alias.target, calleePath, point)
}

func (p Projection) signatureAliasActiveAt(alias signatureAlias, calleePath path.Path, point cfg.Point) bool {
	return p.aliasTargetActiveAt(alias.point, alias.inherited, alias.target, calleePath, point)
}

// aliasTargetActiveAt reports whether an alias bound at origin (inherited or
// not) still holds at point: its active window must exist, its target must not
// be reassigned within the window, and no invalidating path write may occur.
func (p Projection) aliasTargetActiveAt(origin cfg.Point, inherited bool, target, calleePath path.Path, point cfg.Point) bool {
	startOrder, callOrder, ok := p.activeWindow(origin, inherited, point)
	if !ok {
		return false
	}
	for _, reassigned := range p.reassigned[target.Symbol] {
		reassignedOrder, ok := p.pointOrders[reassigned]
		if ok && reassignedOrder > startOrder && reassignedOrder < callOrder {
			return false
		}
	}
	return !p.hasInvalidatingPathWrite(startOrder, callOrder, target, calleePath)
}

func (p Projection) activeWindow(origin cfg.Point, inherited bool, point cfg.Point) (int, int, bool) {
	callOrder, ok := p.pointOrders[point]
	if !ok {
		return 0, 0, false
	}
	if inherited {
		return -1, callOrder, true
	}
	originOrder, ok := p.pointOrders[origin]
	if !ok || originOrder >= callOrder {
		return 0, 0, false
	}
	return originOrder, callOrder, true
}

func (p Projection) hasInvalidatingPathWrite(startOrder, callOrder int, identityPath, calleePath path.Path) bool {
	for _, write := range p.pathWrites {
		writeOrder, ok := p.pointOrders[write.point]
		if !ok || writeOrder <= startOrder || writeOrder >= callOrder {
			continue
		}
		if write.dynamic {
			if identityPath.HasPrefix(write.target) || calleePath.HasPrefix(write.target) {
				return true
			}
			continue
		}
		if calleePath.HasPrefix(write.target) {
			return true
		}
	}
	return false
}

func (p *Projection) addCapturedImportRoots(fn *ast.FunctionExpr) {
	if p == nil || p.bindings == nil || fn == nil {
		return
	}
	for _, capture := range p.bindings.DirectCaptures(fn) {
		modulePath, ok := LocalRequireModulePath(p.bindings, capture.Captured)
		if !ok {
			continue
		}
		p.addRoot(capture.Captured, moduleRoot{modulePath: modulePath, inherited: true})
	}
}

func (p *Projection) addCapturedAliasNames(fn *ast.FunctionExpr) {
	if p == nil || p.bindings == nil || fn == nil {
		return
	}
	for _, capture := range p.bindings.DirectCaptures(fn) {
		modulePath, ok := LocalRequireModulePath(p.bindings, capture.Captured)
		if !ok {
			continue
		}
		p.addAliasName(capture.CapturedName, modulePath)
	}
}

func (p *Projection) addCapturedObjectAliases(fn *ast.FunctionExpr) {
	if p == nil || p.bindings == nil || fn == nil {
		return
	}
	for _, capture := range p.bindings.DirectCaptures(fn) {
		origin, ok := p.bindings.LocalOrigin(capture.Captured)
		if !ok || origin.Stmt == nil {
			continue
		}
		name := capture.CapturedName
		if name == "" && origin.Index >= 0 && origin.Index < len(origin.Stmt.Names) {
			name = origin.Stmt.Names[origin.Index]
		}
		p.addObjectLiteralAliases(
			path.NewPath(capture.Captured, name),
			exprAt(origin.Stmt.Exprs, origin.Index),
			0,
			true,
		)
	}
}

func (p *Projection) addCapturedSignatureAliases(fn *ast.FunctionExpr) {
	if p == nil || p.bindings == nil || fn == nil {
		return
	}
	for _, capture := range p.bindings.DirectCaptures(fn) {
		origin, ok := p.bindings.LocalOrigin(capture.Captured)
		if !ok || origin.Stmt == nil {
			continue
		}
		name := capture.CapturedName
		if name == "" && origin.Index >= 0 && origin.Index < len(origin.Stmt.Names) {
			name = origin.Stmt.Names[origin.Index]
		}
		p.addSignatureAlias(
			path.NewPath(capture.Captured, name),
			exprAt(origin.Stmt.Exprs, origin.Index),
			0,
			true,
		)
	}
}

func (p *Projection) addObjectLiteralAliases(target path.Path, expr ast.Expr, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 || !staticPathSegments(target.Segments) {
		return
	}
	table, ok := pathexpr.ObjectLiteralTable(expr)
	if !ok {
		return
	}
	for _, entry := range pathexpr.ObjectEntries(table) {
		if len(entry.Suffix.Segments) == 0 || !staticPathSegments(entry.Suffix.Segments) {
			continue
		}
		modulePath, ok := p.moduleIdentityForExpr(entry.Value)
		if !ok {
			continue
		}
		p.addAlias(appendPathSegments(target, entry.Suffix.Segments), modulePath, point, inherited)
	}
}

func (p *Projection) addAssignmentAlias(target path.Path, expr ast.Expr, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 || len(target.Segments) == 0 || !staticPathSegments(target.Segments) {
		return
	}
	modulePath, ok := p.moduleIdentityForExpr(expr)
	if !ok {
		return
	}
	p.addAlias(target, modulePath, point, inherited)
}

func (p *Projection) addSignatureAlias(target path.Path, expr ast.Expr, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 || !staticPathSegments(target.Segments) {
		return
	}
	resolved, ok := pathexpr.Resolve(expr, p.bindings)
	if !ok {
		return
	}
	name, ok := p.signatureNameForPath(point, resolved, true)
	if !ok {
		return
	}
	p.signatures = append(p.signatures, signatureAlias{
		target:    target.Clone(),
		name:      name,
		point:     point,
		inherited: inherited,
	})
}

func (p *Projection) addAlias(target path.Path, modulePath string, point cfg.Point, inherited bool) {
	if p == nil || modulePath == "" || target.Symbol == 0 || len(target.Segments) == 0 || !staticPathSegments(target.Segments) {
		return
	}
	p.aliases = append(p.aliases, moduleAlias{
		target:     target.Clone(),
		modulePath: modulePath,
		point:      point,
		inherited:  inherited,
	})
}

func (p Projection) moduleIdentityForExpr(expr ast.Expr) (string, bool) {
	resolved, ok := pathexpr.Resolve(expr, p.bindings)
	if !ok {
		return "", false
	}
	return p.moduleIdentityForPath(resolved)
}

func (p Projection) moduleIdentityForPath(resolved path.Path) (string, bool) {
	if resolved.Symbol == 0 {
		return "", false
	}
	if len(resolved.Segments) == 0 {
		root, ok := p.rootIdentity(resolved.Symbol)
		if !ok {
			return "", false
		}
		return root.modulePath, true
	}
	for _, alias := range p.aliases {
		remaining, ok := resolved.SuffixAfter(alias.target)
		if ok && len(remaining) == 0 {
			return alias.modulePath, true
		}
	}
	return "", false
}

func exprAt(exprs []ast.Expr, index int) ast.Expr {
	if index < 0 || index >= len(exprs) {
		return nil
	}
	return exprs[index]
}

func staticSignatureSuffix(segments []segment.Segment) (string, bool) {
	if len(segments) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, seg := range segments {
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

func staticPathSegments(segments []segment.Segment) bool {
	for _, seg := range segments {
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			if seg.Name == "" {
				return false
			}
		case segment.SegmentIndexInt:
		default:
			return false
		}
	}
	return true
}

func appendPathSegments(base path.Path, suffix []segment.Segment) path.Path {
	out := base.Clone()
	out.Segments = append(out.Segments, suffix...)
	return out
}
