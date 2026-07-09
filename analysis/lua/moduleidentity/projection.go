package moduleidentity

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
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

// SourceRef identifies a lowered expression source without tying this package
// to the engine's fact carrier.
type SourceRef uint64

// SourceKind classifies the source value shapes module identity needs.
type SourceKind uint8

const (
	SourceExpression SourceKind = iota + 1
	SourceCall
	SourcePath
	SourceStringLiteral
)

// Source is the fact-neutral source identity consumed by Projection.
type Source struct {
	Kind        SourceKind
	Expr        SourceRef
	HasExpr     bool
	CallPoint   cfg.Point
	ResultIndex int
	PathKey     path.PathKey
	String      string
}

// Assignment describes a root or path assignment in fact-neutral form.
type Assignment struct {
	Target       path.Path
	TargetSymbol symbol.ID
	Source       Source
}

// CallSite describes the call-site facts needed to recognize require("x").
type CallSite struct {
	Callee       path.Path
	Args         []Source
	TypeArgCount int
	MethodName   string
}

// ObjectEntry describes one static object-literal entry source.
type ObjectEntry struct {
	Suffix path.Path
	Source Source
}

// FlowFacts is the canonical boundary moduleidentity reads after lowering. The
// producer may be factflow, tests, or a future WIR-native fact schema; this
// package stays below engine/check layers.
type FlowFacts interface {
	LocalAssignment(cfg.Point) (Assignment, bool)
	OrdinaryAssignment(cfg.Point) (Assignment, bool)
	PathAssignment(cfg.Point) (Assignment, bool)
	PathDescendantInvalidation(cfg.Point) (path.Path, bool)
	CallSite(cfg.Point) (CallSite, bool)
	ForEachObjectLiteralEntry(SourceRef, func(ObjectEntry) bool) bool
	ExpressionPath(SourceRef) (path.Path, bool)
}

// NewFromFacts builds a require/module identity projection from canonical WIR
// transfer facts for a function or chunk body.
func NewFromFacts(bindings *bind.Result, graph cfg.Graph, facts FlowFacts, fn *ast.FunctionExpr) Projection {
	out := Projection{bindings: bindings}
	if bindings == nil {
		return out
	}
	if graph != nil {
		points := graph.RPO()
		out.pointOrders = make(map[cfg.Point]int, len(points))
		for i, point := range points {
			out.pointOrders[point] = i
			if fact, ok := facts.LocalAssignment(point); ok {
				if modulePath, ok := out.exactRequireSource(facts, fact.Source); ok {
					name := out.targetName(fact.Target, fact.TargetSymbol)
					out.addAliasName(name, modulePath)
					out.addRoot(fact.TargetSymbol, moduleRoot{modulePath: modulePath, point: point})
				}
			}
			if fact, ok := facts.OrdinaryAssignment(point); ok {
				target := fact.Target
				if fact.TargetSymbol != 0 && len(target.Segments) == 0 {
					if out.reassigned == nil {
						out.reassigned = make(map[symbol.ID][]cfg.Point)
					}
					out.reassigned[fact.TargetSymbol] = append(out.reassigned[fact.TargetSymbol], point)
				}
			}
			if fact, ok := facts.PathAssignment(point); ok {
				target := fact.Target
				if len(target.Segments) != 0 {
					out.pathWrites = append(out.pathWrites, pathWrite{target: target.Clone(), point: point})
				}
			}
			if target, ok := facts.PathDescendantInvalidation(point); ok {
				if len(target.Segments) != 0 {
					out.pathWrites = append(out.pathWrites, pathWrite{target: target.Clone(), point: point, dynamic: true})
				}
			}
		}
		out.addCapturedImportRoots(fn)
		for _, point := range points {
			if fact, ok := facts.LocalAssignment(point); ok {
				target := fact.Target
				source := fact.Source
				out.addRootAliasFromSource(facts, target, source, point, false)
				out.addSignatureAliasFromSource(facts, target, source, point, false)
				out.addObjectLiteralAliasesFromSource(facts, target, source, point, false)
			}
			if fact, ok := facts.PathAssignment(point); ok {
				target := fact.Target
				if len(target.Segments) == 0 {
					continue
				}
				source := fact.Source
				out.addSignatureAliasFromSource(facts, target, source, point, false)
				out.addAssignmentAliasFromSource(facts, target, source, point, false)
			}
		}
	}
	out.addCapturedAliasNames(fn)
	out.addCapturedSignatureAliases(fn)
	out.addCapturedObjectAliases(fn)
	return out
}

