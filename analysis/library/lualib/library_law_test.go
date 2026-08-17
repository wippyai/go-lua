package lualib

import (
	"encoding/hex"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/domain/effect/capability"
	caplabel "github.com/wippyai/go-lua/analysis/domain/effect/capability/label"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// declaredKind projects the sealed library contract surface and resolves one
// declared kind. An instance is authored against the declaration root, never
// against a restatement of it.
func declaredKind(t *testing.T, key schema.Key) *library.Entry {
	t.Helper()
	for position := 0; position < declaredTable(t).Count(); position++ {
		entry, ok := declaredTable(t).At(position)
		if ok && entry != nil && entry.Key() == key {
			return entry
		}
	}
	t.Fatalf("the declaration root declares no contract kind %q", key)
	return nil
}

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

// modeledNamespace returns the modeled standard-library names of one namespace,
// as one-step export names, sorted. It is the inventory the modeled table is
// held to while it still exists: the authored instance is the statement, and
// this is what the table must have been holding all along.
func modeledNamespace(root string) []string {
	prefix := root + "."
	var names []string
	for _, name := range signaturelookup.StdlibSignatureNames() {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		member := strings.TrimPrefix(name, prefix)
		if strings.Contains(member, ".") {
			continue
		}
		names = append(names, member)
	}
	sort.Strings(names)
	return names
}

// libraryCase is one shipped library contract instance and everything the
// package states about it: what it exports, how much of it is address and how
// much is content, and the bytes it publishes as.
type libraryCase struct {
	name       string
	root       string
	exports    []string
	signatures map[string]signature.Function
	authored   func(kind *library.Entry) (*contract.Instance, bool)
	// values and methods are the authored rows this library publishes below its
	// root that are not direct callable exports: the constants and nested
	// aggregates, and the callables reached one step further along the export
	// graph.
	values      []valueExport
	methods     []methodExport
	metatable   string
	refinements int
	suspensions int
	delegations int
	denials     int
	// modeled states whether the retiring standard-library signature table holds
	// this namespace at all. A namespace it models, it must model completely, and
	// the inventory law below derives the whole of it from the authored contract.
	// A namespace it never held has nothing to derive: the contract is the only
	// statement about it, and the law holds the table to holding nothing.
	modeled    bool
	pinnedID   string
	pinnedSize int
}

// libraryCorpus is the shipped Lua library inventory. A library authored in this
// package without a row here is a shipped artifact under no law, which the
// coverage law below states.
func libraryCorpus() []libraryCase {
	return []libraryCase{
		{
			name: "string", root: StringRoot, exports: stringExports, signatures: stringSignatures, authored: StringContract,
			metatable: StringMetatableIndexKey, refinements: 1, delegations: 3, denials: 1, modeled: true,
			pinnedID:   "ec1db5e4d0795faf4b20bbf999dee7a67bf4effcc44000ab47e8202e0225c55c",
			pinnedSize: 6813,
		},
		{
			name: "math", root: MathRoot, exports: mathExports, signatures: mathSignatures, authored: MathContract,
			values: mathConstants, modeled: true,
			pinnedID:   "0436d0b77b9e1f1b42366a096b37d45057a4b11b2e88c8ec745b817fa8477760",
			pinnedSize: 8528,
		},
		{
			name: "table", root: TableRoot, exports: tableExports, signatures: tableSignatures, authored: TableContract,
			modeled:    true,
			pinnedID:   "26ef5f260a5edcb759120177c824e7a850be854321bb58ee5ead2c5093d9a779",
			pinnedSize: 4687,
		},
		{
			name: "os", root: OSRoot, exports: osExports, signatures: osSignatures, authored: OSContract,
			modeled:    true,
			pinnedID:   "e129d33a5d9eff8058ffab31e78e0f452349ee8823f4e71f5f767345d5a64a71",
			pinnedSize: 2678,
		},
		{
			name: "coroutine", root: CoroutineRoot, exports: coroutineExports, signatures: coroutineSignatures, authored: CoroutineContract,
			suspensions: 2, modeled: true,
			pinnedID:   "bd41532b4487ef6e18606f60d6fda9e0e2c9af38e32ddfce5feaebea5725b7a8",
			pinnedSize: 2645,
		},
		{
			name: "utf8", root: UTF8Root, exports: utf8Exports, signatures: utf8Signatures, authored: UTF8Contract,
			values:     utf8Constants,
			pinnedID:   "de876853d93eceeb85046c1ee2c59782226c3b5547ad0758deb69780941ad278",
			pinnedSize: 2014,
		},
		{
			name: "debug", root: DebugRoot, exports: debugExports, signatures: debugSignatures, authored: DebugContract,
			pinnedID:   "3e19e4e2cb75e180666cd594f3a85ac2bbe165f59098a2bdc1aac81c19b1049d",
			pinnedSize: 478,
		},
		{
			name: "errors", root: ErrorsRoot, exports: errorsExports, signatures: errorsSignatures, authored: ErrorsContract,
			values: errorsValues, methods: errorsMethods,
			pinnedID:   "6d3f7702a134670d0bd08416b1f56360a0512185fcb3ab060e86899bcd7b21c1",
			pinnedSize: 4400,
		},
	}
}

func (testCase libraryCase) instance(t *testing.T) *contract.Instance {
	t.Helper()
	instance, ok := testCase.authored(declaredKind(t, composite.LibraryContractKind))
	if !ok {
		t.Fatalf("the %s library contract was rejected by the declared library kind", testCase.name)
	}
	return instance
}

// rows is the member count the authored shape implies: the aggregate export, one
// export value per published constant or nested aggregate, the metatable edge
// when the library publishes one, one callable per export and per method, one
// refinement per refined export, one suspension per suspending export, one
// delegation per delegated export and one denial per member the library declares
// and refuses to publish.
func (testCase libraryCase) rows() int {
	rows := 1 + len(testCase.values) + len(testCase.exports) + len(testCase.methods) +
		testCase.refinements + testCase.suspensions + testCase.delegations + testCase.denials
	if testCase.metatable != "" {
		rows++
	}
	return rows
}

// TestLibraryContractsAreAdmittedByTheDeclaredLibraryKind is the base case: a
// shipped instance is admitted by the kind the analyzer declares, over that
// kind's own codec and payload formats.
func TestLibraryContractsAreAdmittedByTheDeclaredLibraryKind(t *testing.T) {
	kind := declaredKind(t, composite.LibraryContractKind)
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			instance := testCase.instance(t)
			if instance.Kind() != kind.Key() || instance.Codec() != kind.Codec() {
				t.Fatal("the instance is not published under the kind it was admitted against")
			}
			if instance.Class() != library.ClassLibrary {
				t.Fatal("the library is not published as a library contract")
			}
			if instance.Root() != testCase.root {
				t.Fatalf("mount selector is %q, want %q", instance.Root(), testCase.root)
			}
			if instance.Count() != testCase.rows() {
				t.Fatalf("rows=%d want %d", instance.Count(), testCase.rows())
			}
		})
	}
}

