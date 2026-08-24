package measure

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LOC is a non-test/test line-count split for one authored area. NonTest
// and Test count authored lines only; GeneratedNonTest and GeneratedTest
// count lines in files isGeneratedByName or isGeneratedByContent already
// classifies as emitted - the same detection generatedStats uses, so the
// two metrics never disagree on what counts as generated.
type LOC struct {
	NonTest int
	Test    int

	GeneratedNonTest int
	GeneratedTest    int
}

// Add returns the elementwise sum of l and o.
func (l LOC) Add(o LOC) LOC {
	return LOC{
		NonTest:          l.NonTest + o.NonTest,
		Test:             l.Test + o.Test,
		GeneratedNonTest: l.GeneratedNonTest + o.GeneratedNonTest,
		GeneratedTest:    l.GeneratedTest + o.GeneratedTest,
	}
}

// Sub returns the elementwise difference l-o.
func (l LOC) Sub(o LOC) LOC {
	return LOC{
		NonTest:          l.NonTest - o.NonTest,
		Test:             l.Test - o.Test,
		GeneratedNonTest: l.GeneratedNonTest - o.GeneratedNonTest,
		GeneratedTest:    l.GeneratedTest - o.GeneratedTest,
	}
}

// AreaLOC names one immediate subdirectory of domain/ together with its
// authored non-test/test line counts.
type AreaLOC struct {
	Name string
	LOC  LOC
}

// locInDir sums authored and generated, non-test and test, .go line counts
// recursively under dir. A file counts as generated when isGeneratedByName
// matches its name or isGeneratedByContent matches its body - the same
// generated/authored split generatedStats uses - so a generated_*.go file,
// a rule_members.go file, or any file carrying an emitter's "Code
// generated" header (see analysis/schema/rule/emit and emitlaw) lands in
// the Generated* buckets instead of being counted as authored, whether or
// not its name ends in _test.go.
func locInDir(dir string) (LOC, error) {
	var total LOC
	err := walkGoFiles(dir, func(path, name string) error {
		n, err := countLines(path)
		if err != nil {
			return err
		}
		generated := isGeneratedByName(name)
		if !generated {
			generated, err = isGeneratedByContent(path)
			if err != nil {
				return err
			}
		}
		isTest := strings.HasSuffix(name, "_test.go")
		switch {
		case isTest && generated:
			total.GeneratedTest += n
		case isTest:
			total.Test += n
		case generated:
			total.GeneratedNonTest += n
		default:
			total.NonTest += n
		}
		return nil
	})
	return total, err
}

// domainAreas discovers every immediate subdirectory of domainDir and
// measures its LOC. Areas are discovered rather than named, so a domain
// package added or removed between commits changes the row set instead of
// being silently dropped or left at zero.
func domainAreas(domainDir string) ([]AreaLOC, LOC, error) {
	exists, err := dirExists(domainDir)
	if err != nil || !exists {
		return nil, LOC{}, err
	}
	entries, err := os.ReadDir(domainDir)
	if err != nil {
		return nil, LOC{}, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	areas := make([]AreaLOC, 0, len(names))
	var total LOC
	for _, name := range names {
		l, err := locInDir(filepath.Join(domainDir, name))
		if err != nil {
			return nil, LOC{}, err
		}
		areas = append(areas, AreaLOC{Name: name, LOC: l})
		total = total.Add(l)
	}
	return areas, total, nil
}
