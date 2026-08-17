package contract

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// role derives one content identity the way the catalog derives a payload
// format identity, so a toy kind is identified in the same global semantic
// domain the declared kinds are.
func role(t *testing.T, name string) identity.ContentID {
	t.Helper()
	key, ok := vocabulary.Key(name)
	if !ok {
		t.Fatalf("role %q is underivable", name)
	}
	return identity.ContentID(key.Digest())
}

// toyKind admits one declared contract kind of a class, declaring every form
// that class owes over distinct payload formats.
func toyKind(t *testing.T, key schema.Key, class library.Class) *library.Entry {
	t.Helper()
	forms := class.Required()
	members := make([]library.Member, 0, len(forms))
	for _, form := range forms {
		members = append(members, library.Member{Form: form, Payload: role(t, formRole(string(key), form))})
	}
	entry, ok := library.New(library.Spec{
		Key:        key,
		Class:      class,
		Codec:      library.Codec{Format: role(t, "test-contract-codec/"+string(key)), Version: 1},
		Validation: library.LawSet{Resolution: library.ResolutionDeferred, Deferred: role(t, "test-contract-lawset/"+string(key))},
		Addressing: library.AddressingExportPath,
		Members:    members,
	})
	if !ok {
		t.Fatalf("toy kind %q was not admitted", key)
	}
	return entry
}

func formRole(key string, form library.Form) string {
	return "test-contract-payload/" + key + "/" + string(rune('a'+int(form)))
}

// deferredMember authors one row of a form the kind declares.
func deferredMember(t *testing.T, kind *library.Entry, form library.Form, path Path) Member {
	t.Helper()
	payload, ok := kind.Payload(form)
	if !ok {
		t.Fatalf("kind %q declares no payload for form %d", kind.Key(), form)
	}
	return Member{Form: form, Path: path, Payload: payload, Encoding: EncodingDeferred}
}

func toySpec(t *testing.T, kind *library.Entry) Spec {
	t.Helper()
	return Spec{
		Kind:  kind.Key(),
		Codec: kind.Codec(),
		Root:  "toy",
		Members: []Member{
			deferredMember(t, kind, library.FormExportValue, Root()),
			deferredMember(t, kind, library.FormCallableSignature, Export("len")),
		},
	}
}

// TestInstanceIsAdmittedByTheKindItNames is the base case: an authored
// instance whose rows are all forms its kind declares, over the payload formats
// that kind declared, is one instance.
func TestInstanceIsAdmittedByTheKindItNames(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	instance, ok := New(toySpec(t, kind), kind)
	if !ok {
		t.Fatal("an instance of the kind it names was rejected")
	}
	if instance.Kind() != kind.Key() || instance.Codec() != kind.Codec() || instance.Class() != library.ClassLibrary {
		t.Fatal("the admitted instance does not carry the kind it was admitted against")
	}
	if instance.Count() != 2 || instance.Deferred() != 2 {
		t.Fatalf("rows=%d deferred=%d want 2 and 2", instance.Count(), instance.Deferred())
	}
}

// TestInstanceMustNameTheKindItIsAdmittedAgainst refuses an instance whose
// declared kind is not the kind the reader resolved. A contract read under
// another kind's algebra is a contract nobody wrote.
func TestInstanceMustNameTheKindItIsAdmittedAgainst(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	other := toyKind(t, "second", library.ClassLibrary)
	spec := toySpec(t, kind)
	if instance, ok := New(spec, other); ok || instance != nil {
		t.Fatal("an instance was admitted against a kind it does not name")
	}
	if instance, ok := New(spec, nil); ok || instance != nil {
		t.Fatal("an instance was admitted against no kind at all")
	}
}

