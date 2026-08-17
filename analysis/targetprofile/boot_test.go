package profile

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestBootLedgerHasExactIndependentDenominator(t *testing.T) {
	contract := bootContract(t)
	// This is deliberately a literal denominator rather than a projection of
	// Spec, Admitted, or Excluded. Missing and extra root/key rows both fail.
	want := map[string][]string{
		"GlobalEnvRoot": {
			"_G", "_VERSION", "_GOPHER_LUA_VERSION", "assert", "error", "getmetatable", "ipairs", "next", "pairs", "pcall", "print", "rawequal", "rawget", "rawlen", "rawset", "select", "setmetatable", "tonumber", "tostring", "type", "xpcall", "require", "unpack", "load", "loadfile", "dofile", "collectgarbage",
			"table", "string", "math", "coroutine", "utf8", "debug", "errors", "io", "os", "package", "bit32",
		},
		"TableRoot": {
			"concat", "insert", "move", "pack", "remove", "sort", "unpack", "getn", "maxn", "create", "freeze", "isfrozen",
		},
		"StringRoot": {
			"byte", "char", "find", "format", "gmatch", "gfind", "gsub", "len", "lower", "match", "pack", "packsize", "rep", "reverse", "sub", "unpack", "upper", "dump",
		},
		"MathRoot": {
			"abs", "acos", "asin", "atan", "ceil", "cos", "deg", "exp", "floor", "fmod", "log", "max", "min", "modf", "rad", "random", "randomseed", "sin", "sqrt", "tan", "tointeger", "type", "ult", "atan2", "cosh", "sinh", "tanh", "pow", "frexp", "ldexp", "log10", "mod", "pi", "huge", "maxinteger", "mininteger",
		},
		"CoroutineRoot": {"create", "resume", "running", "status", "wrap", "yield", "spawn", "close", "isyieldable"},
		"UTF8Root":      {"char", "codes", "codepoint", "len", "offset", "charpattern"},
		"DebugRoot":     {"getupvalue", "getinfo", "getlocal", "traceback", "setlocal", "setupvalue", "setmetatable", "getmetatable"},
		"ErrorsRoot": {
			"new", "wrap", "is", "Error", "NOT_FOUND", "ALREADY_EXISTS", "INVALID", "PERMISSION_DENIED", "UNAVAILABLE", "INTERNAL", "CANCELED", "CONFLICT", "TIMEOUT", "RATE_LIMITED", "UNKNOWN", "call_stack",
		},
		"ErrorMetatableRoot":  {"__tostring", "__concat", "__index"},
		"ErrorMethodRoot":     {"kind", "retryable", "details", "message", "stack"},
		"StringMetatableRoot": {"__index"},
		"IORoot":              {"close", "flush", "input", "lines", "open", "output", "popen", "read", "tmpfile", "type", "write", "stdin", "stdout", "stderr"},
		"OSRoot":              {"clock", "date", "difftime", "execute", "exit", "getenv", "remove", "rename", "setlocale", "time", "tmpname"},
		"PackageRoot":         {"loadlib", "seeall", "searchpath", "preload", "loaders", "searchers", "loaded", "path", "cpath", "config"},
		"Bit32Root":           {"arshift", "band", "bnot", "bor", "btest", "bxor", "extract", "lrotate", "lshift", "replace", "rrotate", "rshift"},
	}
	if got, wantCount := contract.InitialRootCount(), 15; got != wantCount {
		t.Fatalf("initial roots = %d, want %d", got, wantCount)
	}
	if got, wantCount := contract.InitialEntryCount(), 199; got != wantCount {
		t.Fatalf("initial entries = %d, want %d", got, wantCount)
	}

	expected := make(map[string]struct{}, 199)
	for root, keys := range want {
		for _, key := range keys {
			identity := root + "\x00" + key
			if _, duplicate := expected[identity]; duplicate {
				t.Fatalf("test denominator repeats %s.%s", root, key)
			}
			expected[identity] = struct{}{}
		}
	}
	if got := len(expected); got != 199 {
		t.Fatalf("test denominator = %d, want 199", got)
	}
	seen := make(map[string]struct{}, contract.InitialEntryCount())
	previousRoot, previousKey := target.InitialRoot(0), ""
	for index := 0; index < contract.InitialEntryCount(); index++ {
		root, key, _, _, ok := contract.InitialEntryAt(index)
		if !ok {
			t.Fatalf("initial entry %d missing", index)
		}
		keyText := bootKey(t, contract, key)
		if previousRoot > root || (previousRoot == root && previousKey >= keyText) {
			t.Fatalf("initial entries are not strictly canonical at %d", index)
		}
		previousRoot, previousKey = root, keyText
		rootName, ok := contract.InitialRootIdentity(root)
		if !ok {
			t.Fatalf("initial entry %d has invalid root", index)
		}
		identity := rootName + "\x00" + keyText
		if _, ok := expected[identity]; !ok {
			t.Fatalf("unexpected initial row %s.%s", rootName, keyText)
		}
		seen[identity] = struct{}{}
	}
	for identity := range expected {
		if _, ok := seen[identity]; !ok {
			t.Fatalf("missing initial row %q", identity)
		}
	}

	if got, wantDigest := contract.ContentID().String(), "b848607e4575e1120970a474ab0453b173f9129519c11fcd4eaa4784ac78ca25"; got != wantDigest {
		t.Fatalf("boot content digest = %s, want %s", got, wantDigest)
	}
}

