// Package relationfixture seals the analysis world every axis binds its laws
// against.
//
// Every authority in it is the production one, and it is mounted the way
// production mounts it: the composition's own mount phase derives the sealed
// authorities and the phase's post-mount derivations from one real linked
// program, and this package reads them off the record it produced. It seals
// nothing itself, so a law proven here is proven against the same authorities
// the analyzer runs on and never against a second construction of them.
package relationfixture

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	targetcontract "github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	indexdomain "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// Sealed is one mounted analysis world: the authorities the mount phase sealed
// and the receiver a binding law observes through.
type Sealed struct {
	Heap     heapdomain.Schema
	Values   *valuedomain.Schema
	Calls    *calldomain.Algebra
	Packs    *packdomain.Schema
	Classes  *staticdomain.ClassSet
	Effects  *effectfactor.Algebra
	Topology *indexdomain.Topology
	Receiver valuedomain.Value
	Root     heapdomain.Key
}

const fixtureSource = `local holder = {}
holder.field = nil
return holder.field`

// callingSource makes a call and judges what it returns: the result flows into
// the table whose field the program then reads, so the body call site is one
// the effect fold answers over rather than one it merely sees.
// governedSource calls the host operations the resource protocol governs,
// including the one that declares an escape.
const governedSource = "local resource = require(\"resource\")\n" +
	"local connection = resource.connect()\n" +
	"resource.query(connection)\n" +
	"resource.close(connection)\n" +
	"local handed = resource.connect()\n" +
	"resource.detach(handed)\n"

const callingSource = `local function build()
  local made = {}
  made.field = nil
  return made
end
local holder = build()
return holder.field`

func portableAnyTypes(count int) []schematype.Type {
	values := make([]schematype.Type, count)
	for index := range values {
		value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
		if !ok {
			panic("portable any type")
		}
		values[index] = value
	}
	return values
}

// New mounts the fixture world through the composition's own mount phase.
func New(t testing.TB) Sealed { return seal(t, "relbindgen_binding_law.lua", fixtureSource) }

// NewCalling mounts a world whose program makes a real call and judges what
// the call returns.
//
// It is a second program rather than a change to the first: a law that reads
// what a call site answers needs one, and every law that does not must keep
// reading exactly the world it was written against.
func NewCalling(t testing.TB) Sealed {
	return seal(t, "relbindgen_call_site_law.lua", callingSource)
}

// NewGoverned mounts a world whose program calls the host operations a
// typestate protocol governs.
//
// It is a third program because the judgment it exists for only seals in one.
// A protocol's transitions and escapes are sealed where the pack schema
// resolves an input selector for the operation they are declared over, which
// happens where a mounted program actually calls that operation - so a world
// of local calls reaches a body call site and a world of host calls reaches a
// governed obligation, and neither is the other.
func NewGoverned(t testing.TB) Sealed { return sealGoverned(t, governedSource) }

func seal(t testing.TB, name string, source string) Sealed {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: name, Text: []byte(source)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Seal(&declaration.Spec{Semantics: domaincontract.NewSemantics(), Operations: []vocabulary.OperationSpec{{
		Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
		Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
		Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: portableAnyTypes(1), Tail: vocabulary.ValuesClosed}}},
		Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	return mount(t, contract, linked, true)
}

// sealGoverned links one program against the standard library target, whose
// resource host declares the typestate protocols an obligation is judged under.
func sealGoverned(t testing.TB, source string) Sealed {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "relbindgen_obligation_law.lua", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return mount(t, contract, linked, false)
}

// mount runs the composition's phase over one linked program. receiver says
// whether this world is expected to allocate a Lua table root: the worlds whose
// laws observe one require it, and a world of host calls allocates none.
func mount(t testing.TB, contract *targetcontract.Contract, linked *link.Link, receiver bool) Sealed {
	t.Helper()
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("the binding fixture has no program schema receipt")
	}

	mounts := linked.Project().Mounts()
	programs := make([]programschema.Program, mounts.Count())
	rows := make([]programmount.MountedArtifact, mounts.Count())
	statics := make([]staticdomain.MountedProgram, mounts.Count())
	for index := 0; index < mounts.Count(); index++ {
		shard, shardOK := mounts.At(index)
		source, sourceOK := mounts.Program(shard)
		module, moduleOK := linked.Project().ModuleKey(shard)
		if !shardOK || !sourceOK || source == nil || !moduleOK {
			t.Fatalf("binding fixture mount %d has no artifact source", index)
		}
		artifact, failure := artifactcompiler.CompileDetailed(source, grammar, issuance)
		if failure.Available() || artifact == nil || !artifact.Available() {
			t.Fatalf("binding fixture artifact %d: %v", index, failure)
		}
		lowered := snapshottest.MustLower(t, artifact)
		mounted, mountedOK := programmount.MountedArtifactFromSnapshot(lowered, module)
		if !mountedOK {
			t.Fatalf("binding fixture mounted artifact %d", index)
		}
		programs[index] = artifact.Program()
		rows[index] = mounted
		statics[index] = staticdomain.MountedProgram{Program: mounted.Program.Program, ModuleID: module, NamespaceID: module}
	}

	boundary, _ := linked.Boundary().Target()
	types, typeErr := typeauthority.SealProgramRows(linked.ContentID(), programs, nil)
	if typeErr != nil || types == nil {
		t.Fatalf("binding fixture type seal: %v", typeErr)
	}
	inventory, _, staticErr := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: boundary}, types, statics)
	if staticErr != nil || inventory == nil {
		t.Fatalf("binding fixture static seal: %v", staticErr)
	}

	record, failure := composite.MountLink(compilation, composite.LinkInputs{Source: linked, Artifacts: rows, StaticAuthority: inventory})
	if failure.Available() {
		t.Fatalf("binding fixture mount: %v", failure)
	}
	fixture := Sealed{
		Heap:     record.HeapInput(),
		Values:   record.ValueInput(),
		Calls:    record.CallInput(),
		Packs:    record.PackInput(),
		Classes:  inventory.Classes(),
		Effects:  record.EffectInput(),
		Topology: record.IndexTopology(),
	}
	if receiver {
		fixture.Root, fixture.Receiver = fixtureReceiver(t, fixture.Heap, fixture.Values)
	}
	return fixture
}

// fixtureReceiver returns the table root the program allocates and a receiver
// value that observes exactly that root.
func fixtureReceiver(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema) (heapdomain.Key, valuedomain.Value) {
	t.Helper()
	for index := 0; index < heap.KeyCount(); index++ {
		candidate, ok := heap.KeyAt(index)
		_, _, _, kind, _, source := heap.AllocationOriginForKey(candidate)
		if !ok || !source || kind != heapdomain.AllocationTable {
			continue
		}
		atom, atomOK := values.Allocation(candidate, materialization.Recent)
		if !atomOK {
			continue
		}
		receiver, receiverOK := values.Singleton(atom)
		if !receiverOK {
			continue
		}
		return candidate, receiver
	}
	t.Fatal("binding fixture table root")
	return heapdomain.Key{}, valuedomain.Value{}
}