// TestCodecMustBeTheKindsOwn refuses an instance published in a format or at a
// version its kind does not declare. A reader has no ground to decode it.
func TestCodecMustBeTheKindsOwn(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	format := toySpec(t, kind)
	format.Codec.Format = role(t, "test-contract-codec/foreign")
	if _, ok := New(format, kind); ok {
		t.Fatal("an instance in a foreign format was admitted")
	}
	version := toySpec(t, kind)
	version.Codec.Version = kind.Codec().Version + 1
	if _, ok := New(version, kind); ok {
		t.Fatal("an instance at an undeclared version was admitted")
	}
	unversioned := toySpec(t, kind)
	unversioned.Codec.Version = 0
	if _, ok := New(unversioned, kind); ok {
		t.Fatal("an unversioned instance was admitted")
	}
}

// TestRootSelectorIsRequired refuses an instance that cannot be bound. A
// contract with no mount selector is not a mount-bound contract.
func TestRootSelectorIsRequired(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	spec := toySpec(t, kind)
	spec.Root = ""
	if _, ok := New(spec, kind); ok {
		t.Fatal("an instance with no mount selector was admitted")
	}
}

// TestMemberlessInstanceIsRejected refuses a contract that says nothing. It
// would name a mount and describe none of it.
func TestMemberlessInstanceIsRejected(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	spec := toySpec(t, kind)
	spec.Members = nil
	if _, ok := New(spec, kind); ok {
		t.Fatal("an instance with no members was admitted")
	}
}

// TestMemberFormMustBeDeclaredByItsKind is the law that keeps the environment
// forms out of a library instance. A library kind does not declare a boot root,
// so a library instance cannot carry one, and the refusal is the kind's rather
// than a list this package keeps.
func TestMemberFormMustBeDeclaredByItsKind(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	environment := toyKind(t, "environment", library.ClassEnvironment)
	spec := toySpec(t, kind)
	payload, ok := environment.Payload(library.FormBootRoot)
	if !ok {
		t.Fatal("the environment kind declares no boot root payload")
	}
	spec.Members = append(spec.Members, Member{
		Form: library.FormBootRoot, Path: Root(), Payload: payload, Encoding: EncodingDeferred,
	})
	if _, ok := New(spec, kind); ok {
		t.Fatal("a library instance carrying an environment form was admitted")
	}
	// The same row under the kind that declares it is ordinary contract data,
	// so the refusal above is the library kind's and not a rejection of the form.
	owned := toySpec(t, environment)
	owned.Members = append(owned.Members, deferredMember(t, environment, library.FormBootRoot, Root()))
	if _, ok := New(owned, environment); !ok {
		t.Fatal("an environment instance carrying an environment form was rejected")
	}
	unknown := toySpec(t, kind)
	unknown.Members[0].Form = library.Form(0)
	if _, ok := New(unknown, kind); ok {
		t.Fatal("a member of no form was admitted")
	}
}

// TestLibraryInstanceStatesItsOwnDenials is the class-agnostic half of the
// tenancy law. A denial is owner-declared member data: a library says which of
// its own members it declares and refuses to publish, so the row is admitted
// under the library kind exactly as any other member of the base algebra is.
// Admission derives that from the kind's declared forms alone - this package
// keeps no catalog of its own - which is why a form no kind declares is still
// refused.
func TestLibraryInstanceStatesItsOwnDenials(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	body, err := EncodePath(Export("dump"))
	if err != nil {
		t.Fatalf("the denied address did not encode: %v", err)
	}
	denied := deferredMember(t, kind, library.FormDeniedEntry, Export("dump"))
	denied.Encoding, denied.Body = EncodingResolved, body
	spec := toySpec(t, kind)
	spec.Members = append(spec.Members, denied)
	instance, ok := New(spec, kind)
	if !ok {
		t.Fatal("a library instance stating a member it refuses to publish was rejected")
	}
	row, found := instance.Resolve(library.FormDeniedEntry, Export("dump"))
	if !found {
		t.Fatal("the denial did not resolve at the address it was authored at")
	}
	address, err := DecodePath(row.Body)
	if err != nil {
		t.Fatalf("the denial payload did not decode: %v", err)
	}
	if !address.Equal(Export("dump")) {
		t.Fatal("the denial payload states an address other than the member it refuses")
	}
	// The environment states its own denials over the same form, so neither
	// class is the form's owner.
	environment := toyKind(t, "environment", library.ClassEnvironment)
	owned := toySpec(t, environment)
	owned.Members = append(owned.Members, deferredMember(t, environment, library.FormDeniedEntry, Export("dump")))
	if _, ok := New(owned, environment); !ok {
		t.Fatal("an environment instance stating a denial was rejected")
	}
	// A form outside the catalog is still refused: class-agnostic is not
	// form-agnostic.
	unknown := toySpec(t, kind)
	unknown.Members = append(unknown.Members, Member{
		Form:     library.Form(200),
		Path:     Export("dump"),
		Payload:  role(t, "test-contract-payload/unknown"),
		Encoding: EncodingDeferred,
	})
	if _, ok := New(unknown, kind); ok {
		t.Fatal("a member of a form no kind declares was admitted")
	}
}

