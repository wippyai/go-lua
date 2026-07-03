package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestFirstStringUnpackValueType(t *testing.T) {
	tests := []struct {
		format string
		want   typ.Type
		ok     bool
	}{
		{format: ">I4", want: typ.Integer, ok: true},
		{format: "< i2", want: typ.Integer, ok: true},
		{format: "!8 x B", want: typ.Integer, ok: true},
		{format: ">d", want: typ.Number, ok: true},
		{format: "=f", want: typ.Number, ok: true},
		{format: ">c16", want: typ.String, ok: true},
		{format: "s2", want: typ.String, ok: true},
		{format: "z", want: typ.String, ok: true},
		{format: "", ok: false},
		{format: "?", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, ok := firstStringUnpackValueType(tt.format)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if tt.ok && !typ.TypeEquals(got, tt.want) {
				t.Fatalf("type = %v, want %v", got, tt.want)
			}
		})
	}
}