// NewRequireAliases builds the lexical require-alias subset needed by type
// resolution before WIR/factflow exists. It intentionally records only the
// alias-name projection consumed by ModuleAliases; full flow-sensitive signature
// identity is built by New after lowering has produced body facts.
func NewRequireAliases(bindings *bind.Result, stmts []ast.Stmt, fn *ast.FunctionExpr) Projection {
	out := Projection{bindings: bindings}
	if bindings == nil {
		return out
	}
	out.addRequireAliasesFromStmts(stmts)
	out.addCapturedAliasNames(fn)
	return out
}

func (p *Projection) addRequireAliasesFromStmts(stmts []ast.Stmt) {
	if p == nil || p.bindings == nil {
		return
	}
	for _, stmt := range stmts {
		switch n := stmt.(type) {
		case *ast.LocalAssignStmt:
			for i, name := range n.Names {
				id, ok := p.bindings.LocalSymbolAt(n, i)
				if !ok || id == 0 {
					continue
				}
				expr := exprAt(n.Exprs, i)
				if modulePath, ok := ExactRequireCall(p.bindings, expr); ok {
					p.addAliasName(name, modulePath)
					p.addRoot(id, moduleRoot{modulePath: modulePath, inherited: true})
					continue
				}
				p.addRootAlias(id, name, expr, 0, true)
			}
		case *ast.DoBlockStmt:
			p.addRequireAliasesFromStmts(n.Stmts)
		case *ast.IfStmt:
			p.addRequireAliasesFromStmts(n.Then)
			p.addRequireAliasesFromStmts(n.Else)
		case *ast.WhileStmt:
			p.addRequireAliasesFromStmts(n.Stmts)
		case *ast.RepeatStmt:
			p.addRequireAliasesFromStmts(n.Stmts)
		case *ast.NumberForStmt:
			p.addRequireAliasesFromStmts(n.Stmts)
		case *ast.GenericForStmt:
			p.addRequireAliasesFromStmts(n.Stmts)
		}
	}
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

func (p *Projection) addRootAliasFromSource(facts FlowFacts, target path.Path, source Source, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 {
		return
	}
	modulePath, ok := p.moduleIdentityForSource(facts, source)
	if !ok {
		return
	}
	p.addAliasName(p.targetName(target, target.Symbol), modulePath)
	p.addRoot(target.Symbol, moduleRoot{modulePath: modulePath, point: point, inherited: inherited})
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
		if modulePath, ok := LocalRequireModulePath(p.bindings, capture.Captured); ok {
			p.addAliasName(capture.CapturedName, modulePath)
		}
		origin, ok := p.bindings.LocalOrigin(capture.Captured)
		if !ok || origin.Stmt == nil {
			continue
		}
		name := capture.CapturedName
		if name == "" && origin.Index >= 0 && origin.Index < len(origin.Stmt.Names) {
			name = origin.Stmt.Names[origin.Index]
		}
		p.addRootAlias(capture.Captured, name, exprAt(origin.Stmt.Exprs, origin.Index), 0, true)
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
		p.addAlias(target.AppendPathSuffix(entry.Suffix), modulePath, point, inherited)
	}
}

