package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/analysis/schema/program/publication"
)

// The list position is a dimension of exactly the two families that fan one
// authored statement out over a value list. A read names a single cell, so a
// read carrying a position names nothing Program ever issues and is not an
// identity; a bind and an assignment write are issued once per position, so
// each of their positions is a distinct identity.
func TestStorageTransferRefAdmitsPositionOnlyForPositionalFamilies(t *testing.T) {
	var link, mount, occurrence identity.ContentID
	link[31], mount[31], occurrence[31] = 1, 2, 3

	for _, testcase := range []struct {
		name     string
		kind     storageTransferKind
		position uint32
		valid    bool
	}{
		{name: "read at the only position it has", kind: storageTransferRead, position: 0, valid: true},
		{name: "read carrying a list position", kind: storageTransferRead, position: 1, valid: false},
		{name: "bind at the first target", kind: storageTransferBind, position: 0, valid: true},
		{name: "bind at a later target", kind: storageTransferBind, position: 3, valid: true},
		{name: "write at the first target", kind: storageTransferWrite, position: 0, valid: true},
		{name: "write at a later target", kind: storageTransferWrite, position: 3, valid: true},
		{name: "no family", kind: storageTransferInvalid, position: 0, valid: false},
	} {
		ref := StorageTransferRef{linkID: link, mount: mount, occurrence: occurrence, kind: testcase.kind, position: testcase.position}
		if got := ref.valid(); got != testcase.valid {
			t.Fatalf("%s: identity valid=%t, want %t", testcase.name, got, testcase.valid)
		}
	}
}

// Each admitted position is its own relation: the content identity separates
// two targets of the same statement, so one target's carried value can never
// replay through the other's receipt.
func TestStorageTransferIdentitySeparatesTargetPositions(t *testing.T) {
	var link, mount, occurrence identity.ContentID
	link[31], mount[31], occurrence[31] = 1, 2, 3

	first := storageTransferIdentity(StorageTransferRef{linkID: link, mount: mount, occurrence: occurrence, kind: storageTransferWrite, position: 0})
	second := storageTransferIdentity(StorageTransferRef{linkID: link, mount: mount, occurrence: occurrence, kind: storageTransferWrite, position: 1})
	if !first.Available() || !second.Available() {
		t.Fatal("positional write identities are unavailable")
	}
	if first == second {
		t.Fatal("two target positions share one content identity")
	}
}

func storageTransferLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

// canonicalStorageTransferLawProgram emits the same parent/input rows that
// Artifact publishes. These laws therefore exercise the mounted index against
// canonical spans instead of hand-assembling the retired occurrence shape.
func canonicalStorageTransferLawProgram(t *testing.T, memberPositions []uint64) (programschema.Program, map[identity.ContentID]int, map[identity.ContentID]lifecycle.StorageLifetime, []identity.ContentID) {
	t.Helper()
	body := storageTransferLawID(1)
	valuesID := storageTransferLawID(2)
	cellIDs := []identity.ContentID{storageTransferLawID(3), storageTransferLawID(4)}
	memberIDs := []identity.ContentID{storageTransferLawID(5), storageTransferLawID(6)}
	subjectIDs := []identity.ContentID{storageTransferLawID(13), storageTransferLawID(14)}
	bindID := storageTransferLawID(7)
	transferIDs := []identity.ContentID{storageTransferLawID(8), storageTransferLawID(9)}

	var occurrences []programschema.Occurrence
	var inputs []programschema.OccurrenceInput
	occurrenceOrdinals := make(map[identity.ContentID]int)
	appendOccurrence := func(kind programschema.OccurrenceKind, id identity.ContentID, code uint64, operands ...identity.ContentID) {
		offset := uint32(len(inputs))
		row, rowOK := programschema.NewOccurrence(kind, id, body, code, 0, 0, offset, uint32(len(operands)), keyspace.FamilyInvalid, keyspace.LiteralValue{}, false)
		if !rowOK {
			t.Fatalf("canonical occurrence %d", kind)
		}
		for _, operand := range operands {
			input, inputOK := programschema.NewOccurrenceInput(operand)
			if !inputOK {
				t.Fatalf("canonical occurrence input")
			}
			inputs = append(inputs, input)
		}
		occurrenceOrdinals[id] = len(occurrences)
		occurrences = append(occurrences, row)
	}

	appendOccurrence(programschema.OccurrenceValues, valuesID, 0)
	for index, position := range memberPositions {
		appendOccurrence(programschema.OccurrenceValuesMember, memberIDs[index], position, valuesID, subjectIDs[index])
	}
	appendOccurrence(programschema.OccurrenceStorageBind, bindID, 0, valuesID, cellIDs[0], cellIDs[1])
	for index, transferID := range transferIDs {
		appendOccurrence(programschema.OccurrenceStorageBindTransfer, transferID, uint64(index), bindID, memberIDs[index], cellIDs[index])
	}

	lifetimes := map[identity.ContentID]lifecycle.StorageLifetime{
		cellIDs[0]: lifecycle.StorageLifetimeFrame,
		cellIDs[1]: lifecycle.StorageLifetimeFrame,
	}
	tail, tailOK := programschema.NewValuesTail(identity.ContentID{}, identity.ContentID{}, programschema.ValuesTailInvalid, false)
	if !tailOK {
		t.Fatal("canonical Values tail")
	}
	valuesRow, valuesOK := programschema.NewValues(valuesID, body, identity.ContentID{}, 0, uint32(len(memberIDs)), tail)
	if !valuesOK {
		t.Fatal("canonical Values row")
	}
	valueMembers := make([]programschema.ValuesMember, 0, len(memberIDs))
	for _, memberID := range memberIDs {
		member, memberOK := programschema.NewValuesMember(memberID)
		if !memberOK {
			t.Fatal("canonical Values member")
		}
		valueMembers = append(valueMembers, member)
	}
	storageRows := make([]lifecycle.StorageCellLifetime, 0, len(lifetimes))
	for _, cellID := range cellIDs {
		row, rowOK := lifecycle.NewStorageCellLifetime(cellID, lifetimes[cellID])
		if !rowOK {
			t.Fatal("canonical storage lifetime")
		}
		storageRows = append(storageRows, row)
	}
	schemaID := storageTransferLawID(10)
	catalog, catalogOK := programcatalog.CatalogID(schemaID)
	if !catalogOK {
		t.Fatal("canonical storage catalog")
	}
	frozen, sealed := (publication.Publication{
		Values:           []programschema.Values{valuesRow},
		ValuesMembers:    valueMembers,
		Occurrences:      occurrences,
		OccurrenceInputs: inputs,
		Lifecycle:        lifecycle.Publication{StorageCellLifetimes: storageRows},
	}).Seal(catalog, identity.StoreID(1))
	program := programschema.Program{
		Frozen: frozen, ArtifactID: storageTransferLawID(11), ProgramID: storageTransferLawID(12), SchemaID: schemaID,
	}
	if !sealed || !program.Available() {
		t.Fatal("canonical storage publication")
	}
	return program, occurrenceOrdinals, lifetimes, transferIDs
}

