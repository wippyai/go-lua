package program

import (
	"fmt"
	"hash/fnv"
	"slices"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type callContextRef struct {
	owner summary.SummaryKey
	expr  factflow.ExprRef
}

type functionExpressionRef struct {
	owner summary.SummaryKey
	expr  factflow.ExprRef
}

type programKeys struct {
	rootKey                  summary.SummaryKey
	functions                []keyedFunction
	contexts                 contextIndex
	functionByKey            map[summary.SummaryKey]*ast.FunctionExpr
	functionKeys             map[symbol.ID]summary.SummaryKey
	functionIDs              map[identity.ID]summary.SummaryKey
	targetKeys               map[symbol.ID]summary.SummaryKey
	pathKeys                 map[factflow.CalleePathKey]summary.SummaryKey
	pathMultiKeys            map[factflow.CalleePathKey][]summary.SummaryKey
	functionTypes            map[summary.SummaryKey]*typ.Function
	metatableProof           metatableMethodProof
	metatableMethodReceivers map[symbol.ID]typ.Type
	metatableSeedReceivers   map[symbol.ID]typ.Type
	closedDynamicAllValues   []factapply.ClosedDynamicAllValueInvariant

	// inferredParamSeeds carries the call-site parameter seed per function so both
	// the summary fixpoint and the materialization pass re-check each body with
	// the same inferred parameter values.
	inferredParamSeeds map[*ast.FunctionExpr][]paramSeed

	// bindings and enclosed support parameter inference: enclosed is the set of
	// function symbols whose complete call-site set is statically known, so their
	// parameters may be inferred from call sites and body usage.
	bindings *bind.Result
	enclosed map[symbol.ID]struct{}

	// queryDependencies is compact scheduling evidence extracted while each
	// context-discovery prepass Result is already live. Edges point from caller
	// to runtime-resolved lexical callees; no body graph is retained.
	queryDependencies map[summary.SummaryKey]map[summary.SummaryKey]struct{}

	// certifyRelationContexts keeps the expensive full-domain entry proof off
	// the legacy hot path. It is enabled only by relation activation/audits.
	certifyRelationContexts bool
}

// functionSymbol returns the function symbol owning fn.
func (k programKeys) functionSymbol(fn *ast.FunctionExpr) (symbol.ID, bool) {
	if k.bindings == nil || fn == nil {
		return 0, false
	}
	return k.bindings.FunctionSymbol(fn)
}

func (k programKeys) summaryKeyForFunction(fn *ast.FunctionExpr) (summary.SummaryKey, bool) {
	sym, ok := k.functionSymbol(fn)
	if !ok || sym == 0 {
		return summary.SummaryKey{}, false
	}
	key, ok := k.functionKeys[sym]
	return key, ok
}

// functionSymbolsByKey inverts functionKeys so a resolved call summary key maps
// back to its callee function symbol for call-site parameter inference.
func (k programKeys) functionSymbolsByKey() map[summary.SummaryKey]symbol.ID {
	if len(k.functionKeys) == 0 {
		return nil
	}
	out := make(map[summary.SummaryKey]symbol.ID, len(k.functionKeys))
	for sym, key := range k.functionKeys {
		out[key] = sym
	}
	return out
}

// materializedOwnerRoutingDigest fences only the call-context routing visible
// to one body. The old whole-program shape digest changed whenever any body
// discovered a context, invalidating every materialized body even when its own
// provider routing and all summary reads were unchanged.
func materializedOwnerRoutingDigest(keys programKeys, owner summary.SummaryKey) uint64 {
	h := fnv.New64a()
	writeSummaryKeyDigest(h, owner)
	expressions := keys.contexts.FunctionExpressionKeysForOwner(owner)
	refs := make([]factflow.ExprRef, 0, len(expressions))
	for expr := range expressions {
		refs = append(refs, expr)
	}
	slices.Sort(refs)
	for _, expr := range refs {
		fmt.Fprintf(h, "expr:%d=", expr)
		writeSummaryKeyDigest(h, expressions[expr])
	}
	return h.Sum64()
}

// summaryOwnerResolutionDigest fences the non-summary input consulted by call
// outcome providers. Summary payloads are validated separately by the solve
// cache; this digest covers only which summary key a body can select for a
// call. It is content-derived, so equal independently parsed units retain
// sharing while a changed call-resolution graph cannot reuse a stale result.
func summaryOwnerResolutionDigest(keys programKeys, owner summary.SummaryKey) uint64 {
	h := fnv.New64a()
	writeSummaryKeyDigest(h, owner)
	fmt.Fprintf(h, "owner-routing:%d;", materializedOwnerRoutingDigest(keys, owner))
	writeSymbolSummaryKeySetDigest(h, keys.functionKeys)
	writeIdentitySummaryKeySetDigest(h, keys.functionIDs)
	writeSymbolSummaryKeySetDigest(h, keys.targetKeys)
	writeCalleePathKeySetDigest(h, keys.pathKeys)
	writeCalleePathMultiKeySetDigest(h, keys.pathMultiKeys)
	return h.Sum64()
}

func writeSymbolSummaryKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[symbol.ID]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]symbol.ID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "map:%d=", key)
		writeSummaryKeyDigest(h, values[key])
	}
}

func writeIdentitySummaryKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[identity.ID]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]identity.ID, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b identity.ID) int {
		if a.Kind != b.Kind {
			if a.Kind < b.Kind {
				return -1
			}
			return 1
		}
		if a.Site != b.Site {
			if a.Site < b.Site {
				return -1
			}
			return 1
		}
		if a.Index < b.Index {
			return -1
		}
		if a.Index > b.Index {
			return 1
		}
		return 0
	})
	for _, key := range keys {
		fmt.Fprintf(h, "id:%s/%s/%d=", key.Kind, key.Site, key.Index)
		writeSummaryKeyDigest(h, values[key])
	}
}

func writeCalleePathKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[factflow.CalleePathKey]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]factflow.CalleePathKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "path:%s=", key)
		writeSummaryKeyDigest(h, values[key])
	}
}

func writeCalleePathMultiKeySetDigest(h interface{ Write([]byte) (int, error) }, values map[factflow.CalleePathKey][]summary.SummaryKey) {
	if len(values) == 0 {
		return
	}
	keys := make([]factflow.CalleePathKey, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		fmt.Fprintf(h, "multi:%s=", key)
		summaryKeys := append([]summary.SummaryKey(nil), values[key]...)
		slices.SortFunc(summaryKeys, func(a, b summary.SummaryKey) int {
			if a.Less(b) {
				return -1
			}
			if b.Less(a) {
				return 1
			}
			return 0
		})
		for _, summaryKey := range summaryKeys {
			writeSummaryKeyDigest(h, summaryKey)
		}
	}
}

func writeSummaryKeyDigest(h interface{ Write([]byte) (int, error) }, key summary.SummaryKey) {
	if key.Ref.Kind == ref.KindCFG {
		panic("program: process-local CFG function reference cannot cross a digest/artifact boundary")
	}
	fmt.Fprintf(
		h,
		"%d/%d/%d/%d/%d;",
		key.Ref.Kind,
		key.Ref.ID,
		key.Entry.Values,
		key.Entry.Facts,
		key.Entry.References,
	)
}

func collectKeys(bindings *bind.Result, root summary.SummaryKey, reg *axis.Registry, external typeannotation.Resolver, moduleExports importlookup.Source, stmts ...[]ast.Stmt) programKeys {
	metatableContext := collectMetatableMethodContext(bindings, external, moduleExports, stmts...)
	out := programKeys{
		rootKey:                  root,
		functionByKey:            make(map[summary.SummaryKey]*ast.FunctionExpr),
		functionKeys:             make(map[symbol.ID]summary.SummaryKey),
		functionIDs:              make(map[identity.ID]summary.SummaryKey),
		targetKeys:               make(map[symbol.ID]summary.SummaryKey),
		pathKeys:                 make(map[factflow.CalleePathKey]summary.SummaryKey),
		pathMultiKeys:            make(map[factflow.CalleePathKey][]summary.SummaryKey),
		functionTypes:            make(map[summary.SummaryKey]*typ.Function),
		contexts:                 newContextIndex(),
		metatableProof:           metatableContext.proof,
		metatableMethodReceivers: metatableContext.methodReceivers,
		metatableSeedReceivers:   metatableContext.seedReceivers,
		bindings:                 bindings,
	}
	if bindings == nil {
		return out
	}
	pathTargets := collectFunctionPathTargets(bindings, stmts...)
	ambiguousPathKeys := make(map[factflow.CalleePathKey]struct{})
	bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.Symbol == 0 || origin.Func == nil {
			return true
		}
		key := summary.DefaultSummaryKey(ref.FromSymbol(origin.Symbol))
		out.functions = append(out.functions, keyedFunction{funcExpr: origin.Func, key: key})
		out.functionByKey[key] = origin.Func
		out.functionKeys[origin.Symbol] = key
		out.functionIDs[identity.LuaFunction(uint64(origin.Symbol))] = key
		if fnType, ok := lowerFunctionOriginType(origin, bindings, external, out.metatableProof); ok {
			out.functionTypes[key] = fnType
		}
		if origin.HasTargetSymbol && origin.TargetSymbol != 0 && functionTargetCanUseDirectSymbolKey(bindings, origin.TargetSymbol) {
			out.targetKeys[origin.TargetSymbol] = key
		}
		targetPath, hasTargetPath := pathTargets[origin.Func]
		if !hasTargetPath && origin.HasTargetSymbol && origin.TargetSymbol != 0 {
			targetPath = path.NewPath(origin.TargetSymbol, bindings.Name(origin.TargetSymbol))
			hasTargetPath = true
		}
		if hasTargetPath && (!origin.HasTargetSymbol || functionTargetCanUseStaticPathKey(bindings, origin.TargetSymbol)) {
			pathKey, ok := factflow.CalleePathKeyFromPath(targetPath)
			if !ok {
				return true
			}
			if existing, seen := out.pathKeys[pathKey]; seen && existing != key {
				ambiguousPathKeys[pathKey] = struct{}{}
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], existing)
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], key)
			} else {
				out.pathMultiKeys[pathKey] = appendSummaryKeyUnique(out.pathMultiKeys[pathKey], key)
			}
			out.pathKeys[pathKey] = key
		}
		return true
	})
	// A path bound to more than one function definition is not a sound static
	// callee target: the call resolves through the current value identity instead.
	for pathKey := range ambiguousPathKeys {
		delete(out.pathKeys, pathKey)
	}
	applyMetatableMethodReceiverEntryStates(&out, bindings, reg, external, moduleExports, stmts...)
	return out
}

