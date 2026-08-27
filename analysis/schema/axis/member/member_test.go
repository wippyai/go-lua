package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func axisRef(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

func reducerInput(axis schema.EntryReference, carrier, tag carrier.Key) ReducerInput {
	return ReducerInput{
		Axis:         axis,
		Carrier:      carrier,
		Form:         Exact,
		Multiplicity: MultiplicityOne,
		Tag:          tag,
	}
}

func reducerOutput(axis schema.EntryReference, carrier carrier.Key) ReducerOutput {
	return ReducerOutput{Axis: axis, Carrier: carrier}
}

func relationProvider(axis schema.EntryReference, member schema.Key) RelationRef {
	return RelationRef{Axis: axis, Member: member}
}

// testAuthorities is the frozen carrier ABI used by the raw catalog
// fixtures in this package. The declaration-only catalog cannot infer a
// carrier's owner or capability from its spelling, so every carrier occurring
// in a fixture is listed explicitly as a local authority.
func testAuthorities(keys ...carrier.Key) []carrier.Authority {
	authorities := make([]carrier.Authority, 0, len(keys))
	seen := make(map[carrier.Key]struct{}, len(keys))
	for _, key := range keys {
		if !key.Available() {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		authorities = append(authorities, carrier.Authority{Carrier: key, Capability: carrier.DecodeOnly})
	}
	return authorities
}

func completeCatalog() Catalog {
	catalog, ok := NewCatalog(
		testAuthorities(
			"subject", "input/key", "input/value", "projection/key/result",
			"projection/predicate/result", "input/carrier", "output/carrier",
		),
		[]carrier.Binding{},
		[]Relation{{
			Key: "relation/input", Subject: "subject", Inputs: []carrier.Key{"input/key", "input/value"},
			CandidateProvider: AxisRelationCandidate(relationProvider(axisRef("axis/source"), "relation/input")),
		}},
		[]Projection{
			{Key: "projection/key", Relation: "relation/input", Role: Key, Result: "projection/key/result", CandidateProvider: AxisRelationCandidate(relationProvider(axisRef("axis/source"), "relation/input"))},
			{Key: "projection/predicate", Relation: "relation/input", Role: Predicate, Result: "projection/predicate/result", CandidateProvider: AxisRelationCandidate(relationProvider(axisRef("axis/source"), "relation/input"))},
		},
		[]Reducer{{
			Key:     "reducer/output",
			Inputs:  []ReducerInput{reducerInput(axisRef("axis/source"), "input/carrier", "")},
			Outputs: []ReducerOutput{reducerOutput(axisRef("axis/result"), "output/carrier")},
		}},
		[]CarryTransform{},
	)
	if !ok {
		panic("complete member catalog rejected")
	}
	return catalog
}

func TestNewCatalogAdmitsCompleteDeclaration(t *testing.T) {
	catalog := completeCatalog()
	if !catalog.Available() || catalog.MemberCount() != 4 {
		t.Fatalf("catalog = %#v, available=%t count=%d", catalog, catalog.Available(), catalog.MemberCount())
	}
	if relation, ok := catalog.Relation("relation/input"); !ok || relation.Subject != "subject" || len(relation.Inputs) != 2 {
		t.Fatalf("relation resolution = %#v/%t", relation, ok)
	}
	if ordinal, ok := catalog.ProjectionOrdinal("projection/predicate"); !ok || ordinal != 1 {
		t.Fatalf("projection ordinal = %d/%t", ordinal, ok)
	}
	if ordinal, ok := catalog.ReducerOrdinal("reducer/output"); !ok || ordinal != 0 {
		t.Fatalf("reducer ordinal = %d/%t", ordinal, ok)
	}
	if references := catalog.References(); len(references) != 5 ||
		references[0] != axisRef("axis/source") || references[1] != axisRef("axis/source") ||
		references[2] != axisRef("axis/source") || references[3] != axisRef("axis/source") ||
		references[4] != axisRef("axis/result") {
		t.Fatalf("catalog references = %#v", references)
	}
}

func TestNewCatalogRejectsDuplicateKeysAcrossKinds(t *testing.T) {
	if _, ok := NewCatalog(
		testAuthorities(),
		[]carrier.Binding{},
		[]Relation{{Key: "same"}},
		[]Projection{{Key: "same", Relation: "same", Role: Key}},
		[]Reducer{},
		[]CarryTransform{},
	); ok {
		t.Fatal("catalog admitted a relation/projection duplicate key")
	}
}

func TestNewCatalogRejectsProjectionWithMissingRelation(t *testing.T) {
	if _, ok := NewCatalog(testAuthorities("result"), []carrier.Binding{}, nil, []Projection{{Key: "projection", Relation: "missing", Role: Key, Result: "result"}}, nil, nil); ok {
		t.Fatal("catalog admitted a projection with a missing relation")
	}
}

func TestNewCatalogRejectsMalformedReducer(t *testing.T) {
	for name, reducer := range map[string]Reducer{
		"zero outputs": {Key: "reducer"},
		"foreign input": {Key: "reducer", Inputs: []ReducerInput{reducerInput(
			schema.EntryReference{Surface: schema.SurfaceKindRule, Key: "rule"}, "carrier", "tag",
		)}, Outputs: []ReducerOutput{reducerOutput(axisRef("axis/result"), "output")}},
		"empty input": {Key: "reducer", Inputs: []ReducerInput{reducerInput(
			schema.EntryReference{Surface: schema.SurfaceKindAxis}, "carrier", "tag",
		)}, Outputs: []ReducerOutput{reducerOutput(axisRef("axis/result"), "output")}},
		"missing input carrier": {Key: "reducer", Inputs: []ReducerInput{reducerInput(
			axisRef("axis/source"), "", "",
		)}, Outputs: []ReducerOutput{reducerOutput(axisRef("axis/result"), "output")}},
		"invalid input form": {Key: "reducer", Inputs: []ReducerInput{{
			Axis: axisRef("axis/source"), Carrier: "carrier", Multiplicity: MultiplicityOne, Tag: "tag",
		}}, Outputs: []ReducerOutput{reducerOutput(axisRef("axis/result"), "output")}},
		"missing output carrier": {Key: "reducer", Inputs: []ReducerInput{reducerInput(
			axisRef("axis/source"), "carrier", "",
		)}, Outputs: []ReducerOutput{reducerOutput(axisRef("axis/result"), "")}},
	} {
		t.Run(name, func(t *testing.T) {
			var carriers []carrier.Key
			for _, input := range reducer.Inputs {
				carriers = append(carriers, input.Carrier, input.Tag, input.Route)
			}
			for _, output := range reducer.Outputs {
				carriers = append(carriers, output.Carrier)
			}
			if _, ok := NewCatalog(testAuthorities(carriers...), []carrier.Binding{}, nil, nil, []Reducer{reducer}, nil); ok {
				t.Fatal("catalog admitted malformed reducer")
			}
		})
	}
}

// TestReducerInputConditionalCarriersFollowTheirReadForm states which of an
// input's two conditional carriers its read form can have at all.
//
// A Summary's tag IS its selection projection, so a Summary read always carries
// one. An Exact or Complete read selects nothing and routes nowhere, so it
// carries neither. A Selected read is the one form whose conditions this row
// cannot answer: its tag is required exactly when the join it reads declares a
// Predicate, and its route coordinate exactly when an output writes through
// that join - both statements of the reading rule's plan, not of this
// declaration. So this law leaves both open on Selected, and the rule model is
// where they are settled against the plan that makes them true.
func TestReducerInputConditionalCarriersFollowTheirReadForm(t *testing.T) {
	axis := axisRef("axis/source")
	for name, input := range map[string]ReducerInput{
		"exact with tag": {
			Axis: axis, Carrier: "carrier", Form: ReadFormExact,
			Multiplicity: MultiplicityOne, Tag: "tag",
		},
		"exact with route": {
			Axis: axis, Carrier: "carrier", Form: ReadFormExact,
			Multiplicity: MultiplicityOne, Route: "route",
		},
		"summary without tag": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSummary,
			Multiplicity: MultiplicityOne,
		},
		"summary with route": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSummary,
			Multiplicity: MultiplicityOne, Tag: "tag", Route: "route",
		},
		"complete with tag": {
			Axis: axis, Carrier: "carrier", Form: ReadFormComplete,
			Multiplicity: MultiplicityMany, Tag: "tag",
		},
		"complete with route": {
			Axis: axis, Carrier: "carrier", Form: ReadFormComplete,
			Multiplicity: MultiplicityMany, Route: "route",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if input.Available() {
				t.Fatal("reducer input admitted a carrier its read form cannot have")
			}
		})
	}
	for name, input := range map[string]ReducerInput{
		"exact": {
			Axis: axis, Carrier: "carrier", Form: ReadFormExact,
			Multiplicity: MultiplicityOne,
		},
		"selected tagged": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSelected,
			Multiplicity: MultiplicityMany, Tag: "tag",
		},
		"selected untagged": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSelected,
			Multiplicity: MultiplicityMany,
		},
		"selected routed": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSelected,
			Multiplicity: MultiplicityOne, Route: "route",
		},
		"selected routed and tagged": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSelected,
			Multiplicity: MultiplicityMany, Tag: "tag", Route: "route",
		},
		"summary": {
			Axis: axis, Carrier: "carrier", Form: ReadFormSummary,
			Multiplicity: MultiplicityOne, Tag: "tag",
		},
		"complete": {
			Axis: axis, Carrier: "carrier", Form: ReadFormComplete,
			Multiplicity: MultiplicityMany,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if !input.Available() {
				t.Fatal("reducer input rejected a shape its read form admits")
			}
		})
	}
}

