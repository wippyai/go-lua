package programsupply

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/binder"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/programlaw"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/staticlaw"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/semanticsource"
	"github.com/wippyai/go-lua/analysis/schema/relations"
)

func TestGeneratedEvidenceIsCurrent(t *testing.T) {
	if err := Generate(filepath.Join(packageDir(t), "evidence_gen.go"), true); err != nil {
		t.Fatal(err)
	}
	current, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(current.ProgramLaws) != len(programlaw.Requirements()) ||
		len(current.StaticLaws) != len(staticlaw.Requirements()) ||
		len(current.BinderLaws) != len(binder.Required()) {
		t.Fatal("generated Program supply does not cover the exact typed denominators")
	}
}

func TestEvidenceRejectsTypedDenominatorMutations(t *testing.T) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*Evidence)
	}{
		{name: "missing-program-first", mutate: func(e *Evidence) { e.ProgramLaws = e.ProgramLaws[1:] }},
		{name: "missing-static-middle", mutate: func(e *Evidence) {
			middle := len(e.StaticLaws) / 2
			e.StaticLaws = append(e.StaticLaws[:middle:middle], e.StaticLaws[middle+1:]...)
		}},
		{name: "missing-binder-last", mutate: func(e *Evidence) { e.BinderLaws = e.BinderLaws[:len(e.BinderLaws)-1] }},
		{name: "extra", mutate: func(e *Evidence) { e.StaticLaws = append(e.StaticLaws, e.StaticLaws[0]) }},
		{name: "reordered", mutate: func(e *Evidence) { e.ProgramLaws[0], e.ProgramLaws[1] = e.ProgramLaws[1], e.ProgramLaws[0] }},
		{name: "terminal-missing", mutate: func(e *Evidence) {
			row := staticIndex(t, e.StaticLaws, staticlaw.FamilySignature)
			e.StaticLaws[row].Terminals = e.StaticLaws[row].Terminals[:1]
		}},
		{name: "terminal-extra", mutate: func(e *Evidence) {
			e.StaticLaws[0].Terminals = append(e.StaticLaws[0].Terminals, e.StaticLaws[0].Terminals[0])
		}},
		{name: "terminal-reordered", mutate: func(e *Evidence) {
			row := staticIndex(t, e.StaticLaws, staticlaw.FamilySignature)
			e.StaticLaws[row].Terminals[0], e.StaticLaws[row].Terminals[1] = e.StaticLaws[row].Terminals[1], e.StaticLaws[row].Terminals[0]
		}},
		{name: "wrong-terminal", mutate: func(e *Evidence) { e.ProgramLaws[0].Terminals = append([]Reference(nil), e.StaticLaws[0].Terminals...) }},
		{name: "wrong-terminal-revision", mutate: func(e *Evidence) { e.ProgramLaws[0].Terminals[0].Revision++ }},
		{name: "wrong-tag", mutate: func(e *Evidence) { e.ProgramLaws[0].Requirement.Site = programlaw.SiteCall }},
		{name: "wrong-static-family", mutate: func(e *Evidence) { e.StaticLaws[0].Family = staticlaw.FamilyAnnotated }},
		{name: "negative-made-positive", mutate: func(e *Evidence) {
			row := binderIndex(t, e.BinderLaws, binder.TransitionRuntimeShadowRejected)
			e.BinderLaws[row].Positive = e.BinderLaws[row].Forbidden
			e.BinderLaws[row].Forbidden = nil
		}},
		{name: "negative-terminal-missing", mutate: func(e *Evidence) {
			row := binderIndex(t, e.BinderLaws, binder.TransitionRuntimeShadowRejected)
			e.BinderLaws[row].Forbidden = e.BinderLaws[row].Forbidden[1:]
		}},
		{name: "both-polarities", mutate: func(e *Evidence) {
			row := binderIndex(t, e.BinderLaws, binder.TransitionRuntimeShadowRejected)
			e.BinderLaws[row].Positive = append([]Reference(nil), e.BinderLaws[row].Forbidden...)
		}},
		{name: "neither-polarity", mutate: func(e *Evidence) {
			row := binderIndex(t, e.BinderLaws, binder.TransitionTypeDeclaration)
			e.BinderLaws[row].Positive = nil
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			changed := clone(evidence)
			test.mutate(&changed)
			changed.Digest = resign(t, changed)
			if err := changed.Validate(schema); err == nil {
				t.Fatal("mutated Program supply evidence was accepted")
			}
		})
	}
}

