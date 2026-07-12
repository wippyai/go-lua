package effectlowering

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestStaticScalarSignatureReturnsAcceptsFormatRejectsBorrowingConcat(t *testing.T) {
	reg := standard.Registry()
	source := signaturelookup.Source{IncludeStdlib: true}
	format, _ := source.Lookup("string.format")
	got, ok := StaticScalarSignatureReturns(reg, nil, format)
	if !ok || len(got) != 1 {
		t.Fatalf("string.format static returns=%#v/%v", got, ok)
	}
	concat, _ := source.Lookup("table.concat")
	if got, ok := StaticScalarSignatureReturns(reg, nil, concat); ok || got != nil {
		t.Fatalf("table.concat borrowing effect compiled as pure: %#v", got)
	}
}