func functionTargetCanUseDirectSymbolKey(bindings *bind.Result, target symbol.ID) bool {
	if bindings == nil || target == 0 {
		return false
	}
	kind, ok := bindings.Kind(target)
	return ok && kind != symbol.Global
}

func functionTargetCanUseStaticPathKey(bindings *bind.Result, target symbol.ID) bool {
	if bindings == nil || target == 0 {
		return false
	}
	kind, ok := bindings.Kind(target)
	if !ok {
		return false
	}
	return kind != symbol.Global || len(bindings.WriteIdents(target)) <= 1
}

func appendSummaryKeyUnique(keys []summary.SummaryKey, key summary.SummaryKey) []summary.SummaryKey {
	for _, existing := range keys {
		if existing == key {
			return keys
		}
	}
	return append(keys, key)
}

func collectFunctionPathTargets(bindings *bind.Result, roots ...[]ast.Stmt) map[*ast.FunctionExpr]path.Path {
	if bindings == nil {
		return nil
	}
	out := make(map[*ast.FunctionExpr]path.Path)
	for _, stmts := range roots {
		collectFunctionPathTargetsInStmts(out, bindings, stmts)
	}
	bindings.ForEachFunctionOrigin(func(origin bind.FunctionOrigin) bool {
		if origin.Func == nil {
			return true
		}
		collectFunctionPathTargetsInStmts(out, bindings, origin.Func.Stmts)
		return true
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectFunctionPathTargetsInStmts(out map[*ast.FunctionExpr]path.Path, bindings *bind.Result, stmts []ast.Stmt) {
	for _, stmt := range stmts {
		switch stmt := stmt.(type) {
		case *ast.LocalAssignStmt:
			symbols := bindings.LocalSymbols(stmt)
			for i, expr := range stmt.Exprs {
				if i >= len(symbols) || symbols[i] == 0 {
					continue
				}
				root := path.NewPath(symbols[i], bindings.Name(symbols[i]))
				collectFunctionPathTargetsInExpr(out, root, expr)
			}
		case *ast.AssignStmt:
			for i, expr := range stmt.Rhs {
				if i >= len(stmt.Lhs) {
					continue
				}
				target, ok := pathexpr.Resolve(stmt.Lhs[i], bindings)
				if !ok || target.IsEmpty() {
					continue
				}
				collectFunctionPathTargetsInExpr(out, target, expr)
			}
		case *ast.FuncDefStmt:
			target, ok := pathexpr.ResolveFuncName(stmt.Name, bindings)
			if ok && !target.IsEmpty() && stmt.Func != nil {
				out[stmt.Func] = target
			}
		case *ast.DoBlockStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.IfStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Then)
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Else)
		case *ast.WhileStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.RepeatStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.NumberForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		case *ast.GenericForStmt:
			collectFunctionPathTargetsInStmts(out, bindings, stmt.Stmts)
		}
	}
}

func collectFunctionPathTargetsInExpr(out map[*ast.FunctionExpr]path.Path, root path.Path, expr ast.Expr) {
	if root.IsEmpty() {
		return
	}
	expr = unwrapFunctionValueTarget(expr)
	switch expr := expr.(type) {
	case *ast.FunctionExpr:
		out[expr] = root
	case *ast.TableExpr:
		collectFunctionPathTargetsInTable(out, root, expr)
	}
}

func collectFunctionPathTargetsInTable(out map[*ast.FunctionExpr]path.Path, root path.Path, table *ast.TableExpr) {
	if table == nil {
		return
	}
	arrayIndex := 0
	for _, field := range table.Fields {
		suffix, ok := pathexpr.ResolveTableFieldSuffix(field, &arrayIndex)
		if !ok {
			continue
		}
		if !suffix.CanNameSummaryPath() {
			continue
		}
		target := appendPath(root, suffix.Path)
		collectFunctionPathTargetsInExpr(out, target, field.Value)
	}
}

func unwrapFunctionValueTarget(expr ast.Expr) ast.Expr {
	for {
		switch wrapped := expr.(type) {
		case *ast.CastExpr:
			expr = wrapped.Expr
		case *ast.NonNilAssertExpr:
			expr = wrapped.Expr
		default:
			return expr
		}
	}
}

func appendPath(root path.Path, suffix path.Path) path.Path {
	return root.AppendSegments(suffix.Segments)
}

func rootKey(configured summary.SummaryKey) summary.SummaryKey {
	if !configured.Ref.IsZero() {
		return configured
	}
	return summary.DefaultSummaryKey(ref.Root())
}
