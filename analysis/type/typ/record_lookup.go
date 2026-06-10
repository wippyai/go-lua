package typ

import "sort"

// HasMapComponent returns true if the record has a map component (MapKey and MapValue set).
func (r *Record) HasMapComponent() bool {
	return r.MapKey != nil && r.MapValue != nil
}

// GetField returns the field with the given name, or nil.
func (r *Record) GetField(name string) *Field {
	if r.sorted {
		i := sort.Search(len(r.Fields), func(i int) bool {
			return r.Fields[i].Name >= name
		})
		if i < len(r.Fields) && r.Fields[i].Name == name {
			return &r.Fields[i]
		}
		return nil
	}

	for i := range r.Fields {
		if r.Fields[i].Name == name {
			return &r.Fields[i]
		}
	}

	return nil
}

// GetStaticStringIndex returns the exact bracket-string member with the given
// key, or nil when no such member is carried.
func (r *Record) GetStaticStringIndex(name string) *StaticMember {
	return r.GetStaticMember(StaticMemberStringIndex, name, 0)
}

// GetStaticIntIndex returns the exact bracket-integer member with the given key,
// or nil when no such member is carried.
func (r *Record) GetStaticIntIndex(index int64) *StaticMember {
	return r.GetStaticMember(StaticMemberIntIndex, "", index)
}

// GetStaticMember returns the exact bracket member with the given key, or nil
// when no such member is carried.
func (r *Record) GetStaticMember(kind StaticMemberKind, name string, index int64) *StaticMember {
	if r == nil || len(r.StaticMembers) == 0 {
		return nil
	}
	i := sort.Search(len(r.StaticMembers), func(i int) bool {
		return compareStaticMemberKey(r.StaticMembers[i], kind, name, index) >= 0
	})
	if i < len(r.StaticMembers) {
		member := &r.StaticMembers[i]
		if member.Kind == kind && member.Name == name && member.Index == index {
			return member
		}
	}
	return nil
}
