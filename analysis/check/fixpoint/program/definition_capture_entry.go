package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ownerHasCapturedFunctionDefinitions(keys *programKeys, owner *ast.FunctionExpr) bool {
	if keys == nil || keys.bindings == nil {
		return false
	}
	for _, fn := range keys.functions {
		origin, ok := keys.bindings.FunctionOrigin(fn.funcExpr)
		if !ok || origin.Parent != owner || origin.Func == nil {
			continue
		}
		if len(keys.bindings.DirectCaptures(origin.Func)) != 0 && definitionEntryPointCandidate(origin) {
			return true
		}
	}
	return false
}

func applyDefinitionCaptureEntryStatesFromResult(keys *programKeys, owner *ast.FunctionExpr, result *body.Result, reg *axis.Registry) {
	if keys == nil || keys.bindings == nil || result == nil || reg == nil {
		return
	}
	entryKeys := result.KeySpace()
	for i := range keys.functions {
		fn := keys.functions[i].funcExpr
		if fn == nil || len(keys.bindings.DirectCaptures(fn)) == 0 {
			continue
		}
		origin, ok := keys.bindings.FunctionOrigin(fn)
		if !ok || origin.Parent != owner {
			continue
		}
		point, ok := definitionEntryPoint(result, origin)
		if !ok {
			continue
		}
		caller, ok := result.StateAtBoundary(point)
		if !ok {
			continue
		}
		entry := keys.functions[i].entryState
		if keys.functions[i].hasEntryState && keys.functions[i].entryKeys != nil {
			entry = entry.RekeyPathEvidence(keys.functions[i].entryKeys, entryKeys)
		}
		updated, seen := applyCapturedClosureEntryState(reg, entryKeys, keys.bindings, fn, caller, entry, captureValueReaderAt(result, point))
		if !seen {
			continue
		}
		keys.functions[i].entryState = updated
		keys.functions[i].entryKeys = entryKeys
		keys.functions[i].hasEntryState = true
	}
	applyEscapedClosureEntryStatesFromResult(keys, owner, result, reg)
}

func applyEscapedClosureEntryStatesFromResult(keys *programKeys, owner *ast.FunctionExpr, result *body.Result, reg *axis.Registry) {
	if keys == nil || keys.bindings == nil || result == nil || reg == nil {
		return
	}
	entryKeys := result.KeySpace()
	graph := result.Graph()
	if graph == nil {
		return
	}
	dom := dominance.ComputeImmediateDominatorInfo(graph)
	seenEscape := make(map[int]struct{})
	for _, point := range result.ReturnPoints() {
		caller, ok := result.StateAtBoundary(point)
		if !ok {
			continue
		}
		for i := range keys.functions {
			fn := keys.functions[i].funcExpr
			if fn == nil || len(keys.bindings.DirectCaptures(fn)) == 0 {
				continue
			}
			origin, ok := keys.bindings.FunctionOrigin(fn)
			if !ok || origin.Parent != owner {
				continue
			}
			if !functionEscapesAtReturnPoint(result, origin, point, dom) {
				continue
			}
			entry := keys.functions[i].entryState
			if keys.functions[i].hasEntryState && keys.functions[i].entryKeys != nil {
				entry = entry.RekeyPathEvidence(keys.functions[i].entryKeys, entryKeys)
			}
			updated, seen := applyCapturedClosureEntryState(reg, entryKeys, keys.bindings, fn, caller, entry, captureValueReaderAt(result, point))
			if !seen {
				continue
			}
			if _, ok := seenEscape[i]; ok {
				mergeContextEntry(reg, &keys.functions[i], entryKeys, updated)
			} else {
				keys.functions[i].entryState = updated
				keys.functions[i].entryKeys = entryKeys
				keys.functions[i].hasEntryState = true
				seenEscape[i] = struct{}{}
			}
		}
	}
}

func functionEscapesAtReturnPoint(result *body.Result, origin bind.FunctionOrigin, point cfg.Point, dom *dominance.ImmediateDominators) bool {
	if result == nil || origin.Func == nil {
		return false
	}
	if defPoint, ok := definitionEntryPoint(result, origin); ok && dom != nil && dom.Dominates(defPoint, point) {
		return true
	}
	if origin.Kind != bind.FunctionOriginLiteral || origin.Symbol == 0 {
		return false
	}
	sources, ok := result.ReturnValueSources(point)
	if !ok {
		return false
	}
	for _, source := range sources {
		if returnSourceContainsFunction(result, source, origin.Symbol, nil) {
			return true
		}
	}
	return false
}