func TestBootLedgerSealsPrimitiveStringMetatableAttachment(t *testing.T) {
	contract := bootContract(t)
	if got := contract.InitialMetatableAttachmentCount(); got != 1 {
		t.Fatalf("initial metatable attachments = %d, want 1", got)
	}
	base, metatable, ok := contract.InitialMetatableAttachmentAt(0)
	if !ok || base != target.InitialValueString || metatable != bootRoot(t, contract, "StringMetatableRoot") {
		t.Fatalf("primitive String attachment = %v/%v/%v", base, metatable, ok)
	}
}

// This is a separate root-header denominator. It must not be reconstructed
// from the independently authored InitialMutability of every entry.
func TestBootLedgerHasExactWholeObjectHeaders(t *testing.T) {
	contract := bootContract(t)
	want := map[string]bool{
		"GlobalEnvRoot": false, "TableRoot": false, "StringRoot": false, "MathRoot": false,
		"CoroutineRoot": false, "UTF8Root": false, "DebugRoot": false, "ErrorsRoot": true,
		"IORoot": true, "OSRoot": true, "PackageRoot": true, "Bit32Root": true,
		"StringMetatableRoot": false, "ErrorMetatableRoot": true, "ErrorMethodRoot": true,
	}
	if len(want) != contract.InitialRootCount() {
		t.Fatalf("boot-header denominator = %d, roots = %d", len(want), contract.InitialRootCount())
	}
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, rootOK := contract.InitialRootAt(index)
		identity, identityOK := contract.InitialRootIdentity(root)
		shape, shapeOK := contract.InitialRootBootShape(root)
		immutable, immutableOK := contract.BootShapeImmutable(shape)
		wantImmutable, found := want[identity]
		if !rootOK || !identityOK || !shapeOK || !immutableOK || !found || immutable != wantImmutable {
			t.Fatalf("boot header %q = %v/%v, want %v", identity, immutable, immutableOK, wantImmutable)
		}
		delete(want, identity)
	}
	for identity := range want {
		t.Fatalf("missing boot header %q", identity)
	}
}

