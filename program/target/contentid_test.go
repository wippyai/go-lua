package target

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestContentIDIsCanonicalAndOwned(t *testing.T) {
	left := mustSeal(t, Spec{Operations: []OperationSpec{
		contentIDOperation("alpha", []OutcomeSpec{
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Boolean}, Tail: ValuesClosed}},
		}),
		contentIDOperation("beta", []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}),
	}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{
		contentIDOperation("beta", []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}),
		contentIDOperation("alpha", []OutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Boolean}, Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}},
		}),
	}})
	leftID, rightID := left.ContentID(), right.ContentID()
	if !leftID.Available() || leftID != rightID || leftID != left.ContentID() {
		t.Fatalf("canonical ContentID = %v/%v", leftID, rightID)
	}

	changed := mustSeal(t, Spec{Operations: []OperationSpec{
		contentIDOperation("alpha", []OutcomeSpec{
			{Kind: flowkind.OutcomeThrow, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}},
			{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed}},
		}),
		contentIDOperation("beta", []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Tail: ValuesClosed}}}),
	}})
	if leftID == changed.ContentID() {
		t.Fatal("outcome semantic change did not change ContentID")
	}
}

func TestContentIDTypeOccurrenceAllocationsAreConstant(t *testing.T) {
	seal := func(width int) *Contract {
		values := make([]typ.Type, width)
		for index := range values {
			values[index] = typ.String
		}
		return mustSeal(t, Spec{Operations: []OperationSpec{{
			Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{"allocation"}}},
			Input:    ValuesSpec{Fixed: values, Tail: ValuesClosed},
			Outcomes: []OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: values, Tail: ValuesClosed}}},
			Effects:  RowSpec{Tail: RowClosed},
		}}})
	}
	small, wide := seal(1), seal(4096)
	smallAllocs := testing.AllocsPerRun(100, func() { _ = small.ContentID() })
	wideAllocs := testing.AllocsPerRun(100, func() { _ = wide.ContentID() })
	if wideAllocs != smallAllocs {
		t.Fatalf("ContentID allocations scale with repeated frozen type occurrences: small=%f wide=%f", smallAllocs, wideAllocs)
	}
}

func TestContentIDIncludesDerivedOpaqueSemanticsAndFailsClosed(t *testing.T) {
	empty := mustSeal(t, Spec{})
	if !empty.ContentID().Available() {
		t.Fatal("opaque-only contract has no ContentID")
	}
	if (&Contract{}).ContentID().Available() || (*Contract)(nil).ContentID().Available() {
		t.Fatal("unavailable contract produced a ContentID")
	}

	withProtocol := mustSeal(t, Spec{Operations: []OperationSpec{contentIDOperation("acquire", []OutcomeSpec{{
		Kind: flowkind.OutcomeNormal, Values: ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: ValuesClosed},
	}})}, Protocols: []ProtocolSpec{{
		Acquisitions: []AcquisitionSpec{{Operation: 1, Outcome: 0, Result: 0, State: 1}},
		States:       []StateSpec{{Name: "open"}},
	}}})
	if empty.ContentID() == withProtocol.ContentID() {
		t.Fatal("protocol and its derived opaque escape were omitted from ContentID")
	}
}

func TestContentIDTypeFormalAlphaInvariant(t *testing.T) {
	leftFormal := typ.NewTypeParam("T", typ.String)
	rightFormal := typ.NewTypeParam("Renamed", typ.String)
	left := mustSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("identity", leftFormal)}})
	right := mustSeal(t, Spec{Operations: []OperationSpec{genericBuiltin("identity", rightFormal)}})
	if left.ContentID() != right.ContentID() {
		t.Fatal("alpha-equivalent type formals changed ContentID")
	}
}

// The current Target layout cannot share an identity with its immediately
// preceding namespace, even if every surviving observable row encodes alike.
func TestContentIDNamespaceSeparatesPriorContractIdentity(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{spawnTestOperation("spawn")}})
	current := contract.ContentID()
	if !current.Available() {
		t.Fatal("current contract has no ContentID")
	}
	priorHash := sha256.New()
	var writer canonical.Writer
	if err := writer.Reset(priorHash, "program/target-contract", contentIDCodecVersion-1); err != nil {
		t.Fatal(err)
	}
	if err := encodeContract(&writer, contract); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	var prior keyspace.ContentID
	if sum := priorHash.Sum(prior[:0]); len(sum) != len(prior) {
		t.Fatal("prior target digest has wrong width")
	}
	if current == prior {
		t.Fatal("target schema reused a prior-layout ContentID")
	}
}

func contentIDOperation(name string, outcomes []OutcomeSpec) OperationSpec {
	return OperationSpec{
		Bindings: []BindingSpec{{Namespace: BindingBuiltin, Member: []string{name}}},
		Input:    ValuesSpec{Fixed: []typ.Type{typ.String}, Tail: ValuesClosed},
		Outcomes: outcomes,
		Effects:  RowSpec{Tail: RowClosed},
	}
}
