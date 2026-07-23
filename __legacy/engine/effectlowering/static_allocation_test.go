package effectlowering

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
)

func TestStaticSignatureAllocationTemplateAcceptsTableCreateOnly(t *testing.T) {
	source := signaturelookup.Source{IncludeStdlib: true}
	create, _ := source.Lookup("table.create")
	template, ok := StaticSignatureAllocationTemplate(create)
	if !ok || template.Root != "stdlib.table.create:return:0" || template.ReturnIndex != 0 || len(template.Objects) != 1 {
		t.Fatalf("table.create allocation template = %#v/%v", template, ok)
	}
	format, _ := source.Lookup("string.format")
	if template, ok := StaticSignatureAllocationTemplate(format); ok || template.Root != "" {
		t.Fatalf("string.format invented allocation template: %#v", template)
	}
}
