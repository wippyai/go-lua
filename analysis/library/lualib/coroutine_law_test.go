package lualib

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/library/contract"
	profile "github.com/wippyai/go-lua/analysis/library/lualib/targetprofile"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// The coroutine contract is the one shipped library with a suspending member.
// These laws state what its suspension row says, that both ends of the relation
// name members of the sealed outcome vocabulary rather than ordinals this
// package minted, and - while the target operation catalogue still states the
// same relation - that the two agree, in the direction the retirement
// establishes: the contract is the statement and the catalogue is derived from
// it and held to it.

// sealedOutcomes projects the sealed structural vocabulary. An outcome
// reference is resolved against the declaration root, never against a
// restatement of it.
func sealedOutcomes(t *testing.T) structure.Table {
	t.Helper()
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration root did not seal: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("the sealed table holds no structural vocabulary surface")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("the sealed structural vocabulary did not project")
	}
	return table
}

// declaredOutcome resolves the sealed outcome member one control-outcome kind
// is. The kind's ordinal IS the vocabulary ordinal, so the key is read out of
// the declaration table rather than spelled a second time here.
func declaredOutcome(t *testing.T, table structure.Table, kind flowkind.OutcomeKind) schema.Key {
	t.Helper()
	entry, ok := table.At(structure.CategoryOutcome, uint16(kind))
	if !ok || entry == nil {
		t.Fatalf("the outcome vocabulary declares no member at ordinal %d", kind)
	}
	return entry.Key()
}

// coroutineSuspension resolves the one suspension row the shipped coroutine
// contract publishes.
func coroutineSuspension(t *testing.T, export string) contract.Suspension {
	t.Helper()
	instance, ok := CoroutineContract(declaredKind(t, composite.LibraryContractKind))
	if !ok {
		t.Fatal("the coroutine library contract was rejected by the declared library kind")
	}
	member, found := instance.Resolve(library.FormSuspension, contract.Export(export))
	if !found {
		t.Fatalf("the contract publishes no suspension at the address of %q", export)
	}
	if member.Encoding != contract.EncodingResolved {
		t.Fatalf("the suspension of %q is deferred", export)
	}
	row, err := contract.DecodeSuspension(member.Body)
	if err != nil {
		t.Fatalf("the suspension of %q did not decode: %v", export, err)
	}
	return row
}

// TestCoroutineSuspensionNamesSealedOutcomeMembers is the reference law. Both
// ends of the relation are keys of the declared outcome vocabulary, resolvable
// in the sealed table under the identity the payload derives, so a reader
// interprets the relation through the declaration and not through an ordinal
// this contract invented.
func TestCoroutineSuspensionNamesSealedOutcomeMembers(t *testing.T) {
	table := sealedOutcomes(t)
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration root did not seal: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, _ := sealed.Surface(schema.SurfaceKindStructure)
	// Both suspending members leave control at the yield outcome and re-enter at
	// the normal one, and they differ in the authority that restores control: a
	// resuming call for yield, the activation's own provider for a detached
	// spawn. The authority is the content of the row, so it is stated per member.
	for _, testCase := range []struct {
		export string
		source contract.ReentrySource
	}{
		{"yield", contract.ReentryByCall},
		{"spawn", contract.ReentryByProvider},
	} {
		t.Run(testCase.export, func(t *testing.T) {
			suspension := coroutineSuspension(t, testCase.export)
			for _, outcome := range []schema.Key{suspension.Yield, suspension.Reentry} {
				row, declared := view.ByID(contract.SuspensionOutcomeEntryID(outcome))
				if !declared {
					t.Fatalf("the suspension names the outcome %q, which no sealed row declares", outcome)
				}
				entry, entryOK := row.(*structure.Entry)
				if !entryOK || entry.Category() != structure.CategoryOutcome {
					t.Fatalf("the suspension names %q, which is not a member of the outcome vocabulary", outcome)
				}
				if member, ok := table.At(structure.CategoryOutcome, entry.Ordinal()); !ok || member.Key() != outcome {
					t.Fatalf("the outcome %q is not the member the projection holds at its own ordinal", outcome)
				}
			}
			if suspension.Yield != declaredOutcome(t, table, flowkind.OutcomeYield) {
				t.Fatalf("control leaves at %q, want the declared yield outcome", suspension.Yield)
			}
			if suspension.Reentry != declaredOutcome(t, table, flowkind.OutcomeNormal) {
				t.Fatalf("control re-enters at %q, want the declared normal outcome", suspension.Reentry)
			}
			if suspension.Source != testCase.source || suspension.Multiplicity != contract.ReentryOnce {
				t.Fatalf("the re-entry policy is %+v, want a one-shot restoration by %d",
					suspension, testCase.source)
			}
		})
	}
}

