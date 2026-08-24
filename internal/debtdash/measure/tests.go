package measure

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var testFuncPattern = regexp.MustCompile(`^func (Test[A-Za-z0-9_]*)`)

// testFuncCounts scans every *_test.go file under root for top-level
// Test-prefixed functions, and separately counts those whose name
// contains "Law" - the naming convention the codebase uses for
// law-shaped (property) tests.
func testFuncCounts(root string) (total, law int, err error) {
	err = walkGoFiles(root, func(path, name string) error {
		if !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, ferr := os.Open(path)
		if ferr != nil {
			return ferr
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			m := testFuncPattern.FindStringSubmatch(scanner.Text())
			if m == nil {
				continue
			}
			total++
			if strings.Contains(m[1], "Law") {
				law++
			}
		}
		return scanner.Err()
	})
	return total, law, err
}

// countFilesWithSuffix counts .go files under root whose name ends with
// suffix, e.g. "_law_test.go".
func countFilesWithSuffix(root, suffix string) (int, error) {
	count := 0
	err := walkGoFiles(root, func(_, name string) error {
		if strings.HasSuffix(name, suffix) {
			count++
		}
		return nil
	})
	return count, err
}
