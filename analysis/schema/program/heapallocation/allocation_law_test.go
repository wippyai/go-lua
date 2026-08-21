package heapallocation

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/internal/framing"
)

func TestRowsFailClosedAndBindTheirCatalogFamilies(t *testing.T) {
	if AllocationFamily().Definition() != programcatalog.HeapAllocation() || FieldFamily().Definition() != programcatalog.HeapField() {
		t.Fatal("family definition")
	}
	if _, ok := NewField(identity.ContentID{1}, FieldKindList, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5}, 1, false, false, 0, false); ok {
		t.Fatal("non-key selector accepted")
	}
	field, fieldOK := NewField(identity.ContentID{1}, FieldKindKey, identity.ContentID{2}, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5}, 1, false, true, 7, true)
	allocation, allocationOK := NewAllocation(identity.ContentID{6}, RoleTable, FormClosed, identity.ContentID{7}, 0, 1)
	if !fieldOK || !field.Available() || !allocationOK || !allocation.Available() {
		t.Fatal("declared rows rejected")
	}
}

func TestIdentityPreimagesAreStableAndFailClosed(t *testing.T) {
	occurrence, programID, proof := identity.ContentID{1}, identity.ContentID{2}, identity.ContentID{3}
	field, fieldOK := NewField(identity.ContentID{4}, FieldKindKey, identity.ContentID{5}, identity.ContentID{6}, identity.ContentID{7}, identity.ContentID{8}, 1, false, true, 9, true)
	if !fieldOK {
		t.Fatal("template field")
	}
	got := TemplateID(occurrence, RoleTable, FormClosed, []Field{field})
	want := exactTemplatePreimage(occurrence, RoleTable, FormClosed, []Field{field})
	if !got.Available() || got != want {
		t.Fatalf("template preimage changed: got %x want %x", got, want)
	}
	if TemplateID(occurrence, RoleClosure, FormEmpty, []Field{field}).Available() ||
		TemplateID(occurrence, RoleClosure, FormClosed, nil).Available() ||
		TemplateID(occurrence, RoleTable, FormEmpty, []Field{field}).Available() ||
		TemplateID(occurrence, RoleTable, FormClosed, nil).Available() ||
		TemplateID(identity.ContentID{}, RoleTable, FormEmpty, nil).Available() {
		t.Fatal("invalid template acquired identity")
	}
	const prefix = "program-allocation-field-v1"
	payload := make([]byte, len(prefix)+sha256.Size+sha256.Size)
	copy(payload, prefix)
	copy(payload[len(prefix):], programID[:])
	copy(payload[len(prefix)+sha256.Size:], proof[:])
	if got, want := FieldID(programID, proof), sha256.Sum256(payload); got != want {
		t.Fatal("field preimage changed")
	}
}

func TestTemplateIdentityUsesOnlyDeclaredConstructorGeometry(t *testing.T) {
	occurrence := identity.ContentID{1}
	field, ok := NewField(identity.ContentID{2}, FieldKindKey, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5}, identity.ContentID{6}, 1, false, true, 7, true)
	if !ok {
		t.Fatal("base field")
	}
	base := TemplateID(occurrence, RoleTable, FormClosed, []Field{field})

	transportVariant, ok := NewField(identity.ContentID{8}, FieldKindKey, identity.ContentID{9}, identity.ContentID{10}, identity.ContentID{11}, identity.ContentID{12}, 1, false, false, 7, true)
	if !ok || TemplateID(occurrence, RoleTable, FormClosed, []Field{transportVariant}) != base {
		t.Fatal("transport-only field data entered template identity")
	}
	normalizedVariant, ok := NewField(identity.ContentID{2}, FieldKindKey, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5}, identity.ContentID{6}, 1, false, true, 8, true)
	if !ok || TemplateID(occurrence, RoleTable, FormClosed, []Field{normalizedVariant}) == base {
		t.Fatal("normalized key missing from template identity")
	}
	widthVariant, ok := NewField(identity.ContentID{2}, FieldKindKey, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5}, identity.ContentID{6}, 2, false, true, 7, true)
	if !ok || TemplateID(occurrence, RoleTable, FormClosed, []Field{widthVariant}) == base {
		t.Fatal("width missing from template identity")
	}
	openVariant, ok := NewField(identity.ContentID{2}, FieldKindKey, identity.ContentID{3}, identity.ContentID{4}, identity.ContentID{5}, identity.ContentID{6}, 1, true, true, 7, true)
	if !ok || TemplateID(occurrence, RoleTable, FormFinalOpen, []Field{openVariant}) == base {
		t.Fatal("open form missing from template identity")
	}
}

func TestAllocationRejectsContradictoryRoleFormAndSpanShapes(t *testing.T) {
	id, root := identity.ContentID{1}, identity.ContentID{2}
	cases := []struct {
		role  Role
		form  Form
		count uint32
	}{
		{RoleClosure, FormClosed, 0},
		{RoleClosure, FormFinalOpen, 0},
		{RoleClosure, FormEmpty, 1},
		{RoleTable, FormEmpty, 1},
		{RoleTable, FormClosed, 0},
		{RoleTable, FormFinalOpen, 0},
	}
	for _, test := range cases {
		if _, ok := NewAllocation(id, test.role, test.form, root, 0, test.count); ok {
			t.Fatalf("accepted role=%d form=%d count=%d", test.role, test.form, test.count)
		}
	}
	if _, ok := NewAllocation(id, RoleTable, FormClosed, root, ^uint32(0), 1); ok {
		t.Fatal("overflowing field span accepted")
	}
}

// exactTemplatePreimage is the protocol law, intentionally independent of
// TemplateID. Changing either side requires an explicit identity migration.
func exactTemplatePreimage(occurrence identity.ContentID, role Role, form Form, fields []Field) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/allocation-template", 2) != nil ||
		writer.Record(1) != nil ||
		writer.Bytes(occurrence[:]) != nil ||
		writer.Uint(uint64(role)) != nil ||
		writer.Uint(uint64(form)) != nil {
		return identity.ContentID{}
	}
	if role == RoleTable {
		if writer.Count(uint64(len(fields))) != nil {
			return identity.ContentID{}
		}
		for _, field := range fields {
			normalized, normalizedOK := field.NormalizedKey()
			if writer.Record(1) != nil ||
				writer.Uint(uint64(field.Kind())) != nil ||
				writer.Bool(normalizedOK) != nil {
				return identity.ContentID{}
			}
			if normalizedOK && writer.Uint(normalized) != nil {
				return identity.ContentID{}
			}
			if writer.Uint(uint64(field.Width())) != nil || writer.Bool(field.FinalOpen()) != nil {
				return identity.ContentID{}
			}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
