// Package stdlib exposes signaturelookup-private effect signatures for bounded
// standard-library functions used by module metadata.
package stdlib

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/stringlib"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const (
	Assert         = "assert"
	Error          = "error"
	Integer        = "integer"
	Number         = "number"
	Require        = "require"
	String         = "string"
	ToString       = "tostring"
	Type           = "type"
	Pairs          = "pairs"
	IPairs         = "ipairs"
	PCall          = "pcall"
	XPCall         = "xpcall"
	TableInsert    = "table.insert"
	OwnershipStore = "ownership.store"
)

var registry = map[string]signature.Function{
	Assert: sig(
		typ.Func().
			Param("v", typ.Any).
			OptParam("message", typ.Any).
			Returns(typ.Any).
			Build(),
		postcondition.NormalReturnRefinement{
			Target:     effect.ParamRef{Index: 0},
			Refinement: postcondition.Present{},
		},
	),
	Error: sig(
		typ.Func().
			Param("message", typ.Any).
			OptParam("level", typ.Integer).
			Returns(typ.Never).
			Build(),
	),
	Integer: sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(typ.Integer).
			Build(),
		ownership.BorrowAll{},
	),
	Number: sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(typ.Number).
			Build(),
		ownership.BorrowAll{},
	),
	Require: sig(
		typ.Func().
			Param("modname", typ.String).
			Returns(typ.Any).
			Build(),
		dispatch.ModuleLoad{},
	),
	String: sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(typ.String).
			Build(),
		ownership.BorrowAll{},
	),
	ToString: sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(typ.String).
			Build(),
		ownership.BorrowAll{},
	),
	Type: sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(luaTypeName).
			Build(),
		ownership.BorrowAll{},
	),
	Pairs: sig(
		typ.Func().
			Param("t", typ.Any).
			Returns(typ.Any, typ.Any, typ.Nil).
			Build(),
		iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IterateKeyed,
		},
	),
	IPairs: sig(
		typ.Func().
			Param("t", typ.Any).
			Returns(typ.Any, typ.Any, typ.Integer).
			Build(),
		iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IterateIndexed,
		},
	),
	PCall: sig(
		typ.Func().
			Param("f", typ.Any).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.Any).
			Build(),
		ownership.BorrowAll{},
		returns.Return{
			ReturnIndex: 1,
			Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		},
	),
	XPCall: sig(
		typ.Func().
			Param("f", typ.Any).
			Param("msgh", typ.Any).
			Variadic(typ.Any).
			Returns(typ.Boolean, typ.Any).
			Build(),
		ownership.BorrowAll{},
		returns.Return{
			ReturnIndex: 1,
			Transform:   returns.CallbackReturn{CallbackParam: effect.ParamRef{Index: 0}},
		},
	),

	// Bare globals.
	"print": sig(
		typ.Func().
			Variadic(typ.Any).
			Build(),
		ownership.BorrowAll{},
	),
	"tonumber": sig(
		typ.Func().
			Param("v", typ.Any).
			OptParam("base", typ.Integer).
			Returns(normalize.Optional(typ.Number)).
			Build(),
		ownership.BorrowAll{},
	),
	"next": sig(
		typ.Func().
			Param("table", typ.Any).
			OptParam("index", typ.Any).
			Returns(normalize.Optional(typ.Any), normalize.Optional(typ.Any)).
			Build(),
	),
	"select": sig(
		// select(index, ...) -> ... ; index may be "#" (count) or a numeric
		// position. The result is genuinely dynamic, so any is correct here.
		typ.Func().
			Param("index", typ.Any).
			Variadic(typ.Any).
			Returns(typ.Any).
			Build(),
	),
	"rawget": sig(
		typ.Func().
			Param("table", typ.Any).
			Param("index", typ.Any).
			Returns(typ.Any).
			Build(),
		ownership.BorrowAll{},
	),
	"rawset": sig(
		typ.Func().
			Param("table", typ.Any).
			Param("index", typ.Any).
			Param("value", typ.Any).
			Returns(typ.Any).
			Build(),
		ownership.Store{
			Param: effect.ParamRef{Index: 2},
			Into:  effect.ParamRef{Index: 0},
		},
	),
	OwnershipStore: sig(
		typ.Func().
			Param("value", typ.Any).
			Param("owner", typ.Any).
			Build(),
		ownership.Store{
			Param: effect.ParamRef{Index: 0},
			Into:  effect.ParamRef{Index: 1},
		},
	),
	"rawequal": sig(
		typ.Func().
			Param("v1", typ.Any).
			Param("v2", typ.Any).
			Returns(typ.Boolean).
			Build(),
		ownership.BorrowAll{},
	),
	"rawlen": sig(
		typ.Func().
			Param("v", typ.Any).
			Returns(typ.Integer).
			Build(),
		ownership.BorrowAll{},
	),
	"setmetatable": func() signature.Function {
		tp := typ.NewTypeParam("T", nil)
		return sig(
			typ.Func().
				TypeParamRef(tp).
				Param("table", tp).
				Param("metatable", normalize.Optional(typ.Any)).
				Returns(tp).
				Build(),
			ownership.Retain{Param: effect.ParamRef{Index: 1}},
			returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}},
		)
	}(),
	"getmetatable": sig(
		typ.Func().
			Param("object", typ.Any).
			Returns(normalize.Optional(typ.Any)).
			Build(),
	),
	"collectgarbage": sig(
		typ.Func().
			OptParam("opt", typ.String).
			OptParam("arg", typ.Any).
			Returns(typ.Any).
			Build(),
	),
	"unpack": func() signature.Function {
		elem := typ.NewTypeParam("T", nil)
		return sig(
			typ.Func().
				TypeParamRef(elem).
				Param("list", typ.NewArray(elem)).
				OptParam("i", typ.Integer).
				OptParam("j", typ.Integer).
				Returns(normalize.Optional(elem)).
				Build(),
			ownership.BorrowAll{},
		)
	}(),

	// table library.
	TableInsert: sig(
		typ.Func().
			Param("list", typ.Any).
			Param("pos_or_value", typ.Any).
			OptParam("value", typ.Any).
			Build(),
		mutation.TableMutator{
			Target: effect.ParamRef{Index: 0},
			Value:  effect.ParamRef{Index: -1},
		},
		mutation.LengthChange{
			Target: effect.ParamRef{Index: 0},
			Delta:  1,
		},
		ownership.Store{
			Param: effect.ParamRef{Index: -1},
			Into:  effect.ParamRef{Index: 0},
		},
	),
	"table.remove": func() signature.Function {
		elem := typ.NewTypeParam("T", nil)
		return sig(
			typ.Func().
				TypeParamRef(elem).
				Param("list", typ.NewArray(elem)).
				OptParam("pos", typ.Integer).
				Returns(normalize.Optional(elem)).
				Build(),
			mutation.Mutate{
				Target:    effect.ParamRef{Index: 0},
				Transform: mutation.Unchanged{},
			},
			mutation.LengthChange{
				Target: effect.ParamRef{Index: 0},
				Delta:  -1,
			},
		)
	}(),
	"table.concat": sig(
		typ.Func().
			Param("list", typ.Any).
			OptParam("sep", typ.String).
			OptParam("i", typ.Integer).
			OptParam("j", typ.Integer).
			Returns(typ.String).
			Build(),
		ownership.BorrowAll{},
	),
	"table.sort": sig(
		typ.Func().
			Param("list", typ.Any).
			OptParam("comp", typ.Any).
			Build(),
		mutation.Mutate{
			Target:    effect.ParamRef{Index: 0},
			Transform: mutation.Unchanged{},
		},
	),
	"table.unpack": func() signature.Function {
		elem := typ.NewTypeParam("T", nil)
		return sig(
			typ.Func().
				TypeParamRef(elem).
				Param("list", typ.NewArray(elem)).
				OptParam("i", typ.Integer).
				OptParam("j", typ.Integer).
				Returns(normalize.Optional(elem)).
				Build(),
			ownership.BorrowAll{},
		)
	}(),
	// table.pack collects its arguments into a fresh table: the arguments land
	// under integer keys and n records how many were passed. Both halves are
	// published, so a caller reads the count as an integer and an entry as the
	// argument type the call site supplied.
	"table.pack": func() signature.Function {
		elem := typ.NewTypeParam("T", nil)
		return sig(
			typ.Func().
				TypeParamRef(elem).
				Variadic(elem).
				Returns(typetable.NewRecord().
					Field("n", typ.Integer).
					MapComponent(typ.Integer, elem).
					Build()).
				Build(),
		)
	}(),
	"table.move": sig(
		typ.Func().
			Param("a1", typ.Any).
			Param("f", typ.Integer).
			Param("e", typ.Integer).
			Param("t", typ.Integer).
			OptParam("a2", typ.Any).
			Returns(typ.Any).
			Build(),
	),
	"table.create": func() signature.Function {
		tableType := typetable.NewRecord().Build()
		return signature.Function{
			Type: typ.Func().
				Param("narray", typ.Integer).
				OptParam("nhash", typ.Integer).
				Returns(tableType).
				Build(),
			OperationalEffects: &signature.OperationalEffects{
				ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
					ReturnIndex: 0,
					Root:        "stdlib.table.create:return:0",
					Objects: []signature.AllocationObjectTemplate{{
						ID:   "stdlib.table.create:return:0",
						Type: tableType,
					}},
				}},
			},
		}
	}(),

	// json module.
	"json.encode": sig(
		typ.Func().
			Param("value", typ.Any).
			Returns(typ.String).
			Build(),
		ownership.BorrowAll{},
	),
	"json.decode": sig(
		typ.Func().
			Param("source", typ.String).
			Returns(typ.Any, normalize.Optional(typ.String)).
			Build(),
		ownership.BorrowAll{},
	),

	// env module.
	"env.get": sig(
		typ.Func().
			Param("name", typ.String).
			Returns(normalize.Optional(typ.String), normalize.Optional(typ.String)).
			Build(),
		ownership.BorrowAll{},
	),

	"table.freeze": func() signature.Function {
		tp := typ.NewTypeParam("T", nil)
		return sig(
			typ.Func().
				TypeParamRef(tp).
				Param("t", tp).
				Returns(tp).
				Build(),
		)
	}(),
	"table.isfrozen": sig(
		typ.Func().
			Param("t", typ.Any).
			Returns(typ.Boolean).
			Build(),
		ownership.BorrowAll{},
	),

	// string library: see init() below, populated from type/stringlib (single source).

	// math library.
	"math.abs":   sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.acos":  sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.asin":  sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.atan":  sig(typ.Func().Param("y", typ.Number).OptParam("x", typ.Number).Returns(typ.Number).Build()),
	"math.atan2": sig(typ.Func().Param("y", typ.Number).Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.ceil":  sig(typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()),
	"math.cos":   sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.cosh":  sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.deg":   sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.exp":   sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.floor": sig(typ.Func().Param("x", typ.Number).Returns(typ.Integer).Build()),
	"math.fmod":  sig(typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()),
	"math.frexp": sig(typ.Func().Param("x", typ.Number).Returns(typ.Number, typ.Integer).Build()),
	"math.ldexp": sig(typ.Func().Param("m", typ.Number).Param("e", typ.Integer).Returns(typ.Number).Build()),
	"math.log":   sig(typ.Func().Param("x", typ.Number).OptParam("base", typ.Number).Returns(typ.Number).Build()),
	"math.log10": sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.max":   numericExtremumSig(),
	"math.min":   numericExtremumSig(),
	"math.mod":   sig(typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()),
	"math.modf":  sig(typ.Func().Param("x", typ.Number).Returns(typ.Integer, typ.Number).Build()),
	"math.pow":   sig(typ.Func().Param("x", typ.Number).Param("y", typ.Number).Returns(typ.Number).Build()),
	"math.rad":   sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.random": sig(typ.Func().
		OptParam("m", typ.Integer).
		OptParam("n", typ.Integer).
		Returns(typ.Number).
		Build()),
	"math.randomseed": sig(typ.Func().
		OptParam("x", typ.Integer).
		OptParam("y", typ.Integer).
		Build()),
	"math.sin":       sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.sinh":      sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.sqrt":      sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.tan":       sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.tanh":      sig(typ.Func().Param("x", typ.Number).Returns(typ.Number).Build()),
	"math.tointeger": sig(typ.Func().Param("x", typ.Any).Returns(normalize.Optional(typ.Integer)).Build()),
	"math.type":      sig(typ.Func().Param("x", typ.Any).Returns(normalize.Optional(mathTypeName)).Build()),
	"math.ult":       sig(typ.Func().Param("m", typ.Integer).Param("n", typ.Integer).Returns(typ.Boolean).Build()),

	// coroutine library.
	"coroutine.close": sig(typ.Func().
		Param("co", typ.Any).
		Returns(typ.Boolean, normalize.Optional(typ.Any)).
		Build()),
	"coroutine.create": sig(typ.Func().
		Param("f", typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),
	"coroutine.isyieldable": sig(typ.Func().
		OptParam("co", typ.Any).
		Returns(typ.Boolean).
		Build()),
	"coroutine.resume": suspendingSig(typ.Func().
		Param("co", typ.Any).
		Variadic(typ.Any).
		Returns(typ.Boolean, typ.Any).
		Build(),
		ownership.Send{FromParam: 1}),
	"coroutine.running": sig(typ.Func().
		Returns(typ.Any, typ.Boolean).
		Build()),
	"coroutine.status": sig(typ.Func().
		Param("co", typ.Any).
		Returns(typ.String).
		Build()),
	"coroutine.wrap": suspendingSig(typ.Func().
		Param("f", typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),
	"coroutine.yield": suspendingSig(typ.Func().
		Variadic(typ.Any).
		Returns(typ.Any).
		Build(),
		ownership.Send{FromParam: 0}),

	// os library.
	"os.clock": sig(typ.Func().Returns(typ.Number).Build()),
	"os.date": sig(typ.Func().
		OptParam("format", typ.String).
		OptParam("time", typ.Integer).
		Returns(typ.Any).
		Build()),
	"os.difftime": sig(typ.Func().
		Param("t2", typ.Number).
		Param("t1", typ.Number).
		Returns(typ.Number).
		Build()),
	"os.getenv": sig(typ.Func().
		Param("varname", typ.String).
		Returns(normalize.Optional(typ.String)).
		Build()),
	"os.time": sig(typ.Func().
		OptParam("t", typ.Any).
		Returns(typ.Integer).
		Build()),
	"os.tmpname": sig(typ.Func().
		Returns(typ.String).
		Build()),
	"os.exit": sig(typ.Func().
		OptParam("code", typ.Any).
		OptParam("close", typ.Boolean).
		Returns(typ.Never).
		Build()),
	"os.remove": sig(typ.Func().
		Param("filename", typ.String).
		Returns(normalize.Optional(typ.Boolean), normalize.Optional(typ.String)).
		Build()),
	"os.rename": sig(typ.Func().
		Param("oldname", typ.String).
		Param("newname", typ.String).
		Returns(normalize.Optional(typ.Boolean), normalize.Optional(typ.String)).
		Build()),
	"os.execute": sig(typ.Func().
		OptParam("command", typ.String).
		Returns(typ.Any).
		Build()),
}

// luaTypeName is the complete Lua 5.4 type-name result domain.  It is kept as
// literals rather than string so a call-result consumer can retain the exact
// contract instead of silently weakening type(v) to an arbitrary string.
var luaTypeName = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("nil"),
	typ.LiteralString("boolean"),
	typ.LiteralString("number"),
	typ.LiteralString("string"),
	typ.LiteralString("table"),
	typ.LiteralString("function"),
	typ.LiteralString("thread"),
	typ.LiteralString("userdata"),
})

