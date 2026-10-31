package lua

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Flags each conversion accepts, as in the Lua 5.4 reference lstrlib.
const (
	formatFlagsFloat    = "-+#0 "
	formatFlagsHex      = "-#0"
	formatFlagsInteger  = "-+0 "
	formatFlagsUnsigned = "-0"
	formatFlagsChar     = "-"
	formatMaxSpec       = 32
)

// formatSpec is one parsed conversion: the text after '%' up to and
// including the conversion character.
type formatSpec struct {
	text      string
	flags     string
	width     int
	precision int
	hasPrec   bool
	conv      byte
}

func (s formatSpec) has(flag byte) bool { return strings.IndexByte(s.flags, flag) >= 0 }

// strFormat implements string.format with Lua 5.4 semantics: every
// conversion validates its specification and converts its argument the way
// the reference implementation does before it is rendered.
func strFormat(L *LState) int {
	format := L.CheckString(1)
	top := L.GetTop()
	arg := 1
	var buf strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			buf.WriteByte(c)
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			buf.WriteByte('%')
			continue
		}
		arg++
		if arg > top {
			L.ArgError(arg, "no value")
		}
		spec := scanFormatSpec(L, format, i)
		i += len(spec.text) - 1
		formatItem(L, &buf, spec, arg)
	}
	L.Push(LString(buf.String()))
	return 1
}

// scanFormatSpec spans flags, width and precision plus the conversion
// character, as getformat does in lstrlib.
func scanFormatSpec(L *LState, format string, start int) formatSpec {
	end := start
	for end < len(format) && strings.IndexByte(formatFlagsFloat+"123456789.", format[end]) >= 0 {
		end++
	}
	if end-start+1 >= formatMaxSpec-10 {
		L.RaiseError("invalid format string to 'format'")
	}
	spec := formatSpec{text: format[start:end]}
	if end < len(format) {
		spec.conv = format[end]
		spec.text = format[start : end+1]
	}
	return spec
}

// checkFormat validates the specification against the flags its conversion
// accepts and records them; width and precision take at most two digits.
func checkFormat(L *LState, spec *formatSpec, flags string, precision bool) {
	body := spec.text
	i := 0
	for i < len(body) && strings.IndexByte(flags, body[i]) >= 0 {
		i++
	}
	spec.flags = body[:i]
	if i < len(body) && body[i] != '0' {
		spec.width, i = twoDigits(body, i)
		if i < len(body) && body[i] == '.' && precision {
			spec.hasPrec = true
			spec.precision, i = twoDigits(body, i+1)
		}
	}
	if i != len(body)-1 || !isASCIIAlpha(body[i]) {
		L.RaiseError("invalid conversion specification: '%%%s'", body)
	}
}

func twoDigits(s string, i int) (int, int) {
	n := 0
	for digits := 0; digits < 2 && i < len(s) && s[i] >= '0' && s[i] <= '9'; digits++ {
		n = n*10 + int(s[i]-'0')
		i++
	}
	return n, i
}

func isASCIIAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func formatItem(L *LState, buf *strings.Builder, spec formatSpec, arg int) {
	switch spec.conv {
	case 'c':
		n := formatInteger(L, arg)
		checkFormat(L, &spec, formatFlagsChar, false)
		buf.WriteString(padField(string([]byte{byte(n)}), spec))
	case 'd', 'i':
		n := formatInteger(L, arg)
		checkFormat(L, &spec, formatFlagsInteger, true)
		buf.WriteString(formatIntegerValue(spec, n))
	case 'u':
		n := formatInteger(L, arg)
		checkFormat(L, &spec, formatFlagsUnsigned, true)
		buf.WriteString(formatIntegerValue(spec, n))
	case 'o', 'x', 'X':
		n := formatInteger(L, arg)
		checkFormat(L, &spec, formatFlagsHex, true)
		buf.WriteString(formatIntegerValue(spec, n))
	case 'a', 'A':
		n := float64(L.CheckNumber(arg))
		checkFormat(L, &spec, formatFlagsFloat, true)
		buf.WriteString(formatFloat(spec, n))
	case 'e', 'E', 'f', 'F', 'g', 'G':
		n := float64(L.CheckNumber(arg))
		checkFormat(L, &spec, formatFlagsFloat, true)
		buf.WriteString(formatFloat(spec, n))
	case 'p':
		checkFormat(L, &spec, formatFlagsChar, false)
		buf.WriteString(padField(formatPointer(L.Get(arg)), spec))
	case 'q':
		if len(spec.text) != 1 {
			L.RaiseError("%s", "specifier '%q' cannot have modifiers")
		}
		writeLiteral(L, buf, arg)
	case 's':
		s := L.ToStringMeta(L.Get(arg)).String()
		if len(spec.text) == 1 {
			buf.WriteString(s)
			return
		}
		if strings.IndexByte(s, 0) >= 0 {
			L.ArgError(arg, "string contains zeros")
		}
		checkFormat(L, &spec, formatFlagsChar, true)
		if spec.hasPrec && len(s) > spec.precision {
			s = s[:spec.precision]
		}
		buf.WriteString(padField(s, spec))
	default:
		L.RaiseError("invalid conversion '%%%s' to 'format'", spec.text)
	}
}

