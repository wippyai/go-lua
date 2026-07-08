package exportmanifest

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// globalTableName is the Lua global table a module assigns to in order to
// install an ambient global (_G.<name> = value).
const globalTableName = "_G"

// publishProvidedGlobals records the ambient globals a module installs by
// assigning to the global table. It scans the module body and every nested
// function for assignments whose target is _G.<name>, recovers the assigned
// value's type, and declares it on the manifest. Consumers that require the
// module then recognize a bare <name> reference with this type.
func publishProvidedGlobals(m *manifest.Manifest, result program.Result) {
	if m == nil {
		return
	}
	root := result.RootResult()
	if root == nil {
		return
	}
	forwarding := newProvidedGlobalForwarding(root)
	for _, fnResult := range allFunctionResults(root) {
		collectProvidedGlobals(m, fnResult)
	}
	if forwarding != nil {
		forwarding.collect(m)
	}
}

// allFunctionResults returns result and every nested function result reachable
// from it.
func allFunctionResults(result *body.Result) []*body.Result {
	if result == nil {
		return nil
	}
	out := []*body.Result{result}
	for _, child := range result.FunctionResults() {
		out = append(out, allFunctionResults(child)...)
	}
	return out
}

// collectProvidedGlobals records every _G.<name> = value assignment in one
// function body.
func collectProvidedGlobals(m *manifest.Manifest, result *body.Result) {
	graph := result.Graph()
	if graph == nil {
		return
	}
	for _, point := range graph.RPO() {
		fact, ok := result.OrdinaryAssignment(point)
		if !ok || !fact.HasPath || fact.Path.Symbol == 0 {
			continue
		}
		if result.SymbolName(fact.Path.Symbol) != globalTableName {
			continue
		}
		name, ok := singleFieldName(fact.Path.Segments)
		if !ok {
			continue
		}
		m.DefineGlobal(name)
		if t, ok := result.DeclaredExpressionTypeAt(point, fact.Value); ok {
			defineProvidedGlobalType(m, name, t)
			continue
		}
		if fn, ok := result.FunctionValueTypeAtBoundary(point, fact.Value); ok {
			defineProvidedGlobalType(m, name, fn)
			continue
		}
		if t, ok := sourceType(result, point, fact.Source); ok {
			defineProvidedGlobalType(m, name, t)
			continue
		}
		if value, ok := result.OrdinaryAssignmentSourceValueForExplanationAtBoundary(point, fact.Source); ok {
			if t, ok := result.ValueType(value); ok {
				defineProvidedGlobalType(m, name, t)
			}
		}
	}
}

func defineProvidedGlobalType(m *manifest.Manifest, name string, t typ.Type) {
	if t == nil || typ.TypeEquals(t, typ.Nil) {
		return
	}
	m.DefineGlobalType(name, t)
}

// singleFieldName returns the field name when segments addresses exactly one
// named member (_G.<name> or _G["<name>"]), the only shape an ambient global
// install takes.
func singleFieldName(segments []segment.Segment) (string, bool) {
	if len(segments) != 1 {
		return "", false
	}
	seg := segments[0]
	if seg.Kind != segment.SegmentField && seg.Kind != segment.SegmentIndexString {
		return "", false
	}
	if seg.Name == "" {
		return "", false
	}
	return seg.Name, true
}

type providedGlobalForwarding struct {
	root              *body.Result
	results           []*body.Result
	functionForSymbol map[symbol.ID]*ast.FunctionExpr
	watchedCallbacks  map[symbol.ID]struct{}
}

func newProvidedGlobalForwarding(root *body.Result) *providedGlobalForwarding {
	watched := exportedFunctionParamSymbols(root)
	if root == nil || len(watched) == 0 {
		return nil
	}
	f := &providedGlobalForwarding{
		root:              root,
		results:           allFunctionResults(root),
		functionForSymbol: make(map[symbol.ID]*ast.FunctionExpr),
		watchedCallbacks:  watched,
	}
	for _, fnResult := range f.results {
		fn := fnResult.Function()
		if fn == nil {
			continue
		}
		if id, ok := root.FunctionSymbol(fn); ok {
			f.functionForSymbol[id] = fn
		}
		if origin, ok := root.FunctionOrigin(fn); ok && origin.HasTargetSymbol {
			f.functionForSymbol[origin.TargetSymbol] = fn
		}
	}
	f.propagateCallbackParams()
	return f
}

