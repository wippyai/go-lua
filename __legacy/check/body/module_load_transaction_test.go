package body

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestExecutionFactoryFreezesDynamicModuleLoadN0Authority(t *testing.T) {
	stmts, err := parse.ParseString(`local module_name = "alpha"; return require(module_name)`, "module-load-transaction.lua")
	if err != nil {
		t.Fatal(err)
	}
	alpha := manifest.New("alpha")
	alpha.SetExport(typ.Number)
	reg := standard.Registry()
	config := Config{
		Registry: reg, TypeValues: typevalue.NewCache(), Globals: []string{"require"},
		UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("module-load-transaction")),
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{alpha}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: Globals(config)})
	prepared, err := PrepareBoundChunk(stmts, bindings, config)
	if err != nil {
		t.Fatal(err)
	}
	ctx, session := cancellation.Attach(context.Background())
	factory, err := prepared.NewExecutionFactory(ExecutionFactoryConfig{Context: ctx, Session: session})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for raw := 0; raw < factory.Graph().Size(); raw++ {
		point := cfg.Point(raw)
		transaction, ok := factory.ModuleLoadTransaction(point)
		if !ok {
			continue
		}
		found = true
		if !transaction.Valid(reg) || transaction.Argument().Kind == factflow.ValueSourceLiteral ||
			!transaction.OperationID().Available() || !transaction.TableID().Available() {
			t.Fatalf("factory module transaction = %#v", transaction)
		}
		resolved, exact := transaction.Resolve(reg, typevalue.LiteralString(reg, "alpha"))
		if !exact || !resolved.Valid(reg) || !resolved.PostReturnAuthority() {
			t.Fatalf("factory module N0 = %#v/%v", resolved, exact)
		}
	}
	if !found {
		t.Fatal("execution factory omitted prepared dynamic module-load transaction")
	}
}
