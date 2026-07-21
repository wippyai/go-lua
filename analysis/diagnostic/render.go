package diagnostic

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SourceMap provides source text by file name for diagnostic rendering.
type SourceMap map[string]string

// RenderOptions controls source-frame diagnostic rendering.
type RenderOptions struct {
	// Sources maps diagnostic file names to source text.
	Sources SourceMap
	// DisplayFiles maps internal diagnostic file names to user-facing file names.
	DisplayFiles map[string]string
	// Color enables ANSI color in the rendered diagnostic.
	Color bool
	// WitnessTrace renders refuted evidence in causal order: source facts first,
	// claimed obligations next, and missing proofs last.
	WitnessTrace bool
	// ShowSourceLabelRows renders semantic labels next to directional source-line
	// arrows instead of a separate caret row when labels are available. By default
	// source frames use exact carets only; labels still count as rendered, so they
	// do not repeat in a trailing "where" section.
	ShowSourceLabelRows bool
}

type palette struct {
	red    string
	yellow string
	green  string
	blue   string
	cyan   string
	bold   string
	reset  string
}

type renderText struct {
	fallbackSeverity        string
	evidenceSection         string
	labelsSection           string
	helpPrefix              string
	locationArrow           string
	labelFallbackStart      string
	provenHeading           string
	claimedHeading          string
	refutedHeading          string
	factHeading             string
	claimHeading            string
	missingProofHeading     string
	precisionBoundaryHeader string
	evidenceHeading         string
}

var defaultRenderText = renderText{
	fallbackSeverity:        "error",
	evidenceSection:         "because",
	labelsSection:           "where",
	helpPrefix:              "help",
	locationArrow:           "-->",
	labelFallbackStart:      "  = ",
	provenHeading:           "proven",
	claimedHeading:          "claimed",
	refutedHeading:          "refuted",
	factHeading:             "fact",
	claimHeading:            "claim",
	missingProofHeading:     "missing proof",
	precisionBoundaryHeader: "unvalidated value",
	evidenceHeading:         "evidence",
}

type renderStyle struct {
	palette palette
	text    renderText
}

const renderTabWidth = 4

func newRenderStyle(color bool) renderStyle {
	return renderStyle{
		palette: renderPalette(color),
		text:    defaultRenderText,
	}
}

