package lualib

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	"github.com/wippyai/go-lua/analysis/library/contract"
	profile "github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// The drift laws of the initial environment, in the direction the retirement
// establishes: the environment contract is the statement, and the authored
// target profile's initial-root ledger is derived from it and held to it. The
// ledger still identifies its roots by authored names; nothing below reads one.
// Every row it checks is reached by walking exported values from the environment
// root, which is what makes the name column of that ledger removable.

func bootLedger(t *testing.T) *target.Contract {
	t.Helper()
	ledger, err := profile.Contract()
	if err != nil {
		t.Fatalf("the authored target profile did not seal: %v", err)
	}
	return ledger
}

// ledgerKeys indexes the ledger's exact string keys, so a member address can be
// resolved against it without the ledger's own handles leaking into a law.
func ledgerKeys(t *testing.T, ledger *target.Contract) map[string]target.ExactKey {
	t.Helper()
	keys := make(map[string]target.ExactKey, ledger.ExactKeyCount())
	for index := 0; index < ledger.ExactKeyCount(); index++ {
		key, ok := ledger.ExactKeyAt(index)
		if !ok {
			t.Fatal("the ledger holds an unavailable exact key")
		}
		value, valueOK := ledger.ExactKeyValue(key)
		if !valueOK {
			t.Fatal("the ledger holds an exact key with no value")
		}
		if value.Kind == keyspace.LiteralString {
			keys[value.String] = key
		}
	}
	return keys
}

// resolveLedgerRoot walks one export path from the environment root and answers
// with the ledger root it reaches. It is the whole point of the absorption: an
// address the contract publishes is a walk over the ledger's own alias entries,
// so a root needs no name to be identified.
func resolveLedgerRoot(t *testing.T, ledger *target.Contract, keys map[string]target.ExactKey, path contract.Path) (target.InitialRoot, bool) {
	t.Helper()
	root, ok := ledger.GlobalEnvRoot()
	if !ok {
		t.Fatal("the ledger declares no global environment root")
	}
	for position := 0; position < path.Len(); position++ {
		step, stepOK := path.At(position)
		if !stepOK || step.Kind != contract.StepExport {
			return 0, false
		}
		key, keyOK := keys[step.Key]
		if !keyOK {
			return 0, false
		}
		value, _, entryOK := ledger.InitialEntry(root, key)
		if !entryOK {
			return 0, false
		}
		kind, kindOK := ledger.InitialValueKind(value)
		if !kindOK || kind != target.InitialValueRoot {
			return 0, false
		}
		next, nextOK := ledger.InitialValueRoot(value)
		if !nextOK {
			return 0, false
		}
		root = next
	}
	return root, true
}

// resolveLedgerEntry walks one export path to the entry its last step names.
func resolveLedgerEntry(t *testing.T, ledger *target.Contract, keys map[string]target.ExactKey, path contract.Path) (target.InitialValue, target.InitialMutability, bool) {
	t.Helper()
	if path.Len() == 0 {
		return 0, 0, false
	}
	last, lastOK := path.At(path.Len() - 1)
	if !lastOK || last.Kind != contract.StepExport {
		return 0, 0, false
	}
	owner, ownerOK := resolveLedgerRoot(t, ledger, keys, contract.NewPath(pathSteps(path)[:path.Len()-1]...))
	if !ownerOK {
		return 0, 0, false
	}
	key, keyOK := keys[last.Key]
	if !keyOK {
		return 0, 0, false
	}
	value, mutability, ok := ledger.InitialEntry(owner, key)
	return value, mutability, ok
}

func pathSteps(path contract.Path) []contract.Step {
	steps := make([]contract.Step, 0, path.Len())
	for position := 0; position < path.Len(); position++ {
		step, ok := path.At(position)
		if !ok {
			return nil
		}
		steps = append(steps, step)
	}
	return steps
}

func environmentMembers(t *testing.T, form library.Form) []contract.Member {
	t.Helper()
	var rows []contract.Member
	for _, member := range globalsInstance(t).Members() {
		if member.Form == form {
			rows = append(rows, member)
		}
	}
	return rows
}

