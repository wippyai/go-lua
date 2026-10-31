package lua

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	typeio "github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestTypedJSONDecodeStrict(t *testing.T) {
	row := effect.Row{Labels: []effect.Label{
		effect.Return{ReturnIndex: 0, Transform: effect.TypeValueOf{Source: effect.ParamRef{Index: 1}}},
		effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
	}}
	decode := typ.Func().Effects(row).Spec(contract.NewSpec().WithEffectRow(row)).
		Param("str", typ.String).OptParam("target", typ.NewMeta(typ.Any)).
		Returns(typ.Any, typ.NewOptional(typ.LuaError)).Build()
	manifest := typeio.NewManifest("typedjson")
	manifest.SetExport(typ.NewRecord().Field("decode", decode).Build())
	options := []testutil.Option{testutil.WithStdlib(), testutil.WithManifest("typedjson", manifest)}

	for _, tc := range []struct {
		name, code string
		wantError  bool
	}{
		{"typed result and error narrowing", `
			type User = { name: string }
			local u, err = typedjson.decode("{}", User)
			if err then return end
			local name: string = u.name
		`, false},
		{"absent target returns any", `
			local u, err = typedjson.decode("{}")
			if err then return end
			local field = u.arbitrary
		`, false},
		{"wrong field on typed result", `
			type User = { name: string }
			local u, err = typedjson.decode("{}", User)
			if err then return end
			local bad = u.missing
		`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := testutil.Check(tc.code, options...)
			if result.HasError() != tc.wantError {
				t.Fatalf("errors = %v, want error %v", testutil.ErrorMessages(result.Diagnostics), tc.wantError)
			}
			if tc.wantError && !strings.Contains(strings.Join(testutil.ErrorMessages(result.Diagnostics), " "), "missing") {
				t.Fatalf("expected missing-field error, got %v", testutil.ErrorMessages(result.Diagnostics))
			}
		})
	}
}
