package main

import (
	"strings"
	"testing"
)

func TestCensusGeneratorCLIRequiresOutputDestination(t *testing.T) {
	err := run(nil)
	if err == nil || !strings.Contains(err.Error(), "-out is required") {
		t.Fatalf("run() error = %v, want required output", err)
	}
}
