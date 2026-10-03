package pm

import (
	"strings"
	"testing"
)

func TestFind_Literal(t *testing.T) {
	matches, err := Find("hello", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestProgramFindStringOne(t *testing.T) {
	program, err := Compile("a+")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	md, err := program.FindStringOne("xxaaay", 0)
	if err != nil {
		t.Fatalf("FindStringOne failed: %v", err)
	}
	defer ReleaseMatch(md)
	if md == nil {
		t.Fatal("FindStringOne returned nil match")
	}
	if got, want := md.Capture(0), 2; got != want {
		t.Fatalf("match start = %d, want %d", got, want)
	}
	if got, want := md.Capture(1), 5; got != want {
		t.Fatalf("match end = %d, want %d", got, want)
	}
}

func TestFind_Dot(t *testing.T) {
	matches, err := Find("h.llo", []byte("hello hallo hullo"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFind_CharacterClass(t *testing.T) {
	matches, err := Find("%d+", []byte("abc123def456"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestFind_Capture(t *testing.T) {
	matches, err := Find("(%d+)", []byte("test123end"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.CaptureLength() < 4 {
		t.Fatalf("expected at least 4 captures, got %d", m.CaptureLength())
	}
	start := m.Capture(2)
	end := m.Capture(3)
	if start != 4 || end != 7 {
		t.Fatalf("expected capture at 4-7, got %d-%d", start, end)
	}
}

func TestFind_StarRepeat(t *testing.T) {
	matches, err := Find("a*b", []byte("b ab aaab"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) < 1 {
		t.Fatalf("expected at least 1 match, got %d", len(matches))
	}
}

func TestFind_PlusRepeat(t *testing.T) {
	matches, err := Find("a+", []byte("a aa aaa"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFind_MinusRepeat(t *testing.T) {
	matches, err := Find("a-b", []byte("aaab"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_QuestionRepeat(t *testing.T) {
	matches, err := Find("a?b", []byte("b ab"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestFind_AnchorStart(t *testing.T) {
	matches, err := Find("^hello", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matches, err = Find("^hello", []byte("say hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_AnchorEnd(t *testing.T) {
	matches, err := Find("world$", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matches, err = Find("world$", []byte("world hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestFind_CharacterClasses(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		expect  int
	}{
		{"%a+", "abc123", 1},
		{"%d+", "abc123def", 1},
		{"%s+", "a b c", 2},
		{"%w+", "hello world!", 2},
		{"%l+", "Hello", 1},
		{"%u+", "Hello", 1},
	}

	for _, tt := range tests {
		matches, err := Find(tt.pattern, []byte(tt.input), 0, -1)
		if err != nil {
			t.Fatalf("pattern %q on %q: unexpected error: %v", tt.pattern, tt.input, err)
		}
		if len(matches) != tt.expect {
			t.Fatalf("pattern %q on %q: expected %d matches, got %d", tt.pattern, tt.input, tt.expect, len(matches))
		}
	}
}

func TestFind_HeadAnchorRespectsOffset(t *testing.T) {
	matches, err := Find("^%s*(.-)%s*$", []byte(" shared text "), 1, -1)
	if err != nil {
		t.Fatalf("Find anchored with offset: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("anchored pattern with non-zero offset returned %d matches, want 0", len(matches))
	}
	md, err := FindStringOne("^%s*(.-)%s*$", " shared text ", len(" shared text "))
	if err != nil {
		t.Fatalf("FindStringOne anchored with offset: %v", err)
	}
	if md != nil {
		ReleaseMatch(md)
		t.Fatal("anchored FindStringOne with non-zero offset returned a match")
	}
}

func TestFind_Set(t *testing.T) {
	matches, err := Find("[aeiou]+", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
}

func TestFind_NegatedSet(t *testing.T) {
	matches, err := Find("[^aeiou]+", []byte("hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) < 1 {
		t.Fatalf("expected at least 1 match, got %d", len(matches))
	}
}

func TestFind_Range(t *testing.T) {
	matches, err := Find("[a-z]+", []byte("Hello123"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_Backreference(t *testing.T) {
	matches, err := Find("(%a+)%s+%1", []byte("hello hello"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	matches, err = Find("(%a+)%s+%1", []byte("hello world"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matches))
	}
}

func TestCompileRejectsBackreferenceToOpenCapture(t *testing.T) {
	if _, err := Compile("(%1)"); err == nil {
		t.Fatal("Compile accepted backreference to an open capture")
	}
	if _, err := Compile("(%a)%1"); err != nil {
		t.Fatalf("Compile rejected backreference to a closed capture: %v", err)
	}
}

func TestFind_BalancedBraces(t *testing.T) {
	matches, err := Find("%b()", []byte("test (nested (braces)) here"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestFind_PositionCapture(t *testing.T) {
	matches, err := Find("()test()", []byte("test"), 0, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	m := matches[0]
	if m.CaptureLength() < 6 {
		t.Fatalf("expected at least 6 captures for position captures, got %d", m.CaptureLength())
	}
}

func TestFind_Limit(t *testing.T) {
	matches, err := Find("%d", []byte("1234567890"), 0, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches (limited), got %d", len(matches))
	}
}

func TestFind_Offset(t *testing.T) {
	matches, err := Find("test", []byte("test test"), 5, -1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected 1 match (after offset), got %d", len(matches))
	}
}

func TestFind_InvalidPattern(t *testing.T) {
	_, err := Find("(unclosed", []byte("test"), 0, -1)
	if err == nil {
		t.Fatal("expected error for unclosed capture")
	}
}

func TestFind_InvalidCaptureIndex(t *testing.T) {
	_, err := Find("%0", []byte("test"), 0, -1)
	if err == nil {
		t.Fatal("expected error for invalid capture index")
	}
}

func TestFind_DanglingEscape(t *testing.T) {
	_, err := Find("%", []byte("test"), 0, -1)
	if err == nil || !strings.Contains(err.Error(), "unexpected EOS") {
		t.Fatalf("Find error = %v, want dangling escape error", err)
	}
}

func TestFind_IncompleteBalancedEscape(t *testing.T) {
	for _, pattern := range []string{"%b", "%b("} {
		_, err := Find(pattern, []byte("test"), 0, -1)
		if err == nil || !strings.Contains(err.Error(), "unfinished balanced pattern") {
			t.Fatalf("Find(%q) error = %v, want balanced pattern error", pattern, err)
		}
	}
}

func TestFind_UnmatchedParen(t *testing.T) {
	_, err := Find("test)", []byte("test"), 0, -1)
	if err == nil {
		t.Fatal("expected error for unmatched paren")
	}
}

func TestFind_BalancedMatchChargesByteScanBudget(t *testing.T) {
	program, err := Compile("%b()")
	if err != nil {
		t.Fatalf("compile pattern: %v", err)
	}
	err = runPatternWithUsedByteBudget(program, []byte("(x"), MaxVMByteScans)
	if err == nil || !strings.Contains(err.Error(), "byte scan limit") {
		t.Fatalf("run error = %v, want byte scan limit", err)
	}
}

func TestFind_BackrefChargesByteScanBudget(t *testing.T) {
	program, err := Compile("(a)%1")
	if err != nil {
		t.Fatalf("compile pattern: %v", err)
	}
	err = runPatternWithUsedByteBudget(program, []byte("ab"), MaxVMByteScans)
	if err == nil || !strings.Contains(err.Error(), "byte scan limit") {
		t.Fatalf("run error = %v, want byte scan limit", err)
	}
}

func runPatternWithUsedByteBudget(program Program, src []byte, used int) error {
	v := newVM(src, program.insts, nil)
	defer v.release()
	v.byteScans = used
	md := newMatchData(program.capSize)
	defer md.release()
	v.matchData = md
	_, _, err := v.run(0, 0)
	return err
}

func TestError_String(t *testing.T) {
	tests := []struct {
		name    string
		err     *Error
		wantStr string
	}{
		{
			name:    "normal position",
			err:     &Error{Pos: 5, Message: "invalid pattern"},
			wantStr: "invalid pattern",
		},
		{
			name:    "EOS position",
			err:     &Error{Pos: eos, Message: "unexpected end"},
			wantStr: "unexpected end",
		},
		{
			name:    "unknown position",
			err:     &Error{Pos: unknownPos, Message: "unknown error"},
			wantStr: "unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.String(); got != tt.wantStr {
				t.Errorf("String() = %q, want %q", got, tt.wantStr)
			}

			result := "error: " + tt.err.String()
			if result != "error: "+tt.wantStr {
				t.Errorf("concatenation failed")
			}
		})
	}
}

func TestPatternCache_LRU(t *testing.T) {
	cache := newPatternCache(3)

	patterns := []string{"a", "b", "c", "d"}
	for _, p := range patterns {
		pat, err := parsePattern(newScanner([]byte(p)), true)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		insts := compilePattern(pat, nil)
		cache.put(p, insts, pat, calcMaxCaptureSlot(insts))
	}

	// "a" should have been evicted (LRU)
	if _, _, _, ok := cache.get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}

	// "b", "c", "d" should still be present
	for _, p := range []string{"b", "c", "d"} {
		if _, _, _, ok := cache.get(p); !ok {
			t.Errorf("expected %q to be in cache", p)
		}
	}
}

func TestMatchCharClass(t *testing.T) {
	tests := []struct {
		code  int
		ch    int
		match bool
	}{
		{'a', 'x', true},
		{'a', 'X', true},
		{'a', '1', false},
		{'A', 'x', false},
		{'A', '1', true},
		{'d', '5', true},
		{'d', 'a', false},
		{'D', '5', false},
		{'D', 'a', true},
		{'s', ' ', true},
		{'s', 'a', false},
		{'w', 'a', true},
		{'w', '5', true},
		{'w', ' ', false},
	}

	for _, tt := range tests {
		result := matchCharClass(tt.code, tt.ch)
		if result != tt.match {
			t.Errorf("matchCharClass(%c, %c) = %v, want %v", tt.code, tt.ch, result, tt.match)
		}
	}
}

func TestFind_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		wantN   int
		wantErr bool
	}{
		{"empty pattern", "", "test", 5, false},
		{"empty input", "a", "", 0, false},
		{"both empty", "", "", 1, false},
		{"trailing percent", "%", "test", 0, true},
		{"trailing backslash b", "%b", "test", 0, true},
		{"unclosed bracket", "[abc", "test", 0, true},
		{"deeply nested", "((((a))))", "a", 1, false},
		{"pattern too complex", string(make([]byte, 300)), "test", 0, true},
		{"dollar in middle", "a$b", "a$b", 1, false},
		{"caret in middle", "a^b", "a^b", 1, false},
		{"multiple anchors", "^test$", "test", 1, false},
		{"anchor no match", "^test$", "testing", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a complex pattern for "pattern too complex" test
			pat := tt.pattern
			if tt.name == "pattern too complex" {
				pat = ""
				for i := 0; i < 250; i++ {
					pat += "("
				}
			}

			matches, err := Find(pat, []byte(tt.input), 0, -1)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if len(matches) != tt.wantN {
				t.Errorf("expected %d matches, got %d", tt.wantN, len(matches))
			}
		})
	}
}

func TestFind_MaxBacktracks(t *testing.T) {
	// Pattern that causes exponential backtracking
	pattern := "a*a*a*a*a*a*a*a*a*a*b"
	input := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 31 a's, no b

	matches, err := Find(pattern, []byte(input), 0, -1)
	if err == nil {
		t.Fatal("expected backtrack limit error")
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestPUCBigStringAnchoredRepeats(t *testing.T) {
	input := strings.Repeat("a", 300000)

	md, err := FindStringOne("^a*.?$", input, 0)
	if err != nil {
		t.Fatalf("FindStringOne(^a*.?$) error = %v", err)
	}
	if md == nil {
		t.Fatal("FindStringOne(^a*.?$) returned no match")
	}
	ReleaseMatch(md)

	md, err = FindStringOne("^a*.?b$", input, 0)
	if err != nil {
		t.Fatalf("FindStringOne(^a*.?b$) error = %v", err)
	}
	if md != nil {
		ReleaseMatch(md)
		t.Fatal("FindStringOne(^a*.?b$) returned a match")
	}

	md, err = FindStringOne("^a-.?$", input, 0)
	if err != nil {
		t.Fatalf("FindStringOne(^a-.?$) error = %v", err)
	}
	if md == nil {
		t.Fatal("FindStringOne(^a-.?$) returned no match")
	}
	ReleaseMatch(md)
}

func TestFind_RejectsPatternByteLimit(t *testing.T) {
	_, err := Find(strings.Repeat("a", MaxPatternBytes+1), []byte("a"), 0, 1)
	if err == nil {
		t.Fatal("expected pattern byte limit error")
	}
	if !strings.Contains(err.Error(), "pattern too large") {
		t.Fatalf("error = %v", err)
	}
}

func TestFind_RejectsCaptureSlotLimit(t *testing.T) {
	_, err := Find(strings.Repeat("()", MaxCaptureSlots/2), []byte(""), 0, 1)
	if err == nil {
		t.Fatal("expected capture slot limit error")
	}
	if !strings.Contains(err.Error(), "too many captures") {
		t.Fatalf("error = %v", err)
	}
}

func TestFind_SearchPositionLimit(t *testing.T) {
	input := make([]byte, MaxSearchPositions+1)
	for i := range input {
		input[i] = 'a'
	}

	matches, err := Find("b", input, 0, 1)
	if err == nil {
		t.Fatal("expected search position limit error")
	}
	if !strings.Contains(err.Error(), "pattern search position limit exceeded") {
		t.Fatalf("error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0 on limit error", len(matches))
	}
}

func TestFind_LimitStopsBeforeSearchPositionLimit(t *testing.T) {
	input := make([]byte, MaxSearchPositions+1)
	for i := range input {
		input[i] = 'a'
	}

	matches, err := Find(".", input, 0, 1)
	if err != nil {
		t.Fatalf("limit=1 should stop after first match: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
}

func TestFind_AnchoredPatternSkipsSearchPositionLimit(t *testing.T) {
	input := make([]byte, MaxSearchPositions+1)
	for i := range input {
		input[i] = 'a'
	}

	matches, err := Find("^b", input, 0, -1)
	if err != nil {
		t.Fatalf("anchored miss should not scan every offset: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0", len(matches))
	}
}

func TestFind_NegativeOffsetClampsToStart(t *testing.T) {
	matches, err := Find("a", []byte("abc"), -100, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(matches))
	}
}

func TestFind_UnboundedMatchCountLimit(t *testing.T) {
	input := make([]byte, MaxMatches+1)
	for i := range input {
		input[i] = 'a'
	}
	matches, err := Find(".", input, 0, -1)
	if err == nil {
		t.Fatal("expected match count limit error")
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0 on limit error", len(matches))
	}
}

func TestFind_LargePositiveLimitUsesBoundedCapacity(t *testing.T) {
	if got := boundedMatchCap(MaxMatches + 1); got != MaxMatches {
		t.Fatalf("boundedMatchCap(MaxMatches+1) = %d, want %d", got, MaxMatches)
	}
	if got := boundedMatchCap(3); got != 3 {
		t.Fatalf("boundedMatchCap(3) = %d, want 3", got)
	}
	if got := boundedMatchCap(-1); got != 0 {
		t.Fatalf("boundedMatchCap(-1) = %d, want 0", got)
	}

	matches, err := Find(".", []byte("abc"), 0, MaxMatches*100)
	if err != nil {
		t.Fatalf("Find with large positive limit failed: %v", err)
	}
	if len(matches) != 3 {
		t.Fatalf("matches = %d, want 3", len(matches))
	}
	ReleaseMatches(matches)
}

func TestFind_EmptyPatternUnboundedMatchCountLimit(t *testing.T) {
	input := make([]byte, MaxMatches)
	for i := range input {
		input[i] = 'a'
	}
	matches, err := Find("", input, 0, -1)
	if err == nil {
		t.Fatal("expected match count limit error for zero-width matches")
	}
	if !strings.Contains(err.Error(), "pattern match count limit exceeded") {
		t.Fatalf("error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0 on limit error", len(matches))
	}
}

func TestFind_OffsetPastEndSkipsExplosiveMatcher(t *testing.T) {
	input := make([]byte, MaxBacktrackStack+1)
	for i := range input {
		input[i] = 'a'
	}

	matches, err := Find(".*b", input, len(input)+1, -1)
	if err != nil {
		t.Fatalf("offset past end should skip explosive matcher: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0", len(matches))
	}
}

func TestFind_LimitZeroSkipsSearchWork(t *testing.T) {
	input := make([]byte, MaxBacktrackStack+1)
	for i := range input {
		input[i] = 'a'
	}

	matches, err := Find(".*b", input, 0, 0)
	if err != nil {
		t.Fatalf("limit=0 should not execute explosive matcher: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0", len(matches))
	}
}

func TestFind_BacktrackStackLimit(t *testing.T) {
	input := make([]byte, MaxBacktrackStack+1)
	for i := range input {
		input[i] = 'a'
	}
	matches, err := Find(".*b", input, 0, 1)
	if err == nil {
		t.Fatal("expected backtrack stack limit error")
	}
	if len(matches) != 0 {
		t.Fatalf("matches = %d, want 0 on limit error", len(matches))
	}
}

func TestFind_ConcurrentSafety(t *testing.T) {
	pattern := "%d+"
	inputs := []string{
		"test123abc",
		"456def789",
		"no digits here",
		"1",
		"123456789",
	}

	done := make(chan bool, len(inputs))
	for _, input := range inputs {
		go func(in string) {
			for i := 0; i < 100; i++ {
				_, _ = Find(pattern, []byte(in), 0, -1)
			}
			done <- true
		}(input)
	}

	for i := 0; i < len(inputs); i++ {
		<-done
	}
}

func TestMatchData_Bounds(t *testing.T) {
	md := &MatchData{captures: []uint32{0, 10, 20}}

	// Test out of bounds access
	if md.Capture(-1) != 0 {
		t.Error("Capture(-1) should return 0")
	}
	if md.Capture(100) != 0 {
		t.Error("Capture(100) should return 0")
	}
	if md.IsPosCapture(-1) {
		t.Error("IsPosCapture(-1) should return false")
	}
	if md.IsPosCapture(100) {
		t.Error("IsPosCapture(100) should return false")
	}
}

func TestReleaseMatchesAcceptsReturnedMatches(t *testing.T) {
	ReleaseMatches(nil)

	matches, err := Find("a", []byte("abc"), 0, 1)
	if err != nil {
		t.Fatalf("Find failed: %v", err)
	}
	if len(matches) != 1 {
		ReleaseMatches(matches)
		t.Fatalf("matches = %d, want 1", len(matches))
	}
	ReleaseMatches(matches)
	ReleaseMatches(matches)
}

func TestFindOneStreamsOneMatch(t *testing.T) {
	md, err := FindStringOne(".", "abc", 0)
	if err != nil {
		t.Fatalf("FindStringOne failed: %v", err)
	}
	if md == nil {
		t.Fatal("FindStringOne returned nil match")
	}
	if got := md.Capture(0); got != 0 {
		ReleaseMatch(md)
		t.Fatalf("match start = %d, want 0", got)
	}
	if got := md.Capture(1); got != 1 {
		ReleaseMatch(md)
		t.Fatalf("match end = %d, want 1", got)
	}
	ReleaseMatch(md)
	ReleaseMatch(md)

	md, err = FindStringOne(".", "abc", 1)
	if err != nil {
		t.Fatalf("FindStringOne with offset failed: %v", err)
	}
	if md == nil {
		t.Fatal("FindStringOne with offset returned nil match")
	}
	if got := md.Capture(0); got != 1 {
		ReleaseMatch(md)
		t.Fatalf("offset match start = %d, want 1", got)
	}
	ReleaseMatch(md)

	md, err = FindStringOne("%a+", "hello world", 0)
	if err != nil {
		t.Fatalf("FindStringOne word failed: %v", err)
	}
	if md == nil {
		t.Fatal("FindStringOne word returned nil match")
	}
	if got := md.CaptureLength(); got != 2 {
		ReleaseMatch(md)
		t.Fatalf("word capture length = %d, want 2", got)
	}
	if got := "hello world"[md.Capture(0):md.Capture(1)]; got != "hello" {
		ReleaseMatch(md)
		t.Fatalf("word match = %q, want hello", got)
	}
	ReleaseMatch(md)
}

func TestFind_ConcurrentAdversarialLimits(t *testing.T) {
	cases := []struct {
		pattern string
		input   string
		limit   int
		wantErr bool
	}{
		{strings.Repeat("a", MaxPatternBytes+1), "a", 1, true},
		{strings.Repeat("()", MaxCaptureSlots/2), "", 1, true},
		{".*b", strings.Repeat("a", MaxBacktrackStack+1), 1, true},
		{"a*a*a*a*a*a*a*a*a*a*b", strings.Repeat("a", 64), -1, true},
		{"^b", strings.Repeat("a", 1024), -1, false},
		{"%b()", strings.Repeat("(", 1024), 1, false},
	}

	errc := make(chan error, 8)
	for worker := 0; worker < 8; worker++ {
		go func() {
			for i := 0; i < 20; i++ {
				for _, tc := range cases {
					_, err := FindString(tc.pattern, tc.input, 0, tc.limit)
					if tc.wantErr && err == nil {
						errc <- newError(unknownPos, "expected adversarial limit error for %q", tc.pattern)
						return
					}
					if !tc.wantErr && err != nil {
						errc <- err
						return
					}
				}
			}
			errc <- nil
		}()
	}

	for i := 0; i < 8; i++ {
		if err := <-errc; err != nil {
			t.Fatalf("concurrent adversarial Find failed: %v", err)
		}
	}
}

func FuzzFindBoundedAdversarial(f *testing.F) {
	for _, seed := range []struct {
		pattern string
		src     string
		offset  int
		limit   int
	}{
		{"", "", 0, -1},
		{".*b", strings.Repeat("a", 128), 0, 1},
		{"a*a*a*a*a*a*a*a*a*a*b", strings.Repeat("a", 64), 0, 1},
		{"(%a+)%s+%1", "hello hello", 0, 4},
		{strings.Repeat("()", 16), "", 0, 1},
		{"[%z-\xff]+", "abc\x00def", -8, 2},
		{"%b()", strings.Repeat("(", 64), 0, 1},
	} {
		f.Add(seed.pattern, seed.src, seed.offset, seed.limit)
	}

	f.Fuzz(func(t *testing.T, pattern, src string, offset, limit int) {
		if len(pattern) > 512 || len(src) > 2048 {
			return
		}
		if limit < 0 {
			limit = -1
		} else {
			limit %= 9
		}
		if len(src) > 0 {
			offset %= len(src) * 2
		} else {
			offset = 0
		}
		_, _ = FindString(pattern, src, offset, limit)
	})
}