// formatInteger is luaL_checkinteger: floats and numeric strings convert only
// when they have an exact integer value.
func formatInteger(L *LState, arg int) LInteger {
	value := L.Get(arg)
	if n, ok := exactInteger(value); ok {
		return n
	}
	isNumber := value.Type() == LTNumber || value.Type() == LTInteger
	if s, ok := value.(LString); ok {
		_, err := parseNumberValue(string(s))
		isNumber = err == nil
	}
	if isNumber {
		L.ArgError(arg, "number has no integer representation")
	}
	L.TypeError(arg, LTNumber)
	return 0
}

// formatIntegerValue renders d, i, u, o, x and X as C does: u, o and x read
// the integer as unsigned, the precision is a minimum digit count (zero with
// precision 0 prints no digits) and zero padding yields to a precision.
func formatIntegerValue(spec formatSpec, n LInteger) string {
	sign := ""
	var digits string
	switch spec.conv {
	case 'd', 'i':
		magnitude := uint64(n)
		if n < 0 {
			magnitude = -magnitude
		}
		digits = strconv.FormatUint(magnitude, 10)
		sign = numberSign(spec, n < 0)
	case 'u':
		digits = strconv.FormatUint(uint64(n), 10)
	case 'o':
		digits = strconv.FormatUint(uint64(n), 8)
	default:
		digits = strconv.FormatUint(uint64(n), 16)
	}
	if spec.hasPrec && spec.precision == 0 && n == 0 {
		digits = ""
	}
	if spec.hasPrec && len(digits) < spec.precision {
		digits = strings.Repeat("0", spec.precision-len(digits)) + digits
	}
	prefix := ""
	if spec.has('#') {
		switch {
		case spec.conv == 'o' && !strings.HasPrefix(digits, "0"):
			digits = "0" + digits
		case spec.conv == 'x' && n != 0:
			prefix = "0x"
		case spec.conv == 'X' && n != 0:
			prefix = "0X"
		}
	}
	if spec.conv == 'X' {
		digits = strings.ToUpper(digits)
	}
	return padNumber(sign, prefix, digits, spec, spec.has('0') && !spec.hasPrec)
}

// goVerb renders a validated float specification as a Go format directive.
// Go's fmt agrees with C for e, f and g once the precision C defaults to is
// explicit.
func goVerb(spec formatSpec, verb byte) string {
	var b strings.Builder
	b.WriteByte('%')
	b.WriteString(spec.flags)
	if spec.width > 0 {
		b.WriteString(strconv.Itoa(spec.width))
	}
	if spec.hasPrec {
		b.WriteByte('.')
		b.WriteString(strconv.Itoa(spec.precision))
	}
	b.WriteByte(verb)
	return b.String()
}

func formatFloat(spec formatSpec, n float64) string {
	upper := spec.conv >= 'A' && spec.conv <= 'Z'
	if math.IsInf(n, 0) || math.IsNaN(n) {
		word := "inf"
		if math.IsNaN(n) {
			word = "nan"
		}
		if upper {
			word = strings.ToUpper(word)
		}
		return padNumber(numberSign(spec, math.Signbit(n)), "", word, spec, false)
	}
	if spec.conv == 'a' || spec.conv == 'A' {
		prefix, digits := hexFloat(math.Abs(n), spec)
		if upper {
			prefix, digits = strings.ToUpper(prefix), strings.ToUpper(digits)
		}
		return padNumber(numberSign(spec, math.Signbit(n)), prefix, digits, spec, spec.has('0'))
	}
	if !spec.hasPrec {
		spec.hasPrec, spec.precision = true, 6
	}
	verb := spec.conv
	if verb == 'F' {
		verb = 'f'
	}
	return fmt.Sprintf(goVerb(spec, verb), n)
}