func TestCanonicalEvidenceIsDeterministicAndOwnsDigest(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	first := evidence.Canonical()
	second := clone(evidence).Canonical()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("equivalent Program supply evidence has nondeterministic canonical bytes")
	}
	sum := sha256.Sum256(first)
	if got := hex.EncodeToString(sum[:]); got != evidence.Digest {
		t.Fatalf("evidence digest = %s, want SHA-256 of canonical bytes %s", evidence.Digest, got)
	}
	changedDigestField := clone(evidence)
	changedDigestField.Digest = "not canonical content"
	if !bytes.Equal(first, changedDigestField.Canonical()) {
		t.Fatal("self-reported digest changed package-owned canonical content")
	}
}

func TestCanonicalEvidenceChangesForSemanticMutation(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	changed := clone(evidence)
	changed.ProgramLaws[0].Terminals[0].Revision++
	if bytes.Equal(evidence.Canonical(), changed.Canonical()) {
		t.Fatal("semantic terminal mutation preserved canonical bytes")
	}
	changed = clone(evidence)
	changed.BinderLaws[0].Requirement.Transition++
	if bytes.Equal(evidence.Canonical(), changed.Canonical()) {
		t.Fatal("typed requirement mutation preserved canonical bytes")
	}
}

func TestCanonicalEvidenceRejectsMalformedIdentity(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	evidence.SchemaDigest = "not-a-sha256"
	if encoded := evidence.Canonical(); encoded != nil {
		t.Fatalf("malformed evidence canonical bytes = %x, want nil", encoded)
	}
	if _, err := evidenceDigest(evidence); err == nil {
		t.Fatal("malformed evidence produced a digest")
	}
}

func TestCanonicalEvidenceSnapshotsAreDetached(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	first := evidence.Canonical()
	want := append([]byte(nil), first...)
	first[0] ^= 0xff
	if got := evidence.Canonical(); !bytes.Equal(got, want) {
		t.Fatal("mutating returned canonical bytes changed Evidence")
	}
}

func TestAnnotationTerminalAndSchemaClosure(t *testing.T) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	var terminals []Reference
	for _, row := range evidence.StaticLaws {
		if row.Family == staticlaw.FamilyAnnotated {
			terminals = row.Terminals
			break
		}
	}
	annotation := schemaReference(t, schema, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation)
	values := schemaReference(t, schema, semanticsource.OriginProgramFlowValues, 0)
	static := schemaReference(t, schema, semanticsource.OriginProgramStatic, 0)
	typeRef := schemaReference(t, schema, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef)
	if len(terminals) != 1 || terminals[0] != annotation {
		t.Fatalf("annotated terminal vector = %#v, want only %#v", terminals, annotation)
	}

	var direct []Reference
	for _, row := range schema.Rows() {
		if reference(row.Definition.Token()) != annotation {
			continue
		}
		for _, parent := range row.Parents {
			direct = append(direct, reference(parent))
		}
	}
	if len(direct) != 2 || !hasReference(direct, values) || !hasReference(direct, typeRef) {
		t.Fatalf("annotation direct parents = %#v, want Values and TypeRef", direct)
	}

	closure, err := Closure(schema, terminals)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []Reference{annotation, values, static} {
		if !hasOutput(closure, required) {
			t.Fatalf("annotation closure omitted %#v", required)
		}
	}
	seen := make(map[Reference]bool, len(closure))
	for _, output := range closure {
		if seen[output.Relation] {
			t.Fatalf("annotation closure repeated %#v", output.Relation)
		}
		seen[output.Relation] = true
		if output.Owner < relations.OwnerProgramSource || output.Owner > relations.OwnerProgramModule || output.Form == relations.FormUnset {
			t.Fatalf("annotation closure has non-Program schema projection %#v", output)
		}
	}
}

