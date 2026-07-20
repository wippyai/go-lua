package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestAttachMetatableOperationUsesCanonicalBindingIdentity(t *testing.T) {
	base := Config{Registry: standard.Registry(), TypeValues: typevalue.NewCache(), Signatures: signaturelookup.Source{IncludeStdlib: true}}
	tests := []struct {
		name      string
		source    string
		want      bool
		configure func(*Config)
	}{
		{name: "canonical", source: `function f(instance, mt) return setmetatable(instance, mt) end`, want: true},
		{name: "local-shadow", source: `function f(instance, mt) local setmetatable = function(a) return a end return setmetatable(instance, mt) end`},
		{name: "global-replaced", source: `function f(instance, mt) setmetatable = function(a) return a end return setmetatable(instance, mt) end`},
		{name: "global-table-observed", source: `function f(instance, mt) local globals = _G return setmetatable(instance, mt) end`},
		{name: "method-spelling", source: `function f(instance, mt) local object = { setmetatable = function(self, a, b) return a end } return object:setmetatable(instance, mt) end`},
		{name: "wrong-arity", source: `function f(instance) return setmetatable(instance) end`},
		{name: "typed-global-override", source: `function f(instance, mt) return setmetatable(instance, mt) end`, configure: func(config *Config) {
			config.GlobalTypes = map[string]typ.Type{"setmetatable": typ.Func().Param("value", typ.Any).Param("meta", typ.Any).Returns(typ.Any).Build()}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := base
			if tc.configure != nil {
				tc.configure(&config)
			}
			prepared, err := PrepareFunction(parseFunction(t, tc.source), config)
			if err != nil {
				t.Fatalf("PrepareFunction: %v", err)
			}
			found := false
			for point := cfg.Point(0); int(point) < prepared.OperationPlan().PointCount(); point++ {
				op, ok := prepared.OperationPlan().AttachMetatableOperation(point)
				if !ok {
					continue
				}
				found = true
				site, siteOK := prepared.OperationPlan().Facts().CallSiteView(point)
				table, tableOK := site.ArgumentSourceAt(0)
				metatable, metatableOK := site.ArgumentSourceAt(1)
				if !siteOK || !tableOK || !metatableOK || !factflow.ValueSourceEqual(op.Table(), table) || !factflow.ValueSourceEqual(op.Metatable(), metatable) {
					t.Fatalf("typed operation at %d drifted from exact call operands", point)
				}
			}
			if found != tc.want {
				t.Fatalf("typed attach-metatable operation = %v, want %v", found, tc.want)
			}
		})
	}
}
