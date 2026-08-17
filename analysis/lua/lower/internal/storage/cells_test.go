package storage

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"testing"
)

func TestStorageGlobalSelectionRequiresBinderAndCollectorAuthority(t *testing.T) {
	var writer Writer
	if _, err := writer.Global(bind.GlobalIdentity{}); err == nil {
		t.Fatal("Global accepted an unavailable storage authority")
	}
}
