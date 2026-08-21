package storage

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/internal/framing"
)

func TestStorageCodecsUseFixedFramingAndPreimages(t *testing.T) {
	programID := storageLawID(0x10)
	bodyPath := storageLawID(0x20)
	bodyID := storageLawID(0x30)
	readPath := storageLawID(0x40)
	entryID := storageLawID(0x50)
	finishID := storageLawID(0x60)
	bindID := storageLawID(0x70)
	assignmentID := storageLawID(0x80)
	predecessorID := storageLawID(0x90)
	routeID := storageLawID(0xa0)
	digestID := storageLawID(0xb0)

	tests := []struct {
		name string
		got  func() (identity.ContentID, bool)
		want identity.ContentID
	}{
		{
			name: "storage read",
			got: func() (identity.ContentID, bool) {
				return StorageReadIdentity(programID, bodyPath, bodyID, readPath, entryID, finishID)
			},
			want: storageLawIdentity(t, "program/transformer/storage-read", programID, func(writer *framing.Writer) bool {
				return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(bodyID[:]) == nil &&
					writer.Bytes(readPath[:]) == nil && writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
			}),
		},
		{
			name: "storage bind",
			got: func() (identity.ContentID, bool) {
				return StorageBindIdentity(programID, bodyPath, 3, bodyID, entryID, finishID)
			},
			want: storageLawIdentity(t, "program/transformer/storage-bind", programID, func(writer *framing.Writer) bool {
				return writer.Bytes(bodyPath[:]) == nil && writer.Count(3) == nil && writer.Bytes(bodyID[:]) == nil &&
					writer.Bytes(entryID[:]) == nil && writer.Bytes(finishID[:]) == nil
			}),
		},
		{
			name: "storage bind transfer",
			got: func() (identity.ContentID, bool) {
				return StorageBindTransferIdentity(programID, bindID, 2)
			},
			want: storageLawIdentity(t, "program/transformer/storage-bind-transfer", programID, func(writer *framing.Writer) bool {
				return writer.Bytes(bindID[:]) == nil && writer.Uint(2) == nil
			}),
		},
		{
			name: "assignment predecessor",
			got: func() (identity.ContentID, bool) {
				return AssignmentPredecessorIdentity(programID, finishID, routeID, digestID)
			},
			want: storageLawIdentity(t, "program/transformer/assignment-predecessor", programID, func(writer *framing.Writer) bool {
				return writer.Bytes(finishID[:]) == nil && writer.Bytes(routeID[:]) == nil && writer.Bytes(digestID[:]) == nil
			}),
		},
		{
			name: "storage write transfer",
			got: func() (identity.ContentID, bool) {
				return StorageWriteTransferIdentity(programID, assignmentID, 4, finishID, predecessorID)
			},
			want: storageLawIdentity(t, "program/transformer/storage-write-transfer", programID, func(writer *framing.Writer) bool {
				return writer.Bytes(assignmentID[:]) == nil && writer.Uint(4) == nil &&
					writer.Bytes(finishID[:]) == nil && writer.Bytes(predecessorID[:]) == nil
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := test.got()
			if !ok || !got.Available() || got != test.want {
				t.Fatalf("identity = %v/%t, want %v/true", got, ok, test.want)
			}
		})
	}
}

func TestStorageCodecsFailClosedOnUnavailableInputs(t *testing.T) {
	programID := storageLawID(0x10)
	field := storageLawID(0x20)
	zero := identity.ContentID{}

	cases := []struct {
		name string
		call func() (identity.ContentID, bool)
	}{
		{"read program", func() (identity.ContentID, bool) {
			return StorageReadIdentity(zero, field, field, field, field, field)
		}},
		{"read field", func() (identity.ContentID, bool) {
			return StorageReadIdentity(programID, zero, field, field, field, field)
		}},
		{"bind width", func() (identity.ContentID, bool) {
			return StorageBindIdentity(programID, field, -1, field, field, field)
		}},
		{"bind field", func() (identity.ContentID, bool) {
			return StorageBindIdentity(programID, field, 0, zero, field, field)
		}},
		{"bind transfer position", func() (identity.ContentID, bool) {
			return StorageBindTransferIdentity(programID, field, -1)
		}},
		{"bind transfer field", func() (identity.ContentID, bool) {
			return StorageBindTransferIdentity(programID, zero, 0)
		}},
		{"predecessor field", func() (identity.ContentID, bool) {
			return AssignmentPredecessorIdentity(programID, zero, field, field)
		}},
		{"write transfer position", func() (identity.ContentID, bool) {
			return StorageWriteTransferIdentity(programID, field, -1, field, field)
		}},
		{"write transfer field", func() (identity.ContentID, bool) {
			return StorageWriteTransferIdentity(programID, field, 0, zero, field)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if id, ok := test.call(); ok || id.Available() {
				t.Fatalf("invalid identity = %v/%t", id, ok)
			}
		})
	}
}

func TestReadIdentityAtOwnsProgramAndIndex(t *testing.T) {
	input, err := lower.Lower(lower.Source{
		Name: "storage-identity-law.lua",
		Text: []byte(`local value = 1; local copy = value; return copy`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if input == nil || !input.Available() {
		t.Fatal("lowered Program unavailable")
	}
	if id, term, ok := ReadIdentityAt(nil, 0); ok || id.Available() || term != 0 {
		t.Fatalf("nil ReadIdentityAt = %v/%08x/%t", id, uint32(term), ok)
	}
	if id, term, ok := ReadIdentityAt(input, -1); ok || id.Available() || term != 0 {
		t.Fatalf("negative ReadIdentityAt = %v/%08x/%t", id, uint32(term), ok)
	}

	reads := input.Flow().Authored().Storage().Reads()
	if id, term, ok := ReadIdentityAt(input, reads.Count()); ok || id.Available() || term != 0 {
		t.Fatalf("out-of-range ReadIdentityAt = %v/%08x/%t", id, uint32(term), ok)
	}

	var accepted int
	for index := 0; index < reads.Count(); index++ {
		id, term, ok := ReadIdentityAt(input, index)
		if !ok {
			continue
		}
		wantTerm, termOK := reads.At(index)
		if !termOK || term != wantTerm || !id.Available() {
			t.Fatalf("ReadIdentityAt(%d) = %v/%08x/%t, row = %08x/%t", index, id, uint32(term), ok, uint32(wantTerm), termOK)
		}
		owner, _, _, relationOK := reads.Get(term)
		bodyPath, bodyID, bodyOK := input.Flow().BodyContextIDs(owner)
		readPath, pathOK := input.Flow().SemanticTermPath(term)
		_, entryTerm, finishTerm, spanOK := input.EvaluationSpan(term)
		entry, entryOK := input.Flow().Causal().Sites().ForTerm(entryTerm)
		finish, finishOK := input.Flow().Causal().Sites().ForTerm(finishTerm)
		want, wantOK := StorageReadIdentity(input.ContentID(), bodyPath, bodyID, readPath, entry.ContextID(), finish.ContextID())
		foreignRoot := input.ContentID()
		foreignRoot[0] ^= 0xff
		foreign, foreignOK := StorageReadIdentity(foreignRoot, bodyPath, bodyID, readPath, entry.ContextID(), finish.ContextID())
		if !relationOK || !bodyOK || !pathOK || !spanOK || !entryOK || !finishOK || !wantOK || want != id || !foreignOK || foreign == id {
			t.Fatalf("ReadIdentityAt(%d) authority/preimage mismatch", index)
		}
		accepted++
	}
	if accepted == 0 {
		t.Fatal("fixture admitted no authored StorageRead")
	}
}

func storageLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func storageLawIdentity(t *testing.T, domain string, programID identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
	t.Helper()
	hash := sha256.New()
	var writer framing.Writer
	if err := writer.Reset(hash, domain, 1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(1); err != nil {
		t.Fatal(err)
	}
	if err := writer.Bytes(programID[:]); err != nil || !write(&writer) {
		t.Fatal("failed to write expected storage preimage")
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