func returnSourceContainsFunction(result *body.Result, source factflow.ValueSource, fn symbol.ID, active map[factflow.ExprRef]struct{}) bool {
	if result == nil || fn == 0 || !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	if found, ok := result.ExpressionFunction(source.ExprRef); ok && found == fn {
		return true
	}
	if _, seen := active[source.ExprRef]; seen {
		return false
	}
	if active == nil {
		active = make(map[factflow.ExprRef]struct{}, 1)
	}
	active[source.ExprRef] = struct{}{}
	defer delete(active, source.ExprRef)

	lit, ok := result.ObjectLiteralExpr(source.ExprRef)
	if !ok {
		return false
	}
	found := false
	lit.View().ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if returnSourceContainsFunction(result, entry.Source(), fn, active) {
			found = true
			return false
		}
		return true
	})
	return found
}

func definitionEntryPoint(result *body.Result, origin bind.FunctionOrigin) (cfg.Point, bool) {
	if result == nil || origin.Func == nil || !definitionEntryPointCandidate(origin) {
		return 0, false
	}
	graph := result.Graph()
	if graph == nil {
		return 0, false
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.FunctionDefinition(point); ok && fact.Func == origin.Func {
			return point, true
		}
		switch origin.Kind {
		case bind.FunctionOriginDeclaration, bind.FunctionOriginMethod:
			fact, ok := result.FunctionDefinition(point)
			if ok && fact.Func == origin.Func {
				return point, true
			}
		case bind.FunctionOriginLocalAssignment:
			fact, ok := result.LocalAssignment(point)
			if ok && fact.Stmt == origin.Stmt && fact.Index == origin.LocalIndex {
				return point, true
			}
		case bind.FunctionOriginLiteral:
			fact, ok := result.OrdinaryAssignment(point)
			if ok && fact.Value == origin.Func {
				return point, true
			}
		}
	}
	return 0, false
}

func definitionEntryPointCandidate(origin bind.FunctionOrigin) bool {
	switch origin.Kind {
	case bind.FunctionOriginDeclaration, bind.FunctionOriginMethod, bind.FunctionOriginLocalAssignment:
		return origin.Stmt != nil
	case bind.FunctionOriginLiteral:
		return true
	default:
		return false
	}
}

func applyCapturedUpvaluePathEntryState(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	caller state.State,
	entry state.State,
) (state.State, bool) {
	if reg == nil || ks == nil || bindings == nil || fn == nil {
		return entry, false
	}
	captures := bindings.DirectCaptures(fn)
	if len(captures) == 0 {
		return entry, false
	}
	bottom := product.Bottom(reg)
	out := entry
	edit := out.EditPathEvidence(reg)
	seen := false

	if snapshot := caller.PathRefinementsSnapshot(ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			rebased, ok := rebaseCapturedPathKey(pathKey, captures)
			if !ok {
				continue
			}
			edit.WritePathKey(ks, rebased, value)
			seen = true
		}
	}
	if snapshot := caller.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			rebased, ok := rebaseCapturedPathKey(pathKey, captures)
			if !ok {
				continue
			}
			edit.WritePathStaticMember(ks, rebased, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func rebaseCapturedPathKey(pathKey pathdom.PathKey, captures []bind.Capture) (pathdom.PathKey, bool) {
	local, ok := pathaddr.LocalPathFromKey(pathKey)
	if !ok || local.Symbol == 0 {
		return "", false
	}
	for _, capture := range captures {
		if capture.Captured != local.Symbol {
			continue
		}
		from := pathdom.Path{Symbol: local.Symbol, Version: local.Version}.Key()
		to := pathdom.Path{Root: capture.CapturedName, Symbol: capture.Captured, Version: 1}.Key()
		if from == to && pathaddr.PathKeyHasPrefix(pathKey, from) {
			return pathKey, true
		}
		rebased, ok := pathaddr.RebasePathKey(pathKey, from, to)
		if !ok || rebased == "" {
			return "", false
		}
		return rebased, true
	}
	return "", false
}