// TestModeledStandardLibraryInventoryIsDerivedFromTheAuthoredContracts is the
// drift law of the inventory, and it runs in the direction the retirement
// establishes. The authored contract is the statement of which members a library
// has; the modeled signature table it will replace is derived from it here and
// held to it. While both exist they must be the same inventory, and when they
// differ the derived side is the one that is wrong.
func TestModeledStandardLibraryInventoryIsDerivedFromTheAuthoredContracts(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			expected := append([]string(nil), testCase.exports...)
			sort.Strings(expected)
			modeled := modeledNamespace(testCase.root)
			if !testCase.modeled {
				// A namespace the retiring table never held. The contract is the
				// only statement about it, so there is nothing to derive - and a
				// partial inventory appearing in the model later would be a second
				// statement, which is what this holds against.
				if len(modeled) != 0 {
					t.Fatalf("the modeled table holds %d members of a namespace it does not model", len(modeled))
				}
				expected = nil
			}
			if len(modeled) != len(expected) {
				t.Fatalf("the authored contract exports %d members and the modeled table holds %d",
					len(expected), len(modeled))
			}
			for index, name := range expected {
				if modeled[index] != name {
					t.Fatalf("authored export %d is %q and the modeled table holds %q", index, name, modeled[index])
				}
			}
			instance := testCase.instance(t)
			for _, name := range testCase.exports {
				if _, found := instance.Resolve(library.FormCallableSignature, contract.Export(name)); !found {
					t.Fatalf("the contract publishes no callable at the address of %q", name)
				}
			}
			if _, found := instance.Resolve(library.FormCallableSignature, contract.Export("absent")); found {
				t.Fatal("the contract publishes a callable it does not author")
			}
		})
	}
}