func TestBootLedgerBindsExactGlobalDenominator(t *testing.T) {
	contract := bootContract(t)
	want := map[string]target.InitialBindingClass{
		"_G": target.InitialBindingOrdinary, "table": target.InitialBindingOrdinary, "string": target.InitialBindingOrdinary,
		"math": target.InitialBindingOrdinary, "coroutine": target.InitialBindingOrdinary, "utf8": target.InitialBindingOrdinary,
		"debug": target.InitialBindingOrdinary, "errors": target.InitialBindingOrdinary, "io": target.InitialBindingOrdinary,
		"os": target.InitialBindingOrdinary, "package": target.InitialBindingOrdinary, "bit32": target.InitialBindingOrdinary,
		"_VERSION": target.InitialBindingOrdinary, "_GOPHER_LUA_VERSION": target.InitialBindingOrdinary,
		"assert": target.InitialBindingAdmitted, "error": target.InitialBindingAdmitted, "getmetatable": target.InitialBindingAdmitted,
		"ipairs": target.InitialBindingAdmitted, "next": target.InitialBindingAdmitted, "pairs": target.InitialBindingAdmitted,
		"pcall": target.InitialBindingAdmitted, "print": target.InitialBindingAdmitted, "rawequal": target.InitialBindingAdmitted,
		"rawget": target.InitialBindingAdmitted, "rawlen": target.InitialBindingAdmitted, "rawset": target.InitialBindingAdmitted,
		"select": target.InitialBindingAdmitted, "setmetatable": target.InitialBindingAdmitted, "tonumber": target.InitialBindingAdmitted,
		"tostring": target.InitialBindingAdmitted, "type": target.InitialBindingAdmitted, "xpcall": target.InitialBindingAdmitted,
		"require": target.InitialBindingAdmitted, "unpack": target.InitialBindingAdmitted,
		"load": target.InitialBindingDenied, "loadfile": target.InitialBindingDenied,
		"dofile": target.InitialBindingDenied, "collectgarbage": target.InitialBindingDenied,
	}
	if got := contract.InitialBindingCount(); got != len(want) {
		t.Fatalf("initial global bindings = %d, want %d", got, len(want))
	}
	previous := ""
	for index := 0; index < contract.InitialBindingCount(); index++ {
		name, class, _, root, key, ok := contract.InitialBindingAt(index)
		if !ok || name <= previous {
			t.Fatalf("initial binding order invalid at %d", index)
		}
		previous = name
		wantClass, exists := want[name]
		if !exists {
			t.Fatalf("unexpected global initial binding %q", name)
		}
		if class != wantClass || root != bootRoot(t, contract, "GlobalEnvRoot") || bootKey(t, contract, key) != name {
			t.Fatalf("initial binding %q = class %d root %d key %q", name, class, root, key)
		}
		delete(want, name)
	}
	for name := range want {
		t.Fatalf("missing global initial binding %q", name)
	}
}

