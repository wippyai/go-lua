package lua

import "math"

// exactInteger is the Lua 5.3 float-to-integer conversion: fractional,
// non-finite and out-of-range values have no integer representation.
func exactInteger(value LValue) (LInteger, bool) {
	switch v := value.(type) {
	case LInteger:
		return v, true
	case LNumber:
		return floatToInteger(v)
	case LString:
		if n, err := parseNumberValue(string(v)); err == nil {
			return exactInteger(n)
		}
	}
	return 0, false
}

// floatToInteger is shared by arithmetic conversion and numeric key identity.
func floatToInteger(v LNumber) (LInteger, bool) {
	if v >= -0x1p63 && v < 0x1p63 {
		i := LInteger(v)
		if LNumber(i) == v {
			return i, true
		}
	}
	return 0, false
}

func integerFloorDiv(lhs, rhs LInteger) LInteger {
	q := lhs / rhs
	if (lhs^rhs) < 0 && lhs%rhs != 0 {
		q--
	}
	return q
}

func integerModulo(lhs, rhs LInteger) LInteger {
	r := lhs % rhs
	if r != 0 && (r^rhs) < 0 {
		r += rhs
	}
	return r
}

// integerShift implements a logical shift in either direction, including
// negative counts and counts outside the machine word. Check before negation
// so MinInt64 is handled without overflow.
func integerShift(value, count LInteger, left bool) LInteger {
	if count >= 64 || count < -63 {
		return 0
	}
	if count < 0 {
		left = !left
		count = -count
	}
	if left {
		return value << uint(count)
	}
	return LInteger(uint64(value) >> uint(count))
}

// arithOperand applies Lua's string-to-number coercion to an arithmetic
// operand: numbers pass through and numeric strings convert with the
// integer or float subtype of their numeral.
func arithOperand(v LValue) (LValue, bool) {
	switch lv := v.(type) {
	case LInteger, LNumber:
		return v, true
	case LString:
		if n, err := parseNumberValue(string(lv)); err == nil {
			return n, true
		}
	}
	return nil, false
}

// arithValues applies +, -, *, /, % or ^ to two numbers. Integer operands
// give integer results for +, -, * and %, matching the VM fast paths.
func arithValues(L *LState, opcode int, lhs, rhs LValue) LValue {
	if l, ok := lhs.(LInteger); ok {
		if r, ok := rhs.(LInteger); ok {
			switch opcode {
			case OP_ADD:
				return lintegerToValue(l + r)
			case OP_SUB:
				return lintegerToValue(l - r)
			case OP_MUL:
				return lintegerToValue(l * r)
			case OP_MOD:
				if r == 0 {
					L.RaiseError("attempt to perform modulo by zero")
					return LNil
				}
				return lintegerToValue(integerModulo(l, r))
			}
		}
	}
	l, _ := toNumber(lhs)
	r, _ := toNumber(rhs)
	return lnumberToValue(numberArith(L, opcode, l, r))
}

// floorDivision preserves the numeric subtype. A false result defers invalid
// operations (including integer division by zero) to the VM's error path.
func floorDivision(lhs, rhs LValue) (LValue, bool) {
	if l, ok := lhs.(LInteger); ok {
		if r, ok := rhs.(LInteger); ok {
			if r == 0 {
				return nil, false
			}
			return lintegerToValue(integerFloorDiv(l, r)), true
		}
	}
	l, lok := toNumber(lhs)
	r, rok := toNumber(rhs)
	if !lok || !rok {
		return nil, false
	}
	return lnumberToValue(LNumber(math.Floor(float64(l) / float64(r)))), true
}

func bitwiseOperand(L *LState, value LValue) LInteger {
	if i, ok := exactInteger(value); ok {
		return i
	}
	if value.Type() == LTInteger || value.Type() == LTNumber {
		L.RaiseError("number has no integer representation")
	} else {
		L.RaiseError("attempt to perform bitwise operation on a %s value", value.Type().String())
	}
	return 0
}
