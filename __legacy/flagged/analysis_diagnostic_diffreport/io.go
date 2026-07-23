package diffreport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// ReadJSONL decodes diagnostic baseline records from JSON Lines.
func ReadJSONL(r io.Reader) ([]Record, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var records []Record
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, fmt.Errorf("decode line %d: %w", lineNo, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

// ReadJSONLFile decodes diagnostic baseline records from path.
func ReadJSONLFile(path string) ([]Record, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ReadJSONL(file)
}

// WriteReport writes report in either "human", "text", "json", or "jsonl"
// format.
func WriteReport(w io.Writer, report Report, format string) error {
	switch strings.ToLower(format) {
	case "", "human", "text":
		return WriteText(w, report)
	case "json", "jsonl":
		return WriteJSONL(w, report)
	default:
		return fmt.Errorf("unknown diff report format %q", format)
	}
}

// WriteJSONL emits one JSON object per classified delta.
func WriteJSONL(w io.Writer, report Report) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, record := range report.New {
		record = outputRecord(record)
		row := jsonDelta{Delta: "new", Record: &record}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	for _, record := range report.Removed {
		record = outputRecord(record)
		row := jsonDelta{Delta: "removed", Record: &record}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	for _, change := range report.Changed {
		baseline := outputRecord(change.Baseline)
		current := outputRecord(change.Current)
		row := jsonDelta{
			Delta:    "changed",
			Baseline: &baseline,
			Current:  &current,
		}
		if err := enc.Encode(row); err != nil {
			return err
		}
	}
	return nil
}

type jsonDelta struct {
	Delta    string  `json:"delta"`
	Record   *Record `json:"record,omitempty"`
	Baseline *Record `json:"baseline,omitempty"`
	Current  *Record `json:"current,omitempty"`
}

func outputRecord(r Record) Record {
	if r.Kind == "" && diagnosticRecord(r) {
		r.Kind = "diagnostic"
	}
	if r.Span.StartLine == 0 && r.Span.StartCol == 0 && r.Span.EndLine == 0 && r.Span.EndCol == 0 {
		r.Span = primarySpan(r)
	}
	return r
}

// WriteText emits a deterministic human-readable report.
func WriteText(w io.Writer, report Report) error {
	if _, err := fmt.Fprintf(w, "new: %d\nremoved: %d\nchanged: %d\n", len(report.New), len(report.Removed), len(report.Changed)); err != nil {
		return err
	}
	if report.Empty() {
		_, err := fmt.Fprintln(w, "\nno diagnostic delta")
		return err
	}
	if len(report.New) > 0 {
		if err := writeRecordSection(w, "new", report.New); err != nil {
			return err
		}
	}
	if len(report.Removed) > 0 {
		if err := writeRecordSection(w, "removed", report.Removed); err != nil {
			return err
		}
	}
	if len(report.Changed) > 0 {
		if _, err := fmt.Fprintln(w, "\nchanged"); err != nil {
			return err
		}
		for _, change := range report.Changed {
			if _, err := fmt.Fprintf(w, "  %s\n", recordSummary(change.Baseline)); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "    before: %s\n", change.Baseline.Message); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "    after:  %s\n", change.Current.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeRecordSection(w io.Writer, name string, records []Record) error {
	if _, err := fmt.Fprintf(w, "\n%s\n", name); err != nil {
		return err
	}
	for _, record := range records {
		if _, err := fmt.Fprintf(w, "  %s\n", recordSummary(record)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "    %s\n", record.Message); err != nil {
			return err
		}
	}
	return nil
}

func recordSummary(r Record) string {
	var parts []string
	scope := scopeKey(r)
	entry := entryKey(r)
	file := fileKey(r)
	line, col := primaryStart(r)

	if scope != "" {
		parts = append(parts, scope)
	}
	if entry != "" && entry != file {
		parts = append(parts, entry)
	}
	location := file
	if location == "" {
		location = "?"
	}
	if line > 0 {
		if col > 0 {
			location = fmt.Sprintf("%s:%d:%d", location, line, col)
		} else {
			location = fmt.Sprintf("%s:%d", location, line)
		}
	}
	parts = append(parts, location)
	if r.Code != "" {
		parts = append(parts, r.Code)
	}
	if subject := subjectDiscriminator(r); subject != "" {
		parts = append(parts, "["+subject+"]")
	}
	return strings.Join(parts, " ")
}