// propagateCallbackParams closes the watched-callback set over local helper
// calls. If an exported callback parameter flows into a helper parameter whose
// declared type can be called, that helper parameter becomes watched too. The
// set is finite: each iteration can only add a bound parameter symbol.
func (f *providedGlobalForwarding) propagateCallbackParams() {
	if f == nil || len(f.watchedCallbacks) == 0 {
		return
	}
	for changed := true; changed; {
		changed = false
		for _, result := range f.results {
			graph := result.Graph()
			if graph == nil {
				continue
			}
			for _, point := range graph.RPO() {
				site, ok := result.CallSiteView(point)
				if !ok {
					continue
				}
				callee, args, ok := f.localCalleeAndArgs(result, site)
				if !ok {
					continue
				}
				slots := f.root.FunctionParamSlots(callee)
				for i, arg := range args {
					if i >= len(slots) || slots[i].Symbol == 0 || !parameterSlotCanBeCallback(slots[i].Type) {
						continue
					}
					if !f.sourceIsWatchedCallback(result, arg) {
						continue
					}
					if _, ok := f.watchedCallbacks[slots[i].Symbol]; ok {
						continue
					}
					f.watchedCallbacks[slots[i].Symbol] = struct{}{}
					changed = true
				}
			}
		}
	}
}

// collect republishes ambient globals from an imported provider when an exported
// callback parameter, possibly forwarded through local helpers or pcall, reaches
// that provider. This covers wrapper modules such as:
// `define(fn) -> function() run(fn) end`, `run(fn) -> pcall(core.define, fn)`.
func (f *providedGlobalForwarding) collect(m *manifest.Manifest) {
	if f == nil || m == nil || len(f.watchedCallbacks) == 0 {
		return
	}
	for _, result := range f.results {
		graph := result.Graph()
		if graph == nil {
			continue
		}
		for _, point := range graph.RPO() {
			site, ok := result.CallSiteView(point)
			if !ok {
				continue
			}
			provider, globals, providerArgs := providerGlobalsForCall(result, point, site)
			if len(globals) == 0 || !f.callPassesWatchedCallback(result, providerArgs) {
				continue
			}
			for _, name := range globals {
				m.DefineGlobal(name)
				if provider != nil {
					defineProvidedGlobalType(m, name, provider.GlobalTypes[name])
				}
			}
		}
	}
}

func (f *providedGlobalForwarding) localCalleeAndArgs(result *body.Result, site factflow.CallSiteView) (*ast.FunctionExpr, []factflow.ValueSource, bool) {
	if f == nil {
		return nil, nil, false
	}
	if fn, ok := f.localFunctionForPath(site.CalleePathRef()); ok {
		return fn, site.ArgumentSources(), true
	}
	if isBareGlobalCallSite(result, site, "pcall") && site.ArgumentSourceCount() != 0 {
		source, _ := site.ArgumentSourceAt(0)
		if p, ok := result.ValueSourcePath(source); ok {
			if fn, ok := f.localFunctionForPath(p); ok {
				return fn, callSiteArgumentSourcesFrom(site, 1), true
			}
		}
	}
	return nil, nil, false
}

func (f *providedGlobalForwarding) localFunctionForPath(p pathdom.Path) (*ast.FunctionExpr, bool) {
	if f == nil || p.Symbol == 0 || len(p.Segments) != 0 {
		return nil, false
	}
	fn, ok := f.functionForSymbol[p.Symbol]
	return fn, ok && fn != nil
}

func callSiteArgumentSourcesFrom(site factflow.CallSiteView, start int) []factflow.ValueSource {
	if start <= 0 {
		return site.ArgumentSources()
	}
	if start >= site.ArgumentSourceCount() {
		return nil
	}
	out := make([]factflow.ValueSource, 0, site.ArgumentSourceCount()-start)
	for i := start; i < site.ArgumentSourceCount(); i++ {
		source, ok := site.ArgumentSourceAt(i)
		if ok {
			out = append(out, source)
		}
	}
	return out
}

func (f *providedGlobalForwarding) callPassesWatchedCallback(result *body.Result, args []factflow.ValueSource) bool {
	if f == nil || len(args) == 0 || len(f.watchedCallbacks) == 0 {
		return false
	}
	for _, arg := range args {
		if f.sourceIsWatchedCallback(result, arg) {
			return true
		}
	}
	return false
}

func (f *providedGlobalForwarding) sourceIsWatchedCallback(result *body.Result, source factflow.ValueSource) bool {
	if f == nil || result == nil {
		return false
	}
	p, ok := result.ValueSourcePath(source)
	if !ok || p.Symbol == 0 || len(p.Segments) != 0 {
		return false
	}
	_, ok = f.watchedCallbacks[p.Symbol]
	return ok
}

func providerGlobalsForCall(result *body.Result, point cfg.Point, site factflow.CallSiteView) (*manifest.Manifest, []string, []factflow.ValueSource) {
	name, ok := result.CallSignatureNameAtPoint(point)
	if ok {
		if provider, globals := providerGlobalsForSignatureName(result, name); len(globals) != 0 {
			return provider, globals, site.ArgumentSources()
		}
	}
	if isBareGlobalCallSite(result, site, "pcall") && site.ArgumentSourceCount() != 0 {
		source, _ := site.ArgumentSourceAt(0)
		if p, ok := result.ValueSourcePath(source); ok {
			name, ok := result.PathSignatureNameAt(point, p)
			if !ok {
				return nil, nil, nil
			}
			if provider, globals := providerGlobalsForSignatureName(result, name); len(globals) != 0 {
				return provider, globals, callSiteArgumentSourcesFrom(site, 1)
			}
		}
	}
	return nil, nil, nil
}