func TestExactTypedTerminalVectors(t *testing.T) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	ref := func(origin semanticsource.Origin, facet semanticsource.Facet) Reference {
		return schemaReference(t, schema, origin, facet)
	}
	for _, row := range evidence.ProgramLaws {
		var want []Reference
		switch row.Requirement.Site {
		case programlaw.SiteUnary:
			switch row.Requirement.Unary {
			case flowkind.UnaryNeg, flowkind.UnaryBitNot:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric)}
			case flowkind.UnaryLen:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength)}
			case flowkind.UnaryNot:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, 0)}
			default:
				t.Fatalf("unexpected unary operation %d", row.Requirement.Unary)
			}
		case programlaw.SiteBinary:
			switch row.Requirement.Binary {
			case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul, flowkind.BinaryDiv,
				flowkind.BinaryIDiv, flowkind.BinaryMod, flowkind.BinaryPow:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic)}
			case flowkind.BinaryConcat:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat)}
			case flowkind.BinaryBitAnd, flowkind.BinaryBitOr, flowkind.BinaryBitXor,
				flowkind.BinaryShiftLeft, flowkind.BinaryShiftRight:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise)}
			case flowkind.BinaryEqual, flowkind.BinaryNotEqual:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality)}
			case flowkind.BinaryLess, flowkind.BinaryLessEqual, flowkind.BinaryGreater, flowkind.BinaryGreaterEqual:
				want = []Reference{ref(semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder)}
			default:
				t.Fatalf("unexpected binary operation %d", row.Requirement.Binary)
			}
		case programlaw.SiteSelect:
			want = []Reference{ref(semanticsource.OriginProgramFlowOperators, 0)}
		case programlaw.SiteCall:
			want = []Reference{ref(semanticsource.OriginProgramFlowCall, 0)}
		case programlaw.SiteValues:
			want = []Reference{ref(semanticsource.OriginProgramFlowValues, 0)}
		case programlaw.SiteOutcome:
			want = []Reference{ref(semanticsource.OriginProgramFlowOutcome, 0)}
		default:
			t.Fatalf("unexpected Program site %d", row.Requirement.Site)
		}
		assertReferences(t, row.Terminals, want)
		assertCanonicalClosure(t, schema, row.Terminals)
	}
	for _, row := range evidence.StaticLaws {
		want := []Reference{ref(semanticsource.OriginProgramStatic, 0)}
		switch row.Family {
		case staticlaw.FamilyTypeRef:
			want = []Reference{ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef)}
		case staticlaw.FamilySignature:
			want = []Reference{
				ref(semanticsource.OriginProgramStatic, 0),
				ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticFunctionContract),
			}
		case staticlaw.FamilyAssertion:
			want = []Reference{
				ref(semanticsource.OriginProgramStatic, 0),
				ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticClaimTarget),
			}
		case staticlaw.FamilyTypeOf:
			want = []Reference{ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeof)}
		case staticlaw.FamilyAnnotated:
			want = []Reference{ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation)}
		}
		assertReferences(t, row.Terminals, want)
		assertCanonicalClosure(t, schema, row.Terminals)
	}
	for _, row := range evidence.BinderLaws {
		var positive, forbidden []Reference
		switch row.Requirement.Transition {
		case binder.TransitionTypeDeclaration:
			positive = []Reference{ref(semanticsource.OriginProgramStatic, 0)}
		case binder.TransitionTypeParameter, binder.TransitionUnresolvedTypeReference, binder.TransitionQualifiedTypeRoot:
			positive = []Reference{ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef)}
		case binder.TransitionRuntimePrimitive:
			positive = []Reference{
				ref(semanticsource.OriginProgramStatic, 0),
				ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget),
			}
		case binder.TransitionRuntimeDeclaration:
			positive = []Reference{
				ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget),
				ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef),
			}
		case binder.TransitionRuntimeShadowRejected:
			forbidden = []Reference{
				ref(semanticsource.OriginProgramFlowTypeValue, 0),
				ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget),
			}
		case binder.TransitionStaticPublicationPair:
			positive = []Reference{ref(semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication)}
		case binder.TransitionDirectRequireGlobal:
			positive = []Reference{ref(semanticsource.OriginProgramModuleImport, 0)}
		default:
			t.Fatalf("unexpected binder transition %d", row.Requirement.Transition)
		}
		assertReferences(t, row.Positive, positive)
		assertReferences(t, row.Forbidden, forbidden)
		if positive != nil {
			assertCanonicalClosure(t, schema, row.Positive)
		} else {
			assertCanonicalClosure(t, schema, row.Forbidden)
		}
	}
}