// TestBootRootsAreDerivedFromTheLedgerRoots is the content law of the boot-root
// form. Each root the environment publishes is reached by walking its own export
// graph, and what the ledger holds at the root that walk arrives at - the
// aggregate it boots as, the seal on its object - is derived from the row the
// contract publishes there.
func TestBootRootsAreDerivedFromTheLedgerRoots(t *testing.T) {
	ledger := bootLedger(t)
	keys := ledgerKeys(t, ledger)
	rows := environmentMembers(t, library.FormBootRoot)
	if len(rows) != len(environmentBootRoots) {
		t.Fatalf("the contract publishes %d boot roots, want %d", len(rows), len(environmentBootRoots))
	}
	reached := make(map[target.InitialRoot]bool, len(rows))
	for _, member := range rows {
		published, err := contract.DecodeBootRoot(member.Body)
		if err != nil {
			t.Fatalf("a boot root did not decode: %v", err)
		}
		root, ok := resolveLedgerRoot(t, ledger, keys, member.Path)
		if !ok {
			t.Fatalf("the boot root at %v reaches no ledger root", pathSteps(member.Path))
		}
		if reached[root] {
			t.Fatalf("two boot root members reach one ledger root")
		}
		reached[root] = true
		shape, shapeOK := ledger.InitialRootBootShape(root)
		if !shapeOK {
			t.Fatal("the ledger root has no boot shape")
		}
		aggregate, aggregateOK := ledger.BootShapeAggregate(shape)
		immutable, immutableOK := ledger.BootShapeImmutable(shape)
		if !aggregateOK || !immutableOK {
			t.Fatal("the ledger boot shape is incomplete")
		}
		wantAggregate := contract.BootAggregateTable
		if aggregate == target.BootAggregateMetatable {
			wantAggregate = contract.BootAggregateMetatable
		}
		wantMutability := contract.MutabilityMutable
		if immutable {
			wantMutability = contract.MutabilitySealed
		}
		if published.Aggregate != wantAggregate || published.Mutability != wantMutability {
			t.Fatalf("the contract publishes %+v at %v and the ledger boots %v/%v",
				published, pathSteps(member.Path), aggregate, immutable)
		}
	}
	// The ledger has one root more than the environment addresses, and it is the
	// one no export path reaches: the metatable of the string primitive. It is
	// stated by the attachment member instead, so the ledger is fully covered.
	if ledger.InitialRootCount() != len(rows)+1 {
		t.Fatalf("the ledger boots %d roots and the contract addresses %d", ledger.InitialRootCount(), len(rows))
	}
}

