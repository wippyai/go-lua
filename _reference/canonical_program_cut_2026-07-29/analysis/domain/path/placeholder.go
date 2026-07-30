package path

// PlaceholderIndexFromString extracts a placeholder index from the canonical $N form.
// Returns -1 for invalid syntax, negative values, and overflow.
func PlaceholderIndexFromString(s string) int {
	if len(s) >= 2 && s[0] == '$' {
		return parseNonNegativeDecimal(s[1:])
	}
	return -1
}

// ReturnSlotIndexFromString extracts a return-slot index from the canonical
// ret[N] root form. Returns -1 for invalid syntax, negative values, and
// overflow.
func ReturnSlotIndexFromString(s string) int {
	const prefix = "ret["
	if len(s) < len(prefix)+2 || s[:len(prefix)] != prefix || s[len(s)-1] != ']' {
		return -1
	}
	return parseNonNegativeDecimal(s[len(prefix) : len(s)-1])
}

// ReturnSlotIndex returns the result slot index if this path's root is a
// return slot (ret[0], ret[1], etc.). Returns -1 if not a return-slot path.
func (p Path) ReturnSlotIndex() int {
	if p.Symbol != 0 {
		return -1
	}
	return ReturnSlotIndexFromString(p.Root)
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
