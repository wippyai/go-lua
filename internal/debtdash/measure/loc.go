package measure

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LOC is a non-test/test line-count split for one authored area.
type LOC struct {
	NonTest int
	Test    int
}

// Add returns the elementwise sum of l and o.
func (l LOC) Add(o LOC) LOC {
	return LOC{NonTest: l.NonTest + o.NonTest, Test: l.Test + o.Test}
}

// Sub returns the elementwise difference l-o.
func (l LOC) Sub(o LOC) LOC {
	return LOC{NonTest: l.NonTest - o.NonTest, Test: l.Test - o.Test}
}

// AreaLOC names one immediate subdirectory of domain/ together with its
// authored non-test/test line counts.
type AreaLOC struct {
	Name string
	LOC  LOC
}

// locInDir sums non-test and test .go line counts recursively under dir.
func locInDir(dir string) (LOC, error) {
	var total LOC
	err := walkGoFiles(dir, func(path, name string) error {
		n, err := countLines(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, "_test.go") {
			total.Test += n
		} else {
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
