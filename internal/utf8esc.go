package internal

// MaxUTF8Esc is the largest value the Lua UTF-8 encoding represents.
const MaxUTF8Esc = 0x7FFFFFFF

// AppendUTF8Esc appends the encoding of x (at most MaxUTF8Esc) using up to six
// bytes, as luaO_utf8esc does. Surrogates and values above U+10FFFF encode
// like any other value.
func AppendUTF8Esc(buf []byte, x uint32) []byte {
	if x < 0x80 {
		return append(buf, byte(x))
	}
	var tmp [6]byte
	n := len(tmp)
	mfb := uint32(0x3F)
	for {
		n--
		tmp[n] = byte(0x80 | (x & 0x3F))
		x >>= 6
		mfb >>= 1
		if x <= mfb {
			break
		}
	}
	n--
	tmp[n] = byte((^mfb << 1) | x)
	return append(buf, tmp[n:]...)
}