// Render formats a diagnostic with source frames, evidence, labels, and help.
func Render(d Diagnostic, opts RenderOptions) string {
	var b strings.Builder
	style := newRenderStyle(opts.Color)
	p := style.palette

	severity := d.Severity.String()
	if severity == "unknown" {
		severity = style.text.fallbackSeverity
	}
	b.WriteString(style.colorSeverity(d.Severity, severity))
	if d.Code != "" {
		b.WriteString("[")
		b.WriteString(d.Code.String())
		b.WriteString("]")
	}
	b.WriteString(": ")
	b.WriteString(p.bold)
	b.WriteString(d.Message)
	b.WriteString(p.reset)
	b.WriteString("\n")

	primaryFile := d.Position.File
	primarySpan := primarySpan(d)
	labelMessages := labelMessagesByFrame(d.Labels, primaryFile)
	labelsByLine := labelsBySourceLine(d.Labels, primaryFile)
	renderedLabels := make(map[labelRenderKey]struct{})
	primaryRendered := false
	renderedFrames := make(map[sourceFrameKey]struct{})
	if primarySpan.Valid() || primaryFile != "" || d.Position.Valid() {
		primaryKey := sourceFrameKey{file: primaryFile, span: primarySpan}
		primaryMessage := frameLabelMessage(labelMessages[primaryKey])
		primaryResult := writeFrame(&b, opts, primaryFile, primarySpan, primaryMessage, frameLineLabels(labelsByLine, primaryFile, primarySpan), style)
		primaryRendered = primaryResult.rendered
		if primaryRendered {
			markFrameRendered(renderedFrames, primaryFile, primarySpan)
			markLabelsRendered(renderedLabels, primaryFile, primaryResult.labels)
		}
	}

	witnessTrace := opts.WitnessTrace || d.Explanation.WitnessTrace()
	if evidence := renderEvidence(d.Explanation.Evidence(), primaryFile, witnessTrace); len(evidence) > 0 {
		b.WriteString("\n")
		b.WriteString(p.blue)
		b.WriteString(style.text.evidenceSection)
		b.WriteString(p.reset)
		b.WriteString(":\n")
		for i, item := range evidence {
			fmt.Fprintf(&b, "  %d. %s: %s\n", i+1, style.evidenceHeading(item), evidenceMessage(item))
			itemFile := evidenceFile(primaryFile, item)
			if item.Span.Valid() || item.File != "" {
				key := sourceFrameKey{file: itemFile, span: item.Span}
				if frameAlreadyRendered(renderedFrames, itemFile, item.Span) {
					continue
				}
				labelMessage := frameLabelMessage(labelMessages[key])
				result := writeFrame(&b, opts, itemFile, item.Span, labelMessage, frameLineLabels(labelsByLine, itemFile, item.Span), style)
				if result.rendered {
					renderedFrames[key] = struct{}{}
					markLabelsRendered(renderedLabels, primaryFile, result.labels)
				}
			}
		}
	}

	if labels := remainingLabels(d.Labels, primaryFile, renderedLabels); len(labels) > 0 {
		b.WriteString("\n")
		b.WriteString(p.blue)
		b.WriteString(style.text.labelsSection)
		b.WriteString(p.reset)
		b.WriteString(":\n")
		sort.SliceStable(labels, func(i, j int) bool {
			leftFile := labelFile(primaryFile, labels[i])
			rightFile := labelFile(primaryFile, labels[j])
			if leftFile != rightFile {
				return leftFile < rightFile
			}
			if labels[i].Span.StartLine != labels[j].Span.StartLine {
				return labels[i].Span.StartLine < labels[j].Span.StartLine
			}
			return labels[i].Span.StartCol < labels[j].Span.StartCol
		})
		writtenLabelFrames := make(map[sourceFrameKey]struct{}, len(labels))
		for _, label := range labels {
			labelFile := labelFile(primaryFile, label)
			if label.Span.Valid() {
				key := sourceFrameKey{file: labelFile, span: label.Span}
				if _, ok := writtenLabelFrames[key]; ok {
					continue
				}
				writtenLabelFrames[key] = struct{}{}
				labelMessage := frameLabelMessage(labelMessages[key])
				if _, ok := renderedFrames[key]; ok {
					writeLabelReference(&b, opts, labelFile, label, labelMessage, style)
					continue
				}
				if writeFrame(&b, opts, labelFile, label.Span, labelMessage, nil, style).rendered {
					renderedFrames[key] = struct{}{}
				} else {
					writeLabelFallback(&b, label, labelMessage, style)
				}
				continue
			}
			if file := displayFile(opts, labelFile); file != "" {
				b.WriteString(" ")
				b.WriteString(p.blue)
				b.WriteString(style.text.locationArrow)
				b.WriteString(p.reset)
				b.WriteString(" ")
				b.WriteString(file)
				b.WriteString("\n")
			}
			b.WriteString(style.text.labelFallbackStart)
			b.WriteString(label.Message)
			b.WriteString("\n")
		}
	}

	if d.Help != "" {
		b.WriteString("\n")
		b.WriteString(style.text.helpPrefix)
		b.WriteString(": ")
		b.WriteString(p.green)
		b.WriteString(d.Help)
		b.WriteString(p.reset)
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func primarySpan(d Diagnostic) Span {
	if d.Span.Valid() {
		return d.Span
	}
	if d.Position.Valid() {
		return Span{
			StartLine: d.Position.Line,
			StartCol:  d.Position.Column,
			EndLine:   d.Position.EndLine,
			EndCol:    d.Position.EndColumn,
		}
	}
	return Span{}
}

func remainingLabels(labels []Label, primaryFile string, rendered map[labelRenderKey]struct{}) []Label {
	out := make([]Label, 0, len(labels))
	for _, label := range labels {
		if label.Message == "" {
			continue
		}
		if !label.Span.Valid() && label.File == "" {
			continue
		}
		key := labelKey(primaryFile, label)
		if _, ok := rendered[key]; ok {
			continue
		}
		out = append(out, label)
	}
	return out
}

type sourceLineKey struct {
	file string
	line int
}

func labelsBySourceLine(labels []Label, primaryFile string) map[sourceLineKey][]Label {
	out := make(map[sourceLineKey][]Label)
	for _, label := range labels {
		if !label.Span.Valid() || label.Message == "" {
			continue
		}
		file := labelFile(primaryFile, label)
		key := sourceLineKey{file: file, line: label.Span.StartLine}
		out[key] = append(out[key], label)
	}
	return out
}

func frameLineLabels(labels map[sourceLineKey][]Label, file string, span Span) []Label {
	if !span.Valid() || len(labels) == 0 {
		return nil
	}
	return labels[sourceLineKey{file: file, line: span.StartLine}]
}

func labelMessagesByFrame(labels []Label, primaryFile string) map[sourceFrameKey][]string {
	out := make(map[sourceFrameKey][]string, len(labels))
	for _, label := range labels {
		if !label.Span.Valid() || label.Message == "" {
			continue
		}
		key := sourceFrameKey{file: labelFile(primaryFile, label), span: label.Span}
		out[key] = append(out[key], label.Message)
	}
	return out
}

func frameLabelMessage(messages []string) string {
	if len(messages) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(messages))
	out := make([]string, 0, len(messages))
	for _, message := range messages {
		if message == "" {
			continue
		}
		if _, ok := seen[message]; ok {
			continue
		}
		seen[message] = struct{}{}
		out = append(out, message)
	}
	return strings.Join(out, "; ")
}

func evidenceMessage(item Evidence) string {
	if item.Message != "" {
		return item.Message
	}
	return fallbackEvidenceMessage(item)
}

func evidenceFile(defaultFile string, item Evidence) string {
	if item.File != "" {
		return item.File
	}
	return defaultFile
}

func labelFile(defaultFile string, label Label) string {
	if label.File != "" {
		return label.File
	}
	return defaultFile
}

type sourceFrameKey struct {
	file string
	span Span
}

type labelRenderKey struct {
	file    string
	span    Span
	message string
}

func labelKey(primaryFile string, label Label) labelRenderKey {
	return labelRenderKey{
		file:    labelFile(primaryFile, label),
		span:    label.Span,
		message: label.Message,
	}
}

func markLabelsRendered(seen map[labelRenderKey]struct{}, primaryFile string, labels []Label) {
	for _, label := range labels {
		seen[labelKey(primaryFile, label)] = struct{}{}
	}
}

func markFrameRendered(seen map[sourceFrameKey]struct{}, file string, span Span) {
	if span.Valid() || file != "" {
		seen[sourceFrameKey{file: file, span: span}] = struct{}{}
	}
}

func frameAlreadyRendered(seen map[sourceFrameKey]struct{}, file string, span Span) bool {
	if _, ok := seen[sourceFrameKey{file: file, span: span}]; ok {
		return true
	}
	if !span.Valid() {
		return false
	}
	for rendered := range seen {
		if rendered.file == file && rendered.span.Valid() && rendered.span.StartLine == span.StartLine {
			return true
		}
		if rendered.file == file && spanCoveredBy(rendered.span, span) {
			return true
		}
	}
	return false
}

func spanCoveredBy(container, inner Span) bool {
	if !container.Valid() || !inner.Valid() || !container.SingleLine() || !inner.SingleLine() {
		return false
	}
	return container.StartLine == inner.StartLine &&
		container.StartCol <= inner.StartCol &&
		spanEnd(container) >= spanEnd(inner)
}

func spanEnd(span Span) int {
	if span.EndCol > span.StartCol {
		return span.EndCol
	}
	return span.StartCol + 1
}

type evidenceRenderKey struct {
	kind    EvidenceKind
	trust   TrustKind
	reason  EvidenceReason
	cause   EvidenceCauseKind
	file    string
	span    Span
	message string
}

func uniqueEvidence(items []Evidence, defaultFile string) []Evidence {
	if len(items) < 2 {
		return items
	}
	seen := make(map[evidenceRenderKey]struct{}, len(items))
	out := make([]Evidence, 0, len(items))
	for _, item := range items {
		key := evidenceRenderKey{
			kind:    item.Kind,
			trust:   item.Trust,
			reason:  item.Reason,
			cause:   item.Cause.Kind,
			file:    evidenceFile(defaultFile, item),
			span:    item.Span,
			message: evidenceMessage(item),
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

type witnessTraceEvidenceKey struct {
	file    string
	span    Span
	message string
}

func uniqueWitnessTraceEvidence(items []Evidence, defaultFile string) []Evidence {
	if len(items) < 2 {
		return items
	}
	seen := make(map[witnessTraceEvidenceKey]struct{}, len(items))
	out := make([]Evidence, 0, len(items))
	for _, item := range items {
		key := witnessTraceEvidenceKey{
			file:    evidenceFile(defaultFile, item),
			span:    item.Span,
			message: evidenceMessage(item),
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func renderEvidence(items []Evidence, primaryFile string, witnessTrace bool) []Evidence {
	if witnessTrace {
		return SourceOrderedEvidenceTrace(items, primaryFile)
	}
	return uniqueEvidence(items, primaryFile)
}

// SourceOrderedEvidenceTrace returns the renderer-visible evidence chain in
// source order. Repeated source origins with the same content are collapsed;
// distinct content at one source location remains visible.
func SourceOrderedEvidenceTrace(items []Evidence, primaryFile string) []Evidence {
	return orderWitnessTraceEvidence(uniqueWitnessTraceEvidence(items, primaryFile), primaryFile)
}

func orderWitnessTraceEvidence(items []Evidence, primaryFile string) []Evidence {
	if len(items) < 2 {
		return append([]Evidence(nil), items...)
	}
	out := append([]Evidence(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		leftSpanned := out[i].Span.Valid()
		rightSpanned := out[j].Span.Valid()
		if leftSpanned != rightSpanned {
			return leftSpanned
		}
		if !leftSpanned {
			return false
		}
		return witnessTraceSpanBefore(out[i], out[j], primaryFile)
	})
	return out
}

func witnessTraceSpanBefore(left, right Evidence, primaryFile string) bool {
	leftFile := evidenceOrderFile(left, primaryFile)
	rightFile := evidenceOrderFile(right, primaryFile)
	if leftFile != rightFile {
		return leftFile < rightFile
	}
	if left.Span.StartLine != right.Span.StartLine {
		return left.Span.StartLine < right.Span.StartLine
	}
	if left.Span.StartCol != right.Span.StartCol {
		return left.Span.StartCol < right.Span.StartCol
	}
	return false
}

func evidenceOrderFile(item Evidence, primaryFile string) string {
	if item.File != "" {
		return item.File
	}
	return primaryFile
}

type frameRenderResult struct {
	rendered bool
	labels   []Label
}

func writeFrame(b *strings.Builder, opts RenderOptions, file string, span Span, message string, labels []Label, style renderStyle) frameRenderResult {
	p := style.palette
	location := displayFile(opts, file)
	if !span.Valid() {
		if location != "" {
			b.WriteString(" ")
			b.WriteString(p.blue)
			b.WriteString(style.text.locationArrow)
			b.WriteString(p.reset)
			b.WriteString(" ")
			b.WriteString(location)
			b.WriteString("\n")
			return frameRenderResult{rendered: true}
		}
		return frameRenderResult{}
	}
	if location == "" {
		return frameRenderResult{}
	}

	lineNum := span.StartLine
	col := span.StartCol
	if col < 1 {
		col = 1
	}

	lineNo := strconv.Itoa(lineNum)
	gutterWidth := len(lineNo)

	fmt.Fprintf(b, " %s%s%s %s:%d:%d\n", p.blue, style.text.locationArrow, p.reset, location, lineNum, col)
	sourceLine, ok := sourceLine(opts, file, lineNum)
	if !ok {
		return frameRenderResult{rendered: true}
	}
	displayLine := expandTabs(sourceLine)

	gutter := strings.Repeat(" ", gutterWidth)
	fmt.Fprintf(b, "%s %s|%s\n", gutter, p.blue, p.reset)
	annotations := frameAnnotations(displaySpanForLine(span, sourceLine), message, displayFrameLabels(labels, sourceLine))
	if opts.ShowSourceLabelRows {
		aboveRows, belowRows := compactFrameAnnotationLabelRows(sourceLine, displayLine, annotations, style)
		if len(aboveRows) > 0 || len(belowRows) > 0 {
			writeFrameAnnotationLabelRows(b, gutter, aboveRows, style)
			fmt.Fprintf(b, "%*d %s|%s %s\n", gutterWidth, lineNum, p.blue, p.reset, displayLine)
			writeFrameAnnotationLabelRows(b, gutter, belowRows, style)
			return frameRenderResult{rendered: true, labels: renderedAnnotationLabels(annotations)}
		}
	}
	fmt.Fprintf(b, "%*d %s|%s %s\n", gutterWidth, lineNum, p.blue, p.reset, displayLine)
	writeFrameAnnotations(b, gutter, displayLine, annotations, style)
	return frameRenderResult{rendered: true, labels: renderedAnnotationLabels(annotations)}
}

func writeFrameAnnotationLabelRows(b *strings.Builder, gutter string, rows []string, style renderStyle) bool {
	if len(rows) == 0 {
		return false
	}
	p := style.palette
	wrote := false
	for _, row := range rows {
		if strings.TrimSpace(row) == "" {
			continue
		}
		fmt.Fprintf(b, "%s %s|%s %s\n", gutter, p.blue, p.reset, row)
		wrote = true
	}
	return wrote
}

func compactFrameAnnotationLabelRows(sourceLine, displayLine string, annotations []frameAnnotation, style renderStyle) ([]string, []string) {
	var above, below []frameAnnotation
	for _, annotation := range annotations {
		if annotation.belongsBelow() {
			below = append(below, annotation)
			continue
		}
		above = append(above, annotation)
	}
	return compactFrameAnnotationRows(displayLine, above, style, "↓"), compactFrameAnnotationRows(displayLine, below, style, "↑")
}

func (a frameAnnotation) belongsBelow() bool {
	switch a.placement {
	case LabelPlacementBelow:
		return true
	case LabelPlacementAbove:
		return false
	default:
		return a.primary
	}
}

func compactFrameAnnotationRows(sourceLine string, annotations []frameAnnotation, style renderStyle, marker string) []string {
	byColumn := sortedFrameAnnotationsByColumn(sourceLine, annotations)
	var rows [][]string
	for _, annotation := range byColumn {
		if annotation.message == "" {
			continue
		}
		col := clampColumn(annotation.span.StartCol, sourceLine)
		placed := false
		for i := range rows {
			if placeAnnotationLabelAt(&rows[i], col, annotation.message, style, marker) {
				placed = true
				break
			}
		}
		if placed {
			continue
		}
		row := make([]string, max(1, len(sourceLine)+1))
		placeAnnotationLabelAt(&row, col, annotation.message, style, marker)
		rows = append(rows, row)
	}

	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, strings.TrimRight(annotationCellsString(row), " "))
	}
	return out
}

type frameAnnotation struct {
	span      Span
	message   string
	labels    []Label
	primary   bool
	placement LabelPlacement
}

type frameLabel struct {
	displaySpan Span
	original    Label
}

func displayFrameLabels(labels []Label, sourceLine string) []frameLabel {
	out := make([]frameLabel, 0, len(labels))
	for _, label := range labels {
		if !label.Span.Valid() {
			continue
		}
		out = append(out, frameLabel{
			displaySpan: displaySpanForLine(label.Span, sourceLine),
			original:    label,
		})
	}
	return out
}

func frameAnnotations(primary Span, primaryMessage string, labels []frameLabel) []frameAnnotation {
	sameLineLabels := make([]frameLabel, 0, len(labels))
	for _, label := range labels {
		if !label.displaySpan.Valid() || label.displaySpan.StartLine != primary.StartLine {
			continue
		}
		sameLineLabels = append(sameLineLabels, label)
	}

	annotations := make([]frameAnnotation, 0, len(sameLineLabels)+1)
	index := make(map[Span]int, len(sameLineLabels)+1)
	if primaryMessage != "" || len(sameLineLabels) == 0 {
		index[primary] = len(annotations)
		annotations = append(annotations, frameAnnotation{span: primary, message: primaryMessage, primary: true})
	}

	for _, label := range sameLineLabels {
		i, ok := index[label.displaySpan]
		if !ok {
			i = len(annotations)
			index[label.displaySpan] = i
			annotations = append(annotations, frameAnnotation{span: label.displaySpan})
		}
		if spanEqual(annotationSpanKey(label.displaySpan), annotationSpanKey(primary)) {
			annotations[i].primary = true
		}
		annotations[i].message = appendFrameMessage(annotations[i].message, label.original.Message)
		annotations[i].labels = append(annotations[i].labels, label.original)
		annotations[i].placement = mergeLabelPlacement(annotations[i].placement, label.original.Placement)
	}
	sort.SliceStable(annotations, func(i, j int) bool {
		if annotations[i].primary != annotations[j].primary {
			return annotations[i].primary
		}
		if annotations[i].span.StartCol != annotations[j].span.StartCol {
			return annotations[i].span.StartCol < annotations[j].span.StartCol
		}
		return spanEnd(annotations[i].span) < spanEnd(annotations[j].span)
	})
	return annotations
}

func mergeLabelPlacement(existing, next LabelPlacement) LabelPlacement {
	if next == LabelPlacementAuto {
		return existing
	}
	if existing == LabelPlacementAuto || existing == next {
		return next
	}
	return LabelPlacementAuto
}

func writeFrameAnnotations(b *strings.Builder, gutter, sourceLine string, annotations []frameAnnotation, style renderStyle) {
	if len(annotations) == 0 {
		return
	}
	if len(annotations) == 1 {
		writeSingleFrameAnnotation(b, gutter, sourceLine, annotations[0], style)
		return
	}
	writeCompactFrameAnnotations(b, gutter, sourceLine, annotations, style)
}

func writeSingleFrameAnnotation(b *strings.Builder, gutter, sourceLine string, annotation frameAnnotation, style renderStyle) {
	p := style.palette
	fmt.Fprintf(b, "%s %s|%s %s%s^%s", gutter, p.blue, p.reset, strings.Repeat(" ", clampColumn(annotation.span.StartCol, sourceLine)-1), p.yellow, p.reset)
	b.WriteString("\n")
}

func writeCompactFrameAnnotations(b *strings.Builder, gutter, sourceLine string, annotations []frameAnnotation, style renderStyle) {
	p := style.palette
	fmt.Fprintf(b, "%s %s|%s %s\n", gutter, p.blue, p.reset, compactFrameAnnotationCaretLine(sourceLine, annotations, style))
}

func placeAnnotationLabelAt(cells *[]string, col int, message string, style renderStyle, marker string) bool {
	runes := []rune(message)
	if len(runes) == 0 {
		return true
	}
	start := col
	end := col + len(runes) + 1
	if !annotationTextCellsAvailable(*cells, start, end) {
		return false
	}
	ensureAnnotationCells(cells, end)
	(*cells)[col-1] = style.palette.yellow + marker + style.palette.reset
	(*cells)[col] = " "
	for i, r := range runes {
		(*cells)[col+1+i] = string(r)
	}
	return true
}

func compactFrameAnnotationCaretLine(sourceLine string, annotations []frameAnnotation, style renderStyle) string {
	cells := make([]string, max(1, len(sourceLine)+1))
	for _, annotation := range sortedFrameAnnotationsByColumn(sourceLine, annotations) {
		col := clampColumn(annotation.span.StartCol, sourceLine)
		ensureAnnotationCells(&cells, col)
		if cells[col-1] == "" {
			cells[col-1] = style.palette.yellow + "^" + style.palette.reset
		}
	}
	return strings.TrimRight(annotationCellsString(cells), " ")
}

func sortedFrameAnnotationsByColumn(sourceLine string, annotations []frameAnnotation) []frameAnnotation {
	byColumn := append([]frameAnnotation(nil), annotations...)
	sort.SliceStable(byColumn, func(i, j int) bool {
		left := clampColumn(byColumn[i].span.StartCol, sourceLine)
		right := clampColumn(byColumn[j].span.StartCol, sourceLine)
		if left != right {
			return left < right
		}
		return spanEnd(byColumn[i].span) < spanEnd(byColumn[j].span)
	})
	return byColumn
}

func ensureAnnotationCells(cells *[]string, col int) {
	for len(*cells) < col {
		*cells = append(*cells, "")
	}
}

func annotationTextCellsAvailable(cells []string, start, end int) bool {
	if !annotationCellsAvailable(cells, start, end) {
		return false
	}
	if start > 2 && start-2 <= len(cells) && cells[start-3] != "" {
		return false
	}
	if start > 1 && start-1 <= len(cells) && cells[start-2] != "" {
		return false
	}
	if end+1 <= len(cells) && cells[end] != "" {
		return false
	}
	return true
}

func annotationCellsAvailable(cells []string, start, end int) bool {
	if start < 1 || end < start {
		return false
	}
	for col := start; col <= end; col++ {
		if col <= len(cells) && cells[col-1] != "" {
			return false
		}
	}
	return true
}

func annotationCellsString(cells []string) string {
	var b strings.Builder
	for _, cell := range cells {
		if cell == "" {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(cell)
	}
	return b.String()
}

func annotationSpanKey(span Span) Span {
	return Span{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func spanEqual(left, right Span) bool {
	return left.StartLine == right.StartLine &&
		left.StartCol == right.StartCol &&
		left.EndLine == right.EndLine &&
		left.EndCol == right.EndCol
}

func appendFrameMessage(existing, next string) string {
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	for _, part := range strings.Split(existing, "; ") {
		if part == next {
			return existing
		}
	}
	return existing + "; " + next
}

func renderedAnnotationLabels(annotations []frameAnnotation) []Label {
	var out []Label
	for _, annotation := range annotations {
		out = append(out, annotation.labels...)
	}
	return out
}

func sourceLine(opts RenderOptions, file string, line int) (string, bool) {
	if opts.Sources == nil || line < 1 {
		return "", false
	}
	source, ok := opts.Sources[file]
	if !ok {
		if display := opts.DisplayFiles[file]; display != "" {
			source, ok = opts.Sources[display]
		}
	}
	if !ok && file == "" && len(opts.Sources) == 1 {
		for _, only := range opts.Sources {
			source = only
			ok = true
		}
	}
	if !ok {
		return "", false
	}
	lines := strings.Split(source, "\n")
	if line > len(lines) {
		return "", false
	}
	return strings.TrimSuffix(lines[line-1], "\r"), true
}

func expandTabs(line string) string {
	if !strings.ContainsRune(line, '\t') {
		return line
	}
	var b strings.Builder
	displayCol := 1
	for _, r := range line {
		if r != '\t' {
			b.WriteRune(r)
			displayCol++
			continue
		}
		spaces := spacesToNextTabStop(displayCol)
		b.WriteString(strings.Repeat(" ", spaces))
		displayCol += spaces
	}
	return b.String()
}

func displaySpanForLine(span Span, sourceLine string) Span {
	if !span.Valid() || !strings.ContainsRune(sourceLine, '\t') {
		return span
	}
	out := span
	out.StartCol = displayColumn(sourceLine, span.StartCol)
	if span.EndLine == span.StartLine && span.EndCol > span.StartCol {
		out.EndCol = displayColumn(sourceLine, span.EndCol)
	}
	return out
}

func displayColumn(line string, sourceCol int) int {
	if sourceCol < 1 {
		return 1
	}
	sourceCursor := 1
	displayCol := 1
	for _, r := range line {
		if sourceCursor >= sourceCol {
			return displayCol
		}
		if r == '\t' {
			displayCol += spacesToNextTabStop(displayCol)
		} else {
			displayCol++
		}
		sourceCursor++
	}
	if sourceCol > sourceCursor {
		return displayCol + sourceCol - sourceCursor
	}
	return displayCol
}

func spacesToNextTabStop(displayCol int) int {
	offset := (displayCol - 1) % renderTabWidth
	return renderTabWidth - offset
}

func displayFile(opts RenderOptions, file string) string {
	if display := opts.DisplayFiles[file]; display != "" {
		return display
	}
	if file != "" {
		return file
	}
	if len(opts.Sources) == 1 {
		for only := range opts.Sources {
			return only
		}
	}
	return ""
}

func clampColumn(col int, line string) int {
	if col < 1 {
		return 1
	}
	if col > len(line)+1 {
		return len(line) + 1
	}
	return col
}

func (s renderStyle) evidenceHeading(item Evidence) string {
	p := s.palette
	heading := s.evidenceHeadingText(item)
	switch {
	case item.Kind == EvidenceMissingProof || item.Trust == TrustRefuted:
		return p.red + heading + p.reset
	case item.Kind == EvidencePrecisionBoundary || item.Trust == TrustUnknown:
		return p.yellow + heading + p.reset
	case item.Kind == EvidenceUserAssertion || item.Trust == TrustClaimed:
		return p.cyan + heading + p.reset
	default:
		return p.green + heading + p.reset
	}
}

func (s renderStyle) evidenceHeadingText(item Evidence) string {
	switch item.Kind {
	case EvidenceAbstractFact:
		switch item.Trust {
		case TrustProven:
			return s.text.provenHeading
		case TrustClaimed:
			return s.text.claimedHeading
		case TrustRefuted:
			return s.text.refutedHeading
		default:
			return s.text.factHeading
		}
	case EvidenceUserAssertion:
		switch item.Trust {
		case TrustClaimed:
			return s.text.claimedHeading
		case TrustRefuted:
			return s.text.refutedHeading
		default:
			return s.text.claimHeading
		}
	case EvidenceMissingProof:
		return s.text.missingProofHeading
	case EvidencePrecisionBoundary:
		return s.text.precisionBoundaryHeader
	default:
		switch item.Trust {
		case TrustProven:
			return s.text.provenHeading
		case TrustClaimed:
			return s.text.claimedHeading
		case TrustRefuted:
			return s.text.refutedHeading
		default:
			return s.text.evidenceHeading
		}
	}
}

func writeLabelFallback(b *strings.Builder, label Label, message string, style renderStyle) {
	p := style.palette
	if message == "" {
		message = label.Message
	}
	if label.Span.Valid() {
		fmt.Fprintf(b, "  = line %d:%d: %s\n", label.Span.StartLine, label.Span.StartCol, message)
		return
	}
	b.WriteString(style.text.labelFallbackStart)
	b.WriteString(p.bold)
	b.WriteString(message)
	b.WriteString(p.reset)
	b.WriteString("\n")
}

func writeLabelReference(b *strings.Builder, opts RenderOptions, file string, label Label, message string, style renderStyle) {
	if message == "" {
		message = label.Message
	}
	b.WriteString(style.text.labelFallbackStart)
	if location := displayFile(opts, file); location != "" {
		fmt.Fprintf(b, "%s:%d:%d: ", location, label.Span.StartLine, label.Span.StartCol)
	} else {
		fmt.Fprintf(b, "line %d:%d: ", label.Span.StartLine, label.Span.StartCol)
	}
	b.WriteString(message)
	b.WriteString("\n")
}

func renderPalette(enabled bool) palette {
	if !enabled {
		return palette{}
	}
	return palette{
		red:    "\x1b[1;31m",
		yellow: "\x1b[1;33m",
		green:  "\x1b[1;32m",
		blue:   "\x1b[34m",
		cyan:   "\x1b[36m",
		bold:   "\x1b[1m",
		reset:  "\x1b[0m",
	}
}

func (s renderStyle) colorSeverity(severity Severity, text string) string {
	p := s.palette
	switch severity {
	case SeverityError:
		return p.red + text + p.reset
	case SeverityWarning:
		return p.yellow + text + p.reset
	case SeverityHint:
		return p.cyan + text + p.reset
	default:
		return p.red + text + p.reset
	}
}
