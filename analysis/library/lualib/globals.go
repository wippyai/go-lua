package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// The Lua globals are not a library. A bare global is a SLOT of the initial
// environment bound to the value that initially occupies it, and binding a slot
// is the one place in the whole surface where a name meets a value - which is
// why only the environment class may declare it. Forcing `print` and `pairs`
// into a library instance would have published them as exports of a mounted
// aggregate that does not exist, and the addressing would have been a fiction.
//
// This instance carries the whole of what the standard-library model states
// about the initial environment: which names it always has and what value each
// one initially holds, the typed application of the ones that are functions, the
// result refinement one of them publishes, the sealed native identity another one
// IS, the roots it boots, the entries it refuses or never had, the metatable it
// attaches to the string primitive, and the capabilities its host grants it. The
// rows that are not about a global name are authored in boot.go, beside the
// ledger content they absorb.

// GlobalsRoot is the authored mount selector of the initial environment. It is
// the environment's own name in the language, it selects a mount during project
// construction, and no member address derives from it.
const GlobalsRoot = "_ENV"

// globalSlots is the authored inventory of names the initial environment always
// has: the base functions, the global table and version constants, and the
// standard library aggregates. Each is one slot of the environment, addressed as
// a one-step export path from the environment root.
var globalSlots = []string{
	"_G", "_GOPHER_LUA_VERSION", "_VERSION", "assert", "bit32", "collectgarbage",
	"coroutine", "debug", "dofile", "error", "errors", "getmetatable", "io",
	"ipairs", "load", "loadfile", "math", "next", "os", "package", "pairs",
	"pcall", "print", "rawequal", "rawget", "rawlen", "rawset", "require",
	"select", "setmetatable", "string", "table", "tonumber", "tostring", "type",
	"unpack", "utf8", "xpcall",
}

// globalSlotConstants are the slots that hold a literal rather than a value the
// contract can keep addressing through. A constant terminates the path, so it has
// no address of its own and rides the binding that holds it.
var globalSlotConstants = map[string]contract.Constant{
	"_VERSION":            {Kind: contract.ConstantString, String: "Lua 5.3 - Wippy Modification"},
	"_GOPHER_LUA_VERSION": {Kind: contract.ConstantString, String: "GopherLua 0.2 Wippy Edition"},
}

// globalSlotAliases are the slots whose value lives at an address other than the
// slot's own. There is one: `_G` holds the environment itself, so it binds the
// contract root, and a consumer that walks the binding arrives back where it
// started - which is exactly what the alias means and what a second name for the
// environment could never state.
var globalSlotAliases = map[string]contract.Path{"_G": contract.Root()}

// globalSlotBinding is what one slot initially holds. Every slot is published
// mutable: the initial environment lets a program rebind any global, including
// the ones whose value it refuses to supply, and a slot bound to a frozen
// aggregate is still a rebindable slot.
func globalSlotBinding(name string) contract.EnvironmentSlot {
	if constant, published := globalSlotConstants[name]; published {
		return contract.BindConstant(constant, contract.MutabilityMutable)
	}
	if alias, aliased := globalSlotAliases[name]; aliased {
		return contract.BindValue(alias, contract.MutabilityMutable)
	}
	return contract.BindValue(contract.Export(name), contract.MutabilityMutable)
}

// globalCallables is the authored inventory of environment slots whose initial
// value is a callable, in slot order. The remaining slots hold an aggregate or a
// constant, and what those values are is the export-value and boot-root forms'
// business.
//
// The `string` slot is deliberately not here, and now that the slot binding is
// content the reason is a verdict rather than an open conflict. The binding says
// the slot holds the value at the address of the string library aggregate, and
// the boot root at that address says that value is a table. A callable envelope
// published at the same address would say the same value is a function: one
// address, one value, two contradictory statements about it - and the surface's
// laws would not catch it, because the two are different member forms. Distinct
// forms do not rescue a contradiction; the address is the value.
//
// So the coercion stays withheld, and it is not withheld for want of a place to
// put it. The modeled `string`, `number` and `integer` bare signatures are host
// coercions, and the environment this contract states boots `string` as a plain
// table with no __call edge: in it, `string(v)` is not a call, and publishing a
// signature that says it is would describe an environment nobody boots. A host
// that did grant the coercion would be booting a callable aggregate, and the
// sound spelling of that is a __call metatable edge on the string library's own
// root - a member of the value - never a second member at this slot's name.
var globalCallables = []string{
	"assert", "collectgarbage", "error", "getmetatable", "ipairs", "next",
	"pairs", "pcall", "print", "rawequal", "rawget", "rawlen", "rawset",
	"require", "select", "setmetatable", "tonumber", "tostring", "type",
	"unpack", "xpcall",
}