var mathTypeName = typ.MaterializeUnion([]typ.Type{
	typ.LiteralString("integer"),
	typ.LiteralString("float"),
})

// ResultSlotCondition is a declarative result refinement for one standard
// library call.  The table is part of the Lua library contract; callers must
// satisfy the literal argument predicate before consuming its result type.
type ResultSlotCondition struct {
	ResultIndex    int
	ArgumentIndex  int
	ArgumentString string
	ResultType     typ.Type
}

var conditionalResultSlots = map[string][]ResultSlotCondition{
	"select": {{
		ResultIndex:    0,
		ArgumentIndex:  0,
		ArgumentString: "#",
		ResultType:     typ.Integer,
	}},
}

// PositionalResultSlot declares that a result slot is optional only because a
// position argument may fall outside the subject argument's length. It is part
// of the Lua library contract: a caller that has already proved the subject at
// least as long as the position has discharged that optionality, and nothing
// else about the slot changes. PositionArgument is the argument holding the
// position; when the call omits it the contract's own default position applies.
type PositionalResultSlot struct {
	ResultIndex      int
	SubjectArgument  int
	PositionArgument int
	DefaultPosition  int64
}

// string.byte reads one character of its subject at a position, so its result
// is absent exactly when that position lies past the subject's end. Omitting
// the position reads the first character.
var positionalResultSlots = map[string][]PositionalResultSlot{
	"string.byte": {{
		ResultIndex:      0,
		SubjectArgument:  0,
		PositionArgument: 1,
		DefaultPosition:  1,
	}},
}

