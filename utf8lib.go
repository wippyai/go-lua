package lua

import "github.com/wippyai/go-lua/internal"

// The utf8 library follows lutf8lib.c from Lua 5.4. Strict decoding accepts
// code points up to U+10FFFF without surrogates; the optional lax argument
// accepts any sequence of up to six bytes encoding a value up to 0x7FFFFFFF.

const (
	utf8MaxUnicode = 0x10FFFF
	utf8MaxUTF     = internal.MaxUTF8Esc
	utf8MsgInvalid = "invalid UTF-8 code"
)

func OpenUtf8(L *LState) int {
	mod := L.RegisterGoModule(Utf8LibName, utf8Funcs).(*LTable)
	mod.RawSetString("charpattern", LString("[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"))
	L.Push(mod)
	return 1
}

var utf8Funcs = map[string]LGoFunc{
	"char":      utf8Char,
	"codes":     utf8Codes,
	"codepoint": utf8Codepoint,
	"len":       utf8Len,
	"offset":    utf8Offset,
}

// utf8PosRelat converts a negative string position to a positive one;
// positions before the start become 0.
func utf8PosRelat(pos int64, length int) int64 {
	if pos >= 0 {
		return pos
	}
	if -pos > int64(length) {
		return 0
	}
	return int64(length) + pos + 1
}

// utf8IsCont reports whether the byte at index i is a continuation byte.
// The index just past the end reads as a terminator, as in C.
func utf8IsCont(s string, i int) bool {
	return i < len(s) && s[i]&0xC0 == 0x80
}

var utf8Limits = [...]uint32{^uint32(0), 0x80, 0x800, 0x10000, 0x200000, 0x4000000}

// utf8Decode decodes the sequence starting at index i and returns its value
// and the index after it, or ok == false for an invalid sequence.
func utf8Decode(s string, i int, strict bool) (code uint32, next int, ok bool) {
	c := uint32(s[i])
	if c < 0x80 {
		code = c
	} else {
		count := 0
		for ; c&0x40 != 0; c <<= 1 {
			count++
			if !utf8IsCont(s, i+count) {
				return 0, 0, false
			}
			code = code<<6 | uint32(s[i+count]&0x3F)
		}
		code |= (c & 0x7F) << (count * 5)
		if count > 5 || code > utf8MaxUTF || code < utf8Limits[count] {
			return 0, 0, false
		}
		i += count
	}
	if strict && (code > utf8MaxUnicode || (0xD800 <= code && code <= 0xDFFF)) {
		return 0, 0, false
	}
	return code, i + 1, true
}

func utf8Char(L *LState) int {
	n := L.GetTop()
	buf := make([]byte, 0, n*4)
	for i := 1; i <= n; i++ {
		code := uint64(L.CheckInt64(i))
		if code > utf8MaxUTF {
			L.ArgError(i, "value out of range")
		}
		buf = internal.AppendUTF8Esc(buf, uint32(code))
	}
	L.Push(LString(buf))
	return 1
}

func utf8Len(L *LState) int {
	s := L.CheckString(1)
	posi := utf8PosRelat(L.OptInt64(2, 1), len(s))
	posj := utf8PosRelat(L.OptInt64(3, -1), len(s))
	lax := LVAsBool(L.Get(4))
	posi--
	if posi < 0 || posi > int64(len(s)) {
		L.ArgError(2, "initial position out of bounds")
	}
	posj--
	if posj >= int64(len(s)) {
		L.ArgError(3, "final position out of bounds")
	}
	var n int64
	for i := int(posi); int64(i) <= posj; n++ {
		_, next, ok := utf8Decode(s, i, !lax)
		if !ok {
			L.Push(LNil)
			L.Push(LInteger(i + 1))
			return 2
		}
		i = next
	}
	L.Push(LInteger(n))
	return 1
}

func utf8Codepoint(L *LState) int {
	s := L.CheckString(1)
	posi := utf8PosRelat(L.OptInt64(2, 1), len(s))
	pose := utf8PosRelat(L.OptInt64(3, posi), len(s))
	lax := LVAsBool(L.Get(4))
	if posi < 1 {
		L.ArgError(2, "out of bounds")
	}
	if pose > int64(len(s)) {
		L.ArgError(3, "out of bounds")
	}
	if posi > pose {
		return 0
	}
	n := 0
	for i := int(posi - 1); int64(i) < pose; n++ {
		code, next, ok := utf8Decode(s, i, !lax)
		if !ok {
			L.RaiseError(utf8MsgInvalid)
		}
		L.Push(LInteger(code))
		i = next
	}
	return n
}

func utf8Offset(L *LState) int {
	s := L.CheckString(1)
	n := L.CheckInt64(2)
	defaultPos := int64(1)
	if n < 0 {
		defaultPos = int64(len(s)) + 1
	}
	posi := utf8PosRelat(L.OptInt64(3, defaultPos), len(s))
	posi--
	if posi < 0 || posi > int64(len(s)) {
		L.ArgError(3, "position out of bounds")
	}
	pos := int(posi)
	if n == 0 {
		for pos > 0 && utf8IsCont(s, pos) {
			pos--
		}
	} else {
		if utf8IsCont(s, pos) {
			L.RaiseError("initial position is a continuation byte")
		}
		if n < 0 {
			for n < 0 && pos > 0 {
				pos--
				for pos > 0 && utf8IsCont(s, pos) {
					pos--
				}
				n++
			}
		} else {
			n--
			for n > 0 && pos < len(s) {
				pos++
				for utf8IsCont(s, pos) {
					pos++
				}
				n--
			}
		}
	}
	if n == 0 {
		L.Push(LInteger(pos + 1))
	} else {
		L.Push(LNil)
	}
	return 1
}

func utf8Codes(L *LState) int {
	lax := LVAsBool(L.Get(2))
	s := L.CheckString(1)
	if utf8IsCont(s, 0) {
		L.ArgError(1, utf8MsgInvalid)
	}
	if lax {
		L.Push(LGoFunc(utf8CodesIterLax))
	} else {
		L.Push(LGoFunc(utf8CodesIterStrict))
	}
	L.Push(LString(s))
	L.Push(LInteger(0))
	return 3
}

func utf8CodesIterStrict(L *LState) int { return utf8CodesIter(L, true) }

func utf8CodesIterLax(L *LState) int { return utf8CodesIter(L, false) }

// utf8CodesIter receives the 1-based position of the previous character as
// its control value. That position is the 0-based index of the byte after the
// previous character's first byte, so skipping continuation bytes from there
// reaches the next character.
func utf8CodesIter(L *LState, strict bool) int {
	s := L.CheckString(1)
	n := uint64(L.CheckInt64(2))
	for n < uint64(len(s)) && utf8IsCont(s, int(n)) {
		n++
	}
	if n >= uint64(len(s)) {
		return 0
	}
	code, next, ok := utf8Decode(s, int(n), strict)
	if !ok || utf8IsCont(s, next) {
		L.RaiseError(utf8MsgInvalid)
	}
	L.Push(LInteger(n + 1))
	L.Push(LInteger(code))
	return 2
}
