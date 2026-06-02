package facts

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/ref"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/domain/callbackenv"
	"github.com/wippyai/go-lua/compiler/check/domain/fieldkey"
	"github.com/wippyai/go-lua/compiler/check/domain/guard"
	"github.com/wippyai/go-lua/compiler/check/domain/keyscoll"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestModuleFactsCanonicalSortedAndDefensive(t *testing.T) {
	refA := ref.FuncRef{GraphID: 1}
	refB := ref.FuncRef{GraphID: 2}

	m := Module{
		moduleAliases: []topology.ModuleAlias{
			{Symbol: cfg.SymbolID(9), Type: typ.Number},
			{Symbol: cfg.SymbolID(3), Type: typ.String},
			{Symbol: cfg.SymbolID(3), Type: typ.String},
		},
		noReturn: []ref.FuncRef{refB, refA, refA},
		predicateByFunc: []guard.PredicateFunction{
			{FuncSym: 9, ParamIndex: 1, Kind: "number"},
			{FuncSym: 3, ParamIndex: 0, Kind: "string"},
			{FuncSym: 3, ParamIndex: 2, Kind: "boolean"},
		},
		predicateByCondSym: []predicateResultRow{
			{FuncRef: refB, Result: guard.PredicateResult{CondSym: cfg.SymbolID(8), NarrowSym: 80, Kind: "number"}},
			{FuncRef: refA, Result: guard.PredicateResult{CondSym: cfg.SymbolID(4), NarrowSym: 40, Kind: "string"}},
			{FuncRef: refA, Result: guard.PredicateResult{CondSym: cfg.SymbolID(2), NarrowSym: 20, Kind: "boolean"}},
			{FuncRef: refA, Result: guard.PredicateResult{CondSym: cfg.SymbolID(2), NarrowSym: 99, Kind: "number"}},
		},
		callbackEnv: []callbackEnvRow{
			{FuncRef: refB, Binding: callbackenv.GlobalBinding{Symbol: cfg.SymbolID(7)}},
			{FuncRef: refA, Binding: callbackenv.GlobalBinding{Symbol: cfg.SymbolID(5)}},
			{FuncRef: refA, Binding: callbackenv.GlobalBinding{Symbol: cfg.SymbolID(1)}},
			{FuncRef: refA, Binding: callbackenv.GlobalBinding{Symbol: cfg.SymbolID(1)}},
		},
		keysCollectors: []keysCollectorRow{
			{FuncRef: refB, Info: keyscoll.KeysCollector{ParamIndex: 2, ReturnIndex: 1}},
			{FuncRef: refA, Info: keyscoll.KeysCollector{ParamIndex: 0, ReturnIndex: 0}},
			{FuncRef: refA, Info: keyscoll.KeysCollector{ParamIndex: 0, ReturnIndex: 0}},
		},
		typeChecks: []typeCheckBindRow{
			{FuncRef: refB, Bind: guard.TypeCheckBind{ErrSym: cfg.SymbolID(8), NarrowSyms: []cfg.SymbolID{80}, Type: typ.Boolean}},
			{FuncRef: refA, Bind: guard.TypeCheckBind{ErrSym: cfg.SymbolID(4), NarrowSyms: []cfg.SymbolID{40}, Type: typ.String}},
			{FuncRef: refA, Bind: guard.TypeCheckBind{ErrSym: cfg.SymbolID(2), NarrowSyms: []cfg.SymbolID{20}, Type: typ.Number}},
			{FuncRef: refA, Bind: guard.TypeCheckBind{ErrSym: cfg.SymbolID(2), NarrowSyms: []cfg.SymbolID{20}, Type: typ.Number}},
		},
		functionBindings: []topology.FunctionBinding{
			{Symbol: cfg.SymbolID(9), FuncRef: refB, Order: 2},
			{Symbol: cfg.SymbolID(3), FuncRef: refA, Order: 0},
			{Symbol: cfg.SymbolID(3), FuncRef: refB, Order: 1},
		},
		fieldFunctions: []topology.FieldFunction{
			{ContainerSym: cfg.SymbolID(9), Field: mustFieldKey("z"), FuncRef: refB, Order: 2},
			{ContainerSym: cfg.SymbolID(3), Field: mustFieldKey("new"), FuncRef: refA, Order: 0},
			{ContainerSym: cfg.SymbolID(3), Field: mustFieldKey("new"), FuncRef: refB, Order: 1},
			{ContainerSym: cfg.SymbolID(3), Field: mustFieldKey("new"), FuncRef: refA, Order: 3},
		},
		metatableIndexes: []metatable.Index{
			{MetatableSym: cfg.SymbolID(9), PrototypeSym: cfg.SymbolID(8)},
			{MetatableSym: cfg.SymbolID(3), PrototypeSym: cfg.SymbolID(2)},
			{MetatableSym: cfg.SymbolID(3), PrototypeSym: cfg.SymbolID(2)},
		},
		methodReceivers: []methodReceiverEntry{
			{FuncRef: refB, Info: metatable.MethodReceiver{PrototypeSym: cfg.SymbolID(8), SelfSlot: 0}},
			{FuncRef: refA, Info: metatable.MethodReceiver{PrototypeSym: cfg.SymbolID(2), SelfSlot: 0}},
			{FuncRef: refA, Info: metatable.MethodReceiver{PrototypeSym: cfg.SymbolID(2), SelfSlot: 0}},
		},
		prototypeMethods: []metatable.PrototypeMethod{
			{PrototypeSym: cfg.SymbolID(8), Field: mustFieldKey("z"), FuncRef: flow.FunctionRef{GraphID: 2}},
			{PrototypeSym: cfg.SymbolID(2), Field: mustFieldKey("b"), FuncRef: flow.FunctionRef{GraphID: 1}},
			{PrototypeSym: cfg.SymbolID(2), Field: mustFieldKey("a"), FuncRef: flow.FunctionRef{GraphID: 1}},
			{PrototypeSym: cfg.SymbolID(2), Field: mustFieldKey("a"), FuncRef: flow.FunctionRef{GraphID: 1}},
		},
		setMetatableSites: []setMetatableSiteEntry{
			{FuncRef: refB, Info: metatable.SetMetatableSite{Point: cfg.Point(9), MetatableSym: cfg.SymbolID(9), PrototypeSym: cfg.SymbolID(8)}},
			{FuncRef: refA, Info: metatable.SetMetatableSite{Point: cfg.Point(4), MetatableSym: cfg.SymbolID(3), PrototypeSym: cfg.SymbolID(2)}},
			{FuncRef: refA, Info: metatable.SetMetatableSite{Point: cfg.Point(4), MetatableSym: cfg.SymbolID(3), PrototypeSym: cfg.SymbolID(2)}},
		},
	}
	sortModuleFacts(&m)

	if got, ok := m.ModuleAliasType(cfg.SymbolID(3)); !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("ModuleAliasType(3) = %v/%v, want string/true", got, ok)
	}
	if got, ok := m.ModuleAliasType(cfg.SymbolID(9)); !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("ModuleAliasType(9) = %v/%v, want number/true", got, ok)
	}
	if _, ok := m.ModuleAliasType(cfg.SymbolID(99)); ok {
		t.Fatal("ModuleAliasType(99) should be absent")
	}
	if got := len(m.moduleAliases); got != 2 {
		t.Fatalf("module alias facts len = %d, want 2", got)
	}

	if !m.HasNoReturn(refA) || !m.HasNoReturn(refB) {
		t.Fatal("sorted no-return facts did not retain refs")
	}
	if got := len(m.noReturn); got != 2 {
		t.Fatalf("no-return facts len = %d, want 2", got)
	}

	preds := m.PredicateFacts()
	if got := len(preds); got != 2 {
		t.Fatalf("predicate facts len = %d, want 2", got)
	}
	if preds[0].FuncSym != cfg.SymbolID(3) || preds[1].FuncSym != cfg.SymbolID(9) {
		t.Fatalf("predicate facts not sorted by symbol: %+v", preds)
	}
	preds[0].FuncSym = 99
	if again := m.PredicateFacts(); again[0].FuncSym != cfg.SymbolID(3) {
		t.Fatalf("PredicateFacts exposed mutable backing store: %+v", again)
	}

	guards := m.PredicateGuards(refA)
	if got := len(guards); got != 2 {
		t.Fatalf("predicate guards for refA len = %d, want 2", got)
	}
	if guards[0].CondSym != cfg.SymbolID(2) || guards[1].CondSym != cfg.SymbolID(4) {
		t.Fatalf("predicate guards not sorted by condition symbol: %+v", guards)
	}
	guards[0].CondSym = 99
	if again := m.PredicateGuards(refA); again[0].CondSym != cfg.SymbolID(2) {
		t.Fatalf("PredicateGuards exposed mutable backing store: %+v", again)
	}

	env := m.CallbackEnv(refA)
	if got := len(env); got != 2 {
		t.Fatalf("callback env for refA len = %d, want 2", got)
	}
	if env[0].Symbol != cfg.SymbolID(1) || env[1].Symbol != cfg.SymbolID(5) {
		t.Fatalf("callback env not sorted by symbol: %+v", env)
	}
	env[0].Symbol = 99
	if again := m.CallbackEnv(refA); again[0].Symbol != cfg.SymbolID(1) {
		t.Fatalf("CallbackEnv exposed mutable backing store: %+v", again)
	}

	kc, ok := m.KeysCollector(refA)
	if !ok {
		t.Fatal("KeysCollector(refA) not found")
	}
	if kc.ParamIndex != 0 || kc.ReturnIndex != 0 {
		t.Fatalf("KeysCollector(refA) = %+v, want param 0 return 0", kc)
	}
	if got := len(m.keysCollectors); got != 2 {
		t.Fatalf("keys collector facts len = %d, want 2", got)
	}

	typeChecks := m.TypeChecks(refA)
	if got := len(typeChecks); got != 2 {
		t.Fatalf("type-check binds for refA len = %d, want 2", got)
	}
	if typeChecks[0].ErrSym != cfg.SymbolID(2) || typeChecks[1].ErrSym != cfg.SymbolID(4) {
		t.Fatalf("type-check binds not sorted by err symbol: %+v", typeChecks)
	}
	typeChecks[0].NarrowSyms[0] = 99
	if again := m.TypeChecks(refA); again[0].NarrowSyms[0] != cfg.SymbolID(20) {
		t.Fatalf("TypeChecks exposed mutable backing store: %+v", again)
	}

	if got, ok := m.FunctionRef(cfg.SymbolID(3)); !ok || got != refA {
		t.Fatalf("FunctionRef(3) = %+v/%v, want refA/true", got, ok)
	}
	if got, ok := m.FunctionRef(cfg.SymbolID(9)); !ok || got != refB {
		t.Fatalf("FunctionRef(9) = %+v/%v, want refB/true", got, ok)
	}
	if _, ok := m.FunctionRef(cfg.SymbolID(99)); ok {
		t.Fatal("FunctionRef missing symbol should be absent")
	}
	bindings := m.FunctionBindings()
	if got := len(bindings); got != 2 {
		t.Fatalf("function bindings len = %d, want 2", got)
	}
	if bindings[0].Symbol != cfg.SymbolID(3) || bindings[1].Symbol != cfg.SymbolID(9) {
		t.Fatalf("function bindings not sorted by symbol: %+v", bindings)
	}
	bindings[0].FuncRef = refB
	if again := m.FunctionBindings(); again[0].FuncRef != refA {
		t.Fatalf("FunctionBindings exposed mutable backing store: %+v", again)
	}

	if got, ok := m.FieldFuncRef(cfg.SymbolID(3), mustFieldKey("new")); !ok || got != refA {
		t.Fatalf("FieldFuncRef(3,new) = %+v/%v, want refA/true", got, ok)
	}
	if got, ok := m.FieldFuncRef(cfg.SymbolID(9), mustFieldKey("z")); !ok || got != refB {
		t.Fatalf("FieldFuncRef(9,z) = %+v/%v, want refB/true", got, ok)
	}
	if _, ok := m.FieldFuncRef(cfg.SymbolID(3), mustFieldKey("missing")); ok {
		t.Fatal("FieldFuncRef missing field should be absent")
	}
	if got := len(m.fieldFunctions); got != 3 {
		t.Fatalf("field-function facts len = %d, want 3", got)
	}

	metas := m.MetatableIndexes()
	if got := len(metas); got != 2 {
		t.Fatalf("metatable indexes len = %d, want 2", got)
	}
	if metas[0].MetatableSym != cfg.SymbolID(3) || metas[1].MetatableSym != cfg.SymbolID(9) {
		t.Fatalf("metatable indexes not sorted by metatable: %+v", metas)
	}
	if proto, ok := m.PrototypeForMetatable(cfg.SymbolID(3)); !ok || proto != cfg.SymbolID(2) {
		t.Fatalf("PrototypeForMetatable(3) = %d/%v, want 2/true", proto, ok)
	}
	metas[0].PrototypeSym = 99
	if again := m.MetatableIndexes(); again[0].PrototypeSym != cfg.SymbolID(2) {
		t.Fatalf("MetatableIndexes exposed mutable backing store: %+v", again)
	}

	receivers := m.MethodReceivers(refA)
	if got := len(receivers); got != 1 {
		t.Fatalf("method receivers for refA len = %d, want 1", got)
	}
	if receivers[0].PrototypeSym != cfg.SymbolID(2) || receivers[0].SelfSlot != 0 {
		t.Fatalf("method receiver for refA = %+v, want prototype 2 slot 0", receivers[0])
	}
	receivers[0].PrototypeSym = 99
	if again := m.MethodReceivers(refA); again[0].PrototypeSym != cfg.SymbolID(2) {
		t.Fatalf("MethodReceivers exposed mutable backing store: %+v", again)
	}

	methods := m.PrototypeMethods()
	if got := len(methods); got != 3 {
		t.Fatalf("prototype methods len = %d, want 3", got)
	}
	if methods[0].PrototypeSym != cfg.SymbolID(2) || methods[0].Field != mustFieldKey("a") || methods[0].FuncRef.GraphID != 1 {
		t.Fatalf("prototype method[0] = %+v, want proto 2 field a ref 1", methods[0])
	}
	if methods[1].PrototypeSym != cfg.SymbolID(2) || methods[1].Field != mustFieldKey("b") || methods[1].FuncRef.GraphID != 1 {
		t.Fatalf("prototype method[1] = %+v, want proto 2 field b ref 1", methods[1])
	}
	methods[0].PrototypeSym = 99
	if again := m.PrototypeMethods(); again[0].PrototypeSym != cfg.SymbolID(2) {
		t.Fatalf("PrototypeMethods exposed mutable backing store: %+v", again)
	}

	sites := m.SetMetatableSites(refA)
	if got := len(sites); got != 1 {
		t.Fatalf("setmetatable sites for refA len = %d, want 1", got)
	}
	if sites[0].Point != cfg.Point(4) || sites[0].MetatableSym != cfg.SymbolID(3) || sites[0].PrototypeSym != cfg.SymbolID(2) {
		t.Fatalf("setmetatable site for refA = %+v, want point 4 mt 3 proto 2", sites[0])
	}
	sites[0].PrototypeSym = 99
	if again := m.SetMetatableSites(refA); again[0].PrototypeSym != cfg.SymbolID(2) {
		t.Fatalf("SetMetatableSites exposed mutable backing store: %+v", again)
	}
}

func mustFieldKey(name string) fieldkey.Key {
	key, ok := fieldkey.FromName(name)
	if !ok {
		panic("invalid test field key")
	}
	return key
}
