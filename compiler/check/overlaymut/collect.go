package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// MapWriteInfo holds key and value types for dynamic map writes.
type MapWriteInfo struct {
	KeyType   typ.Type
	ValueType typ.Type
}

// SymbolFilter is the minimal root-symbol membership query for filtered
// overlay mutation collection.
type SymbolFilter interface {
	Contains(cfg.SymbolID) bool
}

// CollectFieldAssignments reduces transfer assignment evidence into typed field
// assignments grouped by base symbol.
// The synth function is used to synthesize field value types.
// If filter is non-nil, only symbols in the filter are collected.
func CollectFieldAssignments(
	assignments []api.AssignmentEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filter SymbolFilter,
) FieldAssignments {
	result := make(FieldAssignments)
	if len(assignments) == 0 {
		return result
	}

	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
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

			if sym == 0 || fieldName == "" {
				continue
			}
			if filter != nil && !filter.Contains(sym) {
				continue
			}
			fieldKey, ok := fieldkey.FromName(fieldName)
			if !ok {
				continue
			}

			var fieldType typ.Type
			if source != nil && synth != nil {
				fieldType = synth(source, p)
			}
			if fieldType == nil {
				fieldType = typ.Unknown
			}

			if result[sym] == nil {
				result[sym] = make(fieldkey.Values)
			}
			fieldValue := product.FromType(fieldType)
			if existing := result[sym][fieldKey]; !existing.IsZero() {
				result[sym][fieldKey] = joinFieldValue(existing, fieldValue)
			} else {
				result[sym][fieldKey] = fieldValue
			}
		}
	}

	return result
}

// CollectFunctionFieldAssignments reduces function field/method definitions
// into the same field-assignment product as explicit table writes. Function
// definitions are separate graph events, but semantically:
//
//	function t.run(...) ... end
//	function t:run(...) ... end
//
// both publish a function value at t.run for return/capture overlays.
func CollectFunctionFieldAssignments(
	functions []api.FunctionDefinitionEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filter SymbolFilter,
) FieldAssignments {
	result := make(FieldAssignments)
	if len(functions) == 0 {
		return result
	}
	for _, def := range functions {
		info := def.FuncDef
		if info == nil || info.FuncExpr == nil {
			continue
		}
		if info.TargetKind != cfg.FuncDefField && info.TargetKind != cfg.FuncDefMethod {
			continue
		}
		target := info.TargetPath
		if target.Symbol == 0 || len(target.Segments) != 1 {
			continue
		}
		seg := target.Segments[0]
		if seg.Name == "" {
			continue
		}
		if filter != nil && !filter.Contains(target.Symbol) {
			continue
		}
		fieldKey, ok := fieldkey.FromName(seg.Name)
		if !ok {
			continue
		}
		fieldType := typ.Type(nil)
		if synth != nil {
			fieldType = synth(info.FuncExpr, def.Nested.Point)
		}
		if fieldType == nil {
			fieldType = typ.Unknown
		}
		if result[target.Symbol] == nil {
			result[target.Symbol] = make(fieldkey.Values)
		}
		fieldValue := product.FromType(fieldType)
		if existing := result[target.Symbol][fieldKey]; !existing.IsZero() {
			result[target.Symbol][fieldKey] = joinFieldValue(existing, fieldValue)
		} else {
			result[target.Symbol][fieldKey] = fieldValue
		}
	}
	return result
}

// CollectMapWriteAssignments reduces transfer assignment evidence for map writes.
// Returns a map: symbolID -> []MapWriteInfo representing dynamic writes to each symbol.
func CollectMapWriteAssignments(
	assignments []api.AssignmentEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	filter SymbolFilter,
) map[cfg.SymbolID][]MapWriteInfo {
	result := make(map[cfg.SymbolID][]MapWriteInfo)
	if len(assignments) == 0 {
		return result
	}

	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
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
			if filter != nil && !filter.Contains(sym) {
				continue
			}

			// Skip string literal keys (handled by field assignments)
			if _, ok := target.Key.(*ast.StringExpr); ok {
				continue
			}

			var keyType typ.Type
			switch k := target.Key.(type) {
			case *ast.IdentExpr:
				if synth != nil {
					keyType = synth(k, p)
				}
			case *ast.NumberExpr:
				keyType = typ.Integer
			default:
				if synth != nil && target.Key != nil {
					keyType = synth(target.Key, p)
				}
			}
			keyType = canonicalDynamicKeyType(keyType)

			var valType typ.Type
			if source != nil && synth != nil {
				valType = synth(source, p)
			}
			if valType == nil {
				valType = typ.Unknown
			}

			result[sym] = append(result[sym], MapWriteInfo{
				KeyType:   keyType,
				ValueType: valType,
			})
		}
	}

	return result
}

func canonicalDynamicKeyType(keyType typ.Type) typ.Type {
	if keyType == nil || keyType.Kind().IsPlaceholder() {
		return typ.String
	}
	return keyType
}

// MergeMapWriteMutations merges table.insert-derived map effects into dynamic map writes.
func MergeMapWriteMutations(
	mapWrites map[cfg.SymbolID][]MapWriteInfo,
	mutations map[cfg.SymbolID][]MapWriteInfo,
) {
	for sym, infos := range mutations {
		mapWrites[sym] = append(mapWrites[sym], infos...)
	}
}
