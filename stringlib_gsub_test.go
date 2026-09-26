package lua

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/pm"
)

func denseGsubInput(matches int) (string, []replaceInfo) {
	const segment = "abcdefghi\n"
	str := strings.Repeat(segment, matches)
	info := make([]replaceInfo, matches)
	for i := range info {
		start := i*len(segment) + len(segment) - 1
		info[i] = replaceInfo{Indicies: []int{start, start + 1}, String: "\\n"}
	}
	return str, info
}

func TestGsubReplacementLinear(t *testing.T) {
	str, info := denseGsubInput(100_000) // 1 MB source, 100,000 matches.
	start := time.Now()
	got := strGsubDoReplace(str, info)
	elapsed := time.Since(start)
	if want := strings.Repeat("abcdefghi\\n", 100_000); got != want {
		t.Fatal("incorrect dense replacement result")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("1 MB with 100,000 matches took %v; expected linear replacement under 3s", elapsed)
	}
}

func BenchmarkGsubDenseReplacement(b *testing.B) {
	for _, matches := range []int{10_000, 20_000, 100_000} {
		b.Run(strconv.Itoa(matches), func(b *testing.B) {
			str, info := denseGsubInput(matches)
			b.SetBytes(int64(len(str)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = strGsubDoReplace(str, info)
			}
		})
	}
}

// This is the former replacement loop, kept as an oracle for randomized cases.
func legacyGsubDoReplace(str string, info []replaceInfo) string {
	offset := 0
	buf := []byte(str)
	for _, replace := range info {
		oldlen := len(buf)
		b1 := append([]byte(""), buf[0:offset+replace.Indicies[0]]...)
		b2 := []byte("")
		index2 := offset + replace.Indicies[1]
		if index2 <= len(buf) {
			b2 = append(b2, buf[index2:]...)
		}
		buf = append(b1, replace.String...)
		buf = append(buf, b2...)
		offset += len(buf) - oldlen
	}
	return string(buf)
}

func testGsubReplacementValue(arg LValue) LValue {
	switch LVAsString(arg) {
	case "a", "1":
		return LString("<A>")
	case "b", "2":
		return LFalse
	case "\\":
		return LNumber(42)
	default:
		return LNil
	}
}

func legacyGsubInfo(L *LState, str, replacement string, table *LTable, kind int, matches []*pm.MatchData) []replaceInfo {
	info := make([]replaceInfo, 0, len(matches))
	for _, match := range matches {
		start, end := match.Capture(0), match.Capture(1)
		var value LValue
		switch kind {
		case 0: // string
			sc := newFlagScanner('%', "", "", replacement)
			for c, eos := sc.Next(); !eos; c, eos = sc.Next() {
				if sc.ChangeFlag {
					continue
				}
				if sc.HasFlag {
					if c >= '0' && c <= '9' {
						sc.AppendString(capturedString(L, match, str, 2*int(c-'0')))
					} else {
						sc.AppendChar('%')
						sc.AppendChar(c)
					}
					sc.HasFlag = false
				} else {
					sc.AppendChar(c)
				}
			}
			value = LString(sc.String())
		case 1: // table
			idx := 0
			if match.CaptureLength() > 2 {
				idx = 2
			}
			if match.IsPosCapture(idx) {
				value = L.GetTable(table, LNumber(match.Capture(idx)))
			} else {
				value = L.GetField(table, str[match.Capture(idx):match.Capture(idx+1)])
			}
		case 2: // function, whose return value depends on its first argument
			idx := 0
			if match.CaptureLength() > 2 {
				idx = 2
			}
			if match.IsPosCapture(idx) {
				value = testGsubReplacementValue(LNumber(match.Capture(idx)))
			} else {
				value = testGsubReplacementValue(LString(capturedString(L, match, str, idx)))
			}
		}
		if !LVIsFalse(value) {
			info = append(info, replaceInfo{[]int{start, end}, LVAsString(value)})
		}
	}
	return info
}

func TestGsubMatchesPreviousImplementation(t *testing.T) {
	rng := rand.New(rand.NewSource(20260923))
	patterns := []string{"a", "b", "[ab]", "(a)", "([ab])", "()a", "a*", "", "^a", "b$", "(.)"}
	replacements := []string{"%0", "<%1>", "%%", "%x", "%0-%1", "[%1]%0", "", "\\\""}
	limits := []int{-1, 0, 1, 2, 5}
	chars := "ab\n\"\\"
	for trial := 0; trial < 300; trial++ {
		var src strings.Builder
		for j, n := 0, rng.Intn(30); j < n; j++ {
			src.WriteByte(chars[rng.Intn(len(chars))])
		}
		str := src.String()
		pattern := patterns[rng.Intn(len(patterns))]
		limit := limits[rng.Intn(len(limits))]
		matches, err := pm.Find(pattern, []byte(str), 0, limit)
		if err != nil {
			t.Fatalf("trial %d: %v", trial, err)
		}
		for kind := 0; kind < 3; kind++ {
			L := NewState()
			replacement := replacements[rng.Intn(len(replacements))]
			table := L.NewTable()
			for _, key := range []string{"a", "b", "\\", "\"", "\n", ""} {
				table.RawSetString(key, testGsubReplacementValue(LString(key)))
			}
			for pos := 1; pos <= len(str)+1; pos++ {
				table.RawSetInt(pos, testGsubReplacementValue(LNumber(pos)))
			}
			fn := L.NewFunction(func(L *LState) int {
				L.Push(testGsubReplacementValue(L.Get(1)))
				return 1
			})
			var repl LValue
			switch kind {
			case 0:
				repl = LString(replacement)
			case 1:
				repl = table
			case 2:
				repl = fn
			}
			want := legacyGsubDoReplace(str, legacyGsubInfo(L, str, replacement, table, kind, matches))
			if err := L.CallByParam(P{Fn: L.NewFunction(strGsub), NRet: 2, Protect: true}, LString(str), LString(pattern), repl, LNumber(limit)); err != nil {
				t.Fatalf("trial %d kind %d: %v", trial, kind, err)
			}
			got := string(L.Get(-2).(LString))
			count := int(L.Get(-1).(LNumber))
			if got != want || count != len(matches) {
				t.Fatalf("trial %d kind %d pattern %q limit %d source %q: got (%q, %d), want (%q, %d)", trial, kind, pattern, limit, str, got, count, want, len(matches))
			}
			L.Close()
		}
	}
}

func TestGsubSpecialCases(t *testing.T) {
	cases := []struct {
		source, pattern, replacement, want string
		limit, count                       int
	}{
		{"ab", "", "-", "-a-b-", -1, 3},
		{"abc", "^a", "X", "Xbc", -1, 1},
		{"abc", "c$", "X", "abX", -1, 1},
		{"aba", "a", "X", "Xba", 1, 1},
		{"ab", "(a)(b)", "%2%1%0%%", "baab%", -1, 1},
		{"abcdefghi", "(a)(b)(c)(d)(e)(f)(g)(h)(i)", "%0%1%2%3%4%5%6%7%8%9", "abcdefghiabcdefghi", -1, 1},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q/%q", tc.source, tc.pattern), func(t *testing.T) {
			L := NewState()
			defer L.Close()
			if err := L.CallByParam(P{Fn: L.NewFunction(strGsub), NRet: 2, Protect: true}, LString(tc.source), LString(tc.pattern), LString(tc.replacement), LNumber(tc.limit)); err != nil {
				t.Fatal(err)
			}
			if got, count := string(L.Get(-2).(LString)), int(L.Get(-1).(LNumber)); got != tc.want || count != tc.count {
				t.Fatalf("got (%q, %d), want (%q, %d)", got, count, tc.want, tc.count)
			}
		})
	}
}
