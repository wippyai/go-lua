package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// CollectFieldAssignments scans the graph for field assignments and groups them by base symbol.
// Returns a map: symbolID -> map[fieldName]typ.Type representing fields assigned to each symbol.
// The synth function is used to synthesize field value types.
// If filterSyms is non-nil, only symbols in the filter are collected.
func CollectFieldAssignments(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
	if graph == nil {
		return result
	}
	record := func(sym cfg.SymbolID, name string, fieldType typ.Type) {
		if sym == 0 || name == "" || filterSyms != nil && !filterSyms[sym] {
			return
		}
		if fieldType == nil {
			fieldType = typ.Unknown
		}
		if result[sym] == nil {
			result[sym] = make(map[string]typ.Type)
		}
		if existing := result[sym][name]; existing != nil {
			result[sym][name] = typ.NewUnion(existing, fieldType)
		} else {
			result[sym][name] = fieldType
		}
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		sources := info.Sources
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(sources) {
				source = sources[i]
			}
			var sym cfg.SymbolID
			var fieldName string

			switch target.Kind {
			case cfg.TargetField:
				if target.BaseSymbol != 0 && len(target.FieldPath) == 1 {
					sym = target.BaseSymbol
					fieldName = target.FieldPath[0]
				}
			case cfg.TargetIndex:
				if target.BaseSymbol != 0 && target.Key != nil {
					if strKey, ok := target.Key.(*ast.StringExpr); ok && strKey.Value != "" {
						sym = target.BaseSymbol
						fieldName = strKey.Value
					}
				}
			}

			if sym == 0 || fieldName == "" || filterSyms != nil && !filterSyms[sym] {
				continue
			}

			var fieldType typ.Type
			if source != nil && synth != nil {
				fieldType = synth(source, p)
			}
			record(sym, fieldName, fieldType)
		}
	})
	graph.EachFuncDef(func(p cfg.Point, info *cfg.FuncDefInfo) {
		if info == nil || info.FuncExpr == nil || len(info.TargetPath.Segments) != 1 || info.TargetPath.Segments[0].Kind != constraint.SegmentField {
			return
		}
		sym := info.TargetPath.Symbol
		name := info.TargetPath.Segments[0].Name
		if sym == 0 || name == "" || filterSyms != nil && !filterSyms[sym] {
			return
		}
		var fieldType typ.Type
		if synth != nil {
			fieldType = synth(info.FuncExpr, p)
		}
		record(sym, name, fieldType)
	})

	return result
}

// CollectIndexerAssignments scans the graph for dynamic index assignments (t[k] = v where k is non-const).
// Returns a map: symbolID -> []IndexerInfo representing index assignments to each symbol.
func CollectIndexerAssignments(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID][]mutator.IndexerInfo {
	result := make(map[cfg.SymbolID][]mutator.IndexerInfo)
	if graph == nil {
		return result
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		sources := info.Sources
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(sources) {
				source = sources[i]
			}
			if target.Kind != cfg.TargetIndex {
				continue
			}
			sym := target.BaseSymbol
			if sym == 0 {
				continue
			}
			if filterSyms != nil && !filterSyms[sym] {
				continue
			}

			// Skip string literal keys (handled by field assignments)
			if _, ok := target.Key.(*ast.StringExpr); ok {
				continue
			}

			valType := assignedValueType(source, p, synth)
			if evolved := evolvingIndexedAliasValue(graph, bindings, sym, p, source, synth); evolved != nil {
				valType = evolved
			}
			result[sym] = append(result[sym], mutator.IndexerInfo{
				KeyType: dynamicKeyType(target.Key, p, synth),
				ValType: valType,
			})
		}
	})

	return result
}