// TestEnvironmentSlotsAreDerivedFromTheLedgerBindings is the content law of the
// slot form. The ledger's global bindings are exactly the slots this contract
// binds, and what each binding holds is derived from the binding row: a slot that
// binds an address reaches the ledger value that address resolves to, and a slot
// that binds a constant holds the literal the ledger wrote there.
func TestEnvironmentSlotsAreDerivedFromTheLedgerBindings(t *testing.T) {
	ledger := bootLedger(t)
	keys := ledgerKeys(t, ledger)
	if ledger.InitialBindingCount() != len(globalSlots) {
		t.Fatalf("the ledger binds %d globals and the contract binds %d slots",
			ledger.InitialBindingCount(), len(globalSlots))
	}
	instance := globalsInstance(t)
	for index := 0; index < ledger.InitialBindingCount(); index++ {
		name, _, value, root, key, ok := ledger.InitialBindingAt(index)
		if !ok {
			t.Fatal("the ledger holds an unavailable global binding")
		}
		member, found := instance.Resolve(library.FormEnvironmentSlot, contract.Export(name))
		if !found {
			t.Fatalf("the ledger binds the global %q and the contract has no slot for it", name)
		}
		slot, err := contract.DecodeEnvironmentSlot(member.Body)
		if err != nil {
			t.Fatalf("the slot binding of %q did not decode: %v", name, err)
		}
		_, mutability, entryOK := ledger.InitialEntry(root, key)
		if !entryOK {
			t.Fatalf("the ledger binds %q to no entry", name)
		}
		wantMutability := contract.MutabilitySealed
		if mutability == target.InitialMutable {
			wantMutability = contract.MutabilityMutable
		}
		if slot.Mutability != wantMutability {
			t.Fatalf("the slot %q is published %v and the ledger writes %v", name, slot.Mutability, mutability)
		}
		kind, kindOK := ledger.InitialValueKind(value)
		if !kindOK {
			t.Fatalf("the ledger binds %q to a valueless entry", name)
		}
		switch slot.Binding {
		case contract.SlotBindingConstant:
			if kind != target.InitialValueString {
				t.Fatalf("the slot %q binds a constant and the ledger holds value kind %v", name, kind)
			}
			text, textOK := ledger.InitialValueString(value)
			if !textOK || slot.Constant.Kind != contract.ConstantString || slot.Constant.String != text {
				t.Fatalf("the slot %q binds %+v and the ledger holds %q", name, slot.Constant, text)
			}
		case contract.SlotBindingValue:
			if kind == target.InitialValueRoot {
				want, wantOK := ledger.InitialValueRoot(value)
				bound, boundOK := resolveLedgerRoot(t, ledger, keys, slot.Value)
				if !wantOK || !boundOK || want != bound {
					t.Fatalf("the slot %q binds an address that reaches another root than the ledger holds", name)
				}
				continue
			}
			// A slot that holds a callable or a refused entry binds its own
			// address: the value is published at the slot, and what it is comes
			// from the form that owns it.
			if !slot.Value.Equal(contract.Export(name)) {
				t.Fatalf("the slot %q binds %v rather than its own address", name, pathSteps(slot.Value))
			}
		default:
			t.Fatalf("the slot %q binds nothing", name)
		}
	}
}

// TestEnvironmentDenialsAreDerivedFromTheLedgerEntries is the content law of the
// denied-entry form on the environment side. Every entry the ledger boots refused
// or absent is published as the denial it is, and every denial the contract
// publishes names an entry the ledger did not supply: the refusal is contract
// data, so what the ledger holds is derived from it rather than rediscovered.
func TestEnvironmentDenialsAreDerivedFromTheLedgerEntries(t *testing.T) {
	ledger := bootLedger(t)
	keys := ledgerKeys(t, ledger)
	rows := environmentMembers(t, library.FormDeniedEntry)
	if len(rows) != len(environmentDenials) {
		t.Fatalf("the contract publishes %d denials, want %d", len(rows), len(environmentDenials))
	}
	var refused, absent int
	for _, member := range rows {
		denied, err := contract.DecodeDeniedEntry(member.Body)
		if err != nil {
			t.Fatalf("a denial did not decode: %v", err)
		}
		if !denied.Entry.Equal(member.Path) {
			t.Fatalf("a denial at %v refuses another address", pathSteps(member.Path))
		}
		value, _, ok := resolveLedgerEntry(t, ledger, keys, member.Path)
		if !ok {
			t.Fatalf("the denial at %v names no ledger entry", pathSteps(member.Path))
		}
		kind, kindOK := ledger.InitialValueKind(value)
		if !kindOK {
			t.Fatal("the ledger entry has no value kind")
		}
		switch denied.Denial {
		case contract.DenialRefused:
			refused++
			if kind != target.InitialValueDeniedOperation {
				t.Fatalf("the contract refuses %v and the ledger boots value kind %v", pathSteps(member.Path), kind)
			}
		case contract.DenialAbsent:
			absent++
			if kind != target.InitialValueAbsent {
				t.Fatalf("the contract states %v absent and the ledger boots value kind %v", pathSteps(member.Path), kind)
			}
		default:
			t.Fatalf("the denial at %v states no refusal", pathSteps(member.Path))
		}
	}
	// The counts are the ledger's own, derived: an entry the host stopped
	// supplying without a denial member would leave the environment claiming to
	// publish a member it does not have.
	var ledgerRefused, ledgerAbsent int
	for index := 0; index < ledger.InitialEntryCount(); index++ {
		_, _, value, _, ok := ledger.InitialEntryAt(index)
		if !ok {
			t.Fatal("the ledger holds an unavailable entry")
		}
		kind, kindOK := ledger.InitialValueKind(value)
		if !kindOK {
			t.Fatal("the ledger entry has no value kind")
		}
		switch kind {
		case target.InitialValueDeniedOperation:
			ledgerRefused++
		case target.InitialValueAbsent:
			ledgerAbsent++
		}
	}
	if refused != ledgerRefused || absent != ledgerAbsent {
		t.Fatalf("the contract publishes %d refused and %d absent entries, and the ledger boots %d and %d",
			refused, absent, ledgerRefused, ledgerAbsent)
	}
}

