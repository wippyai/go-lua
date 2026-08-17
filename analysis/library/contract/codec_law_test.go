package contract

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// declaredTable projects the sealed library contract surface out of the
// analyzer declaration root. A decoder resolves the kind an instance names
// against exactly this projection, so the tests below decode against the real
// declared kinds rather than a table of their own.
func declaredTable(t *testing.T) library.Table {
	t.Helper()
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("the declaration root did not seal: law=%d disposition=%s", failure.Law, failure.Disposition)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindLibrary)
	if !viewOK {
		t.Fatal("the sealed table holds no library contract surface")
	}
	table, tableOK := library.NewTable(view)
	if !tableOK {
		t.Fatal("the sealed library contract surface did not project")
	}
	return table
}

func declaredKind(t *testing.T, key schema.Key) *library.Entry {
	t.Helper()
	entry, ok := resolveKind(declaredTable(t), key)
	if !ok {
		t.Fatalf("the declaration root declares no contract kind %q", key)
	}
	return entry
}

// declaredInstance authors one instance of the declared library kind, over the
// real codec and the real payload format identities.
func declaredInstance(t *testing.T) *Instance {
	t.Helper()
	kind := declaredKind(t, composite.LibraryContractKind)
	body, err := EncodePath(Export("target"))
	if err != nil {
		t.Fatalf("the export path payload did not encode: %v", err)
	}
	edge := deferredMember(t, kind, library.FormMetatableEdge, Metatable("__index"))
	edge.Encoding, edge.Body = EncodingResolved, body
	instance, ok := New(Spec{
		Kind:  kind.Key(),
		Codec: kind.Codec(),
		Root:  "codec",
		Members: []Member{
			deferredMember(t, kind, library.FormExportValue, Root()),
			edge,
			deferredMember(t, kind, library.FormCallableSignature, Export("target")),
		},
	}, kind)
	if !ok {
		t.Fatal("the authored instance was rejected by the declared kind")
	}
	return instance
}

// TestInstanceSurvivesItsOwnCodec is the round trip. Every authored fact - the
// kind, the codec, the mount selector, each member's form, address, payload
// format, encoding and body - comes back as it went in.
func TestInstanceSurvivesItsOwnCodec(t *testing.T) {
	instance := declaredInstance(t)
	data, err := Encode(instance)
	if err != nil {
		t.Fatalf("the instance did not encode: %v", err)
	}
	decoded, err := Decode(data, declaredTable(t))
	if err != nil {
		t.Fatalf("the instance did not decode: %v", err)
	}
	if decoded.Kind() != instance.Kind() || decoded.Codec() != instance.Codec() ||
		decoded.Root() != instance.Root() || decoded.Class() != instance.Class() {
		t.Fatal("the decoded instance is not the instance that was written")
	}
	if decoded.Count() != instance.Count() {
		t.Fatalf("decoded rows=%d want %d", decoded.Count(), instance.Count())
	}
	for position := 0; position < instance.Count(); position++ {
		want, _ := instance.At(position)
		got, _ := decoded.At(position)
		if got.Form != want.Form || !got.Path.Equal(want.Path) || got.Payload != want.Payload ||
			got.Encoding != want.Encoding || !bytes.Equal(got.Body, want.Body) {
			t.Fatalf("member %d did not survive its codec", position)
		}
	}
	if ContentID(decoded) != ContentID(instance) {
		t.Fatal("the decoded instance carries another identity than the one written")
	}
}

// TestDecodeAdmitsOnNoWeakerGroundsThanAuthoring keeps one law set. A stream is
// resolved against the sealed table and then admitted by New; there is no
// second, looser path into an instance.
func TestDecodeAdmitsOnNoWeakerGroundsThanAuthoring(t *testing.T) {
	data, err := Encode(declaredInstance(t))
	if err != nil {
		t.Fatalf("the instance did not encode: %v", err)
	}
	if _, err := Decode(data, library.Table{}); err != ErrUnknownKind {
		t.Fatalf("a stream naming a kind no table declares decoded with %v", err)
	}
	for cut := 1; cut < len(data); cut++ {
		if _, err := Decode(data[:cut], declaredTable(t)); err == nil {
			t.Fatalf("a stream truncated at %d decoded as an instance", cut)
		}
	}
	if _, err := Decode(append(append([]byte(nil), data...), 0), declaredTable(t)); err == nil {
		t.Fatal("a stream with trailing bytes decoded as an instance")
	}
	if _, err := Decode(nil, declaredTable(t)); err == nil {
		t.Fatal("an empty stream decoded as an instance")
	}
}

