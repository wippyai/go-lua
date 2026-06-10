package typ

// CompareStaticMembers compares static members by their canonical record key.
func CompareStaticMembers(left, right StaticMember) int {
	return compareStaticMemberKey(left, right.Kind, right.Name, right.Index)
}

func compareStaticMemberKey(left StaticMember, kind StaticMemberKind, name string, index int64) int {
	if left.Kind != kind {
		if left.Kind < kind {
			return -1
		}
		return 1
	}
	switch left.Kind {
	case StaticMemberStringIndex:
		if left.Name < name {
			return -1
		}
		if left.Name > name {
			return 1
		}
	case StaticMemberIntIndex:
		if left.Index < index {
			return -1
		}
		if left.Index > index {
			return 1
		}
	}
	return 0
}

func staticMembersSorted(members []StaticMember) bool {
	for i := 1; i < len(members); i++ {
		if CompareStaticMembers(members[i-1], members[i]) > 0 {
			return false
		}
	}
	return true
}
