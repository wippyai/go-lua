package lua

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"

	"github.com/wippyai/go-lua/types/numparse"
)

func intMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func defaultFormat(v any, f fmt.State, c rune) {
	buf := make([]string, 0, 10)
	buf = append(buf, "%")
	for i := 0; i < 128; i++ {
		if f.Flag(i) {
			buf = append(buf, string(rune(i)))
		}
	}

	if w, ok := f.Width(); ok {
		buf = append(buf, strconv.Itoa(w))
	}
	if p, ok := f.Precision(); ok {
		buf = append(buf, "."+strconv.Itoa(p))
	}
	buf = append(buf, string(c))
	format := strings.Join(buf, "")
	_, _ = fmt.Fprintf(f, format, v)
}

type flagScanner struct {
	flag       byte
	start      string
	end        string
	buf        []byte
	str        string
	Length     int
	Pos        int
	HasFlag    bool
	ChangeFlag bool
}

func newFlagScanner(flag byte, start, end, str string) *flagScanner {
	return &flagScanner{flag, start, end, make([]byte, 0, len(str)), str, len(str), 0, false, false}
}

func (fs *flagScanner) AppendString(str string) { fs.buf = append(fs.buf, str...) }

func (fs *flagScanner) AppendChar(ch byte) { fs.buf = append(fs.buf, ch) }

func (fs *flagScanner) String() string { return string(fs.buf) }

func (fs *flagScanner) Next() (byte, bool) {
	c := byte('\000')
	fs.ChangeFlag = false
	if fs.Pos == fs.Length {
		if fs.HasFlag {
			fs.AppendString(fs.end)
		}
		return c, true
	}
	c = fs.str[fs.Pos]
	if c == fs.flag {
		if fs.Pos < (fs.Length-1) && fs.str[fs.Pos+1] == fs.flag {
			fs.HasFlag = false
			fs.AppendChar(fs.flag)
			fs.Pos += 2
			return fs.Next()
		}
		if fs.Pos != fs.Length-1 {
			if fs.HasFlag {
				fs.AppendString(fs.end)
			}
			fs.AppendString(fs.start)
			fs.ChangeFlag = true
			fs.HasFlag = true
		}
	}
	fs.Pos++
	return c, false
}

// IsIntegerValue checks if the runtime LNumber value has no fractional part.
func IsIntegerValue(v LNumber) bool {
	iv := int64(v)
	return LNumber(iv) == v
}

func isArrayKey(v LNumber) bool {
	iv := int(v)
	return iv > 0 && iv < MaxArrayIndex && LNumber(iv) == v
}

// parseNumberValue converts a numeral to an LInteger or LNumber following
// Lua 5.4 string-to-number conversion (see numparse.Parse).
func parseNumberValue(number string) (LValue, error) {
	n, ok := numparse.Parse(number)
	if !ok {
		return LNil, fmt.Errorf("malformed number: %q", number)
	}
	if n.IsFloat {
		return LNumber(n.Float), nil
	}
	return LInteger(n.Int), nil
}

func readBufioLine(reader *bufio.Reader) ([]byte, error, bool) {
	var result []byte
	var buf []byte
	var err error
	var isprefix = true
	for isprefix {
		buf, isprefix, err = reader.ReadLine()
		if err != nil {
			break
		}
		result = append(result, buf...)
	}
	e := err
	if e != nil && e == io.EOF {
		e = nil
	}

	return result, e, len(result) == 0 && err == io.EOF
}

func int2Fb(val int) int {
	e := 0
	x := val
	for x >= 16 {
		x = (x + 1) >> 1
		e++
	}
	if x < 8 {
		return x
	}
	return ((e + 1) << 3) | (x - 8)
}

func strCmp(s1, s2 string) int {
	len1 := len(s1)
	len2 := len(s2)
	for i := 0; ; i++ {
		c1 := -1
		if i < len1 {
			c1 = int(s1[i])
		}
		c2 := -1
		if i < len2 {
			c2 = int(s2[i])
		}
		switch {
		case c1 < c2:
			return -1
		case c1 > c2:
			return +1
		case c1 < 0:
			return 0
		}
	}
}

func unsafeFastStringToReadOnlyBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
