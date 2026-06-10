package typ

// Contains checks if the union contains a specific type.
func (u *Union) Contains(t Type) bool {
	h := unionMemberHash(t)
	for _, m := range u.Members {
		if unionMemberHash(m) == h && unionMemberEquals(m, t) {
			return true
		}
	}

	return false
}
