package emit

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// The exact fold laws. An exact publication is folded from the canonical
// product its declared reads refine: each read partitions the invocation's
// support, the common refinement of those partitions is the set of cells the
// reads agree on, and one cell is one call to the domain judgment under
// exactly the region that cell holds over.
//
// The declaration states how many reads there are. Nothing here derives the
// arity from a shape probe, and nothing publishes at a region wider than the
// cell its values came from.

const consumerPackage = "example/consumer"

func consumerType(name string) definition.GoType {
	return definition.GoType{PackagePath: consumerPackage, Name: name}
}

func consumerMethod(name, receiver string, resultIndex int8, receiverPackage string) definition.GoSymbol {
	return definition.GoSymbol{
		PackagePath: consumerPackage, Name: name,
		Receiver:    definition.GoType{PackagePath: receiverPackage, Name: receiver},
		ResultIndex: resultIndex,
	}
}

func consumerAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "consumer"}
}

// exactProductRoster is the two-axis vocabulary a heterogeneous exact fold is
// emitted from: the specimen axis owns the candidate directory and the two
// relations the reads observe, and the consumer axis owns the destination the
// publication lands at and the reducer that decides it.
func exactProductRoster(t testing.TB, reads int) definition.Roster {
	t.Helper()
	provider := member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}
	specimen := definition.Definition{
		Name:       "Specimen",
		Axis:       "specimen",
		ImportPath: specimenPackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "KeyCarrier",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "Schema", 0),
		}},
		Signature: definition.Signature{Key: "KeyCarrier", Fact: "FactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "KeyCarrier", Key: "carrier/specimen/key", Type: specimenType("Key")},
			{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")},
		},
		Relations: []definition.Relation{
			{
				Name: "Candidates", Key: "specimen/candidates", Subject: "KeyCarrier",
				CandidateProvider: member.AxisRelationCandidate(provider),
				CandidateResolver: specimenMethod("CandidateForOccurrence", "Schema", 0),
				CandidateOrdinal:  specimenMethod("CandidateOrdinal", "Schema", 0),
				CandidateAt:       specimenMethod("CandidateAt", "Schema", 0),
			},
			{
				Name: "Roots", Key: "specimen/roots", Subject: "FactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "KeyCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
			{
				Name: "Anchors", Key: "specimen/anchors", Subject: "FactCarrier",
				Inputs:            []definition.RelationInput{{Carrier: "KeyCarrier"}},
				CandidateProvider: member.AxisRelationCandidate(provider),
			},
		},
		Projections: []definition.Projection{
			{
				Name: "RootKey", Key: "specimen/root-key", Relation: "Roots",
				CandidateProvider: member.AxisRelationCandidate(provider), Role: member.Key, Result: "KeyCarrier",
				Accessor: specimenMethod("Root", "Key", -1),
			},
			{
				Name: "AnchorKey", Key: "specimen/anchor-key", Relation: "Anchors",
				CandidateProvider: member.AxisRelationCandidate(provider), Role: member.Key, Result: "KeyCarrier",
				Accessor: specimenMethod("Anchor", "Key", -1),
			},
		},
	}
	inputs := make([]definition.ReducerInput, 0, reads)
	for index := 0; index < reads; index++ {
		inputs = append(inputs, definition.ReducerInput{
			Axis: specimenAxis(), Carrier: "SpecimenFactCarrier",
			Form: member.ReadFormExact, Multiplicity: member.MultiplicityOne,
		})
	}
	consumer := definition.Definition{
		Name:       "Consumer",
		Axis:       "consumer",
		ImportPath: consumerPackage,
		Binding: definition.Binding{Key: definition.KeyNormalization{
			Carrier:    "ConsumerKeyCarrier",
			Dense:      definition.GoType{Name: "uint32"},
			Normalizer: consumerMethod("KeyIndex", "Schema", 0, consumerPackage),
		}},
		Signature: definition.Signature{Key: "ConsumerKeyCarrier", Fact: "ConsumerFactCarrier"},
		Carriers: []definition.Carrier{
			{Name: "ConsumerKeyCarrier", Key: "carrier/consumer/key", Type: consumerType("Key")},
			{Name: "ConsumerFactCarrier", Key: "carrier/consumer/fact", Type: consumerType("Fact")},
			{Name: "SpecimenKeyCarrier", Key: "carrier/specimen/key", Type: specimenType("Key")},
			{Name: "SpecimenFactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")},
		},
		Relations: []definition.Relation{{
			Name: "Destinations", Key: "consumer/destinations", Subject: "SpecimenKeyCarrier",
			CandidateProvider: member.AxisRelationCandidate(provider),
		}},
		Projections: []definition.Projection{{
			Name: "Destination", Key: "consumer/destination", Relation: "Destinations",
			CandidateProvider: member.AxisRelationCandidate(provider), Role: member.Destination,
			Result:   "ConsumerKeyCarrier",
			Accessor: consumerMethod("Destination", "Key", -1, specimenPackage),
		}},
	}
	contribution := definition.Contribution{
		Axis: "consumer",
		Rule: "consumer-exact",
		Reducers: []definition.Reducer{{
			Name: "ExactReducer", Key: "consumer/reducer/exact", Candidate: "SpecimenKeyCarrier",
			Inputs:         inputs,
			Outputs:        []definition.ReducerOutput{{Axis: consumerAxis(), Carrier: "ConsumerFactCarrier"}},
			Implementation: definition.GoSymbol{PackagePath: consumerPackage, Name: "ExactFold", ResultIndex: 0},
		}},
	}
	roster, rosterOK := definition.NewRoster(
		definition.Source{Package: "specimen", Name: "specimen", Base: specimen},
		definition.Source{
			Package: "consumer", Name: "consumer", Base: consumer,
			Contributions: []definition.Contribution{contribution},
		},
	)
	if !rosterOK {
		t.Fatal("exact product member roster is not admissible")
	}
	return roster
}

