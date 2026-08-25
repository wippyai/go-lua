// Command axis-member-definition generates one axis's cold member catalog
// from the composition roster: the axis owner's base source folded with the
// reducer contributions its rules declare.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wippyai/go-lua/analysis/schema/axis/member/generator"
	"github.com/wippyai/go-lua/domain/memberroster"
)

func main() {
	sourceName := flag.String("source", "value", "axis member definition source")
	coldPath := flag.String("cold", "", "generated cold catalog path")
	relationPath := flag.String("relations", "", "generated bind-time relation owner path")
	relationsPackage := flag.String("relations-package", "", "package name for the generated relation owner, when it lives apart from the axis's registered package")
	exactFoldPath := flag.String("exact-fold", "", "generated same-axis exact-fold reducer dispatch path")
	check := flag.Bool("check", false, "check generated outputs for freshness")
	flag.Parse()
	if *coldPath == "" && *relationPath == "" && *exactFoldPath == "" {
		fail("-cold, -relations, or -exact-fold is required")
	}
	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		fail("member definition roster is not admissible")
	}
	if _, known := roster.Source(*sourceName); !known {
		fail("unknown member definition source: " + *sourceName)
	}
	packageName, source, sourceOK := roster.Definition(*sourceName)
	if !sourceOK {
		refusal := roster.ComposeRefusal(*sourceName)
		if refusal != "" {
			fail("member definition source does not compose: " + *sourceName + ": " + refusal)
		}
		fail("member definition source does not compose: " + *sourceName)
	}
	relationsPackageName := packageName
	if *relationsPackage != "" {
		relationsPackageName = *relationsPackage
	}
	if *coldPath != "" {
		if err := generator.Generate(packageName, source, filepath.Clean(*coldPath), *check); err != nil {
			fail(err.Error())
		}
	}
	if *relationPath != "" {
		if err := generator.GenerateRelations(relationsPackageName, source, filepath.Clean(*relationPath), *check); err != nil {
			fail(err.Error())
		}
	}
	if *exactFoldPath != "" {
		if err := generator.GenerateExactFold(packageName, source, filepath.Clean(*exactFoldPath), *check); err != nil {
			fail(err.Error())
		}
	}
}

func fail(message string) {
	fmt.Println(message)
	os.Exit(1)
}
