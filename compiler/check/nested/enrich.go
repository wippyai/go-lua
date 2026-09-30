package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/assign"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/mutator"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CollectCapturedFieldAssignments scans a nested function's graph for field assignments
// to captured variables.
//
// When a nested function assigns fields to a captured variable (e.g., `parent.field = v`),
// those assignments affect the type visible in the parent scope. This function collects
// such assignments for propagation back to the parent.
func CollectCapturedFieldAssignments(
	graph *cfg.Graph,
	capturedSyms map[cfg.SymbolID]bool,
	synth func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID]map[string]typ.Type {
	if graph == nil || len(capturedSyms) == 0 {
		return make(map[cfg.SymbolID]map[string]typ.Type)
	}
	return assign.CollectFieldAssignments(graph, synth, capturedSyms)
}

// CollectCapturedContainerMutations scans a nested function's graph for container mutations
// (e.g., channel.send) that target captured variables.
func CollectCapturedContainerMutations(
	graph *cfg.Graph,
	capturedSyms map[cfg.SymbolID]bool,
	synth func(ast.Expr, cfg.Point) typ.Type,
) map[cfg.SymbolID][]api.ContainerMutation {
	result := make(map[cfg.SymbolID][]api.ContainerMutation)
	if graph == nil || len(capturedSyms) == 0 {
		return result
	}

	bindings := graph.Bindings()
	graph.EachCallSite(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		ceu := mutator.ContainerMutatorFromCall(info, p, synth, nil, nil, graph, bindings, nil)
		if ceu == nil {
			return
		}

		targetExpr := callsite.RuntimeArgAt(info, ceu.Container.Index)
		valueExpr := callsite.RuntimeArgAt(info, ceu.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			return
		}

		targetPath := flowpath.FromExprWithBindings(targetExpr, nil, bindings)
		if targetPath.IsEmpty() || targetPath.Symbol == 0 {
			return
		}
		if !capturedSyms[targetPath.Symbol] {
			return
		}

		var valueType typ.Type
		if synth != nil {
			valueType = synth(valueExpr, p)
		}
		if valueType == nil {
			valueType = typ.Unknown
		}
		valueType = subtype.WidenForInference(valueType)

		segs := make([]constraint.Segment, len(targetPath.Segments))
		copy(segs, targetPath.Segments)
		result[targetPath.Symbol] = append(result[targetPath.Symbol], api.ContainerMutation{
			Segments:  segs,
			ValueType: valueType,
		})
	})

	return result
}

// EnrichSelfTypeWithConstructorFields merges constructor instance fields into a self-type.
//
// When a method is defined on a class that has a constructor, the self-type should
// include fields assigned in the constructor. This function looks up constructor
// fields for the class and merges them into the self-type.
//
// This enables the type checker to recognize instance fields in methods:
//
//	function T.new()
//	    local self = setmetatable({}, T)
//	    self.name = ""  -- Collected as constructor field
//	    return self
//	end
//	function T:greet()
//	    print(self.name)  -- self.name is recognized because of constructor fields
//	end
func EnrichSelfTypeWithConstructorFields(selfType typ.Type, classSymbol cfg.SymbolID, store Store) typ.Type {
	if selfType == nil || store == nil || classSymbol == 0 {
		return selfType
	}

	fields := store.LookupConstructorFields(classSymbol)
	if len(fields) == 0 {
		return selfType
	}

	return mergeFieldsIntoSelfType(selfType, fields)
}

// NormalizeMethodSelfType widens literal-heavy self shapes so method-local
// flow constraints do not treat mutable receiver state as compile-time constants.
func NormalizeMethodSelfType(selfType typ.Type) typ.Type {
	if selfType == nil {
		return nil
	}
	return subtype.WidenForInference(selfType)
}

// NormalizeClassTableType widens the class table type as NormalizeMethodSelfType
// does, except that a stable field keeps its own literal type. Literals nested
// inside a field value stay widened because references to that value are not
// tracked.
func NormalizeClassTableType(tableType typ.Type, mutation cfg.TableMutation) typ.Type {
	widened := NormalizeMethodSelfType(tableType)
	rec, ok := tableType.(*typ.Record)
	if !ok {
		return widened
	}
	wideRec, ok := widened.(*typ.Record)
	if !ok || len(wideRec.Fields) != len(rec.Fields) {
		return widened
	}
	fields := append([]typ.Field(nil), wideRec.Fields...)
	kept := false
	for i, f := range rec.Fields {
		if mutation.FieldStable(f.Name) && fields[i].Name == f.Name && typ.TypeEquals(fields[i].Type, subtype.Widen(f.Type)) {
			fields[i].Type = f.Type
			kept = true
		}
	}
	if !kept {
		return widened
	}
	return wideRec.WithChildren(fields, wideRec.Metatable, wideRec.MapKey, wideRec.MapValue)
}

// NormalizeCapturedTableType widens the fields of a captured table that code
// can change after the closure observes them, so the closure reads every value
// such a field may hold. Widening bounds preserve declared slots and widen
// inferred literals; stable fields keep their observed types.
func NormalizeCapturedTableType(tableType typ.Type, mutation cfg.TableMutation, boundType typ.Type) typ.Type {
	rec, ok := tableType.(*typ.Record)
	if !ok {
		return tableType
	}
	fields := append([]typ.Field(nil), rec.Fields...)
	bound, _ := unwrap.Optional(boundType).(*typ.Record)
	changed := false
	for i, f := range fields {
		if mutation.FieldStable(f.Name) {
			continue
		}
		if bound != nil {
			if slot := bound.GetField(f.Name); slot != nil {
				// Structural equality does not preserve an alias's widening bound.
				if f.Type != slot.Type || f.Optional != slot.Optional {
					fields[i] = *slot
					changed = true
				}
				continue
			}
		}
		if widened := subtype.WidenForInference(f.Type); widened != f.Type {
			fields[i].Type = widened
			changed = true
		}
	}
	if !changed {
		return tableType
	}
	return rec.WithChildren(fields, rec.Metatable, rec.MapKey, rec.MapValue)
}

func mergeFieldsIntoSelfType(selfType typ.Type, fields map[string]typ.Type) typ.Type {
	if len(fields) == 0 {
		return selfType
	}

	switch v := selfType.(type) {
	case *typ.Record:
		builder := v.Builder()

		existingFields := make(map[string]bool)
		for _, f := range v.Fields {
			existingFields[f.Name] = true
		}

		for name, t := range fields {
			if !existingFields[name] {
				builder.Field(name, t)
			}
		}

		return builder.Build()

	case *typ.Interface:
		builder := typ.NewRecord().SetOpen(true)
		existingFields := make(map[string]bool)
		for _, m := range v.Methods {
			builder.Field(m.Name, m.Type)
			existingFields[m.Name] = true
		}
		for name, t := range fields {
			if !existingFields[name] {
				builder.Field(name, t)
			}
		}
		return builder.Build()

	default:
		return selfType
	}
}
