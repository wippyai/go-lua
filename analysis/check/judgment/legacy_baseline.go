package judgment

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// LegacyCodeMapper maps a rendered diagnostic code to the semantic judgment
// code used for migration shadowing. Returning false drops the record.
type LegacyCodeMapper func(code string) (string, bool)

// ReadLegacyBaselineShadowRecords reads JSONL diagnostic records from a frozen
// rendered baseline. It intentionally understands only the small stable shape
// emitted by fixture/external baselines: kind, code, file, and span.
func ReadLegacyBaselineShadowRecords(r io.Reader, mapCode LegacyCodeMapper) ([]ShadowRecord, error) {
	if r == nil {
		return nil, nil
	}
	if mapCode == nil {
		mapCode = func(code string) (string, bool) { return code, code != "" }
	}

	var out []ShadowRecord
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		data := bytes.TrimSpace(scanner.Bytes())
		if len(data) == 0 {
			continue
		}
		var record legacyBaselineRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("legacy baseline line %d: %w", line, err)
		}
		if record.Kind != "diagnostic" {
			continue
		}
		code, ok := mapCode(record.Code)
		if !ok {
			continue
		}
		out = append(out, ShadowRecord{
			Code: code,
			Span: SpanRef{
				File:      record.File,
				StartLine: record.Span.StartLine,
				StartCol:  record.Span.StartCol,
				EndLine:   record.Span.EndLine,
				EndCol:    record.Span.EndCol,
			},
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type legacyBaselineRecord struct {
	Kind string             `json:"kind"`
	Code string             `json:"code"`
	File string             `json:"file"`
	Span legacyBaselineSpan `json:"span"`
}

type legacyBaselineSpan struct {
	StartLine int `json:"start_line"`
	StartCol  int `json:"start_col"`
	EndLine   int `json:"end_line"`
	EndCol    int `json:"end_col"`
}
