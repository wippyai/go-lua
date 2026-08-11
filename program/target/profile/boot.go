package profile

import (
	"fmt"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
)

// The root identities below are the closed initial-environment ABI. They are
// names, not runtime handles: Heap owns every later table instance and write.
const (
	globalEnvRoot      = "GlobalEnvRoot"
	tableRoot          = "TableRoot"
	stringRoot         = "StringRoot"
	mathRoot           = "MathRoot"
	coroutineRoot      = "CoroutineRoot"
	utf8Root           = "UTF8Root"
	debugRoot          = "DebugRoot"
	errorsRoot         = "ErrorsRoot"
	ioRoot             = "IORoot"
	osRoot             = "OSRoot"
	packageRoot        = "PackageRoot"
	bit32Root          = "Bit32Root"
	stringMetaRoot     = "StringMetatableRoot"
	errorMetaRoot      = "ErrorMetatableRoot"
	errorMethodRoot    = "ErrorMethodRoot"
	wippyVersionString = "GopherLua 0.2 Wippy Edition"
)

type bootLedgerData struct {
	roots      []target.InitialRootSpec
	entries    []target.InitialEntrySpec
	bindings   []target.InitialBindingSpec
	metatables []target.InitialMetatableAttachmentSpec
}

// bootLedger authors the complete initial environment as ordinary Target
// data. Each operation value is resolved from the catalogue binding that was
// just authored; this avoids a second hand-maintained operation identity list.
func bootLedger(catalogue authoredCatalogue) (bootLedgerData, error) {
	var ledger bootLedgerData
	for _, item := range []struct {
		identity  string
		aggregate target.BootAggregate
		immutable bool
	}{
		{globalEnvRoot, target.BootAggregateTable, false},
		{tableRoot, target.BootAggregateTable, false},
		{stringRoot, target.BootAggregateTable, false},
		{mathRoot, target.BootAggregateTable, false},
		{coroutineRoot, target.BootAggregateTable, false},
		{utf8Root, target.BootAggregateTable, false},
		{debugRoot, target.BootAggregateTable, false},
		{errorsRoot, target.BootAggregateTable, true},
		{ioRoot, target.BootAggregateTable, true},
		{osRoot, target.BootAggregateTable, true},
		{packageRoot, target.BootAggregateTable, true},
		{bit32Root, target.BootAggregateTable, true},
		{stringMetaRoot, target.BootAggregateMetatable, false},
		{errorMetaRoot, target.BootAggregateMetatable, true},
		{errorMethodRoot, target.BootAggregateTable, true},
	} {
		ledger.root(item.identity, item.aggregate, item.immutable)
	}

	// Every unshadowed global slot records the same initial root/key row that
	// supplies its initial Cell value. The root aliases are regular Values.
	for _, item := range []struct {
		key, root string
	}{
		{"_G", globalEnvRoot},
		{"table", tableRoot}, {"string", stringRoot}, {"math", mathRoot},
		{"coroutine", coroutineRoot}, {"utf8", utf8Root}, {"debug", debugRoot},
		{"errors", errorsRoot}, {"io", ioRoot}, {"os", osRoot},
		{"package", packageRoot}, {"bit32", bit32Root},
	} {
		ledger.global(item.key, rootValue(item.root))
	}
	ledger.global("_VERSION", stringValue("Lua 5.3 - Wippy Modification"))
	ledger.global("_GOPHER_LUA_VERSION", stringValue(wippyVersionString))

	for _, name := range []string{
		"assert", "error", "getmetatable", "ipairs", "next", "pairs", "pcall",
		"print", "rawequal", "rawget", "rawlen", "rawset", "select",
		"setmetatable", "tonumber", "tostring", "type", "xpcall", "require", "unpack",
	} {
		if err := ledger.operation(catalogue, globalEnvRoot, name, target.InitialMutable, builtinBindingSpec(name)); err != nil {
			return bootLedgerData{}, err
		}
		ledger.bindGlobal(name)
	}
	for _, name := range []string{"load", "loadfile", "dofile", "collectgarbage"} {
		ledger.global(name, deniedValue(builtinBindingSpec(name)))
	}

	if err := ledger.operations(catalogue, tableRoot, target.InitialMutable, "table", []string{
		"concat", "insert", "move", "pack", "remove", "sort", "unpack", "getn", "maxn", "create", "freeze", "isfrozen",
	}); err != nil {
		return bootLedgerData{}, err
	}
	if err := ledger.operations(catalogue, stringRoot, target.InitialMutable, "string", []string{
		"byte", "char", "find", "format", "gmatch", "gfind", "gsub", "len", "lower", "match", "pack", "packsize", "rep", "reverse", "sub", "unpack", "upper",
	}); err != nil {
		return bootLedgerData{}, err
	}
	ledger.entry(stringRoot, "dump", deniedValue(moduleBindingSpec("string", "dump")), target.InitialMutable)

	if err := ledger.operations(catalogue, mathRoot, target.InitialMutable, "math", []string{
		"abs", "acos", "asin", "atan", "ceil", "cos", "deg", "exp", "floor", "fmod", "log", "max", "min", "modf", "rad", "random", "randomseed", "sin", "sqrt", "tan", "tointeger", "type", "ult",
		"atan2", "cosh", "sinh", "tanh", "pow", "frexp", "ldexp", "log10", "mod",
	}); err != nil {
		return bootLedgerData{}, err
	}
	ledger.entry(mathRoot, "pi", floatValue(0x400921fb54442d18), target.InitialMutable)
	ledger.entry(mathRoot, "huge", floatValue(0x7ff0000000000000), target.InitialMutable)
	ledger.entry(mathRoot, "maxinteger", integerValue(9223372036854775807), target.InitialMutable)
	ledger.entry(mathRoot, "mininteger", integerValue(-9223372036854775808), target.InitialMutable)

	if err := ledger.operations(catalogue, coroutineRoot, target.InitialMutable, "coroutine", []string{
		"create", "resume", "running", "status", "wrap", "yield", "spawn",
	}); err != nil {
		return bootLedgerData{}, err
	}
	for _, name := range []string{"close", "isyieldable"} {
		ledger.entry(coroutineRoot, name, deniedValue(moduleBindingSpec("coroutine", name)), target.InitialMutable)
	}

	if err := ledger.operations(catalogue, utf8Root, target.InitialMutable, "utf8", []string{
		"char", "codes", "codepoint", "len", "offset",
	}); err != nil {
		return bootLedgerData{}, err
	}
	ledger.entry(utf8Root, "charpattern", stringValue("[\x00-\x7F\xC2-\xF4][\x80-\xBF]*"), target.InitialMutable)

	if err := ledger.operation(catalogue, debugRoot, "getupvalue", target.InitialMutable, moduleBindingSpec("debug", "getupvalue")); err != nil {
		return bootLedgerData{}, err
	}
	for _, name := range []string{"getinfo", "getlocal", "traceback", "setlocal", "setupvalue", "setmetatable", "getmetatable"} {
		ledger.entry(debugRoot, name, deniedValue(moduleBindingSpec("debug", name)), target.InitialMutable)
	}

	if err := ledger.operations(catalogue, errorsRoot, target.InitialFrozen, "errors", []string{"new", "wrap", "is"}); err != nil {
		return bootLedgerData{}, err
	}
	ledger.entry(errorsRoot, "Error", rootValue(errorMetaRoot), target.InitialFrozen)
	for _, item := range []struct{ key, value string }{
		{"NOT_FOUND", "NotFound"}, {"ALREADY_EXISTS", "AlreadyExists"},
		{"INVALID", "Invalid"}, {"PERMISSION_DENIED", "PermissionDenied"},
		{"UNAVAILABLE", "Unavailable"}, {"INTERNAL", "Internal"},
		{"CANCELED", "Canceled"}, {"CONFLICT", "Conflict"},
		{"TIMEOUT", "Timeout"}, {"RATE_LIMITED", "RateLimited"}, {"UNKNOWN", ""},
	} {
		ledger.entry(errorsRoot, item.key, stringValue(item.value), target.InitialFrozen)
	}
	ledger.entry(errorsRoot, "call_stack", deniedValue(moduleBindingSpec("errors", "call_stack")), target.InitialFrozen)

	if err := ledger.operation(catalogue, errorMetaRoot, "__tostring", target.InitialFrozen, moduleBindingSpec("errors", "Error", "__tostring")); err != nil {
		return bootLedgerData{}, err
	}
	if err := ledger.operation(catalogue, errorMetaRoot, "__concat", target.InitialFrozen, moduleBindingSpec("errors", "Error", "__concat")); err != nil {
		return bootLedgerData{}, err
	}
	ledger.entry(errorMetaRoot, "__index", rootValue(errorMethodRoot), target.InitialFrozen)
	if err := ledger.operations(catalogue, errorMethodRoot, target.InitialFrozen, "errors", []string{"kind", "retryable", "details", "message"}, "Error"); err != nil {
		return bootLedgerData{}, err
	}
	ledger.entry(errorMethodRoot, "stack", deniedValue(moduleBindingSpec("errors", "Error", "stack")), target.InitialFrozen)

	// String's primitive metatable is distinct from the library table; it
	// shares only the exact __index alias to that library root.
	ledger.entry(stringMetaRoot, "__index", rootValue(stringRoot), target.InitialMutable)
	ledger.metatables = append(ledger.metatables, target.InitialMetatableAttachmentSpec{Base: target.InitialValueString, Metatable: stringMetaRoot})

	for _, name := range []string{"close", "flush", "input", "lines", "open", "output", "popen", "read", "tmpfile", "type", "write"} {
		ledger.entry(ioRoot, name, deniedValue(moduleBindingSpec("io", name)), target.InitialFrozen)
	}
	for _, name := range []string{"stdin", "stdout", "stderr"} {
		ledger.entry(ioRoot, name, absentValue(), target.InitialFrozen)
	}

	for _, name := range []string{"clock", "date", "difftime", "execute", "exit", "getenv", "remove", "rename", "setlocale", "time", "tmpname"} {
		ledger.entry(osRoot, name, deniedValue(moduleBindingSpec("os", name)), target.InitialFrozen)
	}

	for _, name := range []string{"loadlib", "seeall", "searchpath"} {
		ledger.entry(packageRoot, name, deniedValue(moduleBindingSpec("package", name)), target.InitialFrozen)
	}
	for _, name := range []string{"preload", "loaders", "searchers", "loaded", "path", "cpath", "config"} {
		ledger.entry(packageRoot, name, absentValue(), target.InitialFrozen)
	}

	for _, name := range []string{"arshift", "band", "bnot", "bor", "btest", "bxor", "extract", "lrotate", "lshift", "replace", "rrotate", "rshift"} {
		ledger.entry(bit32Root, name, deniedValue(moduleBindingSpec("bit32", name)), target.InitialFrozen)
	}
	return ledger, nil
}