// PositionalResultSlots returns a copied view of the positional conditions the
// Lua standard-library contract declares for one provider.
func PositionalResultSlots(name string) []PositionalResultSlot {
	items := positionalResultSlots[name]
	out := make([]PositionalResultSlot, len(items))
	copy(out, items)
	return out
}

// Lookup returns a cloned effect signature for a known stdlib function name.
// init registers the string-library methods from the canonical type/stringlib
// table as string.<name> global signatures, so the global call and the colon
// method resolve from one source.
func init() {
	for _, name := range stringlib.Names() {
		fn, _ := stringlib.Method(name)
		registry["string."+name] = sig(fn)
	}
}

func Lookup(name string) (signature.Function, bool) {
	sig, ok := LookupView(name)
	if !ok {
		return signature.Function{}, false
	}
	return sig.Clone(), true
}

// LookupView returns the registry-owned immutable signature for name. It is an
// internal analysis hot-path view: callers must not mutate any reachable type
// or effect storage. Lookup remains the ownership-transferring public API.
func LookupView(name string) (signature.Function, bool) {
	sig, ok := registry[name]
	return sig, ok
}

// ResultSlot returns the declared type of one finite result position. Dynamic
// tails deliberately have no invented type; their owner remains conservative.
func ResultSlot(name string, index int) (typ.Type, bool) {
	sig, ok := LookupView(name)
	if !ok || sig.Type == nil || index < 0 || index >= len(sig.Type.Returns) {
		return nil, false
	}
	result := sig.Type.Returns[index]
	return result, result != nil
}

