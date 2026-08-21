package wire

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/iteration"
	"github.com/wippyai/go-lua/domain/effect/mutation"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/projection"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// The iterator-kind spelling and the ordering of a row's labels are both
// commitments the manifest format carries. These laws state them: the token
// written for each declared kind, the agreement between that token and the
// kind's display spelling that holds today, and the exact order a
// representative row records its labels in.

// TestIteratorKindWireTokensAreStable states the written commitment: each
// declared kind serializes to exactly the recorded token, and each token reads
// back as the kind it was written for.
func TestIteratorKindWireTokensAreStable(t *testing.T) {
	recorded := map[iteration.IteratorKind]string{
		iteration.IterateIndexed: "indexed",
		iteration.IterateKeyed:   "keyed",
	}
	if len(recorded) != iteration.IteratorKindCount {
		t.Fatalf("the vocabulary declares %d kinds but %d have a recorded token", iteration.IteratorKindCount, len(recorded))
	}
	seen := make(map[string]iteration.IteratorKind, iteration.IteratorKindCount)
	for _, kind := range iteration.IteratorKinds() {
		want, pinned := recorded[kind]
		if !pinned {
			t.Fatalf("declared iterator kind %d has no recorded wire token", kind)
		}
		token, err := encodeIteratorKind(kind)
		if err != nil {
			t.Fatalf("codec does not write declared iterator kind %d: %v", kind, err)
		}
		if token != want {
			t.Fatalf("iterator kind %d writes token %q, want %q", kind, token, want)
		}
		if prior, duplicate := seen[token]; duplicate {
			t.Fatalf("declared kinds %d and %d are both written as %q", prior, kind, token)
		}
		seen[token] = kind
		read, err := decodeIteratorKind(token)
		if err != nil {
			t.Fatalf("codec writes token %q that it does not read: %v", token, err)
		}
		if read != kind {
			t.Fatalf("token %q reads back as kind %d, written for kind %d", token, read, kind)
		}
	}
}

// TestIteratorKindWireTokenEqualsItsDisplaySpelling pins the agreement that
// holds today between the two spellings without deriving one from the other.
// The wire token is the codec's own commitment and the display spelling is the
// domain's; they coincide, and this law is what would have to be restated
// deliberately if the display spelling ever moved.
func TestIteratorKindWireTokenEqualsItsDisplaySpelling(t *testing.T) {
	for _, kind := range iteration.IteratorKinds() {
		token, err := encodeIteratorKind(kind)
		if err != nil {
			t.Fatalf("codec does not write declared iterator kind %d: %v", kind, err)
		}
		if display := kind.String(); token != display {
			t.Fatalf("iterator kind %d writes token %q but displays as %q; the wire token is pinned, so moving the display spelling means restating this law", kind, token, display)
		}
	}
}

// TestIteratorKindCodecRejectsUndeclaredKinds states the closing half: an
// ordinal the vocabulary does not declare is refused on write, and a token it
// does not spell is refused on read.
func TestIteratorKindCodecRejectsUndeclaredKinds(t *testing.T) {
	for _, undeclared := range []iteration.IteratorKind{-1, iteration.IteratorKind(iteration.IteratorKindCount), 99} {
		if _, err := encodeIteratorKind(undeclared); err == nil {
			t.Fatalf("codec wrote the undeclared iterator kind %d", undeclared)
		}
	}
	for _, token := range []string{"", "unknown", "Indexed", "iterator"} {
		if _, err := decodeIteratorKind(token); err == nil {
			t.Fatalf("codec admitted the iterator token %q", token)
		}
	}
}

// projectionStepWireCase is one representative step and the exact bytes the
// codec writes for it.
type projectionStepWireCase struct {
	name string
	kind projection.StepKind
	step projection.Step
	wire string
}

func projectionStepWireCorpus() []projectionStepWireCase {
	return []projectionStepWireCase{
		{
			name: "field",
			kind: projection.StepField,
			step: projection.Field("inner"),
			wire: `{"kind":"field","field":"inner"}`,
		},
		{
			name: "callableReturn",
			kind: projection.StepCallableReturn,
			step: projection.CallableReturn(),
			wire: `{"kind":"callableReturn"}`,
		},
		{
			name: "genericArg",
			kind: projection.StepGenericArg,
			step: projection.GenericArg(2),
			wire: `{"kind":"genericArg","index":2}`,
		},
		{
			name: "instantiateGeneric",
			kind: projection.StepInstantiateGeneric,
			step: projection.InstantiateGeneric(typ.String),
			wire: `{"kind":"instantiateGeneric","type":{"kind":"string","integrity":"sha256/1:9bddc1fdfb55932e41cf34e4a0bf062564cf848d3e0070d86dcd3a998fdce008"}}`,
		},
	}
}