func TestBootLedgerAliasesAndConstants(t *testing.T) {
	contract := bootContract(t)
	global := bootRoot(t, contract, "GlobalEnvRoot")
	for _, item := range []struct{ key, identity string }{
		{"_G", "GlobalEnvRoot"}, {"table", "TableRoot"}, {"string", "StringRoot"}, {"math", "MathRoot"},
		{"coroutine", "CoroutineRoot"}, {"utf8", "UTF8Root"}, {"debug", "DebugRoot"}, {"errors", "ErrorsRoot"},
		{"io", "IORoot"}, {"os", "OSRoot"}, {"package", "PackageRoot"}, {"bit32", "Bit32Root"},
	} {
		value, mutability := bootEntry(t, contract, global, item.key)
		if mutability != target.InitialMutable || bootRootValue(t, contract, value) != bootRoot(t, contract, item.identity) {
			t.Fatalf("global alias %s is not mutable %s root", item.key, item.identity)
		}
	}

	globalUnpack, _ := bootEntry(t, contract, global, "unpack")
	tableUnpack, _ := bootEntry(t, contract, bootRoot(t, contract, "TableRoot"), "unpack")
	stringGmatch, _ := bootEntry(t, contract, bootRoot(t, contract, "StringRoot"), "gmatch")
	stringGfind, _ := bootEntry(t, contract, bootRoot(t, contract, "StringRoot"), "gfind")
	if bootOperation(t, contract, globalUnpack) != bootOperation(t, contract, tableUnpack) {
		t.Fatal("global unpack and table.unpack are not the same callable")
	}
	if bootOperation(t, contract, stringGmatch) != bootOperation(t, contract, stringGfind) {
		t.Fatal("string.gmatch and string.gfind are not the same callable")
	}
	stringMetaIndex, mutability := bootEntry(t, contract, bootRoot(t, contract, "StringMetatableRoot"), "__index")
	if mutability != target.InitialMutable || bootRootValue(t, contract, stringMetaIndex) != bootRoot(t, contract, "StringRoot") {
		t.Fatal("primitive String metatable __index is not its mutable string-root alias")
	}
	errorRootValue, _ := bootEntry(t, contract, bootRoot(t, contract, "ErrorsRoot"), "Error")
	errorIndex, _ := bootEntry(t, contract, bootRoot(t, contract, "ErrorMetatableRoot"), "__index")
	if bootRootValue(t, contract, errorRootValue) != bootRoot(t, contract, "ErrorMetatableRoot") || bootRootValue(t, contract, errorIndex) != bootRoot(t, contract, "ErrorMethodRoot") {
		t.Fatal("Error metatable/method root aliases drifted")
	}

	bootString(t, contract, global, "_VERSION", "Lua 5.3 - Wippy Modification")
	bootString(t, contract, global, "_GOPHER_LUA_VERSION", "GopherLua 0.2 Wippy Edition")
	mathRoot := bootRoot(t, contract, "MathRoot")
	bootFloat(t, contract, mathRoot, "pi", 0x400921fb54442d18)
	bootFloat(t, contract, mathRoot, "huge", 0x7ff0000000000000)
	bootInteger(t, contract, mathRoot, "maxinteger", 9223372036854775807)
	bootInteger(t, contract, mathRoot, "mininteger", -9223372036854775808)
	bootString(t, contract, bootRoot(t, contract, "UTF8Root"), "charpattern", "[\x00-\x7F\xC2-\xF4][\x80-\xBF]*")
	for _, item := range []struct{ key, value string }{
		{"NOT_FOUND", "NotFound"}, {"ALREADY_EXISTS", "AlreadyExists"}, {"INVALID", "Invalid"}, {"PERMISSION_DENIED", "PermissionDenied"},
		{"UNAVAILABLE", "Unavailable"}, {"INTERNAL", "Internal"}, {"CANCELED", "Canceled"}, {"CONFLICT", "Conflict"},
		{"TIMEOUT", "Timeout"}, {"RATE_LIMITED", "RateLimited"}, {"UNKNOWN", ""},
	} {
		bootString(t, contract, bootRoot(t, contract, "ErrorsRoot"), item.key, item.value)
	}
}

