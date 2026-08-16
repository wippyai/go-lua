package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func assertProductionEmptyAndAbsent(t *testing.T, fixture *pendingFixture, subject, absent keyspace.Term) {
	t.Helper()
	if count, ok := fixture.pending.Count(subject); !ok || count != 0 {
		t.Fatalf("subject %v = %d/%v, want valid empty subject", subject, count, ok)
	}
	if _, ok := fixture.pending.At(subject, 0); ok {
		t.Fatalf("subject %v exposed a payload for its empty set", subject)
	}
	if count, ok := fixture.pending.Count(absent); ok || count != 0 {
		t.Fatalf("absent subject %v = %d/%v, want 0/false", absent, count, ok)
	}
}

func TestSealPendingProductionEmptyAndAbsentEverySubjectPlane(t *testing.T) {
	body := pendingTerm(keyspace.FamilyBody, 1)
	unaryCounts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 1, keyspace.FamilyValues: 1,
		keyspace.FamilyUnary: 1, keyspace.FamilyReturn: 1,
	}
	unaryFixture := openPendingFixture(t, "pending-empty-unary.lua", unaryCounts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{pendingTerm(keyspace.FamilyUnary, 1)},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: pendingTerm(keyspace.FamilyValues, 1)}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{
				Owner: body, Op: kind.UnaryNeg, Operand: pendingTerm(keyspace.FamilyNil, 1),
			}}},
		}, nil, nil, nil, pendingSourceExtras{})
	assertProductionEmptyAndAbsent(t, unaryFixture, pendingTerm(keyspace.FamilyUnary, 1), pendingTerm(keyspace.FamilyUnary, 2))

	binaryCounts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyBody: 1, keyspace.FamilyNil: 2, keyspace.FamilyValues: 1,
		keyspace.FamilyBinary: 1, keyspace.FamilyReturn: 1,
	}
	binaryFixture := openPendingFixture(t, "pending-empty-binary.lua", binaryCounts,
		[][]keyspace.Term{{pendingTerm(keyspace.FamilyReturn, 1)}}, authored.Input{
			Values: authored.ValuesInput{
				Rows:  []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}},
				Terms: []keyspace.Term{pendingTerm(keyspace.FamilyBinary, 1)},
			},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: pendingTerm(keyspace.FamilyValues, 1)}}},
			Operators: authored.OperatorsInput{Binaries: []authored.Binary{{
				Owner: body, Op: kind.BinaryAdd,
				Left: pendingTerm(keyspace.FamilyNil, 1), Right: pendingTerm(keyspace.FamilyNil, 2),
			}}},
		}, nil, nil, nil, pendingSourceExtras{})
	assertProductionEmptyAndAbsent(t, binaryFixture, pendingTerm(keyspace.FamilyBinary, 1), pendingTerm(keyspace.FamilyBinary, 2))

	matrix := openPendingMatrixFixture(t, "pending-empty-read-write-call.lua", pendingRuntimeMatrixFlow())
	assertProductionEmptyAndAbsent(t, matrix, pendingTerm(keyspace.FamilyRead, 1), pendingTerm(keyspace.FamilyRead, 7))
	assertProductionEmptyAndAbsent(t, matrix, pendingTerm(keyspace.FamilyWrite, 2), pendingTerm(keyspace.FamilyWrite, 5))
	assertProductionEmptyAndAbsent(t, matrix, pendingTerm(keyspace.FamilyCall, 1), pendingTerm(keyspace.FamilyCall, 4))

	loop := openPendingLoopFixture(t, "pending-empty-loop.lua")
	assertProductionEmptyAndAbsent(t, loop, pendingTerm(keyspace.FamilyLoop, 4), pendingTerm(keyspace.FamilyLoop, 5))
}
