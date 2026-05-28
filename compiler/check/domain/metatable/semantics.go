// Package metatable owns Lua metatable value semantics shared by synthesis and
// solved-state observation.
package metatable

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// IsSetMetatableCall reports whether ex is the Lua setmetatable primitive call.
//
// When bindings are provided, the callee must resolve to the genuine unshadowed
// global setmetatable; a local or parameter that rebinds the name is not the
// primitive. Without bindings the name match is the only available signal.
func IsSetMetatableCall(ex *ast.FuncCallExpr, bindings *bind.BindingTable) bool {
	if ex == nil || len(ex.Args) < 2 {
		return false
	}
	ident, ok := ex.Func.(*ast.IdentExpr)
	if !ok {
		return false
	}
	if bindings == nil {
		return ident.Value == "setmetatable"
	}
	return bindings.ResolvesToUnshadowedGlobal(ident, "setmetatable")
}

// With returns tableType with metaType attached as its metatable, following
// Lua's setmetatable value semantics. Non-record table values keep their
// original type because the structural metatable edge can only be represented
// on record products.
func With(tableType, metaType typ.Type) typ.Type {
	tableType = unwrap.Alias(tableType)
	if tableType == nil {
		return typ.Unknown
	}
	if zzSealDbg {
		println("ZZWITH table=", zzDump(tableType, 0), " meta=", zzDump(metaType, 0))
	}

	switch t := tableType.(type) {
	case *typ.Record:
		return recordWithMetatableVariants(t, metaType)
	case *typ.Optional:
		return typ.NewOptional(With(t.Inner, metaType))
	case *typ.Union:
		members := make([]typ.Type, 0, len(t.Members))
		for _, member := range t.Members {
			if member == nil || member.Kind() == kind.Nil {
				members = append(members, member)
				continue
			}
			members = append(members, With(member, metaType))
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
		switch {
		case field.Optional && field.Readonly:
			builder.OptReadonlyField(field.Name, field.Type)
		case field.Optional:
			builder.OptField(field.Name, field.Type)
		case field.Readonly:
			builder.ReadonlyField(field.Name, field.Type)
		default:
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
