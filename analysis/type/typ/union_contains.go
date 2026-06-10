package typ

// Contains checks if the union contains a specific type.
func (u *Union) Contains(t Type) bool {
	h := UnionMemberHash(t)
	for _, m := range u.Members {
		if UnionMemberHash(m) == h && SameUnionMember(m, t) {
			return true
		}
	}

	return false
}
