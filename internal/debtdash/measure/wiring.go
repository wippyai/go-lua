package measure

import (
	"os"
	"regexp"
)

var (
	ruleTemplatesGeneratedPattern = regexp.MustCompile(`WireGeneratedRuleWithFamily`)
	ruleTemplatesLegacyPattern    = regexp.MustCompile(`WireRule|\.add\(`)
)

// ruleTemplatesStats finds every file named rule_templates.go under root
// and counts generated-family wiring calls against legacy wiring calls.
func ruleTemplatesStats(root string) (generated, legacy int, err error) {
	err = walkGoFiles(root, func(path, name string) error {
		if name != "rule_templates.go" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		generated += len(ruleTemplatesGeneratedPattern.FindAllIndex(data, -1))
		legacy += len(ruleTemplatesLegacyPattern.FindAllIndex(data, -1))
		return nil
	})
	return generated, legacy, err
}
