package lualib

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/domain/type/stringlib"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

func stringInstance(t *testing.T) *contract.Instance {
	t.Helper()
	instance, ok := StringContract(declaredKind(t, composite.LibraryContractKind))
	if !ok {
		t.Fatal("the string library contract was rejected by the declared library kind")
	}
	return instance
}

// TestStringMethodTableIsDerivedFromTheAuthoredStringContract is the drift law
// against the string library's own model, in the direction the retirement
// establishes. The shared inventory law derives the standard-library signature
// table from the authored contracts; this derives the table that models the
// string METHODS from the same contract, so the two derived sources cannot
// diverge from the one statement without a verdict.
func TestStringMethodTableIsDerivedFromTheAuthoredStringContract(t *testing.T) {
	instance := stringInstance(t)
	names := stringlib.Names()
	if len(names) != len(stringExports) {
		t.Fatalf("the contract exports %d members and the method table holds %d", len(stringExports), len(names))
	}
	for index, name := range stringExports {
		if names[index] != name {
			t.Fatalf("authored export %d is %q and the method table holds %q", index, name, names[index])
		}
		if _, found := instance.Resolve(library.FormCallableSignature, contract.Export(name)); !found {
			t.Fatalf("the contract publishes no callable at the address of %q", name)
		}
	}
	for _, name := range stringPatternDelegations {
		if _, modeled := stringlib.Method(name); !modeled {
			t.Fatalf("the contract delegates result selection for %q, which the string library does not export", name)
		}
		if _, found := instance.Resolve(library.FormRuleDelegation, contract.Export(name)); !found {
			t.Fatalf("the contract publishes no rule delegation at the address of %q", name)
		}
	}
}

// TestStringMethodsResolveThroughTheMetatableEdge is the governing law of the
// string library and the reason the method-provider name path can die.
// `s:upper()` reaches string.upper because the string metatable's __index edge
// resolves to the contract root and upper is an export of that root - not
// because a receiver type was turned back into the name "string.upper".
func TestStringMethodsResolveThroughTheMetatableEdge(t *testing.T) {
	instance := stringInstance(t)
	edge, found := instance.Resolve(library.FormMetatableEdge, contract.Metatable(StringMetatableIndexKey))
	if !found {
		t.Fatal("the string library publishes no metatable edge")
	}
	if edge.Encoding != contract.EncodingResolved {
		t.Fatal("the metatable edge payload is deferred, so the edge names no target")
	}
	target, err := contract.DecodePath(edge.Body)
	if err != nil {
		t.Fatalf("the metatable edge payload did not decode: %v", err)
	}
	if !target.Equal(contract.Root()) {
		t.Fatal("the metatable edge resolves somewhere other than the contract root")
	}
	// Every export the edge makes reachable is an export of the value the edge
	// resolves to, so the colon form and the dotted form are one member.
	for _, name := range stringExports {
		reached := contract.NewPath(
			contract.Step{Kind: contract.StepMetatable, Key: StringMetatableIndexKey},
			contract.Step{Kind: contract.StepExport, Key: name},
		)
		if reached.Len() != 2 {
			t.Fatal("the reached path is not the edge followed by an export")
		}
		if _, found := instance.Resolve(library.FormCallableSignature, contract.Export(name)); !found {
			t.Fatalf("the member %q reached through the edge is not an export of the edge target", name)
		}
	}
}

// TestStringMethodSignaturesAreDerivedFromTheAuthoredEnvelopes derives what the
// string method table must hold for each member from the envelope the authored
// contract publishes at that member's address, so the method table cannot
// describe an application the contract does not state.
func TestStringMethodSignaturesAreDerivedFromTheAuthoredEnvelopes(t *testing.T) {
	instance := stringInstance(t)
	for _, name := range stringExports {
		member, found := instance.Resolve(library.FormCallableSignature, contract.Export(name))
		if !found {
			t.Fatalf("the contract publishes no callable at the address of %q", name)
		}
		expected, err := wire.DecodeCallableSignature(member.Body)
		if err != nil {
			t.Fatalf("the callable envelope of %q did not decode: %v", name, err)
		}
		method, ok := stringlib.Method(name)
		if !ok {
			t.Fatalf("the method table holds no signature for the authored export %q", name)
		}
		if !(signature.Function{Type: method}).Equals(expected) {
			t.Fatalf("the method table applies %q as %s, and the authored contract states %s",
				name, signature.Function{Type: method}, expected)
		}
	}
}

