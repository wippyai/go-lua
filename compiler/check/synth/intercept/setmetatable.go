package intercept

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// SetMetatableIntercept models Lua's setmetatable(table, metatable) primitive.
//
// The normal stdlib signature can express the value identity of the first
// argument, but the abstract state also needs the metatable edge on the returned
// table value so method/field queries see the prototype chain.
type SetMetatableIntercept struct{}

func (s *SetMetatableIntercept) InterceptCall(ex *ast.FuncCallExpr, ctx CallEnv) Result {
	if ex == nil || len(ex.Args) < 2 || ctx.Recurse == nil {
		return Result{}
	}
	ident, ok := ex.Func.(*ast.IdentExpr)
	if !ok || ident.Value != "setmetatable" {
		return Result{}
	}

	tableType := ctx.Recurse(ex.Args[0])
	metaType := ctx.Recurse(ex.Args[1])
	if ctx.StableType != nil {
		metaType = ctx.StableType(ex.Args[1], metaType)
	}
	if tableType == nil {
		return Result{Skip: true, Types: []typ.Type{typ.Unknown}}
	}

	return Result{Skip: true, Types: []typ.Type{withMetatable(tableType, metaType)}}
}

func withMetatable(tableType, metaType typ.Type) typ.Type {
	tableType = unwrap.Alias(tableType)
	if tableType == nil {
		return typ.Unknown
	}

	switch t := tableType.(type) {
	case *typ.Record:
		return recordWithMetatableVariants(t, metaType)
	case *typ.Optional:
		return typ.NewOptional(withMetatable(t.Inner, metaType))
	case *typ.Union:
		members := make([]typ.Type, 0, len(t.Members))
		for _, member := range t.Members {
			if member == nil || member.Kind() == kind.Nil {
				members = append(members, member)
				continue
			}
			members = append(members, withMetatable(member, metaType))
		}
		return typ.NewUnion(members...)
	default:
		return tableType
	}
}

func recordWithMetatableVariants(rec *typ.Record, metaType typ.Type) typ.Type {
	var variants []typ.Type
	for _, meta := range metatableVariants(metaType) {
		variants = append(variants, rebuildRecordWithMetatable(rec, meta))
	}
	if len(variants) == 0 {
		return rebuildRecordWithMetatable(rec, nil)
	}
	if len(variants) == 1 {
		return variants[0]
	}
	return typ.NewUnion(variants...)
}

func metatableVariants(metaType typ.Type) []typ.Type {
	metaType = unwrap.Alias(metaType)
	if metaType == nil {
		return []typ.Type{nil}
	}
	switch m := metaType.(type) {
	case *typ.Optional:
		return []typ.Type{nil, unwrap.Alias(m.Inner)}
	case *typ.Union:
		var variants []typ.Type
		hasNil := false
		for _, member := range m.Members {
			member = unwrap.Alias(member)
			if member == nil || member.Kind() == kind.Nil {
				if !hasNil {
					variants = append(variants, nil)
					hasNil = true
				}
				continue
			}
			variants = append(variants, member)
		}
		return variants
	default:
		if metaType.Kind() == kind.Nil {
			return []typ.Type{nil}
		}
		return []typ.Type{metaType}
	}
}

func rebuildRecordWithMetatable(rec *typ.Record, meta typ.Type) typ.Type {
	if rec == nil {
		return typ.Unknown
	}
	builder := typ.NewRecord()
	for _, field := range rec.Fields {
		if field.Optional {
			builder.OptField(field.Name, field.Type)
		} else {
			builder.Field(field.Name, field.Type)
		}
	}
	if rec.HasMapComponent() {
		builder.MapComponent(rec.MapKey, rec.MapValue)
	}
	if meta != nil {
		builder.Metatable(meta)
	}
	return builder.SetOpen(rec.Open).Build()
}