func TestBootLedgerDenialsAndAbsentValues(t *testing.T) {
	contract := bootContract(t)
	for _, item := range []struct {
		root, key string
		binding   target.BindingSpec
	}{
		{"GlobalEnvRoot", "load", builtinBindingSpec("load")}, {"GlobalEnvRoot", "loadfile", builtinBindingSpec("loadfile")}, {"GlobalEnvRoot", "dofile", builtinBindingSpec("dofile")}, {"GlobalEnvRoot", "collectgarbage", builtinBindingSpec("collectgarbage")},
		{"StringRoot", "dump", moduleBindingSpec("string", "dump")},
		{"CoroutineRoot", "close", moduleBindingSpec("coroutine", "close")}, {"CoroutineRoot", "isyieldable", moduleBindingSpec("coroutine", "isyieldable")},
		{"DebugRoot", "getinfo", moduleBindingSpec("debug", "getinfo")}, {"DebugRoot", "getlocal", moduleBindingSpec("debug", "getlocal")}, {"DebugRoot", "traceback", moduleBindingSpec("debug", "traceback")}, {"DebugRoot", "setlocal", moduleBindingSpec("debug", "setlocal")}, {"DebugRoot", "setupvalue", moduleBindingSpec("debug", "setupvalue")}, {"DebugRoot", "setmetatable", moduleBindingSpec("debug", "setmetatable")}, {"DebugRoot", "getmetatable", moduleBindingSpec("debug", "getmetatable")},
		{"ErrorsRoot", "call_stack", moduleBindingSpec("errors", "call_stack")}, {"ErrorMethodRoot", "stack", moduleBindingSpec("errors", "Error", "stack")},
		{"IORoot", "close", moduleBindingSpec("io", "close")}, {"IORoot", "flush", moduleBindingSpec("io", "flush")}, {"IORoot", "input", moduleBindingSpec("io", "input")}, {"IORoot", "lines", moduleBindingSpec("io", "lines")}, {"IORoot", "open", moduleBindingSpec("io", "open")}, {"IORoot", "output", moduleBindingSpec("io", "output")}, {"IORoot", "popen", moduleBindingSpec("io", "popen")}, {"IORoot", "read", moduleBindingSpec("io", "read")}, {"IORoot", "tmpfile", moduleBindingSpec("io", "tmpfile")}, {"IORoot", "type", moduleBindingSpec("io", "type")}, {"IORoot", "write", moduleBindingSpec("io", "write")},
		{"OSRoot", "clock", moduleBindingSpec("os", "clock")}, {"OSRoot", "date", moduleBindingSpec("os", "date")}, {"OSRoot", "difftime", moduleBindingSpec("os", "difftime")}, {"OSRoot", "execute", moduleBindingSpec("os", "execute")}, {"OSRoot", "exit", moduleBindingSpec("os", "exit")}, {"OSRoot", "getenv", moduleBindingSpec("os", "getenv")}, {"OSRoot", "remove", moduleBindingSpec("os", "remove")}, {"OSRoot", "rename", moduleBindingSpec("os", "rename")}, {"OSRoot", "setlocale", moduleBindingSpec("os", "setlocale")}, {"OSRoot", "time", moduleBindingSpec("os", "time")}, {"OSRoot", "tmpname", moduleBindingSpec("os", "tmpname")},
		{"PackageRoot", "loadlib", moduleBindingSpec("package", "loadlib")}, {"PackageRoot", "seeall", moduleBindingSpec("package", "seeall")}, {"PackageRoot", "searchpath", moduleBindingSpec("package", "searchpath")},
		{"Bit32Root", "arshift", moduleBindingSpec("bit32", "arshift")}, {"Bit32Root", "band", moduleBindingSpec("bit32", "band")}, {"Bit32Root", "bnot", moduleBindingSpec("bit32", "bnot")}, {"Bit32Root", "bor", moduleBindingSpec("bit32", "bor")}, {"Bit32Root", "btest", moduleBindingSpec("bit32", "btest")}, {"Bit32Root", "bxor", moduleBindingSpec("bit32", "bxor")}, {"Bit32Root", "extract", moduleBindingSpec("bit32", "extract")}, {"Bit32Root", "lrotate", moduleBindingSpec("bit32", "lrotate")}, {"Bit32Root", "lshift", moduleBindingSpec("bit32", "lshift")}, {"Bit32Root", "replace", moduleBindingSpec("bit32", "replace")}, {"Bit32Root", "rrotate", moduleBindingSpec("bit32", "rrotate")}, {"Bit32Root", "rshift", moduleBindingSpec("bit32", "rshift")},
	} {
		value, _ := bootEntry(t, contract, bootRoot(t, contract, item.root), item.key)
		bootDeniedBinding(t, contract, value, item.binding)
	}
	for _, item := range []struct{ root, key string }{
		{"IORoot", "stdin"}, {"IORoot", "stdout"}, {"IORoot", "stderr"},
		{"PackageRoot", "preload"}, {"PackageRoot", "loaders"}, {"PackageRoot", "searchers"}, {"PackageRoot", "loaded"}, {"PackageRoot", "path"}, {"PackageRoot", "cpath"}, {"PackageRoot", "config"},
	} {
		value, mutability := bootEntry(t, contract, bootRoot(t, contract, item.root), item.key)
		if kind, _ := contract.InitialValueKind(value); kind != target.InitialValueAbsent || mutability != target.InitialFrozen {
			t.Fatalf("%s.%s = kind %d mutability %d, want frozen absent", item.root, item.key, kind, mutability)
		}
	}
}

func TestProfileBootQueriesDoNotAllocate(t *testing.T) {
	contract := bootContract(t)
	global := bootRoot(t, contract, "GlobalEnvRoot")
	requireKey := bootExactKey(t, contract, "require")
	if allocations := testing.AllocsPerRun(1000, func() {
		_, _, _ = contract.InitialEntry(global, requireKey)
		_, _, _, _, _ = contract.InitialBinding("require")
	}); allocations != 0 {
		t.Fatalf("profile boot queries allocated %v times", allocations)
	}
}