// TestContentIdentityCoversEveryAuthoredFact states that the instance identity
// is the contract. A member added, moved, readdressed, or re-encoded is a
// different contract and the identity says so.
func TestContentIdentityCoversEveryAuthoredFact(t *testing.T) {
	kind := declaredKind(t, composite.LibraryContractKind)
	base := declaredInstance(t)
	identical := declaredInstance(t)
	if ContentID(base) != ContentID(identical) {
		t.Fatal("two instances of one authored contract carry different identities")
	}
	if !ContentID(base).Available() {
		t.Fatal("an admitted instance has no content identity")
	}
	variants := map[string]func(spec *Spec){
		"another mount selector": func(spec *Spec) { spec.Root = "other" },
		"a readdressed member":   func(spec *Spec) { spec.Members[2].Path = Export("other") },
		"a reordered member": func(spec *Spec) {
			spec.Members[0], spec.Members[2] = spec.Members[2], spec.Members[0]
		},
		"a dropped member": func(spec *Spec) { spec.Members = spec.Members[:2] },
		"another payload body": func(spec *Spec) {
			body, err := EncodePath(Root())
			if err != nil {
				t.Fatalf("the export path payload did not encode: %v", err)
			}
			spec.Members[1].Body = body
		},
	}
	for name, mutate := range variants {
		spec := respec(t, base)
		mutate(&spec)
		variant, ok := New(spec, kind)
		if !ok {
			t.Fatalf("%s: the variant was rejected", name)
		}
		if ContentID(variant) == ContentID(base) {
			t.Fatalf("%s: the variant carries the identity of the contract it is not", name)
		}
	}
}

// respec recovers an authored spec from an admitted instance, so a variant is
// built from exactly what the base contract states.
func respec(t *testing.T, instance *Instance) Spec {
	t.Helper()
	return Spec{
		Kind:    instance.Kind(),
		Codec:   instance.Codec(),
		Root:    instance.Root(),
		Members: instance.Members(),
	}
}

// TestExportPathPayloadSurvivesItsOwnCodec is the round trip of the one member
// payload format this package owns.
func TestExportPathPayloadSurvivesItsOwnCodec(t *testing.T) {
	paths := []Path{
		Root(),
		Export("len"),
		Metatable("__index"),
		NewPath(Step{Kind: StepMetatable, Key: "__index"}, Step{Kind: StepExport, Key: "sub"}),
	}
	for _, want := range paths {
		data, err := EncodePath(want)
		if err != nil {
			t.Fatalf("the path did not encode: %v", err)
		}
		got, err := DecodePath(data)
		if err != nil {
			t.Fatalf("the path did not decode: %v", err)
		}
		if !got.Equal(want) {
			t.Fatal("the decoded path is not the path that was written")
		}
	}
	if _, err := EncodePath(NewPath(Step{Kind: StepExport})); err == nil {
		t.Fatal("a path that reaches nothing encoded as a payload")
	}
	if _, err := DecodePath(nil); err == nil {
		t.Fatal("an empty payload decoded as a path")
	}
	instance, err := Encode(declaredInstance(t))
	if err != nil {
		t.Fatalf("the instance did not encode: %v", err)
	}
	if _, err := DecodePath(instance); err == nil {
		t.Fatal("an instance stream decoded as an export path payload")
	}
}

// TestExportPathPayloadWireIsPinned holds the payload bytes still. A metatable
// edge is the one resolved payload a shipped contract carries today, so its
// encoding is a compatibility surface and not an implementation detail.
func TestExportPathPayloadWireIsPinned(t *testing.T) {
	pinned := map[string]string{
		"root":            "0125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d70617468020101040100",
		"export len":      "0125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d7061746802010104010103010305010108036c656e",
		"metatable index": "0125616e616c797369732f6c6962726172792f636f6e74726163742f6578706f72742d7061746802010104010103010305010208075f5f696e646578",
	}
	subjects := map[string]Path{"root": Root(), "export len": Export("len"), "metatable index": Metatable("__index")}
	for name, path := range subjects {
		data, err := EncodePath(path)
		if err != nil {
			t.Fatalf("%s: the path did not encode: %v", name, err)
		}
		if got := hex.EncodeToString(data); got != pinned[name] {
			t.Errorf("%s: payload wire is %s, pinned %s", name, got, pinned[name])
		}
	}
}