// ConditionalResultSlots returns a copied view of result refinements declared
// by the Lua standard-library contract.
func ConditionalResultSlots(name string) []ResultSlotCondition {
	items := conditionalResultSlots[name]
	out := make([]ResultSlotCondition, len(items))
	copy(out, items)
	return out
}

// MethodProvider returns the canonical global provider name for a typed Lua
// standard-library method.  It consults the string-library contract table;
// arbitrary receiver methods are intentionally not recognized here.
func MethodProvider(receiver typ.Type, method string) (string, bool) {
	if method == "" || !typ.TypeEquals(unwrap.Alias(receiver), typ.String) {
		return "", false
	}
	if _, ok := stringlib.Method(method); !ok {
		return "", false
	}
	name := "string." + method
	_, ok := LookupView(name)
	return name, ok
}

// bareGlobals names every Lua standard global that is always present in the
// environment: the base functions, the global table and version constants, and
// the standard library tables. They are recognized by name unconditionally;
// typed signatures for those that are functions are layered on top when the
// stdlib is loaded. This is the principled source for "this name is a known
// global", replacing per-call-site name switches in the diagnostics layer.
var bareGlobals = []string{
	"_G",
	"_GOPHER_LUA_VERSION",
	"_VERSION",
	"assert",
	"collectgarbage",
	"coroutine",
	"debug",
	"dofile",
	"error",
	"getmetatable",
	"io",
	"ipairs",
	"load",
	"loadfile",
	"math",
	"next",
	"os",
	"package",
	"pairs",
	"pcall",
	"print",
	"rawequal",
	"rawget",
	"rawlen",
	"rawset",
	"require",
	"select",
	"setmetatable",
	"string",
	"table",
	"tonumber",
	"tostring",
	"type",
	"unpack",
	"utf8",
	"xpcall",
}

