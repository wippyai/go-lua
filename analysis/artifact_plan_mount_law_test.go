package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

// The Link construction root composes; it does not seal a factor's authority.
// A domain whose axis declares its own mount seals that authority itself, from
// the neutral artifact view the root hands the mount phase, so the root has no
// remaining reason to name that domain at all. The law below reads the sealed
// declaration table for which axes mount themselves and holds the root to it,
// so every domain moved onto the declared path removes one name here and no
// domain can be moved halfway.

// mountingDomainPackages pairs each factor with the package the root would have
// to name to build that factor's mount rows and open its seal by hand.
var mountingDomainPackages = map[programartifact.RuleOutputKind]string{
	programartifact.RuleOutputValue:  "github.com/wippyai/go-lua/analysis/domain/value",
	programartifact.RuleOutputHeap:   "github.com/wippyai/go-lua/analysis/domain/heap",
	programartifact.RuleOutputPack:   "github.com/wippyai/go-lua/analysis/domain/pack",
	programartifact.RuleOutputCall:   "github.com/wippyai/go-lua/analysis/domain/call",
	programartifact.RuleOutputEffect: "github.com/wippyai/go-lua/analysis/domain/effect/factor",
}

const artifactPlanSourcePath = "analysis/artifact_plan.go"

// TestArtifactPlanNamesNoSelfMountingDomain fails when the Link construction
// root still imports a domain whose axis seals its own Link authority.
func TestArtifactPlanNamesNoSelfMountingDomain(t *testing.T) {
	imports := make(map[string]struct{})
	found := false
	architectureBatteryWalk(t, "analysis", func(source architectureBatterySource) {
		if source.path != artifactPlanSourcePath {
			return
		}
		found = true
		for _, path := range source.imports(t) {
			imports[path] = struct{}{}
		}
	})
	if !found {
		t.Fatalf("%s was not walked; the law has nothing to state", artifactPlanSourcePath)
	}
	mounting := 0
	for principal, path := range mountingDomainPackages {
		declared, known := composite.AxisMountDeclared(principal)
		if !known {
			t.Fatalf("principal %v is not a declared axis", principal)
		}
		if !declared {
			continue
		}
		mounting++
		if _, named := imports[path]; named {
			t.Errorf("%s imports %s, whose axis seals its own Link authority; the mount belongs to the domain, not the root", artifactPlanSourcePath, path)
		}
	}
	if mounting == 0 {
		t.Fatalf("no factor axis seals its own authority; the law measures nothing")
	}
}
