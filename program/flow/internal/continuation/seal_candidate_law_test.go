package continuation

import (
	"testing"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func TestContinuationSealCandidateFamiliesAndEndpointAdmission(t *testing.T) {
	fixture := openContinuationFixture(t, continuationCandidateSpec())
	global := continuationTerm(keyspace.FamilyCell, 2)
	endpoints := continuationEndpointTerms(fixture)
	assertContinuationSubject(t, fixture.result, continuationTerm(keyspace.FamilyUnary, 2), true)
	assertContinuationSubject(t, fixture.result, continuationTerm(keyspace.FamilyUnary, 4), true)
	for ordinal := uint32(1); ordinal <= 19; ordinal++ {
		assertContinuationSubject(t, fixture.result, continuationTerm(keyspace.FamilyBinary, ordinal), true)
	}
	for _, ordinal := range []uint32{1, 3} {
		assertContinuationSubject(t, fixture.result, continuationTerm(keyspace.FamilyRead, ordinal), true)
	}
	for _, ordinal := range []uint32{1, 2} {
		assertContinuationSubject(t, fixture.result, continuationTerm(keyspace.FamilyWrite, ordinal), true)
	}
	for _, term := range []keyspace.Term{
		continuationTerm(keyspace.FamilyUnary, 1), // static exact-key negative
		continuationTerm(keyspace.FamilyUnary, 3), // static boolean Not
		continuationTerm(keyspace.FamilyUnary, 5), // dead after Return
		continuationTerm(keyspace.FamilyRead, 2),  // static exact read
		continuationTerm(keyspace.FamilyRead, 4),  // global read excluded from candidates
		continuationTerm(keyspace.FamilyWrite, 3), // global write excluded
		continuationTerm(keyspace.FamilySelect, 1),
		continuationTerm(keyspace.FamilyValues, 1),
	} {
		if _, ok := fixture.result.CellCount(term); ok {
			t.Fatalf("non-subject/static/dead Term %08x entered Cell plane", uint32(term))
		}
		_, guardOK := fixture.result.GuardCount(term)
		_, endpoint := endpoints[term]
		if guardOK != endpoint {
			t.Fatalf("candidate Term %08x Guard endpoint admission = %v, want %v", uint32(term), guardOK, endpoint)
		}
	}
	foreignOutcome := continuationTerm(keyspace.FamilyOutcome, uint32(fixture.sourceView.Identity().FamilyCount(keyspace.FamilyOutcome))+1)
	for _, malformed := range []keyspace.Term{0, continuationTerm(keyspace.FamilyValues, 6), foreignOutcome} {
		if _, ok := fixture.result.GuardCount(malformed); ok {
			t.Fatalf("malformed/non-vertex Term %08x entered Guard plane", uint32(malformed))
		}
	}
	for _, subject := range []keyspace.Term{continuationTerm(keyspace.FamilyUnary, 2), continuationTerm(keyspace.FamilyBinary, 1), continuationTerm(keyspace.FamilyRead, 1), continuationTerm(keyspace.FamilyWrite, 1)} {
		count, ok := fixture.result.CellCount(subject)
		if !ok {
			t.Fatalf("admitted subject %08x lost Cell root", uint32(subject))
		}
		for index := 0; index < count; index++ {
			cell, cellOK := fixture.result.CellAt(subject, index)
			if !cellOK || cell == global {
				t.Fatalf("global Cell %08x entered subject %08x scope at %d", uint32(global), uint32(subject), index)
			}
		}
	}
}

func continuationEndpointTerms(fixture *continuationFixture) map[keyspace.Term]struct{} {
	endpoints := make(map[keyspace.Term]struct{})
	for index := 0; index < fixture.causal.SiteCount(); index++ {
		site, ok := fixture.causal.SiteAt(index)
		if !ok {
			continue
		}
		term, ok := site.Term()
		if ok {
			endpoints[term] = struct{}{}
		}
	}
	return endpoints
}

func assertContinuationSubject(t *testing.T, result *Result, term keyspace.Term, want bool) {
	t.Helper()
	_, got := result.CellCount(term)
	if got != want {
		t.Fatalf("subject admission %08x = %v, want %v", uint32(term), got, want)
	}
	if want {
		if count, ok := result.CellCount(term); !ok || count < 0 {
			t.Fatalf("admitted subject Cell root %08x = %d/%v", uint32(term), count, ok)
		}
		if count, ok := result.GuardCount(term); !ok || count < 0 {
			t.Fatalf("admitted subject Guard root %08x = %d/%v", uint32(term), count, ok)
		}
	}
}

func continuationCandidateSpec() continuationSpec {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 57, keyspace.FamilyInteger: 1,
		keyspace.FamilyKey: 2, keyspace.FamilyValues: 5, keyspace.FamilyLensExact: 3,
		keyspace.FamilyLensKey: 2, keyspace.FamilyCell: 2, keyspace.FamilyRead: 4,
		keyspace.FamilyBind: 1, keyspace.FamilyAssign: 3, keyspace.FamilyWrite: 3,
		keyspace.FamilyUnary: 5, keyspace.FamilyBinary: 19, keyspace.FamilySelect: 2,
		keyspace.FamilyReturn: 1, keyspace.FamilyTypeOf: 1,
	}
	term := continuationTerm
	body := term(keyspace.FamilyBody, 1)
	integer := term(keyspace.FamilyInteger, 1)
	keys := []keyspace.Term{term(keyspace.FamilyKey, 1), term(keyspace.FamilyKey, 2)}
	exact := []keyspace.Term{term(keyspace.FamilyLensExact, 1), term(keyspace.FamilyLensExact, 2), term(keyspace.FamilyLensExact, 3)}
	dynamic := []keyspace.Term{term(keyspace.FamilyLensKey, 1), term(keyspace.FamilyLensKey, 2)}
	localCell, globalCell := term(keyspace.FamilyCell, 1), term(keyspace.FamilyCell, 2)
	reads := []keyspace.Term{term(keyspace.FamilyRead, 1), term(keyspace.FamilyRead, 2), term(keyspace.FamilyRead, 3), term(keyspace.FamilyRead, 4)}
	assigns := []keyspace.Term{term(keyspace.FamilyAssign, 1), term(keyspace.FamilyAssign, 2), term(keyspace.FamilyAssign, 3)}
	bind := term(keyspace.FamilyBind, 1)
	unaries := []keyspace.Term{term(keyspace.FamilyUnary, 1), term(keyspace.FamilyUnary, 2), term(keyspace.FamilyUnary, 3), term(keyspace.FamilyUnary, 4), term(keyspace.FamilyUnary, 5)}
	selects := []keyspace.Term{term(keyspace.FamilySelect, 1), term(keyspace.FamilySelect, 2)}
	binaries := make([]keyspace.Term, 19)
	for ordinal := range binaries {
		binaries[ordinal] = term(keyspace.FamilyBinary, uint32(ordinal+1))
	}
	nilOrdinal := uint32(0)
	nextNil := func() keyspace.Term { nilOrdinal++; return term(keyspace.FamilyNil, nilOrdinal) }
	valueMembers := []keyspace.Term{nextNil(), nextNil(), nextNil(), nextNil()}
	exactBases := []keyspace.Term{nextNil(), nextNil(), nextNil()}
	dynamicBases := []keyspace.Term{nextNil(), nextNil()}
	dynamicKeys := []keyspace.Term{nextNil(), nextNil()}
	for index := 1; index < len(unaries); index++ {
		_ = nextNil()
	}
	binaryOperands := make([][2]keyspace.Term, len(binaries))
	for index := range binaryOperands {
		binaryOperands[index] = [2]keyspace.Term{nextNil(), nextNil()}
	}
	selectOperands := make([][2]keyspace.Term, len(selects))
	for index := range selectOperands {
		selectOperands[index] = [2]keyspace.Term{nextNil(), nextNil()}
	}
	if nilOrdinal != counts[keyspace.FamilyNil] {
		panic("continuation candidate literal allocation mismatch")
	}
	returnTerms := append([]keyspace.Term{}, unaries[1:4]...)
	returnTerms = append(returnTerms, binaries...)
	returnTerms = append(returnTerms, selects...)
	returnTerms = append(returnTerms, reads[0], reads[2], reads[3])
	valueTerms := []keyspace.Term{valueMembers[0], valueMembers[1], valueMembers[2], unaries[4], valueMembers[3]}
	valueTerms = append(valueTerms, returnTerms...)
	nilOwners := make([]keyspace.Term, counts[keyspace.FamilyNil])
	for index := range nilOwners {
		nilOwners[index] = body
	}
	return continuationSpec{
		name: "continuation-candidates.lua", counts: counts,
		rows:       [][]keyspace.Term{{bind, assigns[0], assigns[1], term(keyspace.FamilyReturn, 1), assigns[2]}},
		keys:       []source.KeyInput{source.NameKey(body, "field"), source.NameKey(body, "write")},
		exactAtoms: []keyspace.LiteralValue{{Kind: keyspace.LiteralString, String: "field"}, {Kind: keyspace.LiteralString, String: "write"}},
		intOwners:  []keyspace.Term{body}, nilOwners: nilOwners,
		static: static.Input{Operators: static.OperatorsInput{TypeOf: []static.TypeOf{{Scope: localCell, Operand: reads[1]}}}},
		binds:  []source.BindCells{{Bind: bind, Cells: []keyspace.Term{localCell}}},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: body, Fixed: authored.Range{Start: 2, End: 3}}, {Owner: body, Fixed: authored.Range{Start: 3, End: 5}}, {Owner: body, Fixed: authored.Range{Start: 5, End: uint32(len(valueTerms))}}},
				Terms: valueTerms,
			},
			Access: authored.AccessInput{
				Exact:   []authored.ExactLens{{Owner: body, Base: exactBases[0], Source: keys[0], Kind: kind.FieldName}, {Owner: body, Base: exactBases[1], Source: unaries[0], Kind: kind.FieldExact}, {Owner: body, Base: exactBases[2], Source: keys[1], Kind: kind.FieldName}},
				Dynamic: []authored.DynamicLens{{Owner: body, Base: dynamicBases[0], Key: dynamicKeys[0]}, {Owner: body, Base: dynamicBases[1], Key: dynamicKeys[1]}},
			},
			Storage: authored.StorageInput{
				Cells:   []authored.Cell{{Kind: authored.CellLocal, Body: body}, {Kind: authored.CellGlobal, Key: 1}},
				Reads:   []authored.Read{{Owner: body, Source: exact[0]}, {Owner: body, Source: exact[1]}, {Owner: body, Source: dynamic[0]}, {Owner: body, Source: globalCell}},
				Binds:   []authored.Bind{{Owner: body, Values: term(keyspace.FamilyValues, 1)}},
				Assigns: []authored.Assign{{Owner: body, Values: term(keyspace.FamilyValues, 2)}, {Owner: body, Values: term(keyspace.FamilyValues, 3)}, {Owner: body, Values: term(keyspace.FamilyValues, 4)}},
				Writes:  []authored.Write{{Assign: assigns[0], Target: exact[2]}, {Assign: assigns[1], Target: dynamic[1]}, {Assign: assigns[2], Target: globalCell}},
			},
			Operators: authored.OperatorsInput{
				Unaries:  []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: integer}, {Owner: body, Op: kind.UnaryNeg, Operand: term(keyspace.FamilyNil, 12)}, {Owner: body, Op: kind.UnaryNot, Operand: term(keyspace.FamilyNil, 13)}, {Owner: body, Op: kind.UnaryLen, Operand: term(keyspace.FamilyNil, 14)}, {Owner: body, Op: kind.UnaryBitNot, Operand: term(keyspace.FamilyNil, 15)}},
				Binaries: continuationBinaryRows(body, binaryOperands),
				Selects:  []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: selectOperands[0][0], Right: selectOperands[0][1]}, {Owner: body, Op: kind.SelectOr, Left: selectOperands[1][0], Right: selectOperands[1][1]}},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: term(keyspace.FamilyValues, 5)}}},
		},
	}
}

func continuationBinaryRows(owner keyspace.Term, operands [][2]keyspace.Term) []authored.Binary {
	rows := make([]authored.Binary, len(operands))
	for index := range rows {
		rows[index] = authored.Binary{Owner: owner, Op: kind.BinaryOp(index + 1), Left: operands[index][0], Right: operands[index][1]}
	}
	return rows
}
