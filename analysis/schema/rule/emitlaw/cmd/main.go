// Command rule-law generates one rule's structural law suite from its Program
// declaration and the axis member roster, or checks that the checked-in file
// is already the one that declaration derives.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wippyai/go-lua/analysis/schema/rule/emitlaw"
	"github.com/wippyai/go-lua/domain/familyroster"
	"github.com/wippyai/go-lua/domain/memberroster"
)

func main() {
	ruleKey := flag.String("rule", "", "rule key to emit; every rostered declaration when empty")
	root := flag.String("root", ".", "repository root the generated paths are resolved against")
	check := flag.Bool("check", false, "check generated law suites for freshness instead of writing them")
	flag.Parse()

	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		fail("member definition roster is not admissible")
	}
	matched := 0
	for _, declaration := range familyroster.Declarations() {
		if *ruleKey != "" && string(declaration.Key()) != *ruleKey {
			continue
		}
		matched++
		path := filepath.Join(*root, declaration.Directory, familyroster.GeneratedLawFileName)
		if err := emitlaw.Generate(declaration.Target, roster, path, *check); err != nil {
			fail(err.Error())
		}
	}
	if matched == 0 {
		fail("no rostered declaration matches: " + *ruleKey)
	}
}

func fail(message string) {
	fmt.Println(message)
	os.Exit(1)
}
