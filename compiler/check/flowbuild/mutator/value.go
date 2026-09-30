package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	fbpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ValueSourceFromExpr records solve-time reads and the fallback shape of a
// published value without snapshotting extraction-time flow overlays.
func ValueSourceFromExpr(source ast.Expr, fallback typ.Type, p cfg.Point, bindings *bind.BindingTable, inputs *flow.Inputs) flow.ValueSource {
	value := flow.ValueSource{ValueType: fallback}
	if source == nil || bindings == nil || inputs == nil {
		return value
	}
	constResolver := predicate.BuildConstResolver(inputs, p)
	value.ValuePath = fbpath.FromExprWithBindings(source, constResolver, bindings)
	if value.ValuePath.HasSymbol() {
		return value
	}
	if attr, ok := source.(*ast.AttrGetExpr); ok {
		if base := fbpath.FromExprWithBindings(attr.Object, constResolver, bindings); base.HasSymbol() {
			value.MapElementSource = &flow.MapElementSource{MapPath: base}
			if key := fbpath.FromExprWithBindings(attr.Key, constResolver, bindings); key.HasSymbol() && len(key.Segments) == 0 {
				value.MapElementSource.KeySymbol = key.Symbol
				value.MapElementSource.KeyVar = key.Root
			}
		}
		return value
	}
	tbl, ok := source.(*ast.TableExpr)
	if !ok {
		return value
	}
	record, _ := unwrap.Alias(fallback).(*typ.Record)
	for _, field := range tbl.Fields {
		if field == nil {
			continue
		}
		if field.Key == nil {
			var element typ.Type
			switch shape := unwrap.Alias(fallback).(type) {
			case *typ.Array:
				element = shape.Element
			case *typ.Tuple:
				if i := len(value.ValueElements); i < len(shape.Elements) {
					element = shape.Elements[i]
				}
			}
			value.ValueElements = append(value.ValueElements, ValueSourceFromExpr(field.Value, element, p, bindings, inputs))
			continue
		}
		name := ast.KeyName(field.Key)
		if name == "" {
			continue
		}
		var fieldType typ.Type
		if record != nil {
			if f := record.GetField(name); f != nil {
				fieldType = f.Type
			}
		}
		value.ValueFields = append(value.ValueFields, flow.ValueFieldSource{
			Name:        name,
			ValueSource: ValueSourceFromExpr(field.Value, fieldType, p, bindings, inputs),
		})
	}
	return value
}