func exactRead(relation, key schema.Key) program.JoinDecl {
	return program.JoinDecl{
		Sources:  []program.SourceRef{program.CandidateSource()},
		Relation: member.RelationRef{Axis: specimenAxis(), Member: relation},
		Key:      member.ProjectionRef{Axis: specimenAxis(), Member: key},
		Read: program.ReadDecl{
			PointBound: program.PointBound,
			Input:      0,
			Axis:       program.AxisRef(specimenAxis()),
			Form:       program.Exact,
			Contract: program.ReadContract{
				Order:        program.OrderCanonical,
				Sparse:       program.SparseExplicit,
				OnOpaque:     program.OnOpaqueRefuse,
				Multiplicity: program.MultiplicityOne,
			},
		},
	}
}

func exactProductSpec(reads int) rule.Spec {
	joins := []program.JoinDecl{exactRead("specimen/roots", "specimen/root-key")}
	folded := []program.JoinRef{0}
	if reads > 1 {
		joins = append(joins, exactRead("specimen/anchors", "specimen/anchor-key"))
		folded = append(folded, 1)
	}
	return rule.Spec{
		Key:      "consumer-exact",
		Writes:   "consumer",
		Owner:    "consumer",
		Issues:   []rule.Issuance{{Occurrence: "occurrence/specimen", Requirement: "program-requirement/unrestricted", Form: "program-form/local-successor"}},
		Lane:     rule.LaneMounted,
		Semantic: "semantic/rule/consumer",
		Roles:    []schema.Key{"semantic/operand/consumer"},
		Program: program.Program{
			OperandRole: "semantic/operand/consumer",
			Candidate:   member.AxisRelationCandidate(member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}),
			Joins:       joins,
			Fold: program.FoldDecl{
				Reducer: member.ReducerRef{Axis: consumerAxis(), Member: "consumer/reducer/exact"},
				Inputs:  folded,
				Outputs: []program.OutputDecl{{
					Column:      axis.OutputRef{Axis: consumerAxis(), Key: "consumer/facts"},
					Destination: member.ProjectionRef{Axis: consumerAxis(), Member: "consumer/destination"},
					Mode:        program.ModeExact,
					ValueSlot:   0,
				}},
			},
			Carry: &program.CarryDecl{Input: 0, Mode: program.CarryIdentity},
		},
	}
}

func renderExactProduct(t testing.TB, reads int) string {
	t.Helper()
	rendered, err := Render(Target{
		PackagePath: consumerPackage, PackageName: "consumer", Spec: exactProductSpec(reads),
	}, exactProductRoster(t, reads))
	if err != nil {
		t.Fatalf("exact product family is not emitted: %v", err)
	}
	return string(rendered)
}

// TestExactFoldReducesOneCellPerProductCell is the arity law. A declaration
// with two exact reads folds the common refinement of two partitions, so the
// emitted judgment receives one cell per declared read and the worker chains
// one product extender per read in declaration order.
func TestExactFoldReducesOneCellPerProductCell(t *testing.T) {
	rendered := renderExactProduct(t, 2)
	for _, fragment := range []string{
		"func (fold familyReducer) Reduce(cell0 specimen.Fact, cell1 specimen.Fact) (Fact, structure.ReductionOutcome)",
		"product0 product.Extender[specimen.DenseCoordinate, specimen.Fact, struct{}]",
		"product1 product.Extender[specimen.DenseCoordinate, specimen.Fact, productTuple0]",
		"rows0, status0, status0OK := lane.product0.Extend(ticket, seed.Rows(), row.read0, &lane.read0)",
		"rows1, status1, status1OK := lane.product1.Extend(ticket, rows0, row.read1, &lane.read1)",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("emitted family does not state %q:\n%s", fragment, rendered)
		}
	}
}