func (ledger *bootLedgerData) root(identity string, aggregate target.BootAggregate, immutable bool) {
	ledger.roots = append(ledger.roots, target.InitialRootSpec{
		Identity: identity,
		Shape:    target.BootShapeSpec{Aggregate: aggregate, Immutable: immutable, Value: rootValue(identity)},
	})
}

func (ledger *bootLedgerData) entry(root, key string, value target.InitialValueSpec, mutability target.InitialMutability) {
	ledger.entries = append(ledger.entries, target.InitialEntrySpec{Root: root, Key: exactKey(key), Value: value, Mutability: mutability})
}

func (ledger *bootLedgerData) global(key string, value target.InitialValueSpec) {
	ledger.entry(globalEnvRoot, key, value, target.InitialMutable)
	ledger.bindGlobal(key)
}

func (ledger *bootLedgerData) bindGlobal(key string) {
	ledger.bindings = append(ledger.bindings, target.InitialBindingSpec{Name: key, Root: globalEnvRoot, Key: exactKey(key)})
}

func exactKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}

func (ledger *bootLedgerData) operations(catalogue authoredCatalogue, root string, mutability target.InitialMutability, owner string, names []string, memberPrefix ...string) error {
	for _, name := range names {
		member := append(append([]string(nil), memberPrefix...), name)
		if err := ledger.operation(catalogue, root, name, mutability, moduleBindingSpec(owner, member...)); err != nil {
			return err
		}
	}
	return nil
}