// When a fresh map stores a local list and that list is extended later, the
// map entry still refers to the extended list. Recognize the self-contained
// map lookup-or-empty-list cycle and summarize the later indexed writes.
func evolvingIndexedAliasValue(graph *cfg.Graph, bindings *bind.BindingTable, mapSym cfg.SymbolID, storePoint cfg.Point, source ast.Expr, synth func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	ident, ok := source.(*ast.IdentExpr)
	if !ok || bindings == nil || !bindings.EmptyFreshTablePath(mapSym, nil) {
		return nil
	}
	localSym, ok := bindings.SymbolOf(ident)
	if !ok || localSym == 0 {
		return nil
	}
	var rootAssignments, mapWrites int
	validOrigin := false
	escaped := false
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		for i, target := range info.Targets {
			var assigned ast.Expr
			if i < len(info.Sources) {
				assigned = info.Sources[i]
			}
			if assigned != source && (carriesTableAlias(assigned, bindings, mapSym) || carriesTableAlias(assigned, bindings, localSym)) {
				escaped = true
			}
			if callReceivesTableAlias(assigned, bindings, mapSym, localSym) {
				escaped = true
			}
			if target.Kind == cfg.TargetIndex && target.BaseSymbol == mapSym {
				mapWrites++
			}
			if target.Kind != cfg.TargetIdent || target.Symbol != localSym {
				continue
			}
			rootAssignments++
			if !info.IsLocal || i >= len(info.Sources) || i < len(info.TypeAnnotations) && info.TypeAnnotations[i] != nil {
				continue
			}
			choice, ok := info.Sources[i].(*ast.LogicalOpExpr)
			if !ok || choice.Operator != "or" {
				continue
			}
			if _, ok := choice.Rhs.(*ast.TableExpr); !ok {
				continue
			}
			lookup, ok := choice.Lhs.(*ast.AttrGetExpr)
			if !ok {
				continue
			}
			base, ok := lookup.Object.(*ast.IdentExpr)
			if !ok {
				continue
			}
			baseSym, ok := bindings.SymbolOf(base)
			validOrigin = ok && baseSym == mapSym
		}
	})
	graph.EachStmtCall(func(_ cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}
		for _, arg := range info.Args {
			if carriesTableAlias(arg, bindings, mapSym) || carriesTableAlias(arg, bindings, localSym) {
				escaped = true
			}
		}
		if carriesTableAlias(info.Receiver, bindings, mapSym, localSym) {
			escaped = true
		}
	})
	graph.EachReturn(func(_ cfg.Point, info *cfg.ReturnInfo) {
		for _, expr := range info.Exprs {
			if carriesTableAlias(expr, bindings, mapSym, localSym) || callReceivesTableAlias(expr, bindings, mapSym, localSym) {
				escaped = true
			}
		}
	})
	if escaped || !validOrigin || rootAssignments != 1 || mapWrites != 1 {
		return nil
	}
	var element typ.Type
	var explicitNil bool
	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if !graph.Reachable(storePoint, p, false) {
			return
		}
		for i, target := range info.Targets {
			if target.Kind != cfg.TargetIndex || target.BaseSymbol != localSym || !numericIndexedKey(dynamicKeyType(target.Key, p, synth)) || i >= len(info.Sources) {
				continue
			}
			written := assignedValueType(info.Sources[i], p, synth)
			present := narrow.RemoveNil(written)
			if !typ.TypeEquals(present, written) {
				explicitNil = true
			}
			if present != nil && present.Kind() != kind.Never {
				element = typ.JoinPreferNonSoft(element, present)
			}
		}
	})
	if element == nil {
		return nil
	}
	array := typ.NewInferredArray(element)
	if explicitNil {
		return array.WithExplicitNilWrite()
	}
	return array
}

func callReceivesTableAlias(expr ast.Expr, bindings *bind.BindingTable, symbols ...cfg.SymbolID) bool {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		if carriesTableAlias(e.Receiver, bindings, symbols...) {
			return true
		}
		for _, arg := range e.Args {
			if carriesTableAlias(arg, bindings, symbols...) || callReceivesTableAlias(arg, bindings, symbols...) {
				return true
			}
		}
	case *ast.LogicalOpExpr:
		return callReceivesTableAlias(e.Lhs, bindings, symbols...) || callReceivesTableAlias(e.Rhs, bindings, symbols...)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil && callReceivesTableAlias(field.Value, bindings, symbols...) {
				return true
			}
		}
	case *ast.CastExpr:
		return callReceivesTableAlias(e.Expr, bindings, symbols...)
	case *ast.NonNilAssertExpr:
		return callReceivesTableAlias(e.Expr, bindings, symbols...)
	}
	return false
}

