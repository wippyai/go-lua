package engine

import (
	"encoding/base64"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// A fact states what it is about in the positions its family declares. Some
// families name their subject as a term only because no allocation identity was
// available for it, and some state the container they are about as a
// discriminator rather than as the subject. A recursive summary owes those facts
// to its boundary exactly as it owes the ones whose subject is the term itself;
// reading the declaration rather than the position is what reaches them.

func encodedTerm(symbol wir.SymbolID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(boundaryTerm(symbol)))
}

// TestTermSpelledSubjectIsCarriedForItsBoundary pins the case a positional read
// cannot reach: an index proof over a container the guard could only name by
// path, qualified by the index it proves. The container is the boundary's own
// term, so the proof is the boundary's own fact.
func TestTermSpelledSubjectIsCarriedForItsBoundary(t *testing.T) {
	container := encodedTerm(2)
	index := base64.RawURLEncoding.EncodeToString([]byte("path/sym7"))
	keys := []string{
		heapIndexPresencePrefix + heapSubjectTermPrefix + container + "/" + index + "/op-00000001",
		heapKeyPresencePrefix + heapSubjectTermPrefix + container + "/LmE/op-00000001",
		heapLengthFloorPrefix + heapSubjectTermPrefix + container + "/op-00000001",
		heapTableEscapePrefix + heapSubjectTermPrefix + container + "/op-00000001",
		heapIndexRevokePrefix + heapSubjectTermPrefix + container + "/op-00000001",
	}
	closure := equation.OutputClosure{}
	for _, key := range keys {
		closure.Values = append(closure.Values, equation.Fact{Key: key, Value: []byte("proven")})
	}
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	for _, key := range keys {
		if !kept[key] {
			t.Errorf("recursive summary dropped %q, a fact about its own boundary term", key)
		}
	}
}

// TestQualifierNamedTermIsCarried pins the other position a declaration
// resolves. An index bound states the index as its subject and the container it
// bounds as a discriminator, so the boundary is named by the qualifier.
func TestQualifierNamedTermIsCarried(t *testing.T) {
	container := encodedTerm(2)
	index := base64.RawURLEncoding.EncodeToString([]byte("path/sym7"))
	key := heapIndexUpperPrefix + index + "/" + container + "/op-00000001"
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), equation.OutputClosure{
		Values: []equation.Fact{{Key: key, Value: []byte("proven")}},
	}))
	if !kept[key] {
		t.Errorf("recursive summary dropped %q, whose bounded container is its boundary term", key)
	}
}

// TestSchemaKeepsForeignTermsOut pins that resolving more positions did not
// weaken the exclusion. A term-spelled subject or qualifier naming something
// this boundary does not own stays out, in every position.
func TestSchemaKeepsForeignTermsOut(t *testing.T) {
	foreign := encodedTerm(9)
	index := base64.RawURLEncoding.EncodeToString([]byte("path/sym7"))
	keys := []string{
		heapIndexPresencePrefix + heapSubjectTermPrefix + foreign + "/" + index + "/op-00000001",
		heapLengthFloorPrefix + heapSubjectTermPrefix + foreign + "/op-00000001",
		heapIndexUpperPrefix + index + "/" + foreign + "/op-00000001",
	}
	closure := equation.OutputClosure{}
	for _, key := range keys {
		closure.Values = append(closure.Values, equation.Fact{Key: key, Value: []byte("proven")})
	}
	for key := range summaryKeys(lexicalSCCSummary(sccBoundary(2), closure)) {
		t.Errorf("recursive summary carried %q, which names no term of its boundary", key)
	}
}

// TestUndeclaredFamiliesKeepThePositionalRule pins the fail-closed edge of the
// declaration. A family the schema does not declare is read exactly as before,
// so declaring some families cannot change what the others already did.
func TestUndeclaredFamiliesKeepThePositionalRule(t *testing.T) {
	term := boundaryTerm(2)
	carried := []string{
		"value/" + term + "/op-00000001",
		affineIndexPrefix + term + "/op-00000001",
		booleanResultPrefix + term + "/op-00000001",
	}
	dropped := []string{
		"value/" + boundaryTerm(9) + "/op-00000001",
		"branch/op-00000001",
	}
	closure := equation.OutputClosure{}
	for _, key := range append(append([]string{}, carried...), dropped...) {
		closure.Values = append(closure.Values, equation.Fact{Key: key, Value: []byte("x")})
	}
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), closure))
	for _, key := range carried {
		if !kept[key] {
			t.Errorf("undeclared family key %q stopped being carried", key)
		}
	}
	for _, key := range dropped {
		if kept[key] {
			t.Errorf("undeclared family key %q started being carried", key)
		}
	}
}