// TestProjectionStepWireBytesAreStable states the written commitment: each step
// serializes to exactly the recorded bytes, kind spelling, field placement and
// field omission included.
func TestProjectionStepWireBytesAreStable(t *testing.T) {
	for _, tc := range projectionStepWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := encodeProjectionSteps([]projection.Step{tc.step})
			if err != nil {
				t.Fatalf("encodeProjectionSteps: %v", err)
			}
			if len(encoded) != 1 {
				t.Fatalf("encodeProjectionSteps wrote %d steps for one", len(encoded))
			}
			data, err := json.Marshal(encoded[0])
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("wire bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestProjectionStepRoundTripsThroughItsOwnBytes states the read commitment:
// the recorded bytes parse back into the step they were written from, compared
// by the vocabulary's own structural equality.
func TestProjectionStepRoundTripsThroughItsOwnBytes(t *testing.T) {
	for _, tc := range projectionStepWireCorpus() {
		t.Run(tc.name, func(t *testing.T) {
			var read []projectionStepWire
			if err := json.Unmarshal([]byte("["+tc.wire+"]"), &read); err != nil {
				t.Fatalf("unmarshal recorded bytes: %v", err)
			}
			decoded, err := decodeProjectionSteps(read)
			if err != nil {
				t.Fatalf("decodeProjectionSteps: %v", err)
			}
			if len(decoded) != 1 {
				t.Fatalf("decodeProjectionSteps read %d steps from one", len(decoded))
			}
			if decoded[0].Kind != tc.kind {
				t.Fatalf("decoded step is kind %d, want %d", decoded[0].Kind, tc.kind)
			}
			want := projection.Projection{Steps: []projection.Step{tc.step}}
			if !projection.Equal(projection.Projection{Steps: decoded}, want) {
				t.Fatalf("decoded step = %s, want %s", decoded[0], tc.step)
			}
			rewritten, err := encodeProjectionSteps(decoded)
			if err != nil {
				t.Fatalf("encodeProjectionSteps(decoded): %v", err)
			}
			data, err := json.Marshal(rewritten[0])
			if err != nil {
				t.Fatalf("marshal rewritten: %v", err)
			}
			if string(data) != tc.wire {
				t.Fatalf("rewritten bytes = %s, want %s", data, tc.wire)
			}
		})
	}
}

// TestProjectionStepCodecSpellsEveryDeclaredKindOnce states that the boundary
// vocabulary is total over the domain catalog and injective into wire kinds,
// and that a corpus case pins the bytes of every declared step.
func TestProjectionStepCodecSpellsEveryDeclaredKindOnce(t *testing.T) {
	corpus := make(map[projection.StepKind]bool, projection.StepKindCount)
	for _, tc := range projectionStepWireCorpus() {
		if tc.step.Kind != tc.kind {
			t.Fatalf("corpus case %q stands for kind %d but its step is kind %d", tc.name, tc.kind, tc.step.Kind)
		}
		corpus[tc.kind] = true
	}
	spelled := make(map[string]projection.StepKind, projection.StepKindCount)
	for _, kind := range projection.StepKinds() {
		row := projectionStepWireVariants[kind]
		if row.kind == "" {
			t.Fatalf("declared projection step kind %d has no wire spelling", kind)
		}
		if prior, duplicate := spelled[row.kind]; duplicate {
			t.Fatalf("declared kinds %d and %d are both spelled %q", prior, kind, row.kind)
		}
		spelled[row.kind] = kind
		if row.build == nil {
			t.Fatalf("wire kind %q states no rebuilding", row.kind)
		}
		read, known := projectionStepWireVariantsByKind[row.kind]
		if !known || read != kind {
			t.Fatalf("wire kind %q is written for kind %d but read back as kind %d", row.kind, kind, read)
		}
		if !corpus[kind] {
			t.Fatalf("declared projection step kind %d is serialized by the codec but no corpus case pins its bytes", kind)
		}
	}
}

// TestProjectionStepCodecRejectsUndeclaredKinds states the closing half: a step
// ordinal the vocabulary does not declare is refused on write, and a wire kind
// it does not spell is refused on read.
func TestProjectionStepCodecRejectsUndeclaredKinds(t *testing.T) {
	for _, undeclared := range []projection.StepKind{0, projection.StepKind(projection.StepKindCount + 1), 99} {
		if _, err := encodeProjectionSteps([]projection.Step{{Kind: undeclared}}); err == nil {
			t.Fatalf("codec wrote the undeclared projection step kind %d", undeclared)
		}
	}
	if _, err := decodeProjectionSteps([]projectionStepWire{{Kind: "unsealed"}}); err == nil {
		t.Fatal("codec admitted a projection step kind the vocabulary does not declare")
	}
	if _, err := decodeProjectionSteps([]projectionStepWire{{Kind: "return"}}); err == nil {
		t.Fatal("codec admitted the display spelling of a step as its wire kind")
	}
	if _, err := decodeProjectionSteps([]projectionStepWire{{Kind: "genericArg"}}); err == nil {
		t.Fatal("codec admitted a genericArg step carrying no index")
	}
}

// TestEffectLabelWireSerializes is the law the ordering basis rests on: the
// wire struct carries only serializable fields, so the key an effect label
// sorts under is total. A field type added to effectLabelWire that JSON cannot
// write is a verdict here rather than a manifest whose row order is undefined.
func TestEffectLabelWireSerializes(t *testing.T) {
	index := 1
	ref := &paramRefWire{Index: &index}
	populated := effectLabelWire{
		Kind:         "kind",
		ReturnIndex:  &index,
		ValueIndex:   &index,
		ErrorIndex:   &index,
		Indices:      []int{0, 1},
		FromParam:    &index,
		Delta:        &index,
		Target:       ref,
		Source:       ref,
		Param:        ref,
		Into:         ref,
		Value:        ref,
		IteratorKind: "indexed",
		Transform:    &effectTransformWire{Kind: "mutation.unchanged"},
		ReturnType:   &effectReturnWire{Kind: "returns.sameAs", Source: ref},
		Length:       &exprWire{Kind: "const", Value: 1},
		Refinement:   &effectRefinementWire{Kind: "present"},
		Protocol:     "protocol",
		From:         "from",
		To:           "to",
		Final:        "final",
		Finals:       []string{"final"},
	}
	if _, err := json.Marshal(populated); err != nil {
		t.Fatalf("the effect label wire does not serialize, so the ordering basis is partial: %v", err)
	}
	if key := effectLabelWireKey(populated); key == "" {
		t.Fatal("the ordering key of a populated label is empty")
	}
}

// TestEffectLabelOrderingIsByWireKindThenWrittenBytes states the ordering basis
// directly: the key a label sorts under leads with the wire kind it is written
// as, and the rest of the key is the row's own serialization.
func TestEffectLabelOrderingIsByWireKindThenWrittenBytes(t *testing.T) {
	label := effectLabelWire{Kind: "ownership.borrow", Param: encodeParamRef(effect.ParamRef{Index: 2})}
	key := effectLabelWireKey(label)
	written, err := json.Marshal(label)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := label.Kind + "\x00" + string(written); key != want {
		t.Fatalf("ordering key = %q, want %q", key, want)
	}
}

// orderedRowCorpus is a row whose labels arrive out of order and whose kinds
// include repeats that differ only in payload, so the recorded order below
// exercises both halves of the basis.
func orderedRowCorpus() effect.Row {
	p := func(index int) effect.ParamRef { return effect.ParamRef{Index: index} }
	return effect.Row{Labels: []effect.Label{
		ownership.Store{Param: p(1), Into: p(0)},
		mutation.TableMutator{Target: p(0), Value: p(2)},
		returns.ErrorReturn{ValueIndex: 1, ErrorIndex: 0},
		returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		iteration.Iterator{Source: p(0), Kind: iteration.IterateKeyed},
		ownership.Borrow{Param: p(3)},
		ownership.Borrow{Param: p(1)},
		dispatch.ModuleLoad{},
		mutation.LengthChange{Target: p(0), Delta: 1},
		mutation.LengthChange{Target: p(0), Delta: -1},
		postcondition.NormalReturnRefinement{Target: p(2), Refinement: postcondition.Present{}},
		postcondition.NormalReturnRefinement{Target: p(2), Refinement: postcondition.Absent{}},
		returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p(0)}},
		returns.Return{ReturnIndex: 1, Transform: returns.ElementOf{Source: p(0)}},
		ownership.BorrowAll{},
		ownership.Retain{Param: p(2)},
	}}
}

