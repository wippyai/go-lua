package cutoververify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// LegacyProtocolTokens are the pre-cutover protocol identifiers a completed
// cutover must no longer reference.
var LegacyProtocolTokens = []string{
	"HotRule",
	"BindHot",
	"SchemaFragment",
	"DeclareSchema",
	"DeclareRule",
	"RegisterRule",
	"BindRule",
}

var legacyProtocolPattern = regexp.MustCompile(`\b(` + strings.Join(LegacyProtocolTokens, "|") + `)\b`)

// ProtocolZeroHit is one legacy-token reference found in a non-test file.
type ProtocolZeroHit struct {
	File string
	Line int
	Text string
}

// ProtocolZeroReport is the outcome of scanning one directory tree.
type ProtocolZeroReport struct {
	Hits []ProtocolZeroHit
}

// ClassifyProtocolZero walks dir recursively and reports every legacy
// protocol token found in a non-test .go file. It is pure filesystem
// scanning with no git dependency, so it can be exercised directly against
// a temp-dir fixture.
func ClassifyProtocolZero(dir string) (ProtocolZeroReport, error) {
	var report ProtocolZeroReport
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		hits, err := scanFileForProtocolZero(path)
		if err != nil {
			return err
		}
		report.Hits = append(report.Hits, hits...)
		return nil
	})
	if err != nil {
		return ProtocolZeroReport{}, err
	}
	sort.Slice(report.Hits, func(i, j int) bool {
		if report.Hits[i].File != report.Hits[j].File {
			return report.Hits[i].File < report.Hits[j].File
		}
		return report.Hits[i].Line < report.Hits[j].Line
	})
	return report, nil
}

func scanFileForProtocolZero(path string) ([]ProtocolZeroHit, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	var hits []ProtocolZeroHit
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if legacyProtocolPattern.MatchString(text) {
			hits = append(hits, ProtocolZeroHit{File: path, Line: line, Text: strings.TrimSpace(text)})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return hits, nil
}

// ProtocolZeroCheck runs ClassifyProtocolZero over <clonePath>/<pkg> and
// turns it into a Result. Nonzero hits are reported but only fail the run
// when requireZero is set.
func ProtocolZeroCheck(clonePath, pkg string, requireZero bool) (Result, error) {
	dir := filepath.Join(clonePath, filepath.FromSlash(pkg))
	report, err := ClassifyProtocolZero(dir)
	if err != nil {
		return Result{}, fmt.Errorf("protocol-zero scan of %s: %w", dir, err)
	}
	if len(report.Hits) == 0 {
		return Result{Name: "PROTOCOL-ZERO", Status: StatusPass, Note: "0 legacy token references"}, nil
	}
	lines := make([]string, 0, len(report.Hits))
	for _, hit := range report.Hits {
		rel, err := filepath.Rel(clonePath, hit.File)
		if err != nil {
			rel = hit.File
		}
		lines = append(lines, fmt.Sprintf("%s:%d: %s", rel, hit.Line, hit.Text))
	}
	status := StatusWarn
	if requireZero {
		status = StatusFail
	}
	return Result{
		Name:   "PROTOCOL-ZERO",
		Status: status,
		Note:   fmt.Sprintf("%d legacy token reference(s) found", len(report.Hits)),
		Detail: strings.Join(lines, "\n"),
	}, nil
}