func providerGlobalsForSignatureName(result *body.Result, name string) (*manifest.Manifest, []string) {
	for _, m := range result.SignatureManifests() {
		if m == nil || len(m.Globals) == 0 {
			continue
		}
		if signatureNameBelongsToManifest(name, m.Path) {
			return m, m.Globals
		}
	}
	return nil, nil
}

func isBareGlobalCallSite(result *body.Result, site factflow.CallSiteView, name string) bool {
	if result == nil || name == "" || site.CalleeMemberAccess() {
		return false
	}
	id := site.CalleeSymbol()
	if id == 0 {
		return false
	}
	kind, ok := result.SymbolKind(id)
	return ok && kind == symbol.Global && result.SymbolName(id) == name
}

func signatureNameBelongsToManifest(name, manifestPath string) bool {
	if name == "" || manifestPath == "" || name == manifestPath {
		return false
	}
	if strings.HasPrefix(name, manifestPath+".") {
		return true
	}
	return strings.HasPrefix(name, manifestPath+"[")
}

func exportedFunctionParamSymbols(root *body.Result) map[symbol.ID]struct{} {
	exported := exportedFunctions(root)
	if len(exported) == 0 {
		return nil
	}
	out := make(map[symbol.ID]struct{})
	for _, fnResult := range allFunctionResults(root) {
		fn := fnResult.Function()
		if fn == nil || !functionDescendsFromExport(root, fn, exported) {
			continue
		}
		for _, slot := range root.FunctionParamSlots(fn) {
			if slot.Symbol != 0 && parameterSlotCanBeCallback(slot.Type) {
				out[slot.Symbol] = struct{}{}
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parameterSlotCanBeCallback(t ast.TypeExpr) bool {
	switch n := t.(type) {
	case *ast.FunctionTypeExpr:
		return true
	case *ast.OptionalTypeExpr:
		return parameterSlotCanBeCallback(n.Inner)
	default:
		return false
	}
}

func exportedFunctions(root *body.Result) map[*ast.FunctionExpr]struct{} {
	if root == nil || root.Graph() == nil {
		return nil
	}
	dom := dominance.ComputeImmediateDominatorInfo(root.Graph())
	out := make(map[*ast.FunctionExpr]struct{})
	for _, exportRoot := range returnedExportSourcePaths(root) {
		collectFunctionDefinitionExportedFunctions(out, root, dom, exportRoot)
		collectOrdinaryAssignmentExportedFunctions(out, root, dom, exportRoot)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectFunctionDefinitionExportedFunctions(
	out map[*ast.FunctionExpr]struct{},
	root *body.Result,
	dom *dominance.ImmediateDominators,
	exportRoot returnedSourcePath,
) {
	for _, point := range root.Graph().RPO() {
		fact, ok := root.FunctionDefinition(point)
		if !ok || !dominatesAllReturnPoints(dom, point, exportRoot.points) || fact.Func == nil || fact.Name == nil {
			continue
		}
		if _, ok := functionDefinitionExportMember(root, exportRoot.path, fact.Name); ok {
			out[fact.Func] = struct{}{}
		}
	}
}

func collectOrdinaryAssignmentExportedFunctions(
	out map[*ast.FunctionExpr]struct{},
	root *body.Result,
	dom *dominance.ImmediateDominators,
	exportRoot returnedSourcePath,
) {
	if exportRoot.path.Symbol == 0 {
		return
	}
	for _, point := range root.Graph().RPO() {
		fact, ok := root.OrdinaryAssignment(point)
		if !ok || !dominatesAllReturnPoints(dom, point, exportRoot.points) || !fact.HasPath || fact.Path.Symbol != exportRoot.path.Symbol {
			continue
		}
		if _, ok := directMemberSegment(exportRoot.path.Segments, fact.Path.Segments); !ok {
			continue
		}
		if fn, ok := ordinaryAssignmentRHSExpr(fact).(*ast.FunctionExpr); ok {
			out[fn] = struct{}{}
		}
	}
}

func functionDescendsFromExport(root *body.Result, fn *ast.FunctionExpr, exported map[*ast.FunctionExpr]struct{}) bool {
	for current := fn; current != nil; {
		if _, ok := exported[current]; ok {
			return true
		}
		origin, ok := root.FunctionOrigin(current)
		if !ok {
			return false
		}
		current = origin.Parent
	}
	return false
}