// TestPrimitiveMetatableIsDerivedFromTheLedgerAttachment is the content law of
// the attachment form, and the one place a contract names another contract's
// member. The reference resolves: the string library publishes a metatable edge
// at the key this row names, and that edge resolves to the library root the
// ledger's attached metatable indexes into. The ledger's metatable root is
// reachable from no slot, so this member is the whole of what the environment
// says about it.
func TestPrimitiveMetatableIsDerivedFromTheLedgerAttachment(t *testing.T) {
	ledger := bootLedger(t)
	keys := ledgerKeys(t, ledger)
	rows := environmentMembers(t, library.FormPrimitiveMetatable)
	if len(rows) != 1 || rows[0].Path.Len() != 0 {
		t.Fatal("the environment publishes its attachment set anywhere but at its own root")
	}
	attachments, err := contract.DecodePrimitiveMetatables(rows[0].Body)
	if err != nil {
		t.Fatalf("the attachment set did not decode: %v", err)
	}
	if len(attachments) != ledger.InitialMetatableAttachmentCount() {
		t.Fatalf("the contract attaches %d metatables and the ledger attaches %d",
			len(attachments), ledger.InitialMetatableAttachmentCount())
	}
	for index, attachment := range attachments {
		base, metatable, ok := ledger.InitialMetatableAttachmentAt(index)
		if !ok {
			t.Fatal("the ledger holds an unavailable metatable attachment")
		}
		if base != target.InitialValueString || attachment.Base != contract.ConstantString {
			t.Fatalf("the contract attaches to base %v and the ledger attaches to %v", attachment.Base, base)
		}
		shape, shapeOK := ledger.InitialRootBootShape(metatable)
		if !shapeOK {
			t.Fatal("the attached ledger root has no boot shape")
		}
		aggregate, aggregateOK := ledger.BootShapeAggregate(shape)
		immutable, immutableOK := ledger.BootShapeImmutable(shape)
		if !aggregateOK || !immutableOK || aggregate != target.BootAggregateMetatable {
			t.Fatal("the ledger attaches a root that is not a metatable")
		}
		wantMutability := contract.MutabilityMutable
		if immutable {
			wantMutability = contract.MutabilitySealed
		}
		if attachment.Mutability != wantMutability {
			t.Fatalf("the attached metatable is published %v and the ledger boots immutable=%v",
				attachment.Mutability, immutable)
		}
		// The reference is resolvable: the named contract publishes an edge at
		// the named key, and the edge reaches the same library root the ledger's
		// metatable indexes into.
		if attachment.Contract != StringRoot {
			t.Fatalf("the attachment names the contract %q, and no mounted library is selected by it", attachment.Contract)
		}
		edge, edgeFound := stringInstance(t).Resolve(library.FormMetatableEdge, attachment.Path)
		if !edgeFound {
			t.Fatalf("the string contract publishes no metatable edge at %v", pathSteps(attachment.Path))
		}
		target, err := contract.DecodePath(edge.Body)
		if err != nil {
			t.Fatalf("the string metatable edge did not decode: %v", err)
		}
		if target.Len() != 0 {
			t.Fatal("the string metatable edge resolves to something other than the library root")
		}
		key, keyOK := keys[StringMetatableIndexKey]
		if !keyOK {
			t.Fatal("the ledger holds no key for the metatable index")
		}
		indexed, _, entryOK := ledger.InitialEntry(metatable, key)
		if !entryOK {
			t.Fatal("the ledger metatable holds no index entry")
		}
		indexedRoot, indexedOK := ledger.InitialValueRoot(indexed)
		library, libraryOK := resolveLedgerRoot(t, ledger, keys, contract.Export(StringRoot))
		if !indexedOK || !libraryOK || indexedRoot != library {
			t.Fatal("the ledger metatable indexes into a root other than the string library the slot binds")
		}
	}
}

