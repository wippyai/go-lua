package engine

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// A fact states what it is about in the positions its family declares. Some
// families name their subject as a term only because no allocation identity was
// available for it, and some state the container they are about as a
// discriminator rather than as the subject. A recursive summary owes those facts
// to its boundary exactly as it owes the ones whose subject is the term itself;
// reading the declaration rather than the position is what reaches them.

// TestTermSpelledSubjectIsCarriedForItsBoundary pins the case a positional read
// cannot reach: an index proof over a container the guard could only name by
// path, qualified by the index it proves. The container is the boundary's own
// term, so the proof is the boundary's own fact.
func TestTermSpelledSubjectIsCarriedForItsBoundary(t *testing.T) {
	container := []byte(boundaryTerm(2))
	index := []byte("path/sym7")
	keys := []string{
		factkey.BuildKey(factkey.HeapIndexPresence, []factkey.Part{factkey.TaggedTermPart(container), factkey.EncodedTermPart(index)}, "op-00000001").String(),
		factkey.BuildKey(factkey.HeapKeyPresence, []factkey.Part{factkey.TaggedTermPart(container), factkey.EncodedTermPart([]byte(".a"))}, "op-00000001").String(),
		factkey.BuildKey(factkey.HeapLengthFloor, []factkey.Part{factkey.TaggedTermPart(container)}, "op-00000001").String(),
		factkey.BuildKey(factkey.HeapTableEscape, []factkey.Part{factkey.TaggedTermPart(container)}, "op-00000001").String(),
		factkey.BuildKey(factkey.HeapIndexRevoke, []factkey.Part{factkey.TaggedTermPart(container)}, "op-00000001").String(),
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
	container := []byte(boundaryTerm(2))
	index := []byte("path/sym7")
	key := factkey.BuildKey(
		factkey.HeapIndexUpper, []factkey.Part{factkey.EncodedTermPart(index), factkey.EncodedTermPart(container)}, "op-00000001",
	).String()
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), equation.OutputClosure{
		Values: []equation.Fact{{Key: key, Value: []byte("proven")}},
	}))
	if !kept[key] {
		t.Errorf("recursive summary dropped %q, whose bounded container is its boundary term", key)
	}
}

// TestEncodedTermSubjectIsCarriedForItsBoundary pins the family whose subject
// is a term written in encoded form. The origin of a value read at an
// unresolved key names the term that holds it, so a boundary term read that way
// owes its origin to the summary; a positional read of the same key sees the
// encoding rather than the term and reaches nothing.
func TestEncodedTermSubjectIsCarriedForItsBoundary(t *testing.T) {
	key := factkey.BuildKey(
		factkey.HeapKeyedRead, []factkey.Part{factkey.EncodedTermPart([]byte(boundaryTerm(2)))}, "op-00000001",
	).String()
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), equation.OutputClosure{
		Values: []equation.Fact{{Key: key, Value: []byte("alloc-1")}},
	}))
	if !kept[key] {
		t.Errorf("recursive summary dropped %q, the keyed-read origin of its own boundary term", key)
	}
}

// TestEncodedTermSubjectKeepsForeignTermsOut pins the exclusion the same
// declaration owes: a keyed read of a term this boundary does not own stays out.
func TestEncodedTermSubjectKeepsForeignTermsOut(t *testing.T) {
	key := factkey.BuildKey(
		factkey.HeapKeyedRead, []factkey.Part{factkey.EncodedTermPart([]byte(boundaryTerm(9)))}, "op-00000001",
	).String()
	kept := summaryKeys(lexicalSCCSummary(sccBoundary(2), equation.OutputClosure{
		Values: []equation.Fact{{Key: key, Value: []byte("alloc-1")}},
	}))
	if kept[key] {
		t.Errorf("recursive summary carried %q, which names no term of its boundary", key)
	}
}

// TestSchemaKeepsForeignTermsOut pins that resolving more positions did not
// weaken the exclusion. A term-spelled subject or qualifier naming something
// this boundary does not own stays out, in every position.
func TestSchemaKeepsForeignTermsOut(t *testing.T) {
	foreign := []byte(boundaryTerm(9))
	index := []byte("path/sym7")
	keys := []string{
		factkey.BuildKey(factkey.HeapIndexPresence, []factkey.Part{factkey.TaggedTermPart(foreign), factkey.EncodedTermPart(index)}, "op-00000001").String(),
		factkey.BuildKey(factkey.HeapLengthFloor, []factkey.Part{factkey.TaggedTermPart(foreign)}, "op-00000001").String(),
		factkey.BuildKey(factkey.HeapIndexUpper, []factkey.Part{factkey.EncodedTermPart(index), factkey.EncodedTermPart(foreign)}, "op-00000001").String(),
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
		factkey.AffineIndex.Key().String() + term + "/op-00000001",
		factkey.BooleanResult.Key().String() + term + "/op-00000001",
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