// TestOnlyTheSuspendingCoroutineExportPublishesASuspension keeps the member set
// exact. A suspension is a statement about the member it addresses, so an export
// that does not suspend must carry no such row.
func TestOnlyTheSuspendingCoroutineExportPublishesASuspension(t *testing.T) {
	instance, ok := CoroutineContract(declaredKind(t, composite.LibraryContractKind))
	if !ok {
		t.Fatal("the coroutine library contract was rejected by the declared library kind")
	}
	suspending := map[string]bool{"yield": true, "spawn": true}
	for _, name := range CoroutineExports() {
		_, found := instance.Resolve(library.FormSuspension, contract.Export(name))
		if found != suspending[name] {
			t.Fatalf("the export %q publishes a suspension it does not have, or lacks the one it does", name)
		}
	}
	for _, testCase := range libraryCorpus() {
		if testCase.name == "coroutine" {
			continue
		}
		for _, member := range testCase.instance(t).Members() {
			if member.Form == library.FormSuspension {
				t.Fatalf("the %s library publishes a suspension and has no suspending member", testCase.name)
			}
		}
	}
}

// TestModeledOperationSuspensionIsDerivedFromTheAuthoredContract is the drift
// law, and it runs in the direction the retirement establishes. The authored
// contract states the relation; the target operation catalogue that still states
// it too is derived from the contract here and held to it. The catalogue relates
// two positions of one operation's outcome list, so the derivation resolves each
// position to the outcome KIND it carries and then to the sealed vocabulary
// member that kind is - a reference the catalogue can be checked against without
// either side minting a vocabulary.
func TestModeledOperationSuspensionIsDerivedFromTheAuthoredContract(t *testing.T) {
	catalogue, err := profile.Contract()
	if err != nil {
		t.Fatalf("the target operation catalogue did not seal: %v", err)
	}
	table := sealedOutcomes(t)
	for _, export := range []string{"yield", "spawn"} {
		t.Run(export, func(t *testing.T) {
			authored := coroutineSuspension(t, export)
			op, found := catalogue.Lookup(target.BindingSpec{
				Namespace: target.BindingModule, Owner: []string{CoroutineRoot}, Member: []string{export},
			})
			if !found {
				t.Fatalf("the catalogue models no coroutine.%s operation", export)
			}
			if got := catalogue.SuspensionCount(op); got != 1 {
				t.Fatalf("the modeled operation states %d suspensions and the contract publishes 1", got)
			}
			yield, reentry, source, multiplicity, ok := catalogue.SuspensionAt(op, 0)
			if !ok {
				t.Fatal("the modeled suspension did not resolve")
			}
			for _, end := range []struct {
				role     string
				position uint32
				named    schema.Key
			}{
				{"yield", yield, authored.Yield},
				{"reentry", reentry, authored.Reentry},
			} {
				kind, _, kindOK := catalogue.OutcomeAt(op, int(end.position))
				if !kindOK {
					t.Fatalf("the modeled operation has no outcome at the %s position %d", end.role, end.position)
				}
				// The reference is a kind and the catalogue's is a position, so
				// the derivation is lossless only while that kind occurs once in
				// the operation. A second outcome of the same kind would make the
				// reference name two cases, and the contract would be stating
				// less than the catalogue does.
				var occurrences int
				for index := 0; index < catalogue.OutcomeCount(op); index++ {
					if at, _, atOK := catalogue.OutcomeAt(op, index); atOK && at == kind {
						occurrences++
					}
				}
				if occurrences != 1 {
					t.Fatalf("the %s outcome kind %d occurs %d times in the modeled operation, so naming it is not the position",
						end.role, kind, occurrences)
				}
				if declared := declaredOutcome(t, table, kind); declared != end.named {
					t.Fatalf("the modeled %s outcome is the declared member %q and the contract names %q",
						end.role, declared, end.named)
				}
			}
			if (authored.Source == contract.ReentryByCall) != (source == target.ReentryByCall) ||
				(authored.Source == contract.ReentryByProvider) != (source == target.ReentryByProvider) {
				t.Fatalf("the modeled re-entry source is %d and the contract publishes %d", source, authored.Source)
			}
			if (authored.Multiplicity == contract.ReentryOnce) != (multiplicity == target.ReentryOnce) ||
				(authored.Multiplicity == contract.ReentryMany) != (multiplicity == target.ReentryMany) {
				t.Fatalf("the modeled re-entry multiplicity is %d and the contract publishes %d",
					multiplicity, authored.Multiplicity)
			}
		})
	}
}
