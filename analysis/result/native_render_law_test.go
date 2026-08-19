package result

import (
	"math"
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/composite"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// TestNativePublicationColumnSpellingsAreDeclaredLaw states that every column
// vocabulary a native row publishes is declared on the structural surface, and
// that the ordinal a published column carries resolves to the intended member
// there. The publication holds no spelling of its own, so a renderer reads the
// declared name and a consumer comparing a column against authored text
// compares against the same declaration.
func TestNativePublicationColumnSpellingsAreDeclaredLaw(t *testing.T) {
	vocabulary, vocabularyOK := composite.StructureVocabulary()
	if !vocabularyOK {
		t.Fatal("sealed structural vocabulary unavailable")
	}
	spelled := func(category structure.Category, ordinal uint16) string {
		member, memberOK := vocabulary.At(category, ordinal)
		if !memberOK {
			t.Fatalf("category %d declares no member at ordinal %d", category, ordinal)
		}
		resolved, resolvedOK := vocabulary.Spelling(category, member.Spelling())
		if !resolvedOK || resolved != ordinal {
			t.Fatalf("category %d spelling %q resolves to ordinal %d, want %d", category, member.Spelling(), resolved, ordinal)
		}
		return member.Spelling()
	}
	// The pinned categories: the published ordinal is the owner's own ordinal,
	// so a member added to one side and not the other is a verdict here.
	for representation, spelling := range map[programschema.NumericRepresentation]string{
		programschema.NumericRepresentationInteger: "integer",
		programschema.NumericRepresentationFloat:   "float",
		programschema.NumericRepresentationNumber:  "number",
	} {
		if declared := spelled(structure.CategoryNativeNumericRepresentation, uint16(representation)); declared != spelling {
			t.Fatalf("numeric representation %d is declared %q, want %q", representation, declared, spelling)
		}
	}
	if vocabulary.Count(structure.CategoryNativeNumericRepresentation) != int(programschema.NumericRepresentationNumber) {
		t.Fatal("the declared numeric carrier vocabulary is not the Program's own vocabulary")
	}
	for kind, spelling := range map[keyspace.LiteralKind]string{
		keyspace.LiteralBool:    "boolean",
		keyspace.LiteralInteger: "integer",
		keyspace.LiteralFloat:   "float",
		keyspace.LiteralString:  "string",
	} {
		representation, representationOK := nativeScalarRepresentationOf(kind)
		if !representationOK {
			t.Fatalf("literal kind %d has no published carrier", kind)
		}
		if declared := spelled(structure.CategoryNativeScalarRepresentation, representation.Ordinal()); declared != spelling {
			t.Fatalf("scalar carrier %d is declared %q, want %q", representation, declared, spelling)
		}
	}
	if declared := spelled(structure.CategoryNativeScalarRepresentation, NativeScalarRepresentationNil.Ordinal()); declared != "nil" {
		t.Fatalf("the nil carrier is declared %q, want nil", declared)
	}
	for op, spelling := range map[flowkind.BinaryOp]string{
		flowkind.BinaryAdd:  "add",
		flowkind.BinarySub:  "sub",
		flowkind.BinaryMul:  "mul",
		flowkind.BinaryDiv:  "div",
		flowkind.BinaryIDiv: "idiv",
		flowkind.BinaryMod:  "mod",
		flowkind.BinaryPow:  "pow",
	} {
		if declared := spelled(structure.CategoryNativeArithmeticOperator, uint16(op)); declared != spelling {
			t.Fatalf("arithmetic operator %d is declared %q, want %q", op, declared, spelling)
		}
	}
	if vocabulary.Count(structure.CategoryNativeArithmeticOperator) != int(flowkind.BinaryPow) {
		t.Fatal("the declared arithmetic operator vocabulary is not Flow's own arithmetic segment")
	}
	if declared := spelled(structure.CategoryNativeUnaryOperator, uint16(flowkind.UnaryNeg)); declared != "unm" {
		t.Fatalf("the unary negation operator is declared %q, want unm", declared)
	}
	for overflow, spelling := range map[valuedomain.NumericOverflow]string{
		valuedomain.NumericOverflowClosedInteger:          "closed_integer",
		valuedomain.NumericOverflowPromoteIntegerToNumber: "promote_integer_to_number",
		valuedomain.NumericOverflowIEEE754:                "ieee754",
	} {
		if declared := spelled(structure.CategoryNativeNumericOverflow, uint16(overflow)); declared != spelling {
			t.Fatalf("overflow discipline %d is declared %q, want %q", overflow, declared, spelling)
		}
	}
	for property, spelling := range map[programschema.ArithmeticDivisorProperty]string{
		programschema.ArithmeticDivisorNonzero:            "nonzero",
		programschema.ArithmeticDivisorNonzeroNotMinusOne: "nonzero_not_minus_one",
	} {
		published, publishedOK := nativeDivisorPropertyOf(property)
		if !publishedOK || !published.Available() {
			t.Fatalf("divisor proof %d has no published property", property)
		}
		if declared := spelled(structure.CategoryNativeDivisorProperty, published.Ordinal()); declared != spelling {
			t.Fatalf("divisor property %d is declared %q, want %q", property, declared, spelling)
		}
	}
	if declared := spelled(structure.CategoryNativeDivisorProperty, NativeDivisorPropertyNotApplicable.Ordinal()); declared != "not_applicable" {
		t.Fatalf("the inapplicable divisor property is declared %q, want not_applicable", declared)
	}
	// The publication's own vocabularies: their members exist nowhere else, so
	// this surface declares them and holds no second spelling.
	for class, spelling := range map[NativeTruthinessClass]string{
		NativeTruthinessClassAlwaysTruthy: "always_truthy",
		NativeTruthinessClassAlwaysFalsy:  "always_falsy",
		NativeTruthinessClassDynamic:      "dynamic_nil_or_false",
		NativeTruthinessClassUnobserved:   "unobserved",
	} {
		if declared := spelled(structure.CategoryNativeTruthinessClass, class.Ordinal()); declared != spelling {
			t.Fatalf("truthiness class %d is declared %q, want %q", class, declared, spelling)
		}
	}
	for partition, spelling := range map[NativeBranchPartition]string{
		NativeBranchPartitionAlwaysTaken:    "always_taken",
		NativeBranchPartitionAlwaysNotTaken: "always_not_taken",
		NativeBranchPartitionDynamic:        "dynamic",
		NativeBranchPartitionUnobserved:     "unobserved",
	} {
		if declared := spelled(structure.CategoryNativeBranchPartition, partition.Ordinal()); declared != spelling {
			t.Fatalf("branch partition %d is declared %q, want %q", partition, declared, spelling)
		}
	}
	for arm, spelling := range map[NativeBranchArm]string{
		NativeBranchArmThen: "then",
		NativeBranchArmElse: "else",
	} {
		if declared := spelled(structure.CategoryNativeBranchArm, arm.Ordinal()); declared != spelling {
			t.Fatalf("branch arm %d is declared %q, want %q", arm, declared, spelling)
		}
	}
}

// TestNativeExactScalarPublishesEveryProvedBitPatternLaw states that a proved
// exact scalar is published whatever bit pattern it holds. A Lua float
// division by zero is a proved constant, so an infinity or a NaN carries the
// same publication a finite constant does: the bits are the fact, and no value
// the analyzer proves exactly may withhold the publication it belongs to.
func TestNativeExactScalarPublishesEveryProvedBitPatternLaw(t *testing.T) {
	for _, bits := range []uint64{
		math.Float64bits(math.Inf(1)),
		math.Float64bits(math.Inf(-1)),
		math.Float64bits(math.NaN()),
		math.Float64bits(math.Copysign(0, -1)),
		math.Float64bits(42),
	} {
		summary, summaryOK := programschema.NewExactScalarSummary(
			nativeLawID(t, "occurrence"), nativeLawID(t, "subject"), nativeLawID(t, "body"),
			programschema.ExactScalarSummaryResult,
			programschema.SummaryLiteral{Kind: uint8(keyspace.LiteralFloat), FloatBits: bits},
		)
		if !summaryOK {
			t.Fatalf("float bits %#x produced no Program summary", bits)
		}
		rows := make([]nativePublicationRow, 0, 2)
		seen := make(map[identity.ContentID]struct{})
		mount, artifact, point := nativeLawID(t, "mount"), nativeLawID(t, "artifact"), nativeLawID(t, "point")
		if !appendNativeStaticScalarRows(&rows, seen, summary, mount, artifact, nativeLawID(t, "body"), point) {
			t.Fatalf("float bits %#x withheld the whole native publication", bits)
		}
		if len(rows) != 2 {
			t.Fatalf("float bits %#x published %d rows, want the constant and its carrier", bits, len(rows))
		}
		published := rows[0].content.literal
		if rows[0].family != nativePublicationFamilyConstantValue || published.Kind != keyspace.LiteralFloat || published.FloatBits != bits {
			t.Fatalf("float bits %#x published as %#x", bits, published.FloatBits)
		}
		if rows[0].content.scalar != NativeScalarRepresentationFloat || !rows[1].content.exact {
			t.Fatalf("float bits %#x published carrier %d exact=%t", bits, rows[0].content.scalar, rows[1].content.exact)
		}
	}
}

// TestNativeBranchVerdictSeparatesProvedDynamicFromUnobservedLaw states that a
// condition proved to take both truths and a condition whose evidence set was
// not read out at every point are two answers, not one. A consumer that
// licenses a runtime decision on the proof must never receive the absence of a
// proof spelled the same way.
func TestNativeBranchVerdictSeparatesProvedDynamicFromUnobservedLaw(t *testing.T) {
	dynamicClass, dynamicPartition, dynamicArm, dynamicProved := nativeBranchVerdict(valuedomain.TruthTrue|valuedomain.TruthFalse, true)
	unobservedClass, unobservedPartition, unobservedArm, unobservedProved := nativeBranchVerdict(valuedomain.TruthTrue, false)
	if dynamicClass != NativeTruthinessClassDynamic || unobservedClass != NativeTruthinessClassUnobserved || dynamicClass == unobservedClass {
		t.Fatalf("proved-dynamic classifies as %d and the incomplete fold as %d", dynamicClass, unobservedClass)
	}
	if dynamicPartition != NativeBranchPartitionDynamic || unobservedPartition != NativeBranchPartitionUnobserved {
		t.Fatalf("proved-dynamic partitions as %d and the incomplete fold as %d", dynamicPartition, unobservedPartition)
	}
	if dynamicProved || unobservedProved || dynamicArm.Available() || unobservedArm.Available() {
		t.Fatal("a condition with no proved partition named a dead arm")
	}
	subject := nativeLawBranchObservation(t)
	body := nativeLawID(t, "body")
	dynamicRows, unobservedRows := make([]nativePublicationRow, 0, 2), make([]nativePublicationRow, 0, 2)
	if !appendNativeBranchRows(&dynamicRows, make(map[identity.ContentID]struct{}), subject, body, valuedomain.TruthTrue|valuedomain.TruthFalse, true) ||
		!appendNativeBranchRows(&unobservedRows, make(map[identity.ContentID]struct{}), subject, body, valuedomain.TruthTrue, false) {
		t.Fatal("a branch condition published no verdict")
	}
	if len(dynamicRows) != 2 || len(unobservedRows) != 2 {
		t.Fatalf("branch verdicts published %d and %d rows, want two each", len(dynamicRows), len(unobservedRows))
	}
	for index := range dynamicRows {
		if dynamicRows[index].family != unobservedRows[index].family {
			t.Fatal("the two verdicts published different families")
		}
		if dynamicRows[index].id == unobservedRows[index].id {
			t.Fatalf("family %s publishes one identity for a proved-dynamic condition and an unobserved one", dynamicRows[index].family)
		}
	}
	// The evidence set is the row's content, not one representative of it.
	for _, row := range dynamicRows {
		if len(row.content.points) != len(subject.Points) {
			t.Fatalf("family %s published %d evidence points of %d", row.family, len(row.content.points), len(subject.Points))
		}
	}
}

// TestNativePublicationIdentityIsItsTypedContentLaw states the row identity
// law: two rows are the same row exactly when their typed columns agree. The
// identity is derived from the columns themselves, so it cannot depend on how
// any of them is spelled, and a column that changes changes the identity.
func TestNativePublicationIdentityIsItsTypedContentLaw(t *testing.T) {
	base := nativeLawArithmeticRow(t)
	same := nativeLawArithmeticRow(t)
	baseID, baseOK := nativePublicationRowID(base)
	sameID, sameOK := nativePublicationRowID(same)
	if !baseOK || !sameOK || baseID != sameID {
		t.Fatal("two rows with equal typed content carry different identities")
	}
	for name, mutate := range map[string]func(*nativePublicationContent){
		"exact": func(content *nativePublicationContent) { content.exact = !content.exact },
		"left":  func(content *nativePublicationContent) { content.left = programschema.NumericRepresentationFloat },
		"right": func(content *nativePublicationContent) { content.right = programschema.NumericRepresentationFloat },
		"result": func(content *nativePublicationContent) {
			content.representation = programschema.NumericRepresentationFloat
		},
		"operator":   func(content *nativePublicationContent) { content.binary = flowkind.BinarySub },
		"overflow":   func(content *nativePublicationContent) { content.overflow = valuedomain.NumericOverflowIEEE754 },
		"divisor":    func(content *nativePublicationContent) { content.divisor = NativeDivisorPropertyNonzero },
		"truthiness": func(content *nativePublicationContent) { content.truthiness = NativeTruthinessClassDynamic },
		"partition":  func(content *nativePublicationContent) { content.partition = NativeBranchPartitionDynamic },
		"dead arm":   func(content *nativePublicationContent) { content.deadArm = NativeBranchArmThen },
		"reachable":  func(content *nativePublicationContent) { content.deadArmReachable = true },
		"literal": func(content *nativePublicationContent) {
			content.literal = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 7}
		},
		"float bits": func(content *nativePublicationContent) {
			content.literal = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Inf(1))}
		},
		"scalar": func(content *nativePublicationContent) { content.scalar = NativeScalarRepresentationString },
		"text": func(content *nativePublicationContent) {
			content.literal = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "a"}
		},
		"evidence": func(content *nativePublicationContent) {
			content.points = append(content.points, nativeLawID(t, "second point"))
		},
		"operand": func(content *nativePublicationContent) { content.operand = programschema.NumericRepresentationInteger },
		"unary":   func(content *nativePublicationContent) { content.unary = flowkind.UnaryNeg },
	} {
		changed := nativeLawArithmeticRow(t)
		mutate(&changed.content)
		changedID, changedOK := nativePublicationRowID(changed)
		if !changedOK || changedID == baseID {
			t.Fatalf("changing the %s column left the row identity unchanged", name)
		}
	}
	// The literal's exact bits are content: two float constants that are equal
	// as numbers but distinct as bit patterns are distinct rows.
	positive, negative := nativeLawArithmeticRow(t), nativeLawArithmeticRow(t)
	positive.content.literal = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(0)}
	negative.content.literal = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Copysign(0, -1))}
	positiveID, _ := nativePublicationRowID(positive)
	negativeID, _ := nativePublicationRowID(negative)
	if positiveID == negativeID {
		t.Fatal("positive and negative zero published one identity")
	}
}

