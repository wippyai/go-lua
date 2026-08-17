package lualib

import (
	"encoding/hex"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

func globalsInstance(t *testing.T) *contract.Instance {
	t.Helper()
	instance, ok := GlobalsContract(declaredKind(t, composite.EnvironmentContractKind))
	if !ok {
		t.Fatal("the globals contract was rejected by the declared environment kind")
	}
	return instance
}

// modeledBareGlobals returns the modeled standard-library names that are not
// members of any namespace: the signatures a call reaches through a bare global
// name alone.
func modeledBareGlobals() []string {
	var names []string
	for _, name := range signaturelookup.StdlibSignatureNames() {
		if strings.Contains(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// TestGlobalsContractIsAdmittedByTheDeclaredEnvironmentKind is the base case. The
// globals are slots of the initial environment, so the kind that admits them is
// the environment kind and no other.
func TestGlobalsContractIsAdmittedByTheDeclaredEnvironmentKind(t *testing.T) {
	kind := declaredKind(t, composite.EnvironmentContractKind)
	instance := globalsInstance(t)
	if instance.Kind() != kind.Key() || instance.Codec() != kind.Codec() {
		t.Fatal("the instance is not published under the kind it was admitted against")
	}
	if instance.Class() != library.ClassEnvironment {
		t.Fatal("the globals are not published as an environment contract")
	}
	if instance.Root() != GlobalsRoot {
		t.Fatalf("mount selector is %q, want %q", instance.Root(), GlobalsRoot)
	}
	want := len(environmentBootRoots) + len(globalSlots) + len(globalCallables) +
		len(globalRefinements) + len(globalIntrinsics) + len(environmentDenials) + 2
	if instance.Count() != want {
		t.Fatalf("rows=%d want %d", instance.Count(), want)
	}
}

// TestGlobalsContractIsNotALibrary keeps the boundary the surface states. Binding
// a name to a value is the environment's own authority, so an individual library
// kind cannot admit these members at all.
func TestGlobalsContractIsNotALibrary(t *testing.T) {
	if _, ok := GlobalsContract(declaredKind(t, composite.LibraryContractKind)); ok {
		t.Fatal("the globals were authored as a library contract")
	}
	if _, ok := GlobalsContract(nil); ok {
		t.Fatal("the globals were authored against no kind at all")
	}
	var slots int
	for _, member := range globalsInstance(t).Members() {
		if member.Form == library.FormEnvironmentSlot {
			slots++
		}
	}
	if slots != len(globalSlots) {
		t.Fatalf("the contract binds %d environment slots, want %d", slots, len(globalSlots))
	}
}

// TestModeledBareGlobalsAreDerivedFromTheAuthoredSlots is the drift law of the
// slot inventory, in the direction the retirement establishes: the authored slot
// list is the statement of which names the environment always has, and the
// modeled bare-global list is derived from it and held to it.
//
// The environment has two slots the modeled table does not name, and they are
// enumerated rather than tolerated as a count mismatch. `errors` and `bit32` are
// aggregates the initial environment boots - the ledger binds both - and the
// modeled table lists only the bare globals it recognizes by name, which is the
// gap the retirement closes in that table and not a slot this contract invented.
func TestModeledBareGlobalsAreDerivedFromTheAuthoredSlots(t *testing.T) {
	unmodeledSlots := map[string]bool{"bit32": true, "errors": true}
	var derived []string
	for _, name := range globalSlots {
		if !unmodeledSlots[name] {
			derived = append(derived, name)
		}
	}
	modeled := signaturelookup.StdlibBareGlobals()
	if len(modeled) != len(derived) {
		t.Fatalf("the contract binds %d modeled slots and the modeled table declares %d", len(derived), len(modeled))
	}
	for index, name := range derived {
		if modeled[index] != name {
			t.Fatalf("authored slot %d is %q and the modeled table declares %q", index, name, modeled[index])
		}
	}
	bound := make(map[string]bool, len(globalSlots))
	for _, name := range globalSlots {
		bound[name] = true
	}
	for name := range unmodeledSlots {
		if !bound[name] {
			t.Fatalf("the slot %q is named as unmodeled and the environment does not have it", name)
		}
		for _, declared := range modeled {
			if declared == name {
				t.Fatalf("the modeled table declares %q, so naming it unmodeled states nothing", name)
			}
		}
	}
	instance := globalsInstance(t)
	for _, name := range globalSlots {
		member, found := instance.Resolve(library.FormEnvironmentSlot, contract.Export(name))
		if !found {
			t.Fatalf("the contract binds no slot at the address of %q", name)
		}
		if member.Encoding != contract.EncodingResolved {
			t.Fatalf("the slot %q binds nothing, so the environment does not say what it holds", name)
		}
	}
	if _, found := instance.Resolve(library.FormEnvironmentSlot, contract.Export("absent")); found {
		t.Fatal("the contract binds a slot the environment does not have")
	}
}

// TestModeledBareSignaturesAreDerivedFromTheAuthoredEnvelopes is the content law
// of the callable form on the environment side, in the same flipped direction:
// what the modeled table must hold for a global is derived from the envelope
// this contract publishes at that slot, effect row included.
func TestModeledBareSignaturesAreDerivedFromTheAuthoredEnvelopes(t *testing.T) {
	source := signaturelookup.Source{IncludeStdlib: true}
	instance := globalsInstance(t)
	bound := make(map[string]bool, len(globalSlots))
	for _, name := range globalSlots {
		bound[name] = true
	}
	if len(globalsSignatures) != len(globalCallables) {
		t.Fatalf("the contract authors %d signatures for %d callable slots",
			len(globalsSignatures), len(globalCallables))
	}
	for _, name := range globalCallables {
		if !bound[name] {
			t.Fatalf("the contract publishes a callable at %q, which is no slot of the environment", name)
		}
		if sig, authored := globalsSignatures[name]; !authored || sig.Type == nil {
			t.Fatalf("the callable slot %q has no authored signature", name)
		}
		member, found := instance.Resolve(library.FormCallableSignature, contract.Export(name))
		if !found {
			t.Fatalf("the contract publishes no callable at the address of %q", name)
		}
		if member.Encoding != contract.EncodingResolved {
			t.Fatalf("the callable envelope of %q is deferred", name)
		}
		expected, err := wire.DecodeCallableSignature(member.Body)
		if err != nil {
			t.Fatalf("the callable envelope of %q did not decode: %v", name, err)
		}
		if !expected.Equals(globalsSignatures[name]) {
			t.Fatalf("the published envelope of %q is not the signature the instance authors", name)
		}
		modeled, ok := source.LookupView(name)
		if !ok {
			t.Fatalf("the modeled table holds no signature for the authored slot %q", name)
		}
		if !modeled.Equals(expected) {
			t.Fatalf("the modeled table applies %q as %s, and the authored contract states %s",
				name, modeled, expected)
		}
	}
}

// TestGlobalsAbsorbEveryModeledBareSignature is the remainder law. A modeled bare
// signature this contract does not publish is knowledge absorbed by nobody, so
// each one is named here with the reason it is not an environment callable. The
// three that remain are host coercion globals rather than Lua library members,
// and `string` additionally collides with the slot that holds the string library
// aggregate: one name, two values, which value addressing exists to separate.
func TestGlobalsAbsorbEveryModeledBareSignature(t *testing.T) {
	published := make(map[string]bool, len(globalCallables))
	for _, name := range globalCallables {
		published[name] = true
	}
	hostCoercions := map[string]bool{"integer": true, "number": true, "string": true}
	for _, name := range modeledBareGlobals() {
		if published[name] == hostCoercions[name] {
			t.Fatalf("the modeled global %q is published by no contract member and named as no host coercion", name)
		}
	}
	for name := range hostCoercions {
		if _, modeled := (signaturelookup.Source{IncludeStdlib: true}).LookupView(name); !modeled {
			t.Fatalf("the host coercion %q is no longer modeled, so naming it here states nothing", name)
		}
	}
}

// TestGlobalSelectResultIsRefinedByItsLiteralArgument is the content law of the
// refinement form on the environment side. select("#", ...) answers with a count
// and select(n, ...) with a member of the tail: the predicate is a literal the
// caller wrote, so the contract carries both halves of the relation.
func TestGlobalSelectResultIsRefinedByItsLiteralArgument(t *testing.T) {
	member, found := globalsInstance(t).Resolve(library.FormResultRefinement, contract.Export("select"))
	if !found {
		t.Fatal("the environment publishes no result refinement for select")
	}
	if member.Encoding != contract.EncodingResolved {
		t.Fatal("the select refinement is deferred, so it states no predicate")
	}
	decoded, err := wire.DecodeResultRefinement(member.Body)
	if err != nil {
		t.Fatalf("the select refinement did not decode: %v", err)
	}
	modeled := signaturelookup.StdlibConditionalResultSlots("select")
	if len(modeled) != 1 {
		t.Fatalf("the standard library models %d conditional refinements for select, want 1", len(modeled))
	}
	want := wire.LiteralArgumentRefinement{
		Result:   modeled[0].ResultIndex,
		Argument: modeled[0].ArgumentIndex,
		Literal:  modeled[0].ArgumentString,
		Type:     modeled[0].ResultType,
	}
	if !wire.ResultRefinementEquals(decoded, want) {
		t.Fatal("the published refinement is not the one the standard library models")
	}
}

// TestAssertRefinementIsCarriedByItsEnvelopeAlone is the one-fact-one-statement
// law. assert refines its first argument when the call returns normally, which
// is a postcondition of the application and therefore a label of the callable's
// own effect row: it rides the envelope, over the one type codec and the one
// audited capability vocabulary. Publishing it again as a result-refinement
// member would be one fact under two authorities with nothing to keep them
// agreeing, so the refinement form carries only what an effect label cannot say -
// a predicate over caller DATA.
func TestAssertRefinementIsCarriedByItsEnvelopeAlone(t *testing.T) {
	instance := globalsInstance(t)
	member, found := instance.Resolve(library.FormCallableSignature, contract.Export("assert"))
	if !found {
		t.Fatal("the environment publishes no callable at the address of assert")
	}
	envelope, err := wire.DecodeCallableSignature(member.Body)
	if err != nil {
		t.Fatalf("the assert envelope did not decode: %v", err)
	}
	want := postcondition.NormalReturnRefinement{
		Target:     effect.ParamRef{Index: 0},
		Refinement: postcondition.Present{},
	}
	var carried bool
	for _, label := range envelope.Effect.Labels {
		if want.Equals(label) {
			carried = true
		}
	}
	if !carried {
		t.Fatal("the assert envelope carries no normal-return refinement, so the fact is stated nowhere")
	}
	if _, published := instance.Resolve(library.FormResultRefinement, contract.Export("assert")); published {
		t.Fatal("the assert refinement is published a second time as a result-refinement member")
	}
}

// TestGlobalTypeCarriesItsIntrinsicIdentity is the content law of the marker
// form. type(v) answers with the runtime family of a caller value, which its
// signature cannot state, so the contract publishes the sealed identity of the
// operation at the value that performs it - and a consumer reads it from there
// instead of recognizing the callee's name.
func TestGlobalTypeCarriesItsIntrinsicIdentity(t *testing.T) {
	instance := globalsInstance(t)
	member, found := instance.Resolve(library.FormIntrinsicMarker, contract.Export("type"))
	if !found {
		t.Fatal("the environment publishes no intrinsic marker for type")
	}
	if member.Encoding != contract.EncodingResolved {
		t.Fatal("the type marker is deferred, so it names no operation")
	}
	decoded, err := wire.DecodeIntrinsicMarker(member.Body)
	if err != nil {
		t.Fatalf("the type marker did not decode: %v", err)
	}
	if decoded != signature.IntrinsicLuaType {
		t.Fatalf("the marker names intrinsic %d, want %d", decoded, signature.IntrinsicLuaType)
	}
	for _, name := range globalSlots {
		if name == "type" {
			continue
		}
		if _, found := instance.Resolve(library.FormIntrinsicMarker, contract.Export(name)); found {
			t.Fatalf("the contract marks %q as an intrinsic the vocabulary does not seal for it", name)
		}
	}
}

// TestGlobalsContractSerializesEverythingItStates is the honesty receipt. Every
// form the environment publishes now has a landed payload format - the slot
// binding included - so no member carries an address alone. A deferred row here
// would be a fact the environment claims to hold and cannot write down.
func TestGlobalsContractSerializesEverythingItStates(t *testing.T) {
	instance := globalsInstance(t)
	if instance.Deferred() != 0 {
		t.Fatalf("deferred rows=%d want 0", instance.Deferred())
	}
	for _, member := range instance.Members() {
		if member.Encoding != contract.EncodingResolved || len(member.Body) == 0 {
			t.Fatalf("form %d carries no payload at a resolved encoding", member.Form)
		}
	}
}

// TestGlobalsContractWireIsPinned holds the shipped contract's serialized bytes
// still. The instance is a data artifact: a slot added, moved or readdressed is
// a different environment, and this is where that shows.
func TestGlobalsContractWireIsPinned(t *testing.T) {
	const pinned = "3b8ecf082c990268523f9d6a5541afcedf12d0453c4e54d166e4e77e0485ce32"
	const pinnedSize = 28972
	instance := globalsInstance(t)
	data, err := contract.Encode(instance)
	if err != nil {
		t.Fatalf("the globals contract did not encode: %v", err)
	}
	if len(data) != pinnedSize {
		t.Errorf("contract wire is %d bytes, pinned %d", len(data), pinnedSize)
	}
	id := contract.ContentID(instance)
	if got := hex.EncodeToString(id[:]); got != pinned {
		t.Errorf("contract identity is %s, pinned %s", got, pinned)
	}
	decoded, err := contract.Decode(data, declaredTable(t))
	if err != nil {
		t.Fatalf("the globals contract did not decode: %v", err)
	}
	if contract.ContentID(decoded) != id {
		t.Fatal("the decoded contract is not the contract that was written")
	}
}