func bootContract(t *testing.T) *target.Contract {
	t.Helper()
	contract, err := Contract()
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func bootRoot(t *testing.T, contract *target.Contract, identity string) target.InitialRoot {
	t.Helper()
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, _ := contract.InitialRootAt(index)
		if name, _ := contract.InitialRootIdentity(root); name == identity {
			return root
		}
	}
	t.Fatalf("initial root %q missing", identity)
	return 0
}

func bootEntry(t *testing.T, contract *target.Contract, root target.InitialRoot, key string) (target.InitialValue, target.InitialMutability) {
	t.Helper()
	value, mutability, ok := contract.InitialEntry(root, bootExactKey(t, contract, key))
	if !ok {
		t.Fatalf("initial entry %d.%s missing", root, key)
	}
	return value, mutability
}

func bootKey(t *testing.T, contract *target.Contract, key target.ExactKey) string {
	t.Helper()
	value, ok := contract.ExactKeyValue(key)
	if !ok || value.Kind != keyspace.LiteralString {
		t.Fatalf("exact key %d is not a string", key)
	}
	return value.String
}

func bootExactKey(t *testing.T, contract *target.Contract, text string) target.ExactKey {
	t.Helper()
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, _ := contract.ExactKeyAt(index)
		if bootKey(t, contract, key) == text {
			return key
		}
	}
	t.Fatalf("exact key %q missing", text)
	return 0
}

func bootRootValue(t *testing.T, contract *target.Contract, value target.InitialValue) target.InitialRoot {
	t.Helper()
	root, ok := contract.InitialValueRoot(value)
	if !ok {
		t.Fatalf("initial value %d is not a root alias", value)
	}
	return root
}

func bootOperation(t *testing.T, contract *target.Contract, value target.InitialValue) target.Operation {
	t.Helper()
	operation, ok := contract.InitialValueOperation(value)
	if !ok {
		t.Fatalf("initial value %d is not an operation", value)
	}
	return operation
}

func bootString(t *testing.T, contract *target.Contract, root target.InitialRoot, key, want string) {
	t.Helper()
	value, _ := bootEntry(t, contract, root, key)
	if got, ok := contract.InitialValueString(value); !ok || got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

func bootInteger(t *testing.T, contract *target.Contract, root target.InitialRoot, key string, want int64) {
	t.Helper()
	value, _ := bootEntry(t, contract, root, key)
	if got, ok := contract.InitialValueInteger(value); !ok || got != want {
		t.Fatalf("%s = %d, want %d", key, got, want)
	}
}

func bootFloat(t *testing.T, contract *target.Contract, root target.InitialRoot, key string, want uint64) {
	t.Helper()
	value, _ := bootEntry(t, contract, root, key)
	if got, ok := contract.InitialValueFloatBits(value); !ok || got != want {
		t.Fatalf("%s = 0x%x, want 0x%x", key, got, want)
	}
}

func bootDeniedBinding(t *testing.T, contract *target.Contract, value target.InitialValue, want target.BindingSpec) {
	t.Helper()
	if kind, _ := contract.InitialValueKind(value); kind != target.InitialValueDeniedOperation {
		t.Fatalf("initial value %d kind = %d, want denied operation", value, kind)
	}
	if got, ok := contract.InitialValueDeniedNamespace(value); !ok || got != want.Namespace {
		t.Fatalf("denied binding namespace = %d, want %d", got, want.Namespace)
	}
	if got := contract.InitialValueDeniedOwnerCount(value); got != len(want.Owner) {
		t.Fatalf("denied binding owner count = %d, want %d", got, len(want.Owner))
	}
	for index, part := range want.Owner {
		if got, ok := contract.InitialValueDeniedOwnerAt(value, index); !ok || got != part {
			t.Fatalf("denied binding owner %d = %q, want %q", index, got, part)
		}
	}
	if got := contract.InitialValueDeniedMemberCount(value); got != len(want.Member) {
		t.Fatalf("denied binding member count = %d, want %d", got, len(want.Member))
	}
	for index, part := range want.Member {
		if got, ok := contract.InitialValueDeniedMemberAt(value, index); !ok || got != part {
			t.Fatalf("denied binding member %d = %q, want %q", index, got, part)
		}
	}
}