func numberSign(spec formatSpec, negative bool) string {
	switch {
	case negative:
		return "-"
	case spec.has('+'):
		return "+"
	case spec.has(' '):
		return " "
	}
	return ""
}

// padNumber applies width; zero padding goes between sign/prefix and digits.
func padNumber(sign, prefix, digits string, spec formatSpec, zero bool) string {
	body := sign + prefix + digits
	pad := spec.width - len(body)
	if pad <= 0 {
		return body
	}
	if spec.has('-') {
		return body + strings.Repeat(" ", pad)
	}
	if zero {
		return sign + prefix + strings.Repeat("0", pad) + digits
	}
	return strings.Repeat(" ", pad) + body
}

// padField pads a string to the width in bytes, as C does.
func padField(s string, spec formatSpec) string {
	pad := spec.width - len(s)
	if pad <= 0 {
		return s
	}
	if spec.has('-') {
		return s + strings.Repeat(" ", pad)
	}
	return strings.Repeat(" ", pad) + s
}

// hexFloat renders a non-negative finite value as C's %a does: a leading
// digit of 0 for zero and subnormals and 1 otherwise, the fraction in hex
// (exact unless a precision rounds it half to even) and a decimal binary
// exponent.
func hexFloat(n float64, spec formatSpec) (string, string) {
	bits := math.Float64bits(n)
	exponent := int((bits >> 52) & 0x7ff)
	mantissa := bits & (1<<52 - 1)
	lead := uint64(1)
	switch {
	case exponent == 0 && mantissa == 0:
		lead, exponent = 0, 0
	case exponent == 0:
		lead, exponent = 0, -1022
	default:
		exponent -= 1023
	}
	fraction := fmt.Sprintf("%013x", mantissa)
	if spec.hasPrec && spec.precision < 13 {
		shift := uint(13-spec.precision) * 4
		full := lead<<52 | mantissa
		rounded := full >> shift
		rest := full & (1<<shift - 1)
		half := uint64(1) << (shift - 1)
		if rest > half || rest == half && rounded&1 == 1 {
			rounded++
		}
		fractionBits := uint(spec.precision) * 4
		lead = rounded >> fractionBits
		fraction = ""
		if spec.precision > 0 {
			fraction = fmt.Sprintf("%0*x", spec.precision, rounded&(1<<fractionBits-1))
		}
	} else if spec.hasPrec {
		fraction += strings.Repeat("0", spec.precision-13)
	} else {
		fraction = strings.TrimRight(fraction, "0")
	}
	digits := strconv.FormatUint(lead, 16)
	if fraction != "" || spec.has('#') {
		digits += "." + fraction
	}
	sign := "+"
	if exponent < 0 {
		sign = "-"
		exponent = -exponent
	}
	return "0x", digits + "p" + sign + strconv.Itoa(exponent)
}

func formatPointer(v LValue) string {
	switch p := v.(type) {
	case *LTable, *LFunction, *LUserData, *LState:
		return fmt.Sprintf("%p", p)
	}
	return "(null)"
}

// writeLiteral is %q: a value written so that Lua reads it back unchanged.
func writeLiteral(L *LState, buf *strings.Builder, arg int) {
	switch v := L.Get(arg).(type) {
	case LString:
		writeQuoted(buf, string(v))
	case LInteger:
		if v == math.MinInt64 {
			buf.WriteString("0x8000000000000000")
		} else {
			buf.WriteString(strconv.FormatInt(int64(v), 10))
		}
	case LNumber:
		n := float64(v)
		switch {
		case math.IsInf(n, 1):
			buf.WriteString("1e9999")
		case math.IsInf(n, -1):
			buf.WriteString("-1e9999")
		case math.IsNaN(n):
			buf.WriteString("(0/0)")
		default:
			prefix, digits := hexFloat(math.Abs(n), formatSpec{})
			if math.Signbit(n) {
				buf.WriteByte('-')
			}
			buf.WriteString(prefix)
			buf.WriteString(digits)
		}
	case LBool:
		buf.WriteString(v.String())
	default:
		if v == LNil {
			buf.WriteString("nil")
			return
		}
		L.ArgError(arg, "value has no literal form")
	}
}

func writeQuoted(buf *strings.Builder, s string) {
	buf.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\' || c == '\n':
			buf.WriteByte('\\')
			buf.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				fmt.Fprintf(buf, "\\%03d", c)
			} else {
				fmt.Fprintf(buf, "\\%d", c)
			}
		default:
			buf.WriteByte(c)
		}
	}
	buf.WriteByte('"')
}
