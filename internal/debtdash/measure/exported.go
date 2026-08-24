package measure

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var exportedDeclPattern = regexp.MustCompile(`^func [A-Z]|^type [A-Z]|^func \([^)]*\) [A-Z]`)

// exportedSymbolCount counts top-level exported func and type declarations
// in non-test .go files under dir: package-level funcs, types, and
// exported methods.
func exportedSymbolCount(dir string) (int, error) {
	count := 0
	err := walkGoFiles(dir, func(path, name string) error {
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			if exportedDeclPattern.MatchString(scanner.Text()) {
				count++
			}
		}
		return scanner.Err()
	})
	return count, err
}
