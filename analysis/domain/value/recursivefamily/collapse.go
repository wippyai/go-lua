package recursivefamily

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// CollapseUnfoldingToFamily collapses a slot that is an unfolding of family into
// the family body. Every proper descendant subtree that embeds a reference to
// family is itself a deeper unfolding; rewriting each such immediate child to
// family closes a multi-level unfolding tower onto one recursion variable while
// keeping the root's own shape (its non-recursive fields and leaves) as evidence.
//
// A slot that is itself a bare reference to family has no body and is returned as
// family. The result references only the family handle at its recursion edges.
func CollapseUnfoldingToFamily(slot typ.Type, family *typ.Recursive) typ.Type {
	if slot == nil || family == nil {
		return slot
	}
	if typ.IsRecursiveRef(slot, family) {
		return family
	}
	return collapseChildren(slot, family)
}

// collapseChildren rebuilds slot replacing each immediate child that embeds a
// reference to family with the family handle, leaving children that do not embed
// family unchanged. Only the one-level structure is preserved; deeper unfoldings
// fold into the single recursion variable.
func collapseChildren(slot typ.Type, family *typ.Recursive) typ.Type {
	child := func(t typ.Type) typ.Type {
		if t == nil {
			return nil
		}
		if ContainsRecursiveRef(t, family) {
			return family
		}
		return t
	}
	switch v := slot.(type) {
	case *typ.Record:
		builder := typetable.NewRecord().SetOpen(v.Open)
		for _, f := range v.Fields {
			ft := child(f.Type)
			switch {
			case f.Optional && f.Readonly:
				builder.OptReadonlyField(f.Name, ft)
			case f.Optional:
				builder.OptField(f.Name, ft)
			case f.Readonly:
				builder.ReadonlyField(f.Name, ft)
			default:
				builder.Field(f.Name, ft)
			}
		}
		if v.Metatable != nil {
			builder.Metatable(child(v.Metatable))
		}
		if v.HasMapComponent() {
			builder.MapComponent(typetable.NormalizeKey(child(v.MapKey)), child(v.MapValue))
		}
		return builder.Build()
	case *typ.Map:
		return typetable.NewMap(child(v.Key), child(v.Value))
	case *typ.ReadonlyMap:
		return typetable.NewReadonlyMap(child(v.Key), child(v.Value))
	case *typ.Array:
		return typ.NewArray(child(v.Element))
	case *typ.Optional:
		return typ.NewOptional(child(v.Inner))
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		for i, m := range v.Members {
			members[i] = child(m)
		}
		return typ.NewUnion(members...)
	default:
		return slot
	}
}
