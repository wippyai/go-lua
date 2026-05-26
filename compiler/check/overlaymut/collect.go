package overlaymut

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// MapMutatorInfo holds key and value types for map-write mutations.
type MapMutatorInfo struct {
	KeyType   typ.Type
	ValueType typ.Type
}

// CollectFieldAssignments reduces transfer assignment evidence into field assignments grouped by base symbol.
// Returns a map: symbolID -> map[fieldName]typ.Type representing fields assigned to each symbol.
// The synth function is used to synthesize field value types.
// If filterSyms is non-nil, only symbols in the filter are collected.
func CollectFieldAssignments(
	assignments []api.AssignmentEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
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
			if filterSyms != nil && !filterSyms[sym] {
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
				result[sym] = make(map[string]typ.Type)
			}
			if existing := result[sym][fieldName]; existing != nil {
				result[sym][fieldName] = value.JoinPrecise(existing, fieldType)
			} else {
				result[sym][fieldName] = fieldType
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
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID]map[string]typ.Type {
	result := make(map[cfg.SymbolID]map[string]typ.Type)
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
		if filterSyms != nil && !filterSyms[target.Symbol] {
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
			result[target.Symbol] = make(map[string]typ.Type)
		}
		if existing := result[target.Symbol][seg.Name]; existing != nil {
			result[target.Symbol][seg.Name] = value.JoinPrecise(existing, fieldType)
		} else {
			result[target.Symbol][seg.Name] = fieldType
		}
	}
	return result
}

// CollectMapMutatorAssignments reduces transfer assignment evidence for map writes.
// Returns a map: symbolID -> []MapMutatorInfo representing map mutations to each symbol.
func CollectMapMutatorAssignments(
	assignments []api.AssignmentEvidence,
	synth func(ast.Expr, cfg.Point) typ.Type,
	bindings *bind.BindingTable,
	filterSyms map[cfg.SymbolID]bool,
) map[cfg.SymbolID][]MapMutatorInfo {
	result := make(map[cfg.SymbolID][]MapMutatorInfo)
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
			if filterSyms != nil && !filterSyms[sym] {
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

			result[sym] = append(result[sym], MapMutatorInfo{
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

// MergeMapMutatorMutations merges table mutator-derived map effects into map mutations.
func MergeMapMutatorMutations(
	mapMutators map[cfg.SymbolID][]MapMutatorInfo,
	mutations map[cfg.SymbolID][]MapMutatorInfo,
) {
	for sym, infos := range mutations {
		mapMutators[sym] = append(mapMutators[sym], infos...)
	}
}
