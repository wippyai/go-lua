package internal

import "testing"

// Expected bytes come from the Lua 5.4 reference interpreter ("\u{X}" escapes).
func TestAppendUTF8Esc(t *testing.T) {
	tests := []struct {
		x    uint32
		want string
	}{
		{0x0, "\x00"},
		{0x7F, "\x7f"},
		{0x80, "\xc2\x80"},
		{0x7FF, "\xdf\xbf"},
		{0x800, "\xe0\xa0\x80"},
		{0xD800, "\xed\xa0\x80"},
		{0xFFFF, "\xef\xbf\xbf"},
		{0x10000, "\xf0\x90\x80\x80"},
		{0x10FFFF, "\xf4\x8f\xbf\xbf"},
		{0x110000, "\xf4\x90\x80\x80"},
		{0x1FFFFF, "\xf7\xbf\xbf\xbf"},
		{0x200000, "\xf8\x88\x80\x80\x80"},
		{0x3FFFFFF, "\xfb\xbf\xbf\xbf\xbf"},
		{0x4000000, "\xfc\x84\x80\x80\x80\x80"},
		{MaxUTF8Esc, "\xfd\xbf\xbf\xbf\xbf\xbf"},
	}
	for _, tt := range tests {
		if got := string(AppendUTF8Esc([]byte("p"), tt.x)); got != "p"+tt.want {
			t.Errorf("AppendUTF8Esc(%#x) = %q, want %q", tt.x, got[1:], tt.want)
		}
	}
}