func TestForbiddenBinderTerminalHasSchemaDerivedClosure(t *testing.T) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	row := evidence.BinderLaws[binderIndex(t, evidence.BinderLaws, binder.TransitionRuntimeShadowRejected)]
	if len(row.Positive) != 0 || len(row.Forbidden) != 2 {
		t.Fatalf("runtime shadow polarity = %#v", row)
	}
	closure, err := Closure(schema, row.Forbidden)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []Reference{
		schemaReference(t, schema, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeValueTarget),
		schemaReference(t, schema, semanticsource.OriginProgramFlowTypeValue, 0),
	} {
		if !hasOutput(closure, required) {
			t.Fatalf("forbidden runtime type-value closure omitted %#v", required)
		}
	}
}

func TestClosureRejectsUnknownAndEmptyTerminals(t *testing.T) {
	schema, err := relations.CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	for _, terminals := range [][]Reference{nil, {{Origin: semanticsource.OriginProgramStatic, Revision: 99}}} {
		if _, err := Closure(schema, terminals); err == nil {
			t.Fatal("invalid closure terminal vector was accepted")
		}
	}
}

func binderIndex(t *testing.T, rows []BinderRow, transition binder.Transition) int {
	t.Helper()
	for index, row := range rows {
		if row.Requirement.Transition == transition {
			return index
		}
	}
	t.Fatalf("missing binder transition %d", transition)
	return -1
}

func staticIndex(t *testing.T, rows []StaticLawRow, family staticlaw.Family) int {
	t.Helper()
	for index, row := range rows {
		if row.Family == family {
			return index
		}
	}
	t.Fatalf("missing static family %d", family)
	return -1
}

func schemaReference(t *testing.T, schema *relations.Schema, origin semanticsource.Origin, facet semanticsource.Facet) Reference {
	t.Helper()
	for _, row := range schema.Rows() {
		token := row.Definition.Token()
		if token.Origin() == origin && token.Facet() == facet {
			return reference(token)
		}
	}
	t.Fatalf("missing schema reference origin=%d facet=%d", origin, facet)
	return Reference{}
}

func hasReference(rows []Reference, want Reference) bool {
	for _, row := range rows {
		if row == want {
			return true
		}
	}
	return false
}

func hasOutput(rows []Output, want Reference) bool {
	for _, row := range rows {
		if row.Relation == want {
			return true
		}
	}
	return false
}

func assertReferences(t *testing.T, got, want []Reference) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("terminal vector = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("terminal vector = %#v, want %#v", got, want)
		}
	}
}

func assertCanonicalClosure(t *testing.T, schema *relations.Schema, terminals []Reference) {
	t.Helper()
	got, err := Closure(schema, terminals)
	if err != nil {
		t.Fatal(err)
	}
	rows := schema.Rows()
	byReference := make(map[Reference]relations.Row, len(rows))
	for _, row := range rows {
		byReference[reference(row.Definition.Token())] = row
	}
	stack := append([]Reference(nil), terminals...)
	seen := make(map[Reference]bool)
	for len(stack) != 0 {
		last := len(stack) - 1
		item := stack[last]
		stack = stack[:last]
		if seen[item] {
			continue
		}
		row, ok := byReference[item]
		if !ok {
			t.Fatalf("terminal closure fixture omitted %#v", item)
		}
		seen[item] = true
		for _, parent := range row.Parents {
			stack = append(stack, reference(parent))
		}
	}
	if len(got) != len(seen) {
		t.Fatalf("closure length = %d, want %d", len(got), len(seen))
	}
	for index, output := range got {
		row, ok := byReference[output.Relation]
		if !ok || !seen[output.Relation] || output.Owner != row.Owner || output.Form != row.Form {
			t.Fatalf("closure output %d = %#v", index, output)
		}
		if index != 0 && !less(got[index-1].Relation, output.Relation) {
			t.Fatalf("closure is not canonically ordered at %d", index)
		}
	}
}

func resign(t *testing.T, evidence Evidence) string {
	t.Helper()
	digest, err := evidenceDigest(evidence)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate Program supply evidence test")
	}
	return filepath.Dir(file)
}