func TestCatalogCopiesSlicesAndReducerInputs(t *testing.T) {
	inputs := []carrier.Key{"input/one", "input/two"}
	relations := []Relation{{Key: "relation", Subject: "subject", Inputs: inputs, CandidateProvider: AxisRelationCandidate(relationProvider(axisRef("axis/source"), "relation"))}}
	reducerInputs := []ReducerInput{reducerInput(axisRef("axis/source"), "carrier", "")}
	reducerOutputs := []ReducerOutput{reducerOutput(axisRef("axis/result"), "output")}
	reducers := []Reducer{{Key: "reducer", Inputs: reducerInputs, Outputs: reducerOutputs}}
	catalog, ok := NewCatalog(
		testAuthorities("subject", "input/one", "input/two", "carrier", "output"),
		[]carrier.Binding{}, relations, nil, reducers, nil,
	)
	if !ok {
		t.Fatal("complete catalog rejected")
	}
	relations[0].Key = "changed"
	inputs[0] = "changed"
	reducerInputs[0].Carrier = "changed"
	reducerOutputs[0].Carrier = "changed"
	if _, ok := catalog.Relation("relation"); !ok {
		t.Fatal("catalog retained an aliased relation slice")
	}
	relation, ok := catalog.Relation("relation")
	if !ok || len(relation.Inputs) != 2 || relation.Inputs[0] != "input/one" {
		t.Fatalf("catalog retained aliased relation inputs: %#v/%t", relation, ok)
	}
	reducer, ok := catalog.Reducer("reducer")
	if !ok || len(reducer.Inputs) != 1 || reducer.Inputs[0].Carrier != "carrier" || len(reducer.Outputs) != 1 || reducer.Outputs[0].Carrier != "output" {
		t.Fatalf("catalog retained aliased reducer signature: %#v/%t", reducer, ok)
	}
	copy := catalog.Clone()
	copy.Relations[0].Inputs[0] = "copy-mutated"
	copy.Reducers[0].Inputs[0].Carrier = "copy-mutated"
	copy.Reducers[0].Outputs[0].Carrier = "copy-mutated"
	if catalog.Relations[0].Inputs[0] != "input/one" || catalog.Reducers[0].Inputs[0].Carrier != "carrier" || catalog.Reducers[0].Outputs[0].Carrier != "output" {
		t.Fatal("catalog clone shares member signature storage")
	}
}