// TestAuthoredSignaturesCoverExactlyTheAuthoredExports keeps the two authored
// halves of one instance in step. An export with no signature would publish a
// callable member that describes no application, and a signature under no export
// would be a member the contract never publishes: either way the instance would
// hold a statement that reaches nothing.
func TestAuthoredSignaturesCoverExactlyTheAuthoredExports(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			if len(testCase.signatures) != len(testCase.exports) {
				t.Fatalf("the contract authors %d signatures for %d exports",
					len(testCase.signatures), len(testCase.exports))
			}
			for _, name := range testCase.exports {
				sig, authored := testCase.signatures[name]
				if !authored || sig.Type == nil {
					t.Fatalf("the export %q has no authored signature", name)
				}
			}
		})
	}
}

// TestModeledStandardLibrarySignaturesAreDerivedFromTheAuthoredEnvelopes is the
// content law of the callable form, in the same flipped direction. The inventory
// law ties the two member sets; this derives what the modeled table must hold
// for each member from the envelope the instance publishes - effect row included
// - and holds the table to it. The expectation is read out of the serialized
// contract rather than out of the authored map, so the envelope that a consumer
// would decode is the one the table is checked against.
func TestModeledStandardLibrarySignaturesAreDerivedFromTheAuthoredEnvelopes(t *testing.T) {
	source := signaturelookup.Source{IncludeStdlib: true}
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			instance := testCase.instance(t)
			for _, name := range testCase.exports {
				member, found := instance.Resolve(library.FormCallableSignature, contract.Export(name))
				if !found {
					t.Fatalf("the contract publishes no callable at the address of %q", name)
				}
				if member.Encoding != contract.EncodingResolved {
					t.Fatalf("the callable envelope of %q is deferred", name)
				}
				expected, err := wire.DecodeCallableSignature(member.Body)
				if err != nil {
					t.Fatalf("the callable envelope of %q did not decode: %v", name, err)
				}
				if !expected.Equals(testCase.signatures[name]) {
					t.Fatalf("the published envelope of %q is not the signature the instance authors", name)
				}
				if !testCase.modeled {
					continue
				}
				modeled, ok := source.LookupView(testCase.root + "." + name)
				if !ok {
					t.Fatalf("the modeled table holds no signature for the authored export %q", name)
				}
				if !modeled.Equals(expected) {
					t.Fatalf("the modeled table applies %q as %s, and the authored contract states %s",
						name, modeled, expected)
				}
			}
		})
	}
}

// TestAuthoredEffectLabelsAreAuditedCapabilities is the label law of the
// authoring side. A contract that published an effect label the capability
// vocabulary does not audit would be stating a capability nobody governs, and one
// that published a reserved label would be stating a capability that is declared
// not to be exercised yet.
func TestAuthoredEffectLabelsAreAuditedCapabilities(t *testing.T) {
	corpus := map[string]map[string]signature.Function{"_ENV": globalsSignatures}
	for _, testCase := range libraryCorpus() {
		signatures := make(map[string]signature.Function, len(testCase.signatures)+len(testCase.methods))
		for name, sig := range testCase.signatures {
			signatures[name] = sig
		}
		for _, method := range testCase.methods {
			step, ok := method.Path.At(method.Path.Len() - 1)
			if !ok {
				t.Fatalf("the %s library authors a method at no address", testCase.name)
			}
			signatures[step.Key] = method.Signature
		}
		corpus[testCase.root] = signatures
	}
	for root, signatures := range corpus {
		for name, sig := range signatures {
			for _, label := range sig.Effect.Labels {
				descriptor, audited := caplabel.DescriptorFor(label)
				if !audited {
					t.Fatalf("%s.%s publishes the unaudited effect label %T", root, name, label)
				}
				switch descriptor.Status {
				case capability.StatusReserved, capability.StatusReservedHighRisk:
					t.Fatalf("%s.%s publishes the inactive effect label %s (%s)",
						root, name, descriptor.ID, descriptor.Status)
				}
			}
		}
	}
}

