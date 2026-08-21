package link_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// A module request is admitted by exactly two declarations: a host module the
// Target declares, or a sibling module the Link mounts. A request that matches
// neither names a contract no authority holds, so the seal must refuse it and
// say which module it refused. Typing the absent surface as unknown instead
// would launder an undeclared host contract into the type plane.
func TestLinkRefusesRequireOfUndeclaredModule(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	main := admissionProgram(t, "admission-main.lua", `local missing = require("nosuchmodule")
return missing
`)
	sealed, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "main", Program: main}}})
	if sealed != nil || err == nil {
		t.Fatalf("require of an undeclared module sealed: link=%t err=%v", sealed != nil, err)
	}
	if !strings.Contains(err.Error(), "nosuchmodule") {
		t.Fatalf("refusal does not name the module: %v", err)
	}
}

// A host module the Target declares stays require-able: the declaration is the
// admission.
func TestLinkAdmitsRequireOfDeclaredHostModule(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	main := admissionProgram(t, "admission-host.lua", `local uuid = require("uuid")
return uuid.v7()
`)
	sealed, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "main", Program: main}}})
	if err != nil || sealed == nil {
		t.Fatalf("require of the declared uuid host module refused: %v", err)
	}
}

// A sibling module the Link mounts stays require-able: the mount is the
// admission, and it carries no host provider declaration.
func TestLinkAdmitsRequireOfMountedSiblingModule(t *testing.T) {
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	main := admissionProgram(t, "admission-sibling-main.lua", `local sibling = require("sibling")
return sibling
`)
	sibling := admissionProgram(t, "admission-sibling.lua", `return {value = 1}
`)
	sealed, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{
		{Name: "main", Program: main}, {Name: "sibling", Program: sibling},
	}})
	if err != nil || sealed == nil {
		t.Fatalf("require of a mounted sibling module refused: %v", err)
	}
}

func admissionProgram(t *testing.T, name, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: name, Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}