func TestMemberRefsRequireAxisOwnerAndMember(t *testing.T) {
	valid := RelationRef{Axis: axisRef("axis/source"), Member: "relation/input"}
	if !valid.Available() || !(ProjectionRef{Axis: valid.Axis, Member: valid.Member}).Available() ||
		!(ReducerRef{Axis: valid.Axis, Member: valid.Member}).Available() {
		t.Fatal("complete member references rejected")
	}
	for _, available := range []bool{
		(RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindRule, Key: "rule"}, Member: "member"}).Available(),
		(RelationRef{Axis: axisRef("axis/source"), Member: ""}).Available(),
	} {
		if available {
			t.Fatal("malformed member reference admitted")
		}
	}
}

func TestCatalogAdmitsCandidateIndexedFactEndomorphism(t *testing.T) {
	transform := CarryTransform{Key: "transform/value", Candidate: "candidate", Input: "fact", Output: "fact"}
	catalog, ok := NewCatalog(
		testAuthorities("candidate", "fact"),
		[]carrier.Binding{},
		[]Relation{{Key: "relation", Subject: "candidate", CandidateProvider: AxisRelationCandidate(relationProvider(axisRef("axis/source"), "relation"))}},
		[]Projection{},
		[]Reducer{},
		[]CarryTransform{transform},
	)
	if !ok || !catalog.Available() || catalog.CarryTransformCount() != 1 {
		t.Fatalf("carry transform catalog unavailable: %#v/%t", catalog, ok)
	}
	got, gotOK := catalog.CarryTransform("transform/value")
	if !gotOK || got != transform {
		t.Fatalf("carry transform lookup=%#v/%t, want %#v/true", got, gotOK, transform)
	}
	if ordinal, ordinalOK := catalog.CarryTransformOrdinal("transform/value"); !ordinalOK || ordinal != 0 {
		t.Fatalf("carry transform ordinal=%d/%t", ordinal, ordinalOK)
	}
	for _, malformed := range []CarryTransform{
		{Key: "transform/value", Candidate: "candidate", Input: "fact", Output: "other-fact"},
		{Key: "transform/value", Candidate: "candidate", Input: "fact", Output: ""},
	} {
		if _, malformedOK := NewCatalog(
			testAuthorities(malformed.Candidate, malformed.Input, malformed.Output),
			[]carrier.Binding{}, nil, nil, nil, []CarryTransform{malformed},
		); malformedOK {
			t.Fatalf("malformed carry transform admitted: %#v", malformed)
		}
	}
}
