package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestStringUnpackValueTypes(t *testing.T) {
	tests := []struct {
		format string
		want   []typ.Type
		ok     bool
	}{
		{format: ">I4", want: []typ.Type{typ.Integer, typ.Integer}, ok: true},
		{format: "< i2", want: []typ.Type{typ.Integer, typ.Integer}, ok: true},
		{format: "!8 x X B", want: []typ.Type{typ.Integer, typ.Integer}, ok: true},
		{format: ">d", want: []typ.Type{typ.Number, typ.Integer}, ok: true},
		{format: "=f", want: []typ.Type{typ.Number, typ.Integer}, ok: true},
		{format: ">c16", want: []typ.Type{typ.String, typ.Integer}, ok: true},
		{format: "s2", want: []typ.Type{typ.String, typ.Integer}, ok: true},
		{format: "z", want: []typ.Type{typ.String, typ.Integer}, ok: true},
		{format: ">I2 c3 z", want: []typ.Type{typ.Integer, typ.String, typ.String, typ.Integer}, ok: true},
		{format: "", want: []typ.Type{typ.Integer}, ok: true},
		{format: "?", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got, ok := stringUnpackValueTypes(tt.format)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("types = %v, want %v", got, tt.want)
			}
			for i := range got {
				if !typ.TypeEquals(got[i], tt.want[i]) {
					t.Fatalf("type %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