// TestHostCapabilityGrantIsTheAuditedActiveVocabulary is the content law of the
// grant. Every capability the environment grants is one the vocabulary audits and
// does not hold reserved, and every audited capability that is active is granted:
// a capability that becomes active without entering the grant would be exercised
// by contracts published into an environment that never allowed it.
func TestHostCapabilityGrantIsTheAuditedActiveVocabulary(t *testing.T) {
	rows := environmentMembers(t, library.FormHostCapability)
	if len(rows) != 1 || rows[0].Path.Len() != 0 {
		t.Fatal("the environment publishes its host grant anywhere but at its own root")
	}
	granted, err := contract.DecodeHostCapabilities(rows[0].Body)
	if err != nil {
		t.Fatalf("the host grant did not decode: %v", err)
	}
	if len(granted) != len(environmentHostCapabilities) {
		t.Fatalf("the contract grants %d capabilities, want %d", len(granted), len(environmentHostCapabilities))
	}
	held := make(map[string]bool, len(granted))
	for index, id := range granted {
		if index != 0 && granted[index-1] >= id {
			t.Fatalf("the grant is not written in identity order at %q", id)
		}
		descriptor, audited := capability.Lookup(id)
		if !audited {
			t.Fatalf("the environment grants %q, which the vocabulary does not audit", id)
		}
		switch descriptor.Status {
		case capability.StatusReserved, capability.StatusReservedHighRisk:
			t.Fatalf("the environment grants the inactive capability %s (%s)", id, descriptor.Status)
		}
		held[id] = true
	}
	var active []string
	for _, descriptor := range capability.All() {
		switch descriptor.Status {
		case capability.StatusReserved, capability.StatusReservedHighRisk:
			continue
		}
		active = append(active, descriptor.ID)
	}
	sort.Strings(active)
	for _, id := range active {
		if !held[id] {
			t.Fatalf("the vocabulary audits %q as active and the environment grants no such capability", id)
		}
	}
}

// mountedContracts indexes the shipped library instances by the mount selector
// each is chosen by. An entry inside a library aggregate is attributed to its
// owner by the first step of the address the environment publishes the aggregate
// at, so no root name is read to find the contract that owns a member.
func mountedContracts(t *testing.T) map[string]*contract.Instance {
	t.Helper()
	mounted := make(map[string]*contract.Instance, len(libraryCorpus()))
	for _, testCase := range libraryCorpus() {
		instance := testCase.instance(t)
		if _, duplicate := mounted[instance.Root()]; duplicate {
			t.Fatalf("two shipped contracts are selected by the mount %q", instance.Root())
		}
		mounted[instance.Root()] = instance
	}
	return mounted
}

