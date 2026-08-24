package inspect

import "strings"

// diffLayers are the layers a diff compares, in stable order. Each is rendered
// by the same command that renders it for a single fixture, so the two sides
// are always compared through one reading.
var diffLayers = []string{"target", "rows", "publish", "why"}

// formatDiff renders the lines on which two fixtures' layers differ. A fixture
// diffed against itself renders nothing.
func formatDiff(left, right *Session) string {
	if left == nil || right == nil {
		return ""
	}
	var b strings.Builder
	for _, layer := range diffLayers {
		leftText, leftErr := left.Command(layer)
		rightText, rightErr := right.Command(layer)
		if leftErr != nil || rightErr != nil {
			writef(&b, "diff.%s=unavailable", layer)
			continue
		}
		writeLineDiff(&b, layer, left.fixture, right.fixture, leftText, rightText)
	}
	return b.String()
}

// writeLineDiff reports the lines each side holds alone. The renderings are
// stable and one fact per line, so a set difference over lines is exactly the
// set of facts on which the two solves disagree.
func writeLineDiff(b *strings.Builder, layer, leftName, rightName, leftText, rightText string) {
	leftLines := strings.Split(leftText, "\n")
	rightLines := strings.Split(rightText, "\n")
	rightCounts := make(map[string]int, len(rightLines))
	for _, line := range rightLines {
		rightCounts[line]++
	}
	leftCounts := make(map[string]int, len(leftLines))
	for _, line := range leftLines {
		leftCounts[line]++
	}
	for _, line := range leftLines {
		if line == "" {
			continue
		}
		if rightCounts[line] > 0 {
			rightCounts[line]--
			continue
		}
		writef(b, "diff.%s.only[%s]=%s", layer, leftName, line)
	}
	for _, line := range rightLines {
		if line == "" {
			continue
		}
		if leftCounts[line] > 0 {
			leftCounts[line]--
			continue
		}
		writef(b, "diff.%s.only[%s]=%s", layer, rightName, line)
	}
}