// TestLibraryContractsDeclareNoEnvironmentShape keeps the boundary the surface
// states: an individual library owns its own export graph and nothing outside
// it, so it can neither carry an environment form nor be published as the
// environment contract.
func TestLibraryContractsDeclareNoEnvironmentShape(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			for _, member := range testCase.instance(t).Members() {
				if member.Form.Environment() {
					t.Fatalf("the %s library contract carries the environment form %d", testCase.name, member.Form)
				}
			}
			if _, ok := testCase.authored(declaredKind(t, composite.EnvironmentContractKind)); ok {
				t.Fatalf("the %s library was authored as an environment contract", testCase.name)
			}
			if _, ok := testCase.authored(nil); ok {
				t.Fatalf("the %s library was authored against no kind at all", testCase.name)
			}
		})
	}
}

// TestLibraryContractsStateWhatTheyCannotYetSerialize is the honesty receipt. No
// sealed rule owns pattern-capture result selection for a delegation to name, so
// those members carry an address and say so rather than carrying an empty payload
// that looks complete. Every other member form these contracts publish now has a
// landed format and carries its content.
func TestLibraryContractsStateWhatTheyCannotYetSerialize(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			instance := testCase.instance(t)
			if instance.Deferred() != testCase.delegations {
				t.Fatalf("deferred rows=%d want %d", instance.Deferred(), testCase.delegations)
			}
			for _, member := range instance.Members() {
				deferred := member.Form == library.FormRuleDelegation
				if deferred != (member.Encoding == contract.EncodingDeferred) {
					t.Fatalf("form %d states an encoding its payload format does not support", member.Form)
				}
			}
		})
	}
}

// TestLibraryRootsArePublishedAsMutableAggregates is the content law of the
// export-value form. A library root is the aggregate a mount binds to, and the
// mutability it carries is the library's OWN statement about its export: the Lua
// standard library places no seal on its tables. Whether a host boots that
// aggregate frozen is the initial environment's business, carried by forms only
// the environment class declares, so no library instance can state it here.
func TestLibraryRootsArePublishedAsMutableAggregates(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			member, found := testCase.instance(t).Resolve(library.FormExportValue, contract.Root())
			if !found {
				t.Fatal("the contract says nothing about the value its root is")
			}
			if member.Encoding != contract.EncodingResolved {
				t.Fatal("the root export value is deferred, so it states no aggregate")
			}
			decoded, err := contract.DecodeExportValue(member.Body)
			if err != nil {
				t.Fatalf("the root export value did not decode: %v", err)
			}
			if decoded != contract.Aggregate(contract.MutabilityMutable) {
				t.Fatalf("the root is published as %+v, want a mutable aggregate", decoded)
			}
		})
	}
}

// TestPublishedExportValuesAreTheAuthoredValues is the content law of the
// export-value form below the root. A published constant carries the exact value
// it is - the bits of a float, the bytes of a string - and a published aggregate
// carries no constant at all, because there is no constant an aggregate could be.
// The set is exact in both directions: an export value at an address the library
// does not author would be a value nobody wrote.
func TestPublishedExportValuesAreTheAuthoredValues(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			instance := testCase.instance(t)
			for _, value := range testCase.values {
				if value.Path.Len() == 0 {
					t.Fatal("an authored export value addresses the contract root")
				}
				member, found := instance.Resolve(library.FormExportValue, value.Path)
				if !found {
					t.Fatalf("the contract publishes no export value at %v", pathSteps(value.Path))
				}
				decoded, err := contract.DecodeExportValue(member.Body)
				if err != nil {
					t.Fatalf("the export value at %v did not decode: %v", pathSteps(value.Path), err)
				}
				if decoded != value.Value {
					t.Fatalf("the contract publishes %+v at %v and authors %+v",
						decoded, pathSteps(value.Path), value.Value)
				}
			}
			var published int
			for _, member := range instance.Members() {
				if member.Form == library.FormExportValue {
					published++
				}
			}
			if published != len(testCase.values)+1 {
				t.Fatalf("the contract publishes %d export values and authors %d plus its root",
					published, len(testCase.values))
			}
		})
	}
}

