// Command rule-family generates one rule's execution family from its Program
// declaration and the axis member roster, together with the laws that family's
// generated construction owes, or checks that the checked-in files are already
// the ones that declaration derives.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wippyai/go-lua/analysis/schema/rule/emit"
	"github.com/wippyai/go-lua/domain/familyroster"
	"github.com/wippyai/go-lua/domain/memberroster"
)

func main() {
	ruleKey := flag.String("rule", "", "rule key to emit; every rostered rule when empty")
	root := flag.String("root", ".", "repository root the generated paths are resolved against")
	check := flag.Bool("check", false, "check generated families for freshness instead of writing them")
	flag.Parse()

	roster, rosterOK := memberroster.Composition()
	if !rosterOK {
		fail("member definition roster is not admissible")
	}
	matched := 0
	for _, family := range familyroster.Families() {
		if *ruleKey != "" && string(family.Key()) != *ruleKey {
			continue
		}
		matched++
		path := filepath.Join(*root, family.Directory, familyroster.GeneratedFileName)
		if err := emit.Generate(family.Target, roster, path, *check); err != nil {
			fail(err.Error())
		}
		lawPath := filepath.Join(*root, family.Directory, familyroster.GeneratedConstructionLawFileName)
		if err := emit.GenerateLaw(family.Target, roster, lawPath, *check); err != nil {
			fail(err.Error())
		}
	}
	if matched == 0 {
		fail("no rostered rule family matches: " + *ruleKey)
	}
}

func fail(message string) {
	fmt.Println(message)
	os.Exit(1)
}
