package path

// PlaceholderIndexFromString extracts a placeholder index from the canonical $N form.
// Returns -1 for invalid syntax, negative values, and overflow.
func PlaceholderIndexFromString(s string) int {
	if len(s) >= 2 && s[0] == '$' {
		return parseNonNegativeDecimal(s[1:])
	}
	return -1
}

func parseNonNegativeDecimal(s string) int {
	if s == "" {
		return -1
	}

	maxInt := int(^uint(0) >> 1)
	value := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return -1
		}
		digit := int(ch - '0')
		if value > (maxInt-digit)/10 {
			return -1
		}
		value = value*10 + digit
	}
	return value
}
