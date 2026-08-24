package measure

import (
	"os"
	"regexp"
	"strings"
)

var codeGeneratedPattern = regexp.MustCompile(`Code generated`)

// isGeneratedByName reports whether name matches one of the generated-file
// naming conventions the debt dashboard tracks: generated_*.go, or the
// fixed emitter output name rule_members.go.
func isGeneratedByName(name string) bool {
	if name == "rule_members.go" {
		return true
	}
	return strings.HasPrefix(name, "generated_") && strings.HasSuffix(name, ".go")
}

// isGeneratedByContent reports whether the file at path carries a
// "Code generated" marker.
func isGeneratedByContent(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return codeGeneratedPattern.Match(data), nil
}

// generatedStats counts files and total LOC across dirs that are either
// named as generated output or carry a "Code generated" marker.
func generatedStats(dirs []string) (files, loc int, err error) {
	for _, dir := range dirs {
		err = walkGoFiles(dir, func(path, name string) error {
			hit := isGeneratedByName(name)
			if !hit {
				var cerr error
				hit, cerr = isGeneratedByContent(path)
				if cerr != nil {
					return cerr
				}
			}
			if !hit {
				return nil
			}
			n, lerr := countLines(path)
			if lerr != nil {
				return lerr
			}
			files++
			loc += n
			return nil
		})
		if err != nil {
			return 0, 0, err
		}
	}
	return files, loc, nil
}

// emittedContentFiles counts .go files under dir that carry a
// "Code generated" marker, independent of file naming.
func emittedContentFiles(dir string) (int, error) {
	count := 0
	err := walkGoFiles(dir, func(path, name string) error {
		hit, err := isGeneratedByContent(path)
		if err != nil {
			return err
		}
		if hit {
			count++
		}
		return nil
	})
	return count, err
}
