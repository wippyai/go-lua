package oracle

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/composite"
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
var mountingDomainPackages = map[schema.Key]string{
	"value":  "github.com/wippyai/go-lua/domain/value",
	"heap":   "github.com/wippyai/go-lua/domain/heap",
	"pack":   "github.com/wippyai/go-lua/domain/pack",
	"call":   "github.com/wippyai/go-lua/domain/call",
	"effect": "github.com/wippyai/go-lua/domain/effect/factor",
}

// postMountDerivationPackages pairs each post-mount derivation with the package
// that owns it. A derivation over several sealed factors at once is no axis's
// authority to mount, so it is derived by the mount phase after every mount has
// sealed; the root that hands the phase its inputs names neither package.
var postMountDerivationPackages = map[string]string{
	"github.com/wippyai/go-lua/domain/heap/index":      "the receiver-to-root topology",
	"github.com/wippyai/go-lua/domain/call/activation": "the mounted activation catalog",
}

const artifactPlanSourcePath = "analysis/compile.go"

// artifactPlanImports reads the Link construction root's own import set. It is
// the one source both mount laws below are stated over, and is named
// module-relative so the law states which file it is about rather than where a
// test happened to run from.
func artifactPlanImports(t *testing.T) map[string]struct{} {
	t.Helper()
	path := filepath.Join(architectureBatteryRepositoryRoot(t), filepath.FromSlash(artifactPlanSourcePath))
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("%s: %v", artifactPlanSourcePath, err)
	}
	imports := make(map[string]struct{}, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		value, unquoteErr := strconv.Unquote(imported.Path.Value)
		if unquoteErr != nil {
			t.Fatalf("%s: unquote import %s: %v", artifactPlanSourcePath, imported.Path.Value, unquoteErr)
		}
		imports[value] = struct{}{}
	}
	if len(imports) == 0 {
		t.Fatalf("%s declares no import; the law has nothing to state", artifactPlanSourcePath)
	}
	return imports
}

// TestArtifactPlanNamesNoSelfMountingDomain fails when the Link construction
// root still imports a domain whose axis seals its own Link authority.
func TestArtifactPlanNamesNoSelfMountingDomain(t *testing.T) {
	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("sealed composition unavailable")
	}
	imports := artifactPlanImports(t)
	mounting := 0
	for key, path := range mountingDomainPackages {
		declared, known := composite.AxisMountDeclared(compilation, key)
		if !known {
			t.Fatalf("%q is not a declared axis", key)
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

// TestArtifactPlanNamesNoPostMountDerivation fails when the Link construction
// root still names a package whose authority the mount phase derives for
// itself. A derivation over several sealed factors belongs to the phase that
// produced them; the root hands that phase its inputs and reads its verdict.
func TestArtifactPlanNamesNoPostMountDerivation(t *testing.T) {
	imports := artifactPlanImports(t)
	for path, derivation := range postMountDerivationPackages {
		if _, named := imports[path]; named {
			t.Errorf("%s imports %s; %s is derived by the mount phase, not by the root", artifactPlanSourcePath, path, derivation)
		}
	}
}