// TestPublishedMethodsAreReachedThroughAnAuthoredAggregate is the addressing law
// of a member published below the root. A callable deeper than a direct export is
// reached by walking exported values, so every prefix of its address must be a
// value this contract publishes as an aggregate - a value the path may continue
// through. A method hanging off a constant, or off a value the contract never
// states, would be addressed by a walk nobody could perform.
func TestPublishedMethodsAreReachedThroughAnAuthoredAggregate(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			instance := testCase.instance(t)
			for _, method := range testCase.methods {
				if method.Path.Len() < 2 {
					t.Fatalf("the method at %v is a direct export and belongs to the inventory",
						pathSteps(method.Path))
				}
				for prefix := 1; prefix < method.Path.Len(); prefix++ {
					owner := contract.NewPath(pathSteps(method.Path)[:prefix]...)
					member, found := instance.Resolve(library.FormExportValue, owner)
					if !found {
						t.Fatalf("the method at %v is reached through %v, which the contract does not publish",
							pathSteps(method.Path), pathSteps(owner))
					}
					decoded, err := contract.DecodeExportValue(member.Body)
					if err != nil {
						t.Fatalf("the export value at %v did not decode: %v", pathSteps(owner), err)
					}
					if decoded.Shape != contract.ValueShapeAggregate {
						t.Fatalf("the method at %v is reached through %v, which is not an aggregate",
							pathSteps(method.Path), pathSteps(owner))
					}
				}
				member, found := instance.Resolve(library.FormCallableSignature, method.Path)
				if !found {
					t.Fatalf("the contract publishes no callable at %v", pathSteps(method.Path))
				}
				envelope, err := wire.DecodeCallableSignature(member.Body)
				if err != nil {
					t.Fatalf("the callable envelope at %v did not decode: %v", pathSteps(method.Path), err)
				}
				if !envelope.Equals(method.Signature) {
					t.Fatalf("the published envelope at %v is not the signature the instance authors",
						pathSteps(method.Path))
				}
			}
		})
	}
}

// TestLibraryContractWiresArePinned holds the shipped contracts' serialized
// bytes still. An instance is a data artifact: a member added, moved or
// readdressed is a different contract, and this is where that shows.
func TestLibraryContractWiresArePinned(t *testing.T) {
	for _, testCase := range libraryCorpus() {
		t.Run(testCase.name, func(t *testing.T) {
			instance := testCase.instance(t)
			data, err := contract.Encode(instance)
			if err != nil {
				t.Fatalf("the %s library contract did not encode: %v", testCase.name, err)
			}
			// The identity is the digest of exactly these bytes, so pinning both
			// holds the framing and the content still as the separate facts they
			// are.
			if len(data) != testCase.pinnedSize {
				t.Errorf("contract wire is %d bytes, pinned %d", len(data), testCase.pinnedSize)
			}
			id := contract.ContentID(instance)
			if got := hex.EncodeToString(id[:]); got != testCase.pinnedID {
				t.Errorf("contract identity is %s, pinned %s", got, testCase.pinnedID)
			}
			decoded, err := contract.Decode(data, declaredTable(t))
			if err != nil {
				t.Fatalf("the %s library contract did not decode: %v", testCase.name, err)
			}
			if contract.ContentID(decoded) != id {
				t.Fatal("the decoded contract is not the contract that was written")
			}
		})
	}
}

// TestLibraryCorpusCoversTheShippedNamespaces states that every namespace the
// modeled standard library declares is either a shipped instance in this package
// or a mount this package deliberately does not author. A namespace that appears
// in the model and in neither list is a library absorbed by nobody.
func TestLibraryCorpusCoversTheShippedNamespaces(t *testing.T) {
	shipped := map[string]bool{}
	for _, testCase := range libraryCorpus() {
		shipped[testCase.root] = true
	}
	// The host mounts are not the Lua standard library. They are modeled in the
	// same table today, and absorbing them as Lua libraries would publish a host
	// service as part of the language, so they are named here as the separate
	// mounts they are and authored by whoever owns them.
	hosted := map[string]bool{"json": true, "env": true, "ownership": true}
	for _, name := range signaturelookup.StdlibSignatureNames() {
		root, _, dotted := strings.Cut(name, ".")
		if !dotted {
			continue
		}
		if !shipped[root] && !hosted[root] {
			t.Fatalf("the modeled library %q is absorbed by no instance and named as no host mount", root)
		}
	}
}