func TestStorageOccurrenceIndexRequiresDenseValuesMembers(t *testing.T) {
	program, _, lifetimes, _ := canonicalStorageTransferLawProgram(t, []uint64{0, 2})
	if _, ok := storageOccurrenceIndexForProgram(program, lifetimes); ok {
		t.Fatal("ValuesMember gap was admitted as dense geometry")
	}
}

func TestStorageOccurrenceIndexUsesCanonicalBindCellDirectory(t *testing.T) {
	program, ordinals, lifetimes, transferIDs := canonicalStorageTransferLawProgram(t, []uint64{0, 1})
	index, ok := storageOccurrenceIndexForProgram(program, lifetimes)
	if !ok {
		t.Fatal("canonical storage occurrence index")
	}
	bindID := storageTransferLawID(7)
	for position, want := range []identity.ContentID{storageTransferLawID(3), storageTransferLawID(4)} {
		if got, found := index.bindCells[storageBindCellKey{bind: bindID, position: uint32(position)}]; !found || got != want {
			t.Fatalf("bind cell %d = %s/%v, want %s", position, got, found, want)
		}
	}
	for position, transferID := range transferIDs {
		ordinal := ordinals[transferID]
		row, rowOK := program.OccurrenceAt(ordinal)
		if !rowOK {
			t.Fatalf("transfer row %d", position)
		}
		gotPosition, fromID, toID, endpointsOK := storageTransferEndpoints(program, ordinal, row, storageTransferBind, index, lifetimes)
		if !endpointsOK || gotPosition != uint32(position) || fromID != storageTransferLawID(byte(5+position)) || toID != storageTransferLawID(byte(3+position)) {
			t.Fatalf("transfer %d endpoints = %d/%s/%s/%v", position, gotPosition, fromID, toID, endpointsOK)
		}
	}
}

func TestStorageTransferProofIdentityIsTypedAndCanonical(t *testing.T) {
	ref := StorageTransferRef{linkID: storageTransferLawID(20), mount: storageTransferLawID(21), occurrence: storageTransferLawID(22), kind: storageTransferWrite}
	legacy := storageTransferIdentity(ref)
	strong := storageTransferIdentityWithProof(ref, storageTransferLawID(23), storageTransferLawID(24), storageTransferLawID(25), lifecycle.StorageLifetimeFrame)
	if !legacy.Available() || !strong.Available() || legacy == strong {
		t.Fatal("legacy and proof-authenticated identities collapsed")
	}
	if strong == storageTransferIdentityWithProof(ref, storageTransferLawID(23), storageTransferLawID(24), storageTransferLawID(25), lifecycle.StorageLifetimeModule) {
		t.Fatal("lifetime proof was omitted from transfer identity")
	}
	if got := storageTransferIdentityWithProof(ref, identity.ContentID{}, storageTransferLawID(24), storageTransferLawID(25), lifecycle.StorageLifetimeFrame); got.Available() {
		t.Fatal("unavailable proof identity was admitted")
	}
}
