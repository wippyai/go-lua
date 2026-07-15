package projection

import (
	"github.com/wippyai/go-lua/analysis/type/internal/graph"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ElementOf projects the element/value type read from Lua container shapes.
func ElementOf(t typ.Type) (typ.Type, bool) {
	return elementOf(t, &graph.Path{}, nil, false)
}

// KeyOf projects the key type iterated over Lua container shapes. Arrays and
// tuples key by integer; maps key by their declared key type. Records key by
// their map component plus any statically known closed-record members. A union
// keys by the union of its members' key types, skipping nil members.
func KeyOf(t typ.Type) (typ.Type, bool) {
	return keyOf(t, &graph.Path{}, nil, false)
}

type containerProject func(typ.Type, *graph.Path, typ.Type, bool) (typ.Type, bool)

func descendContainer(t typ.Type, active *graph.Path, cycleType typ.Type, cycleOK bool, project containerProject) (typ.Type, bool) {
	t = unwrap.NormalizeNil(t)
	if t == nil {
		return nil, false
	}
	if !active.Enter(t) {
		return cycleType, cycleOK
	}
	defer active.Leave(t)
	switch tt := t.(type) {
	case *typ.Annotated:
		return descendContainer(tt.Inner, active, cycleType, cycleOK, project)
	case *typ.Alias:
		return descendContainer(tt.UnaliasedTarget(), active, cycleType, cycleOK, project)
	case *typ.Optional:
		return descendContainer(tt.Inner, active, cycleType, cycleOK, project)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return nil, false
		}
		return descendContainer(tt.Body, active, cycleType, cycleOK, project)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(tt)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return descendContainer(expanded, active, cycleType, cycleOK, project)
	default:
		return project(t, active, cycleType, cycleOK)
	}
}

func keyOf(t typ.Type, active *graph.Path, cycleType typ.Type, cycleOK bool) (typ.Type, bool) {
	return descendContainer(t, active, cycleType, cycleOK, keyOfProject)
}

func keyOfProject(t typ.Type, active *graph.Path, _ typ.Type, _ bool) (typ.Type, bool) {
	switch tt := t.(type) {
	case *typ.Array:
		return typ.Integer, true
	case *typ.Tuple:
		return typ.Integer, true
	case *typ.Map:
		if unwrap.NormalizeNil(tt.Key) == nil {
			return nil, false
		}
		return tt.Key, true
	case *typ.ReadonlyMap:
		if unwrap.NormalizeNil(tt.Key) == nil {
			return nil, false
		}
		return tt.Key, true
	case *typ.Record:
		return keyOfRecord(tt)
	case *typ.Union:
		return projectContainerUnion(tt.Members, active, keyOf)
	default:
		return nil, false
	}
}

func keyOfRecord(record *typ.Record) (typ.Type, bool) {
	if record == nil {
		return nil, false
	}
	members := make([]typ.Type, 0, len(record.Fields)+len(record.StaticMembers)+1)
	if record.HasMapComponent() && unwrap.NormalizeNil(record.MapKey) != nil {
		members = append(members, record.MapKey)
	}
	if !record.Open {
		for _, field := range record.Fields {
			if field.Name != "" {
				members = append(members, typ.LiteralString(field.Name))
			}
		}
		for _, member := range record.StaticMembers {
			switch member.Kind {
			case typ.StaticMemberStringIndex:
				if member.Name != "" {
					members = append(members, typ.LiteralString(member.Name))
				}
			case typ.StaticMemberIntIndex:
				members = append(members, typ.LiteralInt(member.Index))
			}
		}
	}
	if len(members) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(members...), true
}

func elementOf(t typ.Type, active *graph.Path, cycleType typ.Type, cycleOK bool) (typ.Type, bool) {
	return descendContainer(t, active, cycleType, cycleOK, elementOfProject)
}

func elementOfProject(t typ.Type, active *graph.Path, _ typ.Type, _ bool) (typ.Type, bool) {
	switch tt := t.(type) {
	case *typ.Array:
		if unwrap.NormalizeNil(tt.Element) == nil {
			return nil, false
		}
		return tt.Element, true
	case *typ.Map:
		if unwrap.NormalizeNil(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.ReadonlyMap:
		if unwrap.NormalizeNil(tt.Value) == nil {
			return nil, false
		}
		return tt.Value, true
	case *typ.Record:
		return elementOfRecord(tt)
	case *typ.Tuple:
		if len(tt.Elements) == 0 {
			return nil, false
		}
		if len(tt.Elements) == 1 {
			if unwrap.NormalizeNil(tt.Elements[0]) == nil {
				return nil, false
			}
			return tt.Elements[0], true
		}
		return normalize.UnionForEvidence(tt.Elements...), true
	case *typ.Union:
		return projectContainerUnion(tt.Members, active, elementOf)
	default:
		return nil, false
	}
}

func elementOfRecord(record *typ.Record) (typ.Type, bool) {
	if record == nil {
		return nil, false
	}
	members := make([]typ.Type, 0, len(record.Fields)+len(record.StaticMembers)+1)
	if record.HasMapComponent() && unwrap.NormalizeNil(record.MapValue) != nil {
		members = append(members, record.MapValue)
	}
	if !record.Open {
		for _, field := range record.Fields {
			if unwrap.NormalizeNil(field.Type) != nil {
				members = append(members, field.Type)
			}
		}
		for _, member := range record.StaticMembers {
			if unwrap.NormalizeNil(member.Type) != nil {
				members = append(members, member.Type)
			}
		}
	}
	if len(members) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(members...), true
}

func projectContainerUnion(members []typ.Type, active *graph.Path, project containerProject) (typ.Type, bool) {
	projected := make([]typ.Type, 0, len(members))
	for _, member := range members {
		member = unwrap.NormalizeNil(member)
		if member == nil || member.Kind() == kind.Nil {
			continue
		}
		// Union projection is a must equation. A recursive backedge is its
		// conjunction identity and contributes no projected type by itself.
		out, ok := project(member, active, nil, true)
		if !ok {
			return nil, false
		}
		if out != nil {
			projected = append(projected, out)
		}
	}
	if len(projected) == 0 {
		return nil, false
	}
	return normalize.UnionForEvidence(projected...), true
}
