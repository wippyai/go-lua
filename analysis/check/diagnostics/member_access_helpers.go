package diagnostics

func memberPathName(root, member string) string {
	if member == "" {
		return root
	}
	if member[0] == '[' {
		return root + member
	}
	return root + "." + member
}
