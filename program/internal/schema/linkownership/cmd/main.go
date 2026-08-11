package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/program/internal/schema/linkownership/generator"
)

func main() {
	root := flag.String("root", ".", "repository root")
	modeValue := flag.String("mode", string(generator.ModeInventory), "inventory or final")
	write := flag.Bool("write", false, "write generated output after a complete manifest")
	flag.Parse()

	report, err := generator.Run(*root, generator.Mode(*modeValue), *write)
	packagePath := report.Scan.Root.PackagePath
	linkSurface := "type:" + packagePath + ".Link"
	linkSurfaceID := ""
	for _, surface := range report.Scan.Types.Surfaces {
		if surface.PackagePath == packagePath && surface.Surface == linkSurface {
			linkSurfaceID = surface.FactID
			break
		}
	}
	fields, methods, functions := 0, 0, 0
	for _, field := range report.Scan.Types.Structure.Fields {
		if field.SurfaceID == linkSurfaceID {
			fields++
		}
	}
	for _, declaration := range report.Scan.Types.Declarations {
		if declaration.PackagePath != packagePath {
			continue
		}
		switch {
		case declaration.Kind == "method" && declaration.OwnerType == "Link":
			methods++
		case declaration.Kind == "func" && declaration.OwnerType == "":
			functions++
		}
	}
	fmt.Printf("mode=%s package=%s fields=%d methods=%d funcs=%d declarations=%d callers=%d imports=%d structure-facts=%d source=%x\n", report.Mode, packagePath, fields, methods, functions, len(report.Scan.Types.Declarations), callerCount(report.Scan.Uses, packagePath), len(report.Scan.Dependencies.ImportEdges), structureFactCount(report.Scan.Types.Structure), report.Scan.Build.SourceDigest)
	for _, blocker := range report.FinalBlockers {
		fmt.Printf("blocker=%s\n", blocker)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func callerCount(uses []generator.UseSite, familyPath string) int {
	callers := make(map[string]struct{})
	for _, use := range uses {
		if use.PackagePath == familyPath || len(use.PackagePath) > len(familyPath) && use.PackagePath[:len(familyPath)+1] == familyPath+"/" {
			continue
		}
		callers[use.PackagePath] = struct{}{}
	}
	return len(callers)
}

func structureFactCount(projection generator.StructureProjection) int {
	return len(projection.Fields) + len(projection.Arrays) + len(projection.Slices) + len(projection.Maps) + len(projection.Channels) + len(projection.NamedReferences) + len(projection.MethodReferences) + len(projection.OtherReferences) + len(projection.Cycles)
}
