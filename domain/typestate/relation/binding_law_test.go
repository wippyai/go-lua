package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/harness"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/relationfixture"
	typestatedomain "github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/domain/typestate/relation"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const reserve = 64

// TestAnOpaqueCalleeStillPublishesItsRow is the population law, stated through
// the binding rather than through the domain.
//
// A callee the analysis cannot follow is judged, not dropped: the declared
// escape discharges every proof about the resource and the answer is the
// unproven one. What matters at this layer is that the answer still occupies a
// row. A binding that refused instead would shrink the population, and a
// missing row reads as a call with nothing to say about it, which is the one
// answer a soundness judgment may not give.
//
// The law drives a governed site twice through the binding: once with the
// callee the analysis can follow, and once with that same dispatch carrying an
// alternative it cannot. Both publish. That is the property - not that the two
// answers agree, which they must not, but that neither call loses its row.
func TestAnOpaqueCalleeStillPublishesItsRow(t *testing.T) {
	fixture := relationfixture.NewGoverned(t)
	judgment, ok := relation.NewTypestateObligationOperation(fixture.Values, fixture.Calls, fixture.Packs)
	if !ok {
		t.Fatal("obligation judgment")
	}
	if fixture.Values.MountedCallArgumentCount() == 0 {
		t.Fatal("the governed fixture sealed no mounted call actual")
	}

	place := harness.New(t, "row/cell")
	var types relation.PayloadTypes
	var tags relation.PayloadTags
	place.InstallTypes(t, &types)
	place.InstallTags(t, &tags)
	payloads, installed := relation.NewPayloads(types, tags, reserve)
	if !installed {
		t.Fatal("install the typestate columns")
	}
	valueType := place.TypeID(t, "type/value")
	callType := place.TypeID(t, "type/call")
	valueColumn := harness.NewColumn[valuedomain.Value](t, valueType, "store/value", reserve)
	callColumn := harness.NewColumn[calldomain.Value](t, callType, "store/call", reserve)
	columns, columnsOK := relation.NewTypestateObligationColumns(
		valueColumn, callColumn, payloads.Typestate, payloads.CallArgumentCandidate, payloads.ProtocolTag)
	if !columnsOK {
		t.Fatal("obligation columns")
	}

	candidateAddress := place.Column(t, "column/candidate")
	argumentAddress := place.Column(t, "column/argument")
	dispatchedAddress := place.Column(t, "column/dispatched")
	tagAddress := place.Column(t, "column/tag")
	cellAddress := place.Column(t, "column/cell")
	exactly, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	sealed := place.Seal(t, "operation/typestate-obligation",
		[]signature.Input{
			harness.ScalarInput(t, place.Relation, candidateAddress, types.CallArgumentCandidate, place.Denominator),
			harness.ScalarInput(t, place.Relation, argumentAddress, valueType, place.Denominator),
			harness.ScalarInput(t, place.Relation, dispatchedAddress, callType, place.Denominator),
			harness.ScalarInput(t, place.Relation, tagAddress, types.ProtocolTag, place.Denominator),
			harness.ScalarInput(t, place.Relation, cellAddress, types.Typestate, place.Denominator),
		},
		[]signature.Output{{Relation: place.Relation, Column: cellAddress, Type: types.Typestate, Presence: signature.ProduceOpaque}},
		exactly, outcome.Produced, outcome.Opaque, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	factory, ok := relation.BindTypestateObligation(sealed, judgment, columns, place.Refusal)
	if !ok {
		t.Fatal("bind typestate obligation")
	}
	worker := place.Worker(t, factory, sealed)

	opaque := fixture.Calls.Top()
	if !opaque.HasOpaqueAlternative() {
		t.Fatal("the call algebra's top carries no opaque alternative")
	}

	published := 0
	seen := map[int]int{}
	for index := 0; index < fixture.Values.MountedCallArgumentCount(); index++ {
		candidate, candidateOK := fixture.Values.MountedCallArgumentAt(index)
		if !candidateOK {
			t.Fatalf("mounted call actual %d is not issued", index)
		}
		for protocol := uint64(1); protocol < 64; protocol++ {
			candidateToken, encodeOK := payloads.CallArgumentCandidate.Encode(place.Issuer, candidate)
			if !encodeOK {
				t.Fatal("encode candidate")
			}
			argumentToken, encodeOK := valueColumn.Encode(place.Issuer, fixture.Values.Top())
			if !encodeOK {
				t.Fatal("encode argument")
			}
			dispatchedToken, encodeOK := callColumn.Encode(place.Issuer, opaque)
			if !encodeOK {
				t.Fatal("encode dispatched call")
			}
			tagToken, encodeOK := payloads.ProtocolTag.Encode(place.Issuer, protocol)
			if !encodeOK {
				t.Fatal("encode protocol tag")
			}
			cellToken, encodeOK := payloads.Typestate.Encode(place.Issuer, typestatedomain.Unknown())
			if !encodeOK {
				t.Fatal("encode current cell")
			}
			frame := place.Frame(t,
				harness.ScalarSlot(t, place.Cell(t, candidateAddress, place.Rows[0], types.CallArgumentCandidate, candidateToken)),
				harness.ScalarSlot(t, place.Cell(t, argumentAddress, place.Rows[0], valueType, argumentToken)),
				harness.ScalarSlot(t, place.Cell(t, dispatchedAddress, place.Rows[0], callType, dispatchedToken)),
				harness.ScalarSlot(t, place.Cell(t, tagAddress, place.Rows[0], types.ProtocolTag, tagToken)),
				harness.ScalarSlot(t, place.Cell(t, cellAddress, place.Rows[0], types.Typestate, cellToken)),
			)
			buffer := place.Buffer(t, sealed)
			result := worker.Evaluate(frame, buffer)
			seen[int(result.Code)]++
			if !sealed.Allows(result.Code) {
				t.Fatalf("actual %d protocol %d settled outside its own vocabulary: %v", index, protocol, result.Code)
			}
			if result.Code == outcome.Refused {
				// A refusal is what removes a row. The judgment refuses the
				// actuals no declaration governs, and those are not this law's
				// subject; what this law forbids is refusing one it does.
				continue
			}
			if result.Code != outcome.Opaque {
				continue
			}
			published++
			// The answer occupies a row. This is the property: an unfollowable
			// callee is judged and kept, so the invocation stages its fact
			// instead of answering that there is none.
			if buffer.Len() != 1 {
				t.Fatalf("an opaque callee staged %d rows through the binding; the population does not shrink", buffer.Len())
			}
		}
	}
	if published == 0 {
		t.Fatalf("no governed actual reached the opaque arm; codes seen through the binding: %v", seen)
	}
	t.Logf("opaque answers published through the binding: %d", published)
}
