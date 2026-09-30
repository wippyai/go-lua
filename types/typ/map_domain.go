package typ

// WithMapDomain preserves the domain of unlisted keys when a map is refined
// to records with known fields. Existing components retain their evidence.
func WithMapDomain(t, key, value Type) Type {
	switch v := t.(type) {
	case *Record:
		if v.HasMapComponent() {
			return t
		}
		return v.Builder().SetOpen(false).MapComponent(key, value).Build()
	case *Union:
		members := make([]Type, len(v.Members))
		for i, member := range v.Members {
			members[i] = WithMapDomain(member, key, value)
		}
		return NewUnion(members...)
	case *Optional:
		return NewOptional(WithMapDomain(v.Inner, key, value))
	case *Alias:
		return NewAlias(v.Name, WithMapDomain(v.Target, key, value))
	}
	return t
}
