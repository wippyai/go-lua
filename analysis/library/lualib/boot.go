package lualib

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	"github.com/wippyai/go-lua/analysis/library/contract"
)

// The initial environment as the environment contract states it: the roots it
// boots, the entries it refuses or never had, the metatable it attaches to a
// primitive, and the capabilities its host grants it. Together with the slots and
// envelopes in globals.go this is the whole of what the standard-library model
// says about the environment a Lua program starts in.
//
// Every row here is addressed by a path of exported values from the environment
// root. The initial-environment ledger this absorbs identified its roots by
// authored NAMES - "StringRoot", "ErrorMetatableRoot", "GlobalEnvRoot" - and a
// name is what this surface refuses: the environment root is the contract root,
// the table library is the value one step from it, and the error metatable is the
// value the errors aggregate publishes under Error. Nothing below can be reached
// under a second spelling, because there is no spelling at all.

// bootRootRow is one authored root of the initial environment: where the root is
// reached from the environment root, and what it boots as.
type bootRootRow struct {
	Path contract.Path
	Root contract.BootRoot
}

// environmentBootRoots is the authored initial-root inventory, in path order.
//
// The mutability of a root is the seal on the OBJECT: the aggregates a Wippy host
// boots frozen are frozen whoever holds them, and the alias in a global slot does
// not soften that. Whether the SLOT holding a root may be rebound is the slot
// binding's own statement, and the two are deliberately separate - `errors` is a
// frozen table at a rebindable slot.
//
// One root of the ledger is deliberately absent: the metatable of the string
// primitive is reachable from no slot of the environment, so it has no address
// here. It is not omitted - the primitive-metatable attachment below is where a
// root that no export path reaches is stated, and it carries that root's whole
// content.
var environmentBootRoots = []bootRootRow{
	{contract.Root(), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
	{exportPath("bit32"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilitySealed}},
	{exportPath("coroutine"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
	{exportPath("debug"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
	{exportPath("errors"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilitySealed}},
	{exportPath("errors", "Error"), contract.BootRoot{Aggregate: contract.BootAggregateMetatable, Mutability: contract.MutabilitySealed}},
	{exportPath("errors", "Error", "__index"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilitySealed}},
	{exportPath("io"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilitySealed}},
	{exportPath("math"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
	{exportPath("os"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilitySealed}},
	{exportPath("package"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilitySealed}},
	{exportPath("string"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
	{exportPath("table"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
	{exportPath("utf8"), contract.BootRoot{Aggregate: contract.BootAggregateTable, Mutability: contract.MutabilityMutable}},
}

// denialRow is one authored entry the initial environment does not hand out.
type denialRow struct {
	Path   contract.Path
	Denial contract.Denial
}

// environmentDenials is the authored inventory of entries the environment boots
// without, in path order, refusals before absences within each aggregate.
//
// The two refusals are different facts and are stated as the different facts they
// are. A refused entry is a member of the language the host will not run:
// os.execute exists, and a Wippy actor may not start a process. An absent entry is
// not there at all: package.path is not withheld from a program, it is a field
// the initial environment never wrote, so reading it yields nothing rather than
// an unsupported operation.
//
// A library contract may refuse a member of its own - string.dump is refused by
// the string library because the target cannot load a binary chunk back - and the
// environment refusing the same address is not that statement repeated. The
// library states what it models and will not publish; the environment states what
// the host booted. They agree here, and either could change without the other.
var environmentDenials = []denialRow{
	{exportPath("collectgarbage"), contract.DenialRefused},
	{exportPath("dofile"), contract.DenialRefused},
	{exportPath("load"), contract.DenialRefused},
	{exportPath("loadfile"), contract.DenialRefused},

	{exportPath("bit32", "arshift"), contract.DenialRefused},
	{exportPath("bit32", "band"), contract.DenialRefused},
	{exportPath("bit32", "bnot"), contract.DenialRefused},
	{exportPath("bit32", "bor"), contract.DenialRefused},
	{exportPath("bit32", "btest"), contract.DenialRefused},
	{exportPath("bit32", "bxor"), contract.DenialRefused},
	{exportPath("bit32", "extract"), contract.DenialRefused},
	{exportPath("bit32", "lrotate"), contract.DenialRefused},
	{exportPath("bit32", "lshift"), contract.DenialRefused},
	{exportPath("bit32", "replace"), contract.DenialRefused},
	{exportPath("bit32", "rrotate"), contract.DenialRefused},
	{exportPath("bit32", "rshift"), contract.DenialRefused},

	{exportPath("coroutine", "close"), contract.DenialRefused},
	{exportPath("coroutine", "isyieldable"), contract.DenialRefused},

	{exportPath("debug", "getinfo"), contract.DenialRefused},
	{exportPath("debug", "getlocal"), contract.DenialRefused},
	{exportPath("debug", "getmetatable"), contract.DenialRefused},
	{exportPath("debug", "setlocal"), contract.DenialRefused},
	{exportPath("debug", "setmetatable"), contract.DenialRefused},
	{exportPath("debug", "setupvalue"), contract.DenialRefused},
	{exportPath("debug", "traceback"), contract.DenialRefused},

	{exportPath("errors", "call_stack"), contract.DenialRefused},
	{exportPath("errors", "Error", "__index", "stack"), contract.DenialRefused},

	{exportPath("io", "close"), contract.DenialRefused},
	{exportPath("io", "flush"), contract.DenialRefused},
	{exportPath("io", "input"), contract.DenialRefused},
	{exportPath("io", "lines"), contract.DenialRefused},
	{exportPath("io", "open"), contract.DenialRefused},
	{exportPath("io", "output"), contract.DenialRefused},
	{exportPath("io", "popen"), contract.DenialRefused},
	{exportPath("io", "read"), contract.DenialRefused},
	{exportPath("io", "tmpfile"), contract.DenialRefused},
	{exportPath("io", "type"), contract.DenialRefused},
	{exportPath("io", "write"), contract.DenialRefused},
	{exportPath("io", "stderr"), contract.DenialAbsent},
	{exportPath("io", "stdin"), contract.DenialAbsent},
	{exportPath("io", "stdout"), contract.DenialAbsent},

	{exportPath("os", "clock"), contract.DenialRefused},
	{exportPath("os", "date"), contract.DenialRefused},
	{exportPath("os", "difftime"), contract.DenialRefused},
	{exportPath("os", "execute"), contract.DenialRefused},
	{exportPath("os", "exit"), contract.DenialRefused},
	{exportPath("os", "getenv"), contract.DenialRefused},
	{exportPath("os", "remove"), contract.DenialRefused},
	{exportPath("os", "rename"), contract.DenialRefused},
	{exportPath("os", "setlocale"), contract.DenialRefused},
	{exportPath("os", "time"), contract.DenialRefused},
	{exportPath("os", "tmpname"), contract.DenialRefused},

	{exportPath("package", "loadlib"), contract.DenialRefused},
	{exportPath("package", "searchpath"), contract.DenialRefused},
	{exportPath("package", "seeall"), contract.DenialRefused},
	{exportPath("package", "config"), contract.DenialAbsent},
	{exportPath("package", "cpath"), contract.DenialAbsent},
	{exportPath("package", "loaded"), contract.DenialAbsent},
	{exportPath("package", "loaders"), contract.DenialAbsent},
	{exportPath("package", "path"), contract.DenialAbsent},
	{exportPath("package", "preload"), contract.DenialAbsent},
	{exportPath("package", "searchers"), contract.DenialAbsent},

	{exportPath("string", "dump"), contract.DenialRefused},
}

// environmentPrimitiveMetatables is the authored primitive metatable attachment
// set. A string value reaches string.upper through it, which is what makes
// `s:upper()` a member access rather than a name lookup.
//
// The metatable is not the environment's own: the string library owns it and
// publishes its members through it, and this row states that the attachment
// applies to every string the program produces. The reference is written as the
// library's mount selector and the metatable-key address inside it, so an edit to
// the string library - one export more, one signature refined - leaves this
// statement true. A reference by the string contract's content identity would
// have named a revision instead of a library, and every such edit would have left
// the environment pointing at bytes nobody publishes.
var environmentPrimitiveMetatables = []contract.PrimitiveAttachment{
	{
		Base:       contract.ConstantString,
		Contract:   StringRoot,
		Path:       contract.Metatable(StringMetatableIndexKey),
		Mutability: contract.MutabilityMutable,
	},
}

// environmentHostCapabilities is the authored capability grant the initial
// environment boots under: the audited identities a contract published into this
// environment may exercise in an effect row.
//
// The grant is authored rather than computed from the vocabulary it names. A host
// that granted exactly whatever the vocabulary happened to contain would not be
// granting anything - it would be the vocabulary under a second name, and the two
// could never disagree. Authored, the grant is a statement a law can hold against
// the audit: every identity here is audited, and every audited capability that is
// not reserved is granted, so a capability that becomes active and is not granted
// is a finding rather than a silent widening.
var environmentHostCapabilities = []string{
	capability.DispatchModuleLoad,
	capability.IterationIterator,
	capability.LifecycleAcquire,
	capability.LifecycleEscape,
	capability.LifecycleTransition,
	capability.MutationLengthChange,
	capability.MutationMutate,
	capability.MutationTableMutator,
	capability.OwnershipBorrow,
	capability.OwnershipBorrowAll,
	capability.OwnershipRetain,
	capability.OwnershipSend,
	capability.OwnershipSendParam,
	capability.OwnershipStore,
	capability.PostconditionNormalReturnRefinement,
	capability.ReturnsErrorReturn,
	capability.ReturnsReturnArrayOfCallbackReturn,
	capability.ReturnsReturnCallbackReturn,
	capability.ReturnsReturnConditionalType,
	capability.ReturnsReturnElementOf,
	capability.ReturnsReturnOptionalElementOf,
	capability.ReturnsReturnSameAs,
	capability.ReturnsReturnTypeProjection,
}

// EnvironmentHostCapabilities returns a copy of the authored capability grant.
func EnvironmentHostCapabilities() []string { return copyNames(environmentHostCapabilities) }

// exportPath builds the address of one member reached by walking exported values
// from a contract root.
func exportPath(keys ...string) contract.Path {
	steps := make([]contract.Step, 0, len(keys))
	for _, key := range keys {
		steps = append(steps, contract.Step{Kind: contract.StepExport, Key: key})
	}
	return contract.NewPath(steps...)
}
