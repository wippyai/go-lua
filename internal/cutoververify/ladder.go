package cutoververify

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// LadderFixtures are the two canonical fixtures the landing ritual regresses
// on a two-commit ladder: <commit> against <commit>~1.
var LadderFixtures = []string{"bench/fibonacci", "basic/arithmetic"}

var (
	zzProbeLogPrefix = regexp.MustCompile(`^\s*\S+\.go:\d+:\s*`)
	zzProbeVolatile  = regexp.MustCompile(`\b(solve|compile|seal)=\S+`)
)

// runLadderFixture runs the oracle's single-fixture ZZPROBE lane. It never
// runs the full corpus: -run pins exactly TestZZProbeSolverLadderFixture,
// and ZZPROBE_FIXTURE pins that test to one named fixture.
func runLadderFixture(clonePath, fixture string) (string, error) {
	cmd := exec.Command("go", "test", "./oracle",
		"-tags", "typprobe zzsolveprobe",
		"-run", "^TestZZProbeSolverLadderFixture$",
		"-v", "-count=1")
	cmd.Dir = clonePath
	cmd.Env = append(cmd.Environ(), "ZZPROBE_FIXTURE="+fixture)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// normalizeZZProbeLines extracts the ZZPROBE counter lines from raw go test
// output, strips the "-v" test-log source prefix, and blanks out the
// wall-clock timing fields (solve=, compile=, seal=) that vary run to run
// independently of the counters they surround.
func normalizeZZProbeLines(output string) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "ZZPROBE") {
			continue
		}
		line = zzProbeLogPrefix.ReplaceAllString(line, "")
		line = zzProbeVolatile.ReplaceAllString(line, "")
		line = strings.Join(strings.Fields(line), " ")
		lines = append(lines, line)
	}
	return lines
}

// diffZZProbeLines compares two normalized ZZPROBE line sequences
// positionally and reports every differing or missing position.
func diffZZProbeLines(oldLines, newLines []string) []string {
	var deltas []string
	max := len(oldLines)
	if len(newLines) > max {
		max = len(newLines)
	}
	for i := 0; i < max; i++ {
		var oldLine, newLine string
		if i < len(oldLines) {
			oldLine = oldLines[i]
		}
		if i < len(newLines) {
			newLine = newLines[i]
		}
		if oldLine != newLine {
			deltas = append(deltas, fmt.Sprintf("line %d:\n  ~1: %s\n  @0: %s", i, oldLine, newLine))
		}
	}
	return deltas
}

// LadderResult compares one fixture's ZZPROBE output between commit and its
// parent, ignoring wall-clock timing fields.
func LadderResult(fixture, oldOutput, newOutput string) Result {
	name := "LADDER " + fixture
	oldLines := normalizeZZProbeLines(oldOutput)
	newLines := normalizeZZProbeLines(newOutput)
	deltas := diffZZProbeLines(oldLines, newLines)
	if len(deltas) == 0 {
		return Result{Name: name, Status: StatusPass, Note: "BYTE-IDENTICAL"}
	}
	return Result{
		Name:   name,
		Status: StatusFail,
		Note:   fmt.Sprintf("%d counter line(s) differ from commit~1", len(deltas)),
		Detail: strings.Join(deltas, "\n"),
	}
}

// RunLadderSuite runs LadderFixtures at commit and at commit~1 in the clone
// and returns one Result per fixture. It leaves the clone reset to commit
// when it returns.
func RunLadderSuite(clonePath, commit string, fixtures []string) ([]Result, error) {
	parent, err := ResolveCommit(clonePath, commit+"~1")
	if err != nil {
		return nil, fmt.Errorf("resolve %s~1: %w", commit, err)
	}

	if err := ResetClone(clonePath, commit); err != nil {
		return nil, err
	}
	newOutputs := make(map[string]string, len(fixtures))
	for _, fixture := range fixtures {
		out, err := runLadderFixture(clonePath, fixture)
		if err != nil {
			return nil, fmt.Errorf("run ladder fixture %s at %s: %w\n%s", fixture, commit, err, firstLines(out, 20))
		}
		newOutputs[fixture] = out
	}

	if err := ResetClone(clonePath, parent); err != nil {
		return nil, err
	}
	oldOutputs := make(map[string]string, len(fixtures))
	for _, fixture := range fixtures {
		out, err := runLadderFixture(clonePath, fixture)
		if err != nil {
			return nil, fmt.Errorf("run ladder fixture %s at %s: %w\n%s", fixture, parent, err, firstLines(out, 20))
		}
		oldOutputs[fixture] = out
	}

	if err := ResetClone(clonePath, commit); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(fixtures))
	for _, fixture := range fixtures {
		results = append(results, LadderResult(fixture, oldOutputs[fixture], newOutputs[fixture]))
	}
	return results, nil
}