// TestMemberPayloadMustBeTheFormatItsKindDeclared is what makes an instance
// self-describing. A member that restated another format would decode as a
// shape it is not.
func TestMemberPayloadMustBeTheFormatItsKindDeclared(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	foreign := toySpec(t, kind)
	foreign.Members[1].Payload = role(t, "test-contract-payload/foreign")
	if _, ok := New(foreign, kind); ok {
		t.Fatal("a member over a foreign payload format was admitted")
	}
	crossed := toySpec(t, kind)
	other, ok := kind.Payload(library.FormEffectLabel)
	if !ok {
		t.Fatal("the toy kind declares no effect label payload")
	}
	crossed.Members[1].Payload = other
	if _, ok := New(crossed, kind); ok {
		t.Fatal("a member carrying another form's payload format was admitted")
	}
	empty := toySpec(t, kind)
	empty.Members[1].Payload = identity.ContentID{}
	if _, ok := New(empty, kind); ok {
		t.Fatal("a member with no payload format was admitted")
	}
}

// TestEncodingAndBodyAgree states the honesty law over a member payload. A
// deferred member carries no body, and a resolved one carries a body; an empty
// resolved body is a payload that claims to exist and does not.
func TestEncodingAndBodyAgree(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	hollow := toySpec(t, kind)
	hollow.Members[1].Encoding = EncodingResolved
	if _, ok := New(hollow, kind); ok {
		t.Fatal("a resolved member with no body was admitted")
	}
	stuffed := toySpec(t, kind)
	stuffed.Members[1].Body = []byte{1}
	if _, ok := New(stuffed, kind); ok {
		t.Fatal("a deferred member carrying a body was admitted")
	}
	unstated := toySpec(t, kind)
	unstated.Members[1].Encoding = EncodingInvalid
	if _, ok := New(unstated, kind); ok {
		t.Fatal("a member that does not state its encoding was admitted")
	}
	resolved := toySpec(t, kind)
	resolved.Members[1].Encoding, resolved.Members[1].Body = EncodingResolved, []byte{7}
	instance, ok := New(resolved, kind)
	if !ok {
		t.Fatal("a resolved member with a body was rejected")
	}
	if instance.Deferred() != 1 {
		t.Fatalf("deferred=%d want 1", instance.Deferred())
	}
}

// TestOneFormAtOneAddressIsClaimedOnce refuses a row written twice. Two rows of
// one form over one value leave a reader with no ground to choose between them,
// while one address under two forms and one form at two addresses are both
// ordinary contract data.
func TestOneFormAtOneAddressIsClaimedOnce(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	duplicate := toySpec(t, kind)
	duplicate.Members = append(duplicate.Members, deferredMember(t, kind, library.FormCallableSignature, Export("len")))
	if _, ok := New(duplicate, kind); ok {
		t.Fatal("one form at one address was admitted twice")
	}
	distinct := toySpec(t, kind)
	distinct.Members = append(distinct.Members,
		deferredMember(t, kind, library.FormCallableSignature, Export("sub")),
		deferredMember(t, kind, library.FormEffectLabel, Export("len")),
	)
	if _, ok := New(distinct, kind); !ok {
		t.Fatal("distinct forms and addresses were rejected as duplicates")
	}
}