// TestEveryLedgerEntryIsAContractStatement is the absorption law, and the whole
// of what the authored ledger has left to say. Every initial entry is a statement
// some shipped contract now makes:
//
//   - a global the environment binds as a slot;
//   - an entry the environment refuses or never had, published as a denial;
//   - an alias of a root the environment addresses as a boot root;
//   - an operation inside a mounted aggregate, published as a callable by the
//     contract that owns that aggregate;
//   - a constant inside a mounted aggregate, published as an export value by the
//     same contract, carrying the exact value the ledger wrote;
//   - the index entry of the primitive metatable no export path reaches, which
//     the attachment member states in full.
//
// The remainder is zero, and it is computed rather than declared: an entry the
// host adds and no contract describes fails here with the address it was written
// at. This is the last precondition the ledger's retirement had that this surface
// owned; what remains is the fixture constructors that still read the authored
// profile directly.
func TestEveryLedgerEntryIsAContractStatement(t *testing.T) {
	ledger := bootLedger(t)
	keys := ledgerKeys(t, ledger)
	instance := globalsInstance(t)
	mounted := mountedContracts(t)
	globalRoot, globalOK := ledger.GlobalEnvRoot()
	if !globalOK {
		t.Fatal("the ledger declares no global environment root")
	}
	// The address of every root the environment publishes, so an entry can be
	// attributed to the aggregate that holds it without reading a root name.
	addressed := make(map[target.InitialRoot]contract.Path, len(environmentBootRoots))
	for _, member := range environmentMembers(t, library.FormBootRoot) {
		root, ok := resolveLedgerRoot(t, ledger, keys, member.Path)
		if !ok {
			t.Fatalf("the boot root at %v reaches no ledger root", pathSteps(member.Path))
		}
		addressed[root] = member.Path
	}
	_, attachedMetatable, attachedOK := ledger.InitialMetatableAttachmentAt(0)
	if ledger.InitialMetatableAttachmentCount() != 1 || !attachedOK {
		t.Fatal("the ledger attaches other than the one primitive metatable the environment states")
	}
	for index := 0; index < ledger.InitialEntryCount(); index++ {
		root, key, value, _, ok := ledger.InitialEntryAt(index)
		if !ok {
			t.Fatal("the ledger holds an unavailable entry")
		}
		literal, literalOK := ledger.ExactKeyValue(key)
		if !literalOK || literal.Kind != keyspace.LiteralString {
			t.Fatal("the ledger holds an entry under a non-string key")
		}
		kind, kindOK := ledger.InitialValueKind(value)
		if !kindOK {
			t.Fatal("the ledger entry has no value kind")
		}
		owner, addressedOK := addressed[root]
		if !addressedOK {
			// The one root no export path reaches is the attached primitive
			// metatable, and its single entry is the index the attachment law
			// resolves in full. Nothing is restated here: the entry is covered by
			// that member, and this states which root it belongs to.
			if root != attachedMetatable || literal.String != StringMetatableIndexKey {
				t.Fatalf("the ledger writes %q into a root no contract addresses", literal.String)
			}
			continue
		}
		if root == globalRoot {
			if _, bound := instance.Resolve(library.FormEnvironmentSlot, contract.Export(literal.String)); !bound {
				t.Fatalf("the ledger writes the global %q and the environment binds no slot for it", literal.String)
			}
			continue
		}
		address := contract.NewPath(append(pathSteps(owner), contract.Step{Kind: contract.StepExport, Key: literal.String})...)
		if kind == target.InitialValueDeniedOperation || kind == target.InitialValueAbsent {
			if _, denied := instance.Resolve(library.FormDeniedEntry, address); !denied {
				t.Fatalf("the ledger denies %v and the environment publishes no denial for it", pathSteps(address))
			}
			continue
		}
		if kind == target.InitialValueRoot {
			alias, aliasOK := ledger.InitialValueRoot(value)
			if !aliasOK {
				t.Fatal("the ledger alias entry names no root")
			}
			if _, published := addressed[alias]; !published {
				t.Fatalf("the ledger aliases a root at %v that the environment does not address", pathSteps(address))
			}
		}
		// The entry is a member of a mounted aggregate, so the contract that owns
		// the mount is what states it. The mount is the first step of the owner's
		// address and the member is the rest, which is why the whole attribution
		// needs no root name.
		selector, member := mountedMember(t, address)
		owning, mountedOK := mounted[selector]
		if !mountedOK {
			t.Fatalf("the ledger writes %v into an aggregate no shipped contract is mounted at", pathSteps(address))
		}
		switch kind {
		case target.InitialValueOperation:
			if _, published := owning.Resolve(library.FormCallableSignature, member); !published {
				t.Fatalf("the ledger boots the operation %v and its contract publishes no callable for it",
					pathSteps(address))
			}
		case target.InitialValueRoot:
			published, found := owning.Resolve(library.FormExportValue, member)
			if !found {
				t.Fatalf("the ledger boots the aggregate %v and its contract publishes no export value for it",
					pathSteps(address))
			}
			decoded, err := contract.DecodeExportValue(published.Body)
			if err != nil || decoded.Shape != contract.ValueShapeAggregate {
				t.Fatalf("the contract does not publish %v as an aggregate", pathSteps(address))
			}
		default:
			published, found := owning.Resolve(library.FormExportValue, member)
			if !found {
				t.Fatalf("the ledger boots the constant %v and its contract publishes no export value for it",
					pathSteps(address))
			}
			decoded, err := contract.DecodeExportValue(published.Body)
			if err != nil || decoded.Shape != contract.ValueShapeConstant {
				t.Fatalf("the contract does not publish %v as a constant", pathSteps(address))
			}
			if written := ledgerConstant(t, ledger, value); written != decoded.Constant {
				t.Fatalf("the contract publishes %+v at %v and the ledger writes %+v",
					decoded.Constant, pathSteps(address), written)
			}
		}
	}
}