func (p *Projection) addObjectLiteralAliasesFromSource(facts FlowFacts, target path.Path, source Source, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 || !staticPathSegments(target.Segments) || source.Kind != SourceExpression || !source.HasExpr {
		return
	}
	if !facts.ForEachObjectLiteralEntry(source.Expr, func(entry ObjectEntry) bool {
		if len(entry.Suffix.Segments) == 0 || !staticPathSegments(entry.Suffix.Segments) {
			return true
		}
		modulePath, ok := p.moduleIdentityForSource(facts, entry.Source)
		if !ok {
			return true
		}
		aliased := target.AppendPathSuffix(entry.Suffix)
		p.addAlias(aliased, modulePath, point, inherited)
		return true
	}) {
		return
	}
}

func (p *Projection) addAssignmentAliasFromSource(facts FlowFacts, target path.Path, source Source, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 || len(target.Segments) == 0 || !staticPathSegments(target.Segments) {
		return
	}
	modulePath, ok := p.moduleIdentityForSource(facts, source)
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

func (p *Projection) addSignatureAliasFromSource(facts FlowFacts, target path.Path, source Source, point cfg.Point, inherited bool) {
	if p == nil || target.Symbol == 0 || !staticPathSegments(target.Segments) {
		return
	}
	resolved, ok := p.sourcePath(facts, source)
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

func (p Projection) moduleIdentityForSource(facts FlowFacts, source Source) (string, bool) {
	if modulePath, ok := p.exactRequireSource(facts, source); ok {
		return modulePath, true
	}
	resolved, ok := p.sourcePath(facts, source)
	if !ok {
		return "", false
	}
	return p.moduleIdentityForPath(resolved)
}

func (p Projection) exactRequireSource(facts FlowFacts, source Source) (string, bool) {
	if source.Kind != SourceCall || source.CallPoint == 0 || source.ResultIndex != 0 {
		return "", false
	}
	site, ok := facts.CallSite(source.CallPoint)
	if !ok || len(site.Args) != 1 || site.MethodName != "" || site.TypeArgCount != 0 {
		return "", false
	}
	callee := site.Callee
	if callee.Symbol == 0 || len(callee.Segments) != 0 {
		return "", false
	}
	if p.bindings == nil || !p.bindings.SymbolResolvesToGlobal(callee.Symbol, "require") {
		return "", false
	}
	arg := site.Args[0]
	if arg.Kind != SourceStringLiteral || arg.String == "" {
		return "", false
	}
	return arg.String, true
}

func (p Projection) sourcePath(facts FlowFacts, source Source) (path.Path, bool) {
	switch source.Kind {
	case SourcePath:
		return pathFromSourceKey(source.PathKey)
	case SourceExpression:
		if !source.HasExpr {
			return path.Path{}, false
		}
		return facts.ExpressionPath(source.Expr)
	default:
		return path.Path{}, false
	}
}

func (p Projection) moduleIdentityForPath(resolved path.Path) (string, bool) {
	if resolved.Symbol == 0 {
		return "", false
	}
	if len(resolved.Segments) == 0 {
		root, ok := p.rootIdentity(resolved.Symbol)
		if !ok {
			return p.explicitGlobalModuleRoot(resolved.Symbol)
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

func (p Projection) targetName(target path.Path, id symbol.ID) string {
	if target.Root != "" {
		return target.Root
	}
	if p.bindings != nil && id != 0 {
		return p.bindings.Name(id)
	}
	return ""
}

func (p Projection) explicitGlobalModuleRoot(id symbol.ID) (string, bool) {
	if p.bindings == nil || id == 0 {
		return "", false
	}
	kind, ok := p.bindings.Kind(id)
	if !ok || kind != symbol.Global || p.bindings.IsImplicitGlobalSymbol(id) {
		return "", false
	}
	name := p.bindings.Name(id)
	return name, name != ""
}

func pathFromSourceKey(key path.PathKey) (path.Path, bool) {
	if sym, _, suffix, ok := pathaddr.ParseResolverPath(key); ok {
		segments, segOK := segment.ParseFormattedSegments(suffix)
		if !segOK {
			return path.Path{}, false
		}
		return path.Path{Symbol: sym, Segments: append([]segment.Segment(nil), segments...)}, true
	}
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(key); ok {
		return path.Path{Symbol: sym, Segments: segments}, true
	}
	return path.Path{}, false
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