// orderedRowWire is the exact order and bytes the codec records for that row.
// It is the manifest's ordering commitment, recorded once here so a change to
// the basis has to be a deliberate restatement rather than a silent reshuffle
// of every manifest in the world.
const orderedRowWire = `{"labels":[` +
	`{"kind":"dispatch.moduleLoad"},` +
	`{"kind":"iteration.iterator","source":{"index":0},"iteratorKind":"keyed"},` +
	`{"kind":"mutation.lengthChange","delta":-1,"target":{"index":0}},` +
	`{"kind":"mutation.lengthChange","delta":1,"target":{"index":0}},` +
	`{"kind":"mutation.tableMutator","target":{"index":0},"value":{"index":2}},` +
	`{"kind":"ownership.borrow","param":{"index":1}},` +
	`{"kind":"ownership.borrow","param":{"index":3}},` +
	`{"kind":"ownership.borrowAll"},` +
	`{"kind":"ownership.retain","param":{"index":2}},` +
	`{"kind":"ownership.store","param":{"index":1},"into":{"index":0}},` +
	`{"kind":"postcondition.normalReturnRefinement","target":{"index":2},"refinement":{"kind":"absent"}},` +
	`{"kind":"postcondition.normalReturnRefinement","target":{"index":2},"refinement":{"kind":"present"}},` +
	`{"kind":"returns.errorReturn","valueIndex":0,"errorIndex":1},` +
	`{"kind":"returns.errorReturn","valueIndex":1,"errorIndex":0},` +
	`{"kind":"returns.return","returnIndex":0,"returnType":{"kind":"returns.sameAs","source":{"index":0}}},` +
	`{"kind":"returns.return","returnIndex":1,"returnType":{"kind":"returns.elementOf","source":{"index":0}}}` +
	`]}`

