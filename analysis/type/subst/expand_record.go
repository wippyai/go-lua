package subst

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func expandRecord(v *typ.Record, orig typ.Type, state *expandState, mode expandMode) typ.Type {
	changed := false
	var fields []typ.Field
	for i, f := range v.Fields {
		newType := expandInstantiatedGuardMode(f.Type, state, mode)
		if newType != f.Type {
			if fields == nil {
				fields = make([]typ.Field, len(v.Fields))
				copy(fields, v.Fields)
			}
			changed = true
			fields[i] = typ.Field{Name: f.Name, Type: newType, Optional: f.Optional, Readonly: f.Readonly}
		} else if fields != nil {
			fields[i] = f
		}
	}

	var staticMembers []typ.StaticMember
	for i, m := range v.StaticMembers {
		newType := expandInstantiatedGuardMode(m.Type, state, mode)
		if newType != m.Type {
			if staticMembers == nil {
				staticMembers = make([]typ.StaticMember, len(v.StaticMembers))
				copy(staticMembers, v.StaticMembers)
			}
			changed = true
			staticMembers[i] = typ.StaticMember{
				Kind:     m.Kind,
				Name:     m.Name,
				Index:    m.Index,
				Type:     newType,
				Optional: m.Optional,
				Readonly: m.Readonly,
			}
		} else if staticMembers != nil {
			staticMembers[i] = m
		}
	}

	metatable := v.Metatable
	if v.Metatable != nil {
		newMetatable := expandInstantiatedGuardMode(v.Metatable, state, mode)
		if newMetatable != v.Metatable {
			changed = true
			metatable = newMetatable
		}
	}

	mapKey := v.MapKey
	mapValue := v.MapValue
	if v.HasMapComponent() {
		mapKey = expandInstantiatedGuardMode(v.MapKey, state, mode)
		if mapKey != v.MapKey {
			changed = true
		}
		mapValue = expandInstantiatedGuardMode(v.MapValue, state, mode)
		if mapValue != v.MapValue {
			changed = true
		}
	}

	if !changed && mode == expandModeStructural {
		return orig
	}

	fieldsSrc := v.Fields
	if fields != nil {
		fieldsSrc = fields
	}
	staticMembersSrc := v.StaticMembers
	if staticMembers != nil {
		staticMembersSrc = staticMembers
	}
	return typetable.RebuildRecord(typ.RecordParts{
		Fields:        fieldsSrc,
		StaticMembers: staticMembersSrc,
		Metatable:     metatable,
		MapKey:        mapKey,
		MapValue:      mapValue,
		Open:          v.Open,
		AssumeSorted:  true,
	})
}

func expandInterface(v *typ.Interface, orig typ.Type, state *expandState, mode expandMode) typ.Type {
	changed := false
	var methods []typ.Method
	for idx := range v.Methods {
		m := v.Methods[idx]
		newType := expandInstantiatedGuardMode(m.Type, state, mode)
		fn, ok := newType.(*typ.Function)
		if !ok {
			fn = m.Type
		}
		if fn != m.Type {
			if methods == nil {
				methods = make([]typ.Method, len(v.Methods))
				copy(methods, v.Methods)
			}
			changed = true
			methods[idx] = typ.Method{Name: m.Name, Type: fn}
		} else if methods != nil {
			methods[idx] = m
		}
	}
	if !changed {
		return orig
	}
	return typ.NewInterface(v.Name, methods)
}