func nativeLawArithmeticRow(t *testing.T) nativePublicationRow {
	t.Helper()
	points, pointsOK := nativeEvidencePoints(nativeLawID(t, "point"))
	if !pointsOK {
		t.Fatal("law evidence set unavailable")
	}
	return nativePublicationRow{
		semantic: nativeLawID(t, "semantic"),
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue,
		family: nativePublicationFamilyScalarOperator, trust: NativePublicationTrustProven,
		key: "law", module: "law", term: "law", subject: "law", occurrence: "law",
		content: nativePublicationContent{
			representation: programschema.NumericRepresentationInteger,
			left:           programschema.NumericRepresentationInteger,
			right:          programschema.NumericRepresentationInteger,
			binary:         flowkind.BinaryAdd,
			overflow:       valuedomain.NumericOverflowPromoteIntegerToNumber,
			points:         points,
		},
		provenance: NativePublicationProvenance{
			mount: nativeLawID(t, "mount"), artifact: nativeLawID(t, "artifact"), local: nativeLawID(t, "local"),
			body: nativeLawID(t, "body"), point: nativeLawID(t, "point"), span: nativeLawID(t, "span"),
		},
		provenanceOK: true, validityOK: true,
	}
}

func nativeLawBranchObservation(t *testing.T) anadiag.Observation {
	t.Helper()
	return anadiag.Observation{
		ID:       nativeLawID(t, "observation"),
		Mount:    nativeLawID(t, "mount"),
		Artifact: nativeLawID(t, "artifact"),
		Local:    nativeLawID(t, "local"),
		Kind:     structure.DiagnosticObservationBranchCondition,
		Location: programsource.Span{File: "main.lua", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2},
		Branch: anadiag.Branch{
			Points: []identity.ContentID{nativeLawID(t, "point one"), nativeLawID(t, "point two")},
		},
	}
}

func nativeLawID(t *testing.T, name string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/native-publication/law/v1", []byte(name))
	if !ok {
		t.Fatalf("law identity %q unavailable", name)
	}
	return id
}