// TestExactFoldPublishesUnderTheCellItReduced is the support law. A cell's
// fact is derived from the values that cell carries, so it is staged under
// that cell's own region and never under the invocation's whole support. The
// row's one write transaction spans every staged cell and is closed once.
func TestExactFoldPublishesUnderTheCellItReduced(t *testing.T) {
	rendered := renderExactProduct(t, 2)
	for _, fragment := range []string{
		"region, tuple, tupleOK := rows1.At(index)",
		"if !row.write.Stage(ticket, &lane.write, region, value) {",
		"if !row.write.Close(ticket, &lane.write) {",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("emitted family does not state %q:\n%s", fragment, rendered)
		}
	}
	if strings.Contains(rendered, "ticket.Within()") {
		t.Fatal("emitted family publishes under the invocation support rather than the product cell")
	}
	if strings.Contains(rendered, "PublishExact") {
		t.Fatal("emitted family publishes one cell and abandons the rest of its product")
	}
}

// TestExactFoldCellsAreReadInDeclaredOrder is the correspondence law. The
// extender conses the newest read at the tuple head, so the cell a read
// observed is reached by walking the tail back to that read's declared
// position. A family that read the tuple in any other order would hand the
// judgment one read's value under another read's name.
func TestExactFoldCellsAreReadInDeclaredOrder(t *testing.T) {
	rendered := renderExactProduct(t, 2)
	for _, fragment := range []string{
		"cell0, present0 := row.read0Policy.Cell(tuple.Tail().Head().Value(), tuple.Tail().Head().Present())",
		"cell1, present1 := row.read1Policy.Cell(tuple.Head().Value(), tuple.Head().Present())",
		"Reduce(cell0, cell1)",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("emitted family does not state %q:\n%s", fragment, rendered)
		}
	}
}

// TestExactFoldAbsentCellContributesNoPatch is the sparsity law. A cell whose
// declared reads are not all present is this rule's absent candidate over
// exactly that region: it publishes nothing and does not settle the whole row,
// because the other cells of the same product may still be concrete.
func TestExactFoldAbsentCellContributesNoPatch(t *testing.T) {
	rendered := renderExactProduct(t, 2)
	if !strings.Contains(rendered, "if !present0 || !present1 {\n\t\t\tcontinue\n\t\t}") {
		t.Fatalf("emitted family does not skip an absent cell:\n%s", rendered)
	}
	if !strings.Contains(rendered, "if staged == 0 {\n\t\treturn lane.settle(ticket, structure.NoCandidate)\n\t}") {
		t.Fatalf("emitted family does not settle a wholly absent product as an absent candidate:\n%s", rendered)
	}
}

// TestOneReadIsOneCaseOfTheExactProduct is the uniformity law. A single-read
// declaration is the one-factor product, not a separate shape: it drains the
// same canonical cursor and publishes under the same per-cell region, so a
// read whose partition has more than one cell publishes all of them.
func TestOneReadIsOneCaseOfTheExactProduct(t *testing.T) {
	rendered := renderExactProduct(t, 1)
	for _, fragment := range []string{
		"func (fold familyReducer) Reduce(cell0 specimen.Fact) (Fact, structure.ReductionOutcome)",
		"product0 product.Extender[specimen.DenseCoordinate, specimen.Fact, struct{}]",
		"rows0, status0, status0OK := lane.product0.Extend(ticket, seed.Rows(), row.read0, &lane.read0)",
		"region, tuple, tupleOK := rows0.At(index)",
		"cell0, present0 := row.read0Policy.Cell(tuple.Head().Value(), tuple.Head().Present())",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("emitted family does not state %q:\n%s", fragment, rendered)
		}
	}
}

// TestAnExactFoldRefusesANonExactRead is the refusal law for this shape. The
// product is the common refinement of exact cell partitions; a read that
// delivers anything else has no partition to cross and is named rather than
// silently dropped from the chain.
func TestAnExactFoldRefusesANonExactRead(t *testing.T) {
	spec := exactProductSpec(2)
	spec.Program.Joins[1].Read.Form = program.Summary
	_, err := Render(Target{
		PackagePath: consumerPackage, PackageName: "consumer", Spec: spec,
	}, exactProductRoster(t, 2))
	if err == nil {
		t.Fatal("a summary read was admitted into an exact product")
	}
	if !strings.Contains(err.Error(), "an exact fold beside a Summary read") {
		t.Fatalf("refusal does not name the clause: %v", err)
	}
}
