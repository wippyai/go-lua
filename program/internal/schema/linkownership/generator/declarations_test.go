package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeclarationInventoryHostileJoin(t *testing.T) {
	root := t.TempDir()
	write := func(name, source string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.test\n\ngo 1.23\n")
	write("external/external.go", `package external

type API interface{ Foreign() }
type External struct{}
`)
	write("program/link/link.go", `package link

import "example.test/external"

type Link struct{ Value int }
func (*Link) Do() {}

type Base struct{ Shared int }
func (Base) Promoted() {}

type First struct {
	Shared string
	Base
	Nested struct{ Leaf bool }
	Empty struct{}
	Items []struct{ Item string }
}
type Second struct{ Shared bool }
type Unused struct{ Base }
type AliasFirst = First
type AliasSecond = Second
type ExternalAlias = external.External
type LocalAPI interface{ external.API }
type API interface{ Do() }
type Generic[T any] struct{ Value T }

const Limit = 3
var Global = First{}

func Choose[T any](value T) T { return value }
func localOnly[T any](value T) T {
	var local First
	_ = local
	return value
}

func localShadow() {
	type Link struct{ Shadow int }
	var local Link
	_ = local.Shadow
}
`)
	write("caller/caller.go", `package caller

import l "example.test/program/link"

func Use(value *l.First) {
	var local l.First
	_ = local.Shared
	_ = value.Shared
	_ = value.Promoted
	value.Promoted()
	_ = (*l.First).Promoted
	_ = l.Global
	_ = l.Limit
	_ = l.AliasFirst{}
	_ = l.AliasSecond{}
	_ = l.ExternalAlias{}
	_ = l.Choose(1)
}

func UseAPI(value l.API) {
	value.Do()
}
`)
	scan, err := scanFamily(root, "example.test/program/link")
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Types.Declarations) == 0 {
		t.Fatal("declaration inventory is empty")
	}
	byID := make(map[string]DeclarationInfo, len(scan.Types.Declarations))
	for _, declaration := range scan.Types.Declarations {
		if declaration.FactID == "" || declaration.SourceFile == "" || declaration.Line <= 0 || declaration.Column <= 0 {
			t.Fatalf("inexact declaration: %+v", declaration)
		}
		if strings.Contains(declaration.SourceFile, string(filepath.Separator)) && filepath.IsAbs(declaration.SourceFile) {
			t.Fatalf("absolute declaration source: %+v", declaration)
		}
		if _, duplicate := byID[declaration.FactID]; duplicate {
			t.Fatalf("duplicate declaration fact ID: %s", declaration.FactID)
		}
		if declaration.FactID != declarationFactID(declaration) {
			t.Fatalf("fact ID is not canonical: %+v", declaration)
		}
		byID[declaration.FactID] = declaration
	}
	find := func(kind, owner, name string) DeclarationInfo {
		t.Helper()
		for _, declaration := range scan.Types.Declarations {
			if declaration.Kind == kind && declaration.OwnerType == owner && declaration.Name == name {
				return declaration
			}
		}
		t.Fatalf("missing declaration %s owner=%s name=%s", kind, owner, name)
		return DeclarationInfo{}
	}
	firstShared := find("field", "First", "Shared")
	secondShared := find("field", "Second", "Shared")
	if firstShared.FactID == secondShared.FactID || firstShared.Type == secondShared.Type {
		t.Fatalf("same-name fields were not kept distinct: first=%+v second=%+v", firstShared, secondShared)
	}
	find("field", "First", "Leaf")
	find("field", "First", "Empty")
	find("method", "Base", "Promoted")
	aliasFirst := find("alias", "", "AliasFirst")
	aliasSecond := find("alias", "", "AliasSecond")
	if aliasFirst.FactID == aliasSecond.FactID || aliasFirst.AliasRHS == aliasSecond.AliasRHS || aliasFirst.AliasTargetDeclID == "" || aliasSecond.AliasTargetDeclID == "" {
		t.Fatalf("alias target commitments are not distinct/exact: first=%+v second=%+v", aliasFirst, aliasSecond)
	}
	if aliasFirst.AliasTargetDeclID == aliasSecond.AliasTargetDeclID {
		t.Fatalf("distinct alias targets collapsed: first=%+v second=%+v", aliasFirst, aliasSecond)
	}
	externalAlias := find("alias", "", "ExternalAlias")
	if externalAlias.AliasRHS == "" || externalAlias.AliasTargetDeclID != "" {
		t.Fatalf("external alias target commitment is not explicit: %+v", externalAlias)
	}
	find("const", "", "Limit")
	find("var", "", "Global")
	choose := find("func", "", "Choose")
	apiMethod := find("interface-method", "API", "Do")
	if choose.Signature == "" || apiMethod.Signature == "" {
		t.Fatalf("function/interface signatures missing: choose=%+v api=%+v", choose, apiMethod)
	}
	seenInterfaceSelection := false

	seenPromoted := false
	seenInstance := false
	for _, use := range scan.Uses {
		if use.TargetDeclID == "" {
			t.Fatalf("typed use has no target declaration: %+v", use)
		}
		target, ok := byID[use.TargetDeclID]
		if !ok {
			t.Fatalf("typed use target is not in declaration inventory: %+v", use)
		}
		if strings.HasSuffix(use.Symbol, ".Promoted") && target.OwnerType == "Base" && target.Name == "Promoted" {
			seenPromoted = true
		}
		if use.Evidence == "instance" && target.FactID == choose.FactID {
			seenInstance = true
		}
		if target.FactID == apiMethod.FactID {
			seenInterfaceSelection = true
		}
		if use.Symbol == "example.test/program/link.local" {
			t.Fatalf("local family variable entered typed use inventory: %+v", use)
		}
	}
	if !seenPromoted || !seenInstance || !seenInterfaceSelection {
		t.Fatalf("promoted/instance/interface evidence missing: promoted=%v instance=%v interface=%v uses=%v", seenPromoted, seenInstance, seenInterfaceSelection, formatUseSites(scan.Uses))
	}
	seenUnusedPromotion := false
	seenAliased := false
	seenExposureIDs := make(map[string]struct{}, len(scan.Types.Exposures))
	for _, exposure := range scan.Types.Exposures {
		if exposure.FactID == "" || exposure.FactID != methodExposureFactID(exposure) || exposure.TargetDeclID == "" {
			t.Fatalf("inexact method exposure: %+v", exposure)
		}
		if _, ok := byID[exposure.TargetDeclID]; !ok {
			t.Fatalf("method exposure target is not a declaration: %+v", exposure)
		}
		if _, duplicate := seenExposureIDs[exposure.FactID]; duplicate {
			t.Fatalf("duplicate method exposure fact: %+v", exposure)
		}
		seenExposureIDs[exposure.FactID] = struct{}{}
		if exposure.RootType == "Unused" && exposure.Name == "Promoted" && exposure.Disposition == "promoted" {
			seenUnusedPromotion = true
		}
		if exposure.RootType == "AliasFirst" && exposure.Name == "Promoted" && exposure.Disposition == "aliased" {
			seenAliased = true
		}
	}
	for index := 1; index < len(scan.Types.Exposures); index++ {
		if !methodExposureLess(scan.Types.Exposures[index-1], scan.Types.Exposures[index]) {
			t.Fatalf("method exposures are not in canonical order: previous=%+v current=%+v", scan.Types.Exposures[index-1], scan.Types.Exposures[index])
		}
	}
	if !seenUnusedPromotion || !seenAliased {
		t.Fatalf("unused promoted/aliased exposure missing: unused=%v aliased=%v exposures=%v", seenUnusedPromotion, seenAliased, scan.Types.Exposures)
	}
	seenEmpty := false
	seenSurfaceIDs := make(map[string]struct{}, len(scan.Types.Surfaces))
	for _, surface := range scan.Types.Surfaces {
		if surface.FactID == "" || surface.FactID != surfaceInfoFactID(surface) || surface.OriginDeclID == "" {
			t.Fatalf("inexact surface projection: %+v", surface)
		}
		if _, duplicate := seenSurfaceIDs[surface.FactID]; duplicate {
			t.Fatalf("duplicate surface fact: %+v", surface)
		}
		seenSurfaceIDs[surface.FactID] = struct{}{}
		if surface.RootType == "First" && strings.HasSuffix(surface.Surface, "#Empty") {
			seenEmpty = true
		}
	}
	for index := 1; index < len(scan.Types.Surfaces); index++ {
		if !surfaceInfoLess(scan.Types.Surfaces[index-1], scan.Types.Surfaces[index]) {
			t.Fatalf("surfaces are not in canonical order: previous=%+v current=%+v", scan.Types.Surfaces[index-1], scan.Types.Surfaces[index])
		}
	}
	if !seenEmpty {
		t.Fatalf("empty anonymous surface was lost: %+v", scan.Types.Surfaces)
	}
	originByField := map[string]string{}
	for _, fieldName := range []string{"Nested", "Empty", "Items"} {
		originByField[fieldName] = find("field", "First", fieldName).FactID
	}
	wantSurfaces := map[string]string{
		typeSurfaceID("example.test/program/link", "First") + "#Nested":  originByField["Nested"],
		typeSurfaceID("example.test/program/link", "First") + "#Empty":   originByField["Empty"],
		typeSurfaceID("example.test/program/link", "First") + "#Items[]": originByField["Items"],
	}
	for surfaceName, wantOrigin := range wantSurfaces {
		var matches []SurfaceInfo
		for _, surface := range scan.Types.Surfaces {
			if surface.Surface == surfaceName {
				matches = append(matches, surface)
			}
		}
		if len(matches) != 1 || matches[0].OriginDeclID != wantOrigin {
			t.Fatalf("surface origin is not a 1:1 source-coordinate join: surface=%s want=%s matches=%+v", surfaceName, wantOrigin, matches)
		}
	}
	packageLinkUses := 0
	for _, use := range scan.Uses {
		if use.PackagePath == "example.test/program/link" && use.Symbol == "example.test/program/link.Link" {
			packageLinkUses++
		}
	}
	if packageLinkUses > 1 {
		t.Fatalf("local shadow type became a package Link use: count=%d uses=%v", packageLinkUses, scan.Uses)
	}
}

func formatUseSites(uses []UseSite) string {
	return fmt.Sprintf("%+v", uses)
}
