package nested

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
)

// This file provides type enrichment utilities for self-type resolution.
//
// When a method is defined in a table literal, the function's literal signature
// (with inferred refinements and return types) may be more precise than the initially
// synthesized type. These utilities replace placeholder types with literal sigs.

// EnrichTableTypeWithFunctionLookup replaces method function types in a record
// with function types resolved by symbol.
//
// For table literals with method fields, the initially synthesized record may
// have function types without inferred return types. After analyzing the methods,
// canonical function types are available per symbol. This function updates the
// record with those more precise signatures.
func EnrichTableTypeWithFunctionLookup(
	rec *typ.Record,
	tableExpr *ast.TableExpr,
	graph *cfg.Graph,
	lookup func(cfg.SymbolID) typ.Type,
) typ.Type {
	if rec == nil || tableExpr == nil || graph == nil || lookup == nil {
		return rec
	}

	modified := false
	builder := typ.NewRecord()
	bindings := graph.Bindings()

	for _, f := range rec.Fields {
		fieldType := f.Type
		for _, tf := range tableExpr.Fields {
			if tf.Key == nil {
				continue
			}
			var keyName string
			switch k := tf.Key.(type) {
			case *ast.StringExpr:
				keyName = k.Value
			}
			if keyName != f.Name {
				continue
			}
			fnExpr, ok := tf.Value.(*ast.FunctionExpr)
			if !ok {
				continue
			}
			if bindings != nil {
				if sym, ok := bindings.FuncLitSymbol(fnExpr); ok {
					if t := lookup(sym); t != nil {
						fieldType = t
						modified = true
					}
				}
			}
		}
		if f.Optional {
			builder = builder.OptField(f.Name, fieldType)
		} else {
			builder = builder.Field(f.Name, fieldType)
		}
	}

	if !modified {
		return rec
	}

	if rec.Metatable != nil {
		builder = builder.Metatable(rec.Metatable)
	}
	if rec.HasMapComponent() {
		builder = builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	return builder.SetOpen(rec.Open).Build()
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
func EnrichSelfTypeWithConstructorFields(selfType typ.Type, fields map[string]typ.Type) typ.Type {
	if selfType == nil || len(fields) == 0 {
		return selfType
	}
	return mergeFieldsIntoSelfType(selfType, fields)
}

// MethodSelfTypeFromReceiverSurface constructs the instance-side self contract
// for an unannotated colon method. The receiver expression in `function T:m`
// names the prototype/class table, while calls pass an instance whose metatable
// delegates lookups to that table.
//
// The self contract is an open instance record whose metatable carries
// `__index` pointing at the receiver, so both `self.field` and `self:method`
// resolve through the prototype. Field resolution follows the metatable
// `__index` chain rather than the metatable's own fields, so the receiver must
// be reachable as `__index`; when T is already a metatable record exposing
// `__index` (the `T.__index = T` idiom) reuse it directly to avoid a redundant
// wrapper, otherwise synthesize the `{__index = T}` delegate. This surfaces a
// plain local table's data fields (`local T = {count = 0}` then `T:inc()`) as
// well as a prototype's inherited methods.
func MethodSelfTypeFromReceiverSurface(receiver typ.Type) typ.Type {
	if receiver == nil {
		return nil
	}
	meta := receiver
	if rec, ok := receiver.(*typ.Record); !ok || rec.GetField("__index") == nil {
		meta = typ.NewRecord().Field("__index", receiver).Build()
	}
	return typ.NewRecord().SetOpen(true).Metatable(meta).Build()
}

// NormalizeMethodSelfType widens literal-heavy self shapes so method-local
// flow constraints do not treat mutable receiver state as compile-time constants.
func NormalizeMethodSelfType(selfType typ.Type) typ.Type {
	if selfType == nil {
		return nil
	}
	return value.WidenForConvergence(selfType)
}

func mergeFieldsIntoSelfType(selfType typ.Type, fields map[string]typ.Type) typ.Type {
	if len(fields) == 0 {
		return selfType
	}

	switch v := selfType.(type) {
	case *typ.Record:
		builder := typ.NewRecord()
		if v.Open {
			builder.SetOpen(true)
		}

		existingFields := make(map[string]bool)
		for _, f := range v.Fields {
			fieldType := f.Type
			if constructorType := fields[f.Name]; constructorType != nil && (typ.IsAbsentOrUnknown(fieldType) || typ.IsAny(fieldType)) {
				fieldType = constructorType
			}
			if f.Optional {
				builder.OptField(f.Name, fieldType)
			} else {
				builder.Field(f.Name, fieldType)
			}
			existingFields[f.Name] = true
		}

		for name, t := range fields {
			if !existingFields[name] {
				builder.Field(name, t)
			}
		}

		if v.Metatable != nil {
			builder.Metatable(v.Metatable)
		}
		if v.HasMapComponent() {
			builder.MapComponent(v.MapKey, v.MapValue)
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