// globalRefinements are the result refinements the environment publishes, keyed
// by the slot they refine. select(index, ...) returns the count of its variadic
// tail when its first argument is the literal "#", and returns a member of that
// tail otherwise: the predicate is a literal and the refined result is a type, so
// the whole relation is contract data.
var globalRefinements = map[string]wire.ResultRefinement{
	"select": wire.LiteralArgumentRefinement{
		Result: 0, Argument: 0, Literal: "#", Type: typ.Integer,
	},
}

// globalIntrinsics are the sealed native identities the environment publishes,
// keyed by the slot that holds the operation. type(v) answers with the runtime
// family of a caller value, which no signature can state, so the contract
// publishes the identity of the operation and a consumer reads it from the value
// instead of reconstructing it from the callee's name.
var globalIntrinsics = map[string]signature.Intrinsic{
	"type": signature.IntrinsicLuaType,
}

// GlobalSlots returns a copy of the authored environment slot inventory.
func GlobalSlots() []string { return copyNames(globalSlots) }

// GlobalCallables returns a copy of the authored callable slot inventory.
func GlobalCallables() []string { return copyNames(globalCallables) }

// GlobalsContract authors the Lua globals as an instance of the declared
// environment contract kind. The kind is the authority for the codec, the
// payload format identity of every member form and the addressing law; an
// individual library kind cannot declare an environment slot at all, so handing
// one in rejects this instance rather than publishing the globals as a library.
//
// Member order is the authored order: the boot roots, the slot bindings, the
// callable envelopes, the refinements, the markers, the denials, the primitive
// metatable attachments and the host grant. Order is content - the instance
// identity is the digest of exactly these bytes.
//
// Nothing is deferred. Every form this environment publishes has a landed payload
// format, so each member carries what it has to say instead of its address alone.
func GlobalsContract(kind *library.Entry) (*contract.Instance, bool) {
	if kind == nil || kind.Class() != library.ClassEnvironment {
		return nil, false
	}
	members := make([]contract.Member, 0,
		len(environmentBootRoots)+len(globalSlots)+len(globalCallables)+
			len(globalRefinements)+len(globalIntrinsics)+len(environmentDenials)+2)
	for _, row := range environmentBootRoots {
		body, err := contract.EncodeBootRoot(row.Root)
		if err != nil {
			return nil, false
		}
		members = append(members, resolvedMember(kind, library.FormBootRoot, row.Path, body))
	}
	for _, name := range globalSlots {
		body, err := contract.EncodeEnvironmentSlot(globalSlotBinding(name))
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormEnvironmentSlot, contract.Export(name), body))
	}
	for _, name := range globalCallables {
		envelope, envelopeOK := callableEnvelope(globalsSignatures[name])
		if !envelopeOK {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormCallableSignature, contract.Export(name), envelope))
	}
	// The refinements and markers are walked in slot order rather than in map
	// order, so what the instance publishes is the authored artifact and not a
	// traversal.
	for _, name := range globalSlots {
		refinement, published := globalRefinements[name]
		if !published {
			continue
		}
		body, err := wire.EncodeResultRefinement(refinement)
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormResultRefinement, contract.Export(name), body))
	}
	for _, name := range globalSlots {
		intrinsic, published := globalIntrinsics[name]
		if !published {
			continue
		}
		body, err := wire.EncodeIntrinsicMarker(intrinsic)
		if err != nil {
			return nil, false
		}
		members = append(members,
			resolvedMember(kind, library.FormIntrinsicMarker, contract.Export(name), body))
	}
	for _, row := range environmentDenials {
		body, err := contract.EncodeDeniedEntry(contract.DeniedEntry{Denial: row.Denial, Entry: row.Path})
		if err != nil {
			return nil, false
		}
		members = append(members, resolvedMember(kind, library.FormDeniedEntry, row.Path, body))
	}
	// The attachment set and the host grant are facts about the environment as a
	// whole rather than about any value it exports, so each is one member at the
	// contract root: the environment is the value they are true of.
	attachments, err := contract.EncodePrimitiveMetatables(environmentPrimitiveMetatables)
	if err != nil {
		return nil, false
	}
	members = append(members,
		resolvedMember(kind, library.FormPrimitiveMetatable, contract.Root(), attachments))
	granted, err := contract.EncodeHostCapabilities(environmentHostCapabilities)
	if err != nil {
		return nil, false
	}
	members = append(members,
		resolvedMember(kind, library.FormHostCapability, contract.Root(), granted))
	return contract.New(contract.Spec{
		Kind:    kind.Key(),
		Codec:   kind.Codec(),
		Root:    GlobalsRoot,
		Members: members,
	}, kind)
}
