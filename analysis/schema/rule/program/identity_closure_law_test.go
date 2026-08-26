package program

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/internal/framing"
)

func programColdContent(t testing.TB, value Program) []byte {
	t.Helper()
	var sink bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&sink, contentDomain, contentVersion); err != nil {
		t.Fatalf("program content reset: %v", err)
	}
	if err := value.WriteContent(&writer); err != nil {
		t.Fatalf("program content: %v", err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatalf("program content finish: %v", err)
	}
	return append([]byte(nil), sink.Bytes()...)
}

// TestJoinIdentityFieldsCloseTheCanonicalColdBoundary keeps each optional
// JoinDecl identity in all three common surfaces: the framed cold bytes, the
// Program digest, and the upward reference stream. Each mutation starts from
// the absent form so References must change as well as the bytes and digest.
func TestJoinIdentityFieldsCloseTheCanonicalColdBoundary(t *testing.T) {
	base := lawProgram(t)
	baseContent := programColdContent(t, base)
	baseDigest := base.Digest()
	baseReferences := base.References()

	cases := []struct {
		name   string
		mutate func(*JoinDecl)
	}{
		{
			name: "selection",
			mutate: func(join *JoinDecl) {
				join.Selection = lawSelection("closure/selection")
			},
		},
		{
			name: "key-vector",
			mutate: func(join *JoinDecl) {
				join.KeyVector = member.RelationRef{Axis: lawMemberAxis(), Member: "closure/key-vector"}
			},
		},
		{
			name: "address-identity",
			mutate: func(join *JoinDecl) {
				join.AddressIdentity = lawProjection("closure/address-identity")
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			mutated := base.Clone()
			test.mutate(&mutated.Joins[0])
			if problem, valid := mutated.Check(); !valid {
				t.Fatalf("identity mutation rejected: %+v", problem)
			}
			if bytes.Equal(baseContent, programColdContent(t, mutated)) {
				t.Fatal("identity mutation did not change canonical cold content")
			}
			if baseDigest == mutated.Digest() {
				t.Fatal("identity mutation did not change Program digest")
			}
			if len(mutated.References()) != len(baseReferences)+1 {
				t.Fatalf("references=%d, want %d after declaring %s", len(mutated.References()), len(baseReferences)+1, test.name)
			}

			// Clone is the cold round-trip boundary used by Rule.Template. It
			// must retain the newly-authored field and therefore authenticate to
			// exactly the same content identity.
			roundTrip := mutated.Clone()
			if !bytes.Equal(programColdContent(t, mutated), programColdContent(t, roundTrip)) {
				t.Fatal("cold clone changed canonical content")
			}
			if roundTrip.Digest() != mutated.Digest() {
				t.Fatal("cold clone failed Program digest authentication")
			}
		})
	}
}

func TestJoinIdentityFieldsRejectDeclaredButUnavailableValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*JoinDecl)
	}{
		{
			name: "key-vector",
			mutate: func(join *JoinDecl) {
				join.KeyVector = member.RelationRef{Axis: lawMemberAxis()}
			},
		},
		{
			name: "address-identity",
			mutate: func(join *JoinDecl) {
				join.AddressIdentity = member.ProjectionRef{Axis: lawMemberAxis()}
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			program := lawProgram(t)
			test.mutate(&program.Joins[0])
			problem, valid := program.Check()
			if valid || problem.Kind != ProblemJoin {
				t.Fatalf("declared-but-unavailable %s valid=%v problem=%+v", test.name, valid, problem)
			}
		})
	}
}