func (ledger *bootLedgerData) operation(catalogue authoredCatalogue, root, key string, mutability target.InitialMutability, binding target.BindingSpec) error {
	value, err := operationValue(catalogue, binding)
	if err != nil {
		return err
	}
	ledger.entry(root, key, value, mutability)
	return nil
}

func operationValue(catalogue authoredCatalogue, binding target.BindingSpec) (target.InitialValueSpec, error) {
	for _, operation := range catalogue.operations {
		for _, candidate := range operation.Bindings {
			if sameBinding(candidate, binding) {
				return target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: cloneBinding(binding)}, nil
			}
		}
	}
	return target.InitialValueSpec{}, fmt.Errorf("profile: initial ledger names unknown operation %#v", binding)
}

func rootValue(identity string) target.InitialValueSpec {
	return target.InitialValueSpec{Kind: target.InitialValueRoot, Root: identity}
}

func stringValue(value string) target.InitialValueSpec {
	return target.InitialValueSpec{Kind: target.InitialValueString, String: value}
}

func integerValue(value int64) target.InitialValueSpec {
	return target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: value}
}

func floatValue(bits uint64) target.InitialValueSpec {
	return target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: bits}
}

func deniedValue(binding target.BindingSpec) target.InitialValueSpec {
	return target.InitialValueSpec{Kind: target.InitialValueDeniedOperation, Operation: binding}
}

func absentValue() target.InitialValueSpec {
	return target.InitialValueSpec{Kind: target.InitialValueAbsent}
}

func builtinBindingSpec(name string) target.BindingSpec {
	return target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{name}}
}

func moduleBindingSpec(owner string, member ...string) target.BindingSpec {
	return target.BindingSpec{Namespace: target.BindingModule, Owner: []string{owner}, Member: append([]string(nil), member...)}
}

func cloneBinding(binding target.BindingSpec) target.BindingSpec {
	return target.BindingSpec{Namespace: binding.Namespace, Owner: append([]string(nil), binding.Owner...), Member: append([]string(nil), binding.Member...)}
}

func sameBinding(left, right target.BindingSpec) bool {
	if left.Namespace != right.Namespace || len(left.Owner) != len(right.Owner) || len(left.Member) != len(right.Member) {
		return false
	}
	for index := range left.Owner {
		if left.Owner[index] != right.Owner[index] {
			return false
		}
	}
	for index := range left.Member {
		if left.Member[index] != right.Member[index] {
			return false
		}
	}
	return true
}