// mountedMember splits one environment address into the mount selector its first
// step names and the member address inside that mount.
func mountedMember(t *testing.T, address contract.Path) (string, contract.Path) {
	t.Helper()
	steps := pathSteps(address)
	if len(steps) < 2 || steps[0].Kind != contract.StepExport {
		t.Fatalf("the address %v names no mounted member", steps)
	}
	return steps[0].Key, contract.NewPath(steps[1:]...)
}

// ledgerConstant reads one ledger value as the constant it is, in the closed
// literal domain the export-value payload publishes.
func ledgerConstant(t *testing.T, ledger *target.Contract, value target.InitialValue) contract.Constant {
	t.Helper()
	kind, ok := ledger.InitialValueKind(value)
	if !ok {
		t.Fatal("the ledger value has no kind")
	}
	switch kind {
	case target.InitialValueString:
		text, textOK := ledger.InitialValueString(value)
		if !textOK {
			t.Fatal("the ledger string value holds no text")
		}
		return contract.Constant{Kind: contract.ConstantString, String: text}
	case target.InitialValueInteger:
		number, numberOK := ledger.InitialValueInteger(value)
		if !numberOK {
			t.Fatal("the ledger integer value holds no number")
		}
		return contract.Constant{Kind: contract.ConstantInteger, Integer: number}
	case target.InitialValueFloat:
		bits, bitsOK := ledger.InitialValueFloatBits(value)
		if !bitsOK {
			t.Fatal("the ledger float value holds no bits")
		}
		return contract.Constant{Kind: contract.ConstantFloat, FloatBits: bits}
	}
	t.Fatalf("the ledger value kind %v is not a published constant", kind)
	return contract.Constant{}
}

// TestEnvironmentContractIsNotAuthoredAgainstALibraryKind keeps the boundary the
// surface states over the rows this lane adds. The boot roots, the attachment set
// and the host grant are forms only the environment class declares, so a library
// kind cannot admit this instance at all.
func TestEnvironmentContractIsNotAuthoredAgainstALibraryKind(t *testing.T) {
	if _, ok := GlobalsContract(declaredKind(t, composite.LibraryContractKind)); ok {
		t.Fatal("the environment was authored as a library contract")
	}
	for _, form := range []library.Form{
		library.FormBootRoot, library.FormEnvironmentSlot,
		library.FormPrimitiveMetatable, library.FormHostCapability,
	} {
		if !form.Environment() {
			t.Fatalf("form %d is not an environment form", form)
		}
		if len(environmentMembers(t, form)) == 0 {
			t.Fatalf("the environment publishes no member of form %d", form)
		}
	}
}