// carriesTableAlias recognizes expressions that can retain a table reference.
// Scalar operations such as #table and table[index] do not expose the table.
func carriesTableAlias(expr ast.Expr, bindings *bind.BindingTable, symbols ...cfg.SymbolID) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		sym, ok := bindings.SymbolOf(e)
		if !ok {
			return false
		}
		for _, candidate := range symbols {
			if sym == candidate {
				return true
			}
		}
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil && carriesTableAlias(field.Value, bindings, symbols...) {
				return true
			}
		}
	case *ast.CastExpr:
		return carriesTableAlias(e.Expr, bindings, symbols...)
	case *ast.NonNilAssertExpr:
		return carriesTableAlias(e.Expr, bindings, symbols...)
	case *ast.LogicalOpExpr:
		return carriesTableAlias(e.Lhs, bindings, symbols...) || carriesTableAlias(e.Rhs, bindings, symbols...)
	}
	return false
}

func numericIndexedKey(t typ.Type) bool {
	return t != nil && subtype.IsSubtype(t, typ.Number)
}

// CollectNestedFieldWrites scans the graph for writes into tables that
// targets reach by static fields: t.a.x = v, t.a["x"] = v and t.a[k] = v.
// Writes to the target table itself are collected by CollectFieldAssignments
// and CollectIndexerAssignments. Writes by dynamic keys are recorded under
// flow.IndexerWriteField as the map {[K]: V} they add.
func CollectNestedFieldWrites(
	graph *cfg.Graph,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	targets map[cfg.SymbolID]bool,
) map[cfg.SymbolID]api.FieldWriteSet {
	result := make(map[cfg.SymbolID]api.FieldWriteSet)
	if graph == nil || len(targets) == 0 {
		return result
	}
	add := func(sym cfg.SymbolID, key api.FieldWriteKey, t typ.Type) {
		set := result[sym]
		if set == nil {
			set = make(api.FieldWriteSet)
			result[sym] = set
		}
		set[key] = api.JoinFieldWrite(key, set[key], t)
	}

	graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for i, target := range info.Targets {
			var source ast.Expr
			if i < len(info.Sources) {
				source = info.Sources[i]
			}
			switch target.Kind {
			case cfg.TargetField:
				n := len(target.FieldPath)
				if n < 2 || !targets[target.BaseSymbol] {
					continue
				}
				segments := make([]constraint.Segment, n-1)
				for j, name := range target.FieldPath[:n-1] {
					segments[j] = constraint.Segment{Kind: constraint.SegmentField, Name: name}
				}
				add(target.BaseSymbol, api.NewFieldWriteKey(segments, target.FieldPath[n-1]), assignedValueType(source, p, synth))
			case cfg.TargetIndex:
				if target.Base == nil || target.Key == nil {
					continue
				}
				base := flowpath.FromExprWithBindings(target.Base, nil, bindings)
				if len(base.Segments) == 0 || !targets[base.Symbol] {
					continue
				}
				if strKey, ok := target.Key.(*ast.StringExpr); ok {
					if strKey.Value != "" {
						add(base.Symbol, api.NewFieldWriteKey(base.Segments, strKey.Value), assignedValueType(source, p, synth))
					}
					continue
				}
				if written := api.NewIndexerWrite(dynamicKeyType(target.Key, p, synth), assignedValueType(source, p, synth)); written != nil {
					add(base.Symbol, api.NewFieldWriteKey(base.Segments, flow.IndexerWriteField), written)
				}
			}
		}
	})

	return result
}

// dynamicKeyType returns the type of the dynamic key of an index write at p.
// A key whose type is not known stays unknown: it carries no evidence about
// the key domain of the written table.
func dynamicKeyType(key ast.Expr, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	var keyType typ.Type
	switch k := key.(type) {
	case *ast.NumberExpr:
		keyType = typ.Integer
	default:
		if synth != nil && k != nil {
			keyType = synth(k, p)
		}
	}
	if keyType == nil {
		return typ.Unknown
	}
	return keyType
}

// assignedValueType returns the type of the value source assigns at p.
func assignedValueType(source ast.Expr, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type) typ.Type {
	if source != nil && synth != nil {
		if t := synth(source, p); t != nil {
			return t
		}
	}
	return typ.Unknown
}