// TestPathAddressesAValueOrNothing states the addressing law over a path. A
// step with no key reaches nothing, and a member whose address reaches nothing
// attaches to nothing.
func TestPathAddressesAValueOrNothing(t *testing.T) {
	if !Root().Available() || Root().Len() != 0 {
		t.Fatal("the contract root is not an addressable member")
	}
	if !Export("len").Available() || !Metatable("__index").Available() {
		t.Fatal("a one-step export or metatable path is not addressable")
	}
	if NewPath(Step{Kind: StepExport}).Available() {
		t.Fatal("a step with no key addresses a value")
	}
	if NewPath(Step{Kind: StepInvalid, Key: "len"}).Available() {
		t.Fatal("a step that does not state how it reaches addresses a value")
	}
	kind := toyKind(t, "library", library.ClassLibrary)
	spec := toySpec(t, kind)
	spec.Members[1].Path = NewPath(Step{Kind: StepExport, Key: ""})
	if _, ok := New(spec, kind); ok {
		t.Fatal("a member whose address reaches nothing was admitted")
	}
}

// TestExportAndMetatableStepsAreDistinctAddresses keeps the two ways of
// reaching a value apart. A member published through __index is not the member
// exported under the key __index.
func TestExportAndMetatableStepsAreDistinctAddresses(t *testing.T) {
	if Export("__index").Equal(Metatable("__index")) {
		t.Fatal("an export key and a metatable key address the same value")
	}
	kind := toyKind(t, "library", library.ClassLibrary)
	spec := toySpec(t, kind)
	spec.Members = append(spec.Members,
		deferredMember(t, kind, library.FormMetatableEdge, Export("__index")),
		deferredMember(t, kind, library.FormMetatableEdge, Metatable("__index")),
	)
	if _, ok := New(spec, kind); !ok {
		t.Fatal("an export key and a metatable key of one spelling were read as one address")
	}
}

// TestAdmittedInstanceCannotBeRewrittenThroughWhatItHandsOut keeps an instance
// immutable once built. A reader that could edit the rows it was handed would
// be editing a sealed contract.
func TestAdmittedInstanceCannotBeRewrittenThroughWhatItHandsOut(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	spec := toySpec(t, kind)
	spec.Members[1].Encoding, spec.Members[1].Body = EncodingResolved, []byte{9}
	instance, ok := New(spec, kind)
	if !ok {
		t.Fatal("the instance was rejected")
	}
	before := ContentID(instance)
	// The authored slice the caller still holds is not the instance's.
	spec.Members[1].Body[0] = 8
	spec.Members = spec.Members[:1]
	rows := instance.Members()
	rows[1].Body[0] = 7
	rows[1].Form = library.FormSuspension
	if handed, ok := instance.At(1); !ok || handed.Form != library.FormCallableSignature || handed.Body[0] != 9 {
		t.Fatal("an admitted instance was rewritten through the rows it handed out")
	}
	if ContentID(instance) != before {
		t.Fatal("an admitted instance changed identity without being rewritten")
	}
}

// TestResolveFindsAMemberByFormAndAddress is the read a mount performs: which
// contract row attaches to the value this path reached.
func TestResolveFindsAMemberByFormAndAddress(t *testing.T) {
	kind := toyKind(t, "library", library.ClassLibrary)
	instance, ok := New(toySpec(t, kind), kind)
	if !ok {
		t.Fatal("the instance was rejected")
	}
	if _, found := instance.Resolve(library.FormCallableSignature, Export("len")); !found {
		t.Fatal("an authored member did not resolve at its own address")
	}
	if _, found := instance.Resolve(library.FormCallableSignature, Export("sub")); found {
		t.Fatal("a member resolved at an address it was not authored at")
	}
	if _, found := instance.Resolve(library.FormEffectLabel, Export("len")); found {
		t.Fatal("a member resolved under a form it was not authored under")
	}
}