// TestStringByteResultIsRefinedByItsSubjectLength is the content law of the
// refinement form. string.byte publishes an optional result that is optional
// only because the read position may lie past its subject's end, and the
// contract carries that predicate rather than leaving a caller who proved
// otherwise with an optionality nothing can discharge.
func TestStringByteResultIsRefinedByItsSubjectLength(t *testing.T) {
	member, found := stringInstance(t).Resolve(library.FormResultRefinement, contract.Export("byte"))
	if !found {
		t.Fatal("the string library publishes no result refinement for byte")
	}
	if member.Encoding != contract.EncodingResolved {
		t.Fatal("the byte refinement is deferred, so it states no predicate")
	}
	decoded, err := wire.DecodeResultRefinement(member.Body)
	if err != nil {
		t.Fatalf("the byte refinement did not decode: %v", err)
	}
	modeled := signaturelookup.StdlibPositionalResultSlots(StringRoot + ".byte")
	if len(modeled) != 1 {
		t.Fatalf("the standard library models %d positional refinements for byte, want 1", len(modeled))
	}
	want := wire.SubjectLengthRefinement{
		Result:   modeled[0].ResultIndex,
		Subject:  modeled[0].SubjectArgument,
		Position: modeled[0].PositionArgument,
		Default:  modeled[0].DefaultPosition,
	}
	if !wire.ResultRefinementEquals(decoded, want) {
		t.Fatal("the published refinement is not the one the standard library models")
	}
	// The refinement is stated about one member and only that member: a
	// contract that refined every export would be refining values it has nothing
	// to say about.
	for _, name := range stringExports {
		if name == "byte" {
			continue
		}
		if _, found := stringInstance(t).Resolve(library.FormResultRefinement, contract.Export(name)); found {
			t.Fatalf("the contract refines %q, which the standard library does not refine", name)
		}
	}
}

// TestStringDumpIsDeclaredAndRefused is the denial law of this library and the
// content law of the class-agnostic denied-entry form. string.dump is a member
// the string library HAS: the contract describes its application and states
// that it is refused, so the refusal is one statement owned by the contract
// that owns the member. What a denial has to say is the member it refuses and
// which refusal it is: a library withholds a member it models, and never claims
// a member is absent, because whether a member is there at all is what the host
// that boots the environment decided.
func TestStringDumpIsDeclaredAndRefused(t *testing.T) {
	instance := stringInstance(t)
	member, found := instance.Resolve(library.FormDeniedEntry, contract.Export("dump"))
	if !found {
		t.Fatal("the string library publishes no denial for dump")
	}
	if member.Encoding != contract.EncodingResolved {
		t.Fatal("the dump denial is deferred, so it refuses nothing")
	}
	denied, err := contract.DecodeDeniedEntry(member.Body)
	if err != nil {
		t.Fatalf("the dump denial did not decode: %v", err)
	}
	if !denied.Entry.Equal(contract.Export("dump")) {
		t.Fatal("the dump denial states an address other than the member it refuses")
	}
	if denied.Denial != contract.DenialRefused {
		t.Fatal("the string library states dump as absent rather than refused")
	}
	// A denial is a statement about a declared member, not the absence of one.
	if _, declared := instance.Resolve(library.FormCallableSignature, contract.Export("dump")); !declared {
		t.Fatal("the contract refuses a member it does not declare")
	}
	// The derived direction: the modeled string library carries the refusal as
	// a result no caller can reach, and it carries it for exactly the member the
	// contract refuses.
	for _, name := range stringExports {
		method, modeled := stringlib.Method(name)
		if !modeled {
			t.Fatalf("the method table holds no signature for the authored export %q", name)
		}
		var unreachable bool
		for _, result := range method.Returns {
			if typ.IsNever(result) {
				unreachable = true
			}
		}
		_, refused := instance.Resolve(library.FormDeniedEntry, contract.Export(name))
		if unreachable != refused {
			t.Fatalf("the modeled %q returns an unreachable result=%v and the contract refuses it=%v",
				name, unreachable, refused)
		}
	}
}

// TestStringPatternDelegationsAreDeferredForWantOfARule is the deferral receipt.
// The rule-delegation format landed and can name any entry the rule surface
// declares - which this law proves against the sealed surface itself - and the
// declaration table declares no rule that owns pattern-capture result selection,
// so the three pattern members carry their address and say so. Publishing a
// delegation to an entry that does not exist would be an identity no reader
// could resolve.
func TestStringPatternDelegationsAreDeferredForWantOfARule(t *testing.T) {
	instance := stringInstance(t)
	for _, name := range stringPatternDelegations {
		member, found := instance.Resolve(library.FormRuleDelegation, contract.Export(name))
		if !found {
			t.Fatalf("the contract publishes no rule delegation at the address of %q", name)
		}
		if member.Encoding != contract.EncodingDeferred || len(member.Body) != 0 {
			t.Fatalf("the delegation of %q names a rule, so the deferral it states is stale", name)
		}
	}
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration root did not seal: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	rules, rulesOK := sealed.Surface(schema.SurfaceKindRule)
	if !rulesOK || rules.Count() == 0 {
		t.Fatal("the sealed table declares no rule surface for a delegation to name")
	}
	for position := 0; position < rules.Count(); position++ {
		entry, entryOK := rules.At(position)
		if !entryOK || entry == nil {
			t.Fatalf("the rule surface holds no entry at position %d", position)
		}
		body, err := contract.EncodeRuleDelegation(entry.Key())
		if err != nil {
			t.Fatalf("the delegation format cannot name the sealed rule %q: %v", entry.Key(), err)
		}
		named, err := contract.DecodeRuleDelegation(body)
		if err != nil {
			t.Fatalf("the delegation naming %q did not decode: %v", entry.Key(), err)
		}
		if _, declared := rules.ByID(contract.RuleDelegationEntryID(named)); !declared {
			t.Fatalf("a delegation naming %q resolves to no declared rule", named)
		}
	}
}