// TestEffectRowRecordsItsLabelsInThePinnedOrder states the commitment on a
// representative row: the labels are recorded in exactly the pinned order, and
// the row's bytes are exactly the pinned bytes.
func TestEffectRowRecordsItsLabelsInThePinnedOrder(t *testing.T) {
	wire, err := encodeEffectRow(orderedRowCorpus())
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != orderedRowWire {
		t.Fatalf("row bytes = %s, want %s", data, orderedRowWire)
	}
}

// TestEffectRowOrderDoesNotDependOnArrivalOrder states that the basis is a
// total order over the labels of a row and not a stable sort over the order
// they were built in: the same labels in a different arrival order record the
// same bytes.
func TestEffectRowOrderDoesNotDependOnArrivalOrder(t *testing.T) {
	row := orderedRowCorpus()
	reversed := effect.Row{Labels: make([]effect.Label, 0, len(row.Labels))}
	for i := len(row.Labels) - 1; i >= 0; i-- {
		reversed.Labels = append(reversed.Labels, row.Labels[i])
	}
	wire, err := encodeEffectRow(reversed)
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != orderedRowWire {
		t.Fatalf("reversed row bytes = %s, want %s", data, orderedRowWire)
	}
}

// TestEffectLabelOrderingAgreesWithTheWrittenBytes states the property that
// makes the two-part key the same order the written bytes already imply: no
// wire kind is a prefix of another followed by a byte below the quote that
// closes it, so leading with the kind never reorders labels relative to their
// serialization. It is what lets the basis be stated as kind-then-bytes without
// moving a single existing manifest.
func TestEffectLabelOrderingAgreesWithTheWrittenBytes(t *testing.T) {
	row := orderedRowCorpus()
	encoded := make([]effectLabelWire, 0, len(row.Labels))
	for _, label := range row.Labels {
		wire, err := encodeEffectLabel(label)
		if err != nil {
			t.Fatalf("encodeEffectLabel: %v", err)
		}
		encoded = append(encoded, wire)
	}

	byStatedKey := append([]effectLabelWire(nil), encoded...)
	sort.SliceStable(byStatedKey, func(i, j int) bool {
		return effectLabelWireKey(byStatedKey[i]) < effectLabelWireKey(byStatedKey[j])
	})

	byWrittenBytes := append([]effectLabelWire(nil), encoded...)
	sort.SliceStable(byWrittenBytes, func(i, j int) bool {
		left, err := json.Marshal(byWrittenBytes[i])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		right, err := json.Marshal(byWrittenBytes[j])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return string(left) < string(right)
	})

	for position := range byStatedKey {
		stated, err := json.Marshal(byStatedKey[position])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		written, err := json.Marshal(byWrittenBytes[position])
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(stated) != string(written) {
			t.Fatalf("position %d is %s under the stated basis and %s under the written bytes", position, stated, written)
		}
	}
}
