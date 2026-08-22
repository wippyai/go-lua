package composite

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
)

// TestContextSchemaMountDerivationRetainsExactAuthority states the one
// composition join: ContextSchema is sealed from the mounted Heap and the
// Link's exact detached ContextDirectory, then the same issuer is retained by
// the hot authorities and ProgramBinding.
func TestContextSchemaMountDerivationRetainsExactAuthority(t *testing.T) {
	record := mountedRecord(t, "context-schema-authority", "return 42")
	if !record.contextSchema.Valid() {
		t.Fatal("mount phase did not derive a contextual Heap schema")
	}
	directory := record.Source.ContextDirectory()
	if !directory.Available() || directory.LinkID() != record.Source.ContentID() {
		t.Fatal("mounted Link did not retain its exact ContextDirectory")
	}
	if record.contextSchema.Heap() != record.HeapSchema ||
		!sameContextDirectory(record.contextSchema.Directory(), directory) {
		t.Fatal("ContextSchema was not derived from the exact mounted Heap and Link directory")
	}
	expected, expectedOK := contextdomain.Seal(record.HeapSchema, directory)
	if !expectedOK || expected.ContentID() != record.contextSchema.ContentID() {
		t.Fatal("ContextSchema identity is not the canonical Heap+Directory identity")
	}

	authority := authorities{contextSchema: record.contextSchema}
	if got := authority.ContextSchema(); !got.OwnsSchema(record.contextSchema) {
		t.Fatal("authorities did not retain the exact contextual issuer")
	}

	binding := materializerBinding(t, record)
	got, gotOK := binding.ContextSchema()
	if !gotOK || !got.OwnsSchema(record.contextSchema) || !record.contextSchema.OwnsSchema(got) {
		t.Fatal("ProgramBinding did not retain the exact contextual issuer")
	}
}

// TestContextSchemaCannotBeCallerSupplied states the input boundary. A
// caller may supply the Link, mounted artifacts, and neutral static authority,
// but neither a ContextSchema nor a ContextDirectory is an exported
// LinkInputs field. The only contextual directory path is Source's sealed
// accessor used by LinkInputs.derive.
func TestContextSchemaCannotBeCallerSupplied(t *testing.T) {
	typeOfInputs := reflect.TypeOf(LinkInputs{})
	contextSchemaType := reflect.TypeOf(contextdomain.Schema{})
	directoryType := reflect.TypeOf(executioncontext.Directory{})
	for index := 0; index < typeOfInputs.NumField(); index++ {
		field := typeOfInputs.Field(index)
		if field.Type != contextSchemaType && field.Type != directoryType {
			continue
		}
		if field.PkgPath == "" {
			t.Fatalf("LinkInputs exposes caller-supplied contextual field %q", field.Name)
		}
	}
	if _, found := typeOfInputs.FieldByName("ContextDirectory"); found {
		t.Fatal("LinkInputs grew a caller-supplied ContextDirectory field")
	}
}

// TestContextSchemaRejectsEqualContentForeignIssuer states the identity half
// of the fence. Independent seals over the same typed Heap and Directory are
// deterministic in content but remain foreign issuers; the binding's
// accessor carries only the mount-derived issuer.
func TestContextSchemaRejectsEqualContentForeignIssuer(t *testing.T) {
	record := mountedRecord(t, "context-schema-foreign", "return 42")
	foreign, foreignOK := contextdomain.Seal(record.HeapSchema, record.Source.ContextDirectory())
	if !foreignOK || foreign.ContentID() != record.contextSchema.ContentID() {
		t.Fatal("foreign equal-content contextual fixture did not seal")
	}
	if record.contextSchema.OwnsSchema(foreign) || foreign.OwnsSchema(record.contextSchema) {
		t.Fatal("equal-content foreign contextual issuer crossed the authority fence")
	}

	binding := materializerBinding(t, record)
	got, gotOK := binding.ContextSchema()
	if !gotOK || got.OwnsSchema(foreign) || !got.OwnsSchema(record.contextSchema) {
		t.Fatal("ProgramBinding exposed or accepted a foreign contextual issuer")
	}
}
