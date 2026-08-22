package factor_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// TestMountedPublicationAdmitsUnderAppliedSubject proves that Lua
// under-application is a legal program at the Effect boundary. The send
// descriptor's subject maps to caller formal 1, which the call does not
// author; the publication receipt is still issued and carries the subject as
// a proven-nil mounted input rather than refusing the whole artifact.
func TestMountedPublicationAdmitsUnderAppliedSubject(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, false), "local function sink(left, right) return left end\nsink(1)")
	publications, ok := fixture.factor.SelectedCallMountedPublications(fixture.root, fixture.mountedCall, fixture.owner)
	if !ok || len(publications) != 1 {
		t.Fatalf("SelectedCallMountedPublications() = %d/%v, want one receipt", len(publications), ok)
	}
	publication := publications[0]
	subject, subjectOK := publication.SubjectInput()
	context, contextOK := publication.ContextInput()
	if !publication.Valid() || !subjectOK || !contextOK || !subject.Valid() || !context.Valid() {
		t.Fatal("under-applied send receipt lost subject/context mounted inputs")
	}
	if !subject.IsProvenNil() || subject.IsOpen() || subject.MemberCount() != 0 {
		t.Fatalf("subject provenNil=%t open=%t members=%d, want true/false/0", subject.IsProvenNil(), subject.IsOpen(), subject.MemberCount())
	}
	if context.IsProvenNil() || context.MemberCount() != 1 {
		t.Fatalf("context provenNil=%t members=%d, want false/1", context.IsProvenNil(), context.MemberCount())
	}
	batch, batchOK := fixture.factor.PublicationBatchForMountedCall(fixture.mountedCall)
	if !batchOK || !batch.Valid() || len(batch.Rows()) != 1 {
		t.Fatal("under-applied publication batch refused")
	}
}

// TestMountedPublicationAdmitsTailFedSubject proves the unknown reading also
// reaches Effect intact. The call forwards a vararg tail, so the descriptor's
// subject formal has no fixed actual but may be populated at runtime; the
// receipt is issued with an open zero-member subject.
func TestMountedPublicationAdmitsTailFedSubject(t *testing.T) {
	fixture := newEffectFactorFixture(t, variadicPublicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer), "local function sink(left, right) return left end\nlocal function outer(...)\n  sink(...)\nend\nouter(1)")
	publications, ok := fixture.factor.SelectedCallMountedPublications(fixture.root, fixture.mountedCall, fixture.owner)
	if !ok || len(publications) != 1 {
		t.Fatalf("SelectedCallMountedPublications() = %d/%v, want one receipt", len(publications), ok)
	}
	publication := publications[0]
	subject, subjectOK := publication.SubjectInput()
	if !publication.Valid() || !subjectOK || !subject.Valid() {
		t.Fatal("tail-fed send receipt lost its subject mounted input")
	}
	if !subject.IsOpen() || subject.IsProvenNil() || subject.MemberCount() != 0 {
		t.Fatalf("subject open=%t provenNil=%t members=%d, want true/false/0", subject.IsOpen(), subject.IsProvenNil(), subject.MemberCount())
	}
	batch, batchOK := fixture.factor.PublicationBatchForMountedCall(fixture.mountedCall)
	if !batchOK || !batch.Valid() || len(batch.Rows()) != 1 {
		t.Fatal("tail-fed publication batch refused")
	}
}

// TestMountedPublicationBatchIndexAdmitsUnderAppliedCalls keeps the sealed
// batch directory whole: an under-applied call contributes its batch instead
// of collapsing the index that every Placement and Heap publication rule binds
// against.
func TestMountedPublicationBatchIndexAdmitsUnderAppliedCalls(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, false), "local function sink(left, right) return left end\nsink(1)")
	index, indexOK := effectfactor.NewMountedPublicationBatchIndex(fixture.factor)
	if !indexOK || index == nil || !index.Valid() || index.Count() != fixture.factor.MountedCallCount() {
		t.Fatal("under-applied mounted publication batch index refused")
	}
}

// variadicPublicationEffectFactorSpec reuses the publication descriptor ABI
// with an owner input that accepts an actual tail, so a call can leave a fixed
// formal position to the tail producer.
func variadicPublicationEffectFactorSpec(publicationKind vocabulary.PublicationEffectKind) declaration.Spec {
	spec := publicationEffectFactorSpec(publicationKind, false)
	spec.Operations[0].ValuesVars = 1
	spec.Operations[0].Input = vocabulary.ValuesSpec{Fixed: []schematype.Type{portableAnyType(), portableAnyType()}, Tail: vocabulary.ValuesVariable, Var: 0}
	spec.Operations[0].Outcomes = []vocabulary.OutcomeSpec{{Kind: kind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}
	return spec
}
