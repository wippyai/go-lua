package generator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func TestExactFoldDispatchIsGeneratedFromTheOwnerReducerTable(t *testing.T) {
	source := composedSource(t, "value")
	artifact, err := Render("value", source)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(artifact.ExactFold)
	for _, required := range []string{
		"const ExactFoldArity = 3",
		"func SupportsExactFoldReducer(reducerOrdinal uint32) bool",
		"type ExactFoldMapping struct",
		"func (schema *Schema) ExactFoldMappingAt(",
		"CandidateRelationMember",
		"ReadCount",
		"type ExactFoldPayload struct",
		"owner",
		"available",
		"func (schema *Schema) ExactFoldPayloadAt(",
		"func (schema *Schema) ReduceExactFoldPayload(",
		"candidate.owner != schema",
		"schema.BinaryArithmeticAt(int(candidateOrdinal))",
		"ArithmeticValue(candidate.candidate",
	} {
		if !strings.Contains(generated, required) {
			t.Fatalf("generated exact fold dispatch is missing %q:\n%s", required, generated)
		}
	}
	for _, forbidden := range []string{"map[", "func(", "any", "reflect"} {
		if strings.Contains(generated, forbidden) {
			t.Fatalf("generated exact fold dispatch retained runtime indirection %q:\n%s", forbidden, generated)
		}
	}
	hotStart := strings.Index(generated, "func (schema *Schema) ReduceExactFoldPayload(")
	if hotStart < 0 {
		t.Fatal("generated exact fold hot reducer body is absent")
	}
	hot := generated[hotStart:]
	for _, coldLookup := range []string{"BinaryArithmeticAt(", "BinaryEqualityAt(", "BinaryOrderAt(", "PresenceRefinementAt("} {
		if strings.Contains(hot, coldLookup) {
			t.Fatalf("generated hot reducer reopens the cold candidate directory through %q:\n%s", coldLookup, hot)
		}
	}
}

// TestExactFoldMappingIsTheCatalogsOwnMemberGeometry pins the mapping row of
// the arity-2 arithmetic reducer to the ordinals the composed catalog itself
// assigns. The expectation is derived from the same resolved metadata the
// emitter reads, so a member the catalog renumbers moves both sides together
// while a member the emitter invents moves only one.
func TestExactFoldMappingIsTheCatalogsOwnMemberGeometry(t *testing.T) {
	source := composedSource(t, "value")
	metadata, err := Resolve(source)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := Render("value", source)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(artifact.ExactFold)

	for _, expectation := range []struct {
		reducer     schema.Key
		candidate   schema.Key
		readKeys    []schema.Key
		destination schema.Key
	}{
		{
			reducer:     "value/binary-arithmetic/reducer",
			candidate:   "value/binary-arithmetic/candidates",
			readKeys:    []schema.Key{"value/binary-arithmetic/left", "value/binary-arithmetic/right"},
			destination: "value/binary-arithmetic/write",
		},
	} {
		ordinal, ordinalOK := reducerOrdinalByKey(metadata, expectation.reducer)
		candidate, candidateOK := relationOrdinalByKey(metadata, expectation.candidate)
		destination, destinationOK := projectionOrdinalByKey(metadata, expectation.destination)
		if !ordinalOK || !candidateOK || !destinationOK {
			t.Fatalf("composed value catalog does not declare %s", expectation.reducer)
		}
		relations := make([]string, 0, len(expectation.readKeys))
		keys := make([]string, 0, len(expectation.readKeys))
		for _, key := range expectation.readKeys {
			projection, projectionOK := projectionOrdinalByKey(metadata, key)
			if !projectionOK {
				t.Fatalf("composed value catalog does not declare projection %s", key)
			}
			relation, relationOK := relationOrdinalByKey(metadata, metadata.Projections[projection].Relation)
			if !relationOK {
				t.Fatalf("projection %s names a relation the catalog does not declare", key)
			}
			relations = append(relations, fmt.Sprintf("%d", relation))
			keys = append(keys, fmt.Sprintf("%d", projection))
		}
		row := fmt.Sprintf("ExactFoldMapping{ReducerOrdinal: %d, CandidateRelationMember: %d, ReadCount: %d, ReadRelationMember: [ExactFoldArity]uint32{%s}, ReadKeyMember: [ExactFoldArity]uint32{%s}, DestinationProjectionMember: %d}",
			ordinal, candidate, len(expectation.readKeys), strings.Join(relations, ", "), strings.Join(keys, ", "), destination)
		if !strings.Contains(generated, row) {
			t.Fatalf("generated mapping for %s is not the catalog's own geometry; expected\n\t%s\ngot\n%s", expectation.reducer, row, generated)
		}
	}
}

// TestExactFoldDispatchCarriesEveryDeclaredArity proves the family is one fold
// family and not the arity-2 one under a new name: every reducer the composed
// catalog declares in this shape reaches the dispatch at its own read count,
// and the owner fold is called with exactly that many dense reads.
func TestExactFoldDispatchCarriesEveryDeclaredArity(t *testing.T) {
	source := composedSource(t, "value")
	metadata, err := Resolve(source)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := exactFoldReducers(source, metadata)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := Render("value", source)
	if err != nil {
		t.Fatal(err)
	}
	generated := string(artifact.ExactFold)

	widths := map[int]bool{}
	for _, reducer := range selected {
		widths[len(reducer.readKeys)] = true
		arguments := make([]string, 0, len(reducer.readKeys)+1)
		arguments = append(arguments, fmt.Sprintf("candidate.candidate%d", reducer.ordinal))
		for position := range reducer.readKeys {
			arguments = append(arguments, fmt.Sprintf("reads[%d]", position))
		}
		call := fmt.Sprintf("%s(%s)", reducer.reducer.Implementation.Name, strings.Join(arguments, ", "))
		if !strings.Contains(generated, call) {
			t.Fatalf("generated dispatch does not call %s:\n%s", call, generated)
		}
		fence := fmt.Sprintf("if candidate.readCount != %d {", len(reducer.readKeys))
		if !strings.Contains(generated, fence) {
			t.Fatalf("generated dispatch does not fence the read count of %s:\n%s", reducer.reducer.Key, generated)
		}
	}
	if !widths[1] || !widths[2] {
		t.Fatalf("composed value catalog exercises fold widths %v; the family is not carrying both a unary and a binary reducer", widths)
	}
}

func TestExactFoldDispatchDoesNotCoerceAnotherReducerShape(t *testing.T) {
	artifact, err := Render("placement", externalProviderDefinition())
	if err != nil {
		t.Fatal(err)
	}
	generated := string(artifact.ExactFold)
	if strings.Contains(generated, "case 0:") {
		t.Fatalf("a reducer outside this shape was coerced into exact fold dispatch:\n%s", generated)
	}
}

func reducerOrdinalByKey(metadata Metadata, key schema.Key) (uint32, bool) {
	for ordinal, reducer := range metadata.Reducers {
		if reducer.Key == key {
			return uint32(ordinal), true
		}
	}
	return 0, false
}

func relationOrdinalByKey(metadata Metadata, key schema.Key) (uint32, bool) {
	for ordinal, relation := range metadata.Relations {
		if relation.Key == key {
			return uint32(ordinal), true
		}
	}
	return 0, false
}

func projectionOrdinalByKey(metadata Metadata, key schema.Key) (uint32, bool) {
	for ordinal, projection := range metadata.Projections {
		if projection.Key == key {
			return uint32(ordinal), true
		}
	}
	return 0, false
}