// BareGlobals returns the always-present Lua global names recognized by name.
func BareGlobals() []string {
	return append([]string(nil), bareGlobals...)
}

// Signatures returns cloned registry entries keyed by stable stdlib names.
func Signatures() map[string]signature.Function {
	out := make(map[string]signature.Function, len(registry))
	for name, sig := range registry {
		out[name] = sig.Clone()
	}
	return out
}

// SignatureNames returns the stable stdlib signature names without cloning the
// signature registry. Callers that only need roots/names must use this instead
// of materializing Signatures.
func SignatureNames() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	return out
}

func numericExtremumSig() signature.Function {
	tp := typ.NewTypeParam("T", typ.Number)
	return sig(
		typ.Func().
			TypeParamRef(tp).
			Param("x", tp).
			Variadic(tp).
			Returns(tp).
			Build(),
	)
}

func sig(fn *typ.Function, labels ...effect.Label) signature.Function {
	for _, label := range labels {
		mustAllowStdlibEffectLabel(label)
	}
	return signature.Function{
		Type:   fn,
		Effect: effect.Row{Labels: labels},
	}
}

func suspendingSig(fn *typ.Function, labels ...effect.Label) signature.Function {
	out := sig(fn, labels...)
	out.OperationalEffects = &signature.OperationalEffects{SuspensionKnown: true, MaySuspend: true}
	return out
}

func mustAllowStdlibEffectLabel(label effect.Label) {
	desc, ok := caplabel.DescriptorFor(label)
	if !ok {
		panic(fmt.Sprintf("stdlib: unaudited effect label %T: %v", label, label))
	}
	switch desc.Status {
	case capability.StatusReserved, capability.StatusReservedHighRisk:
		panic(fmt.Sprintf("stdlib: inactive effect label %s (%s): %v", desc.ID, desc.Status, label))
	}
}
