package measure

import "github.com/wippyai/go-lua/internal/cutoververify"

// residueStats classifies legacy protocol-token references under domainDir
// using cutoververify's exported classifier, so the debt dashboard and the
// cutover landing ritual agree on exactly one definition of "residue"
// instead of maintaining a second copy of the token list.
func residueStats(domainDir string) (files, occurrences int, err error) {
	exists, err := dirExists(domainDir)
	if err != nil || !exists {
		return 0, 0, err
	}
	report, err := cutoververify.ClassifyProtocolZero(domainDir)
	if err != nil {
		return 0, 0, err
	}
	occurrences = len(report.Hits)
	seen := make(map[string]struct{}, len(report.Hits))
	for _, hit := range report.Hits {
		seen[hit.File] = struct{}{}
	}
	files = len(seen)
	return files, occurrences, nil
}

// countFilesByName counts .go files anywhere under dir whose base name is
// exactly name, e.g. "family.go" or "hot_rule.go".
func countFilesByName(dir, name string) (int, error) {
	count := 0
	err := walkGoFiles(dir, func(_, fileName string) error {
		if fileName == name {
			count++
		}
		return nil
	})
	return count, err
}
