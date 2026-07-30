// Package diffreport compares stable diagnostic JSONL snapshots.
package diffreport

import "strings"

// Record is the stable diagnostic baseline row shape used by the fixture
// baseline writer. Target, EntryID, Line, and Column are accepted for the
// external lint-harness baseline rows and are normalized into the same identity
// model before comparison.
type Record struct {
	Kind          string `json:"kind"`
	Suite         string `json:"suite,omitempty"`
	Entry         string `json:"entry,omitempty"`
	Status        string `json:"status,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Code          string `json:"code,omitempty"`
	Severity      string `json:"severity,omitempty"`
	SubjectAnchor string `json:"subject_anchor,omitempty"`
	File          string `json:"file,omitempty"`
	Span          Span   `json:"span,omitempty"`
	Message       string `json:"message,omitempty"`
	Help          string `json:"help,omitempty"`

	Evidence []Fact  `json:"evidence,omitempty"`
	Labels   []Label `json:"labels,omitempty"`

	Target     string `json:"target,omitempty"`
	SourcePath string `json:"source_path,omitempty"`
	EntryID    string `json:"entry_id,omitempty"`
	Line       int    `json:"line,omitempty"`
	Column     int    `json:"column,omitempty"`
}

// Span identifies the primary range for a diagnostic record.
type Span struct {
	StartLine int `json:"start_line,omitempty"`
	StartCol  int `json:"start_col,omitempty"`
	EndLine   int `json:"end_line,omitempty"`
	EndCol    int `json:"end_col,omitempty"`
}

// Fact is one evidence item attached to a diagnostic baseline row.
type Fact struct {
	Kind    string `json:"kind"`
	Trust   string `json:"trust"`
	Reason  string `json:"reason"`
	File    string `json:"file,omitempty"`
	Span    Span   `json:"span,omitempty"`
	Message string `json:"message,omitempty"`
}

// Label is one source label attached to a diagnostic baseline row.
type Label struct {
	File      string `json:"file,omitempty"`
	Span      Span   `json:"span,omitempty"`
	Message   string `json:"message,omitempty"`
	Placement string `json:"placement,omitempty"`
}

// Identity is the stable site key for a diagnostic record. Scope and Entry are
// outer namespaces that prevent collisions between fixture suites and external
// lint targets; File, Code, Line, Column, and Subject are the diagnostic site.
type Identity struct {
	Scope   string
	Entry   string
	File    string
	Code    string
	Line    int
	Column  int
	Subject string
}

type driftIdentity struct {
	Scope   string
	Entry   string
	File    string
	Code    string
	Message string
}

type anchoredIdentity struct {
	Scope         string
	Entry         string
	File          string
	Code          string
	SubjectAnchor string
}

// Key returns the exact identity used for first-pass matching.
func Key(r Record) Identity {
	line, col := primaryStart(r)
	return Identity{
		Scope:   scopeKey(r),
		Entry:   entryKey(r),
		File:    fileKey(r),
		Code:    r.Code,
		Line:    line,
		Column:  col,
		Subject: subjectDiscriminator(r),
	}
}

func driftKey(r Record) driftIdentity {
	return driftIdentity{
		Scope:   scopeKey(r),
		Entry:   entryKey(r),
		File:    fileKey(r),
		Code:    r.Code,
		Message: strings.TrimSpace(r.Message),
	}
}

func anchoredKey(r Record) (anchoredIdentity, bool) {
	if strings.TrimSpace(r.SubjectAnchor) == "" {
		return anchoredIdentity{}, false
	}
	return anchoredIdentity{
		Scope:         scopeKey(r),
		Entry:         entryKey(r),
		File:          fileKey(r),
		Code:          r.Code,
		SubjectAnchor: strings.TrimSpace(r.SubjectAnchor),
	}, true
}

func diagnosticRecord(r Record) bool {
	if r.Kind != "" && r.Kind != "diagnostic" {
		return false
	}
	return r.Code != "" && r.Message != ""
}

func normalizedKind(r Record) string {
	if r.Kind != "" {
		return r.Kind
	}
	if r.Code != "" && r.Message != "" {
		return "diagnostic"
	}
	return ""
}

func scopeKey(r Record) string {
	if r.Suite != "" {
		return r.Suite
	}
	return r.Target
}

func entryKey(r Record) string {
	if r.Entry != "" {
		return r.Entry
	}
	return r.EntryID
}

func fileKey(r Record) string {
	if r.File != "" {
		return r.File
	}
	if r.EntryID != "" {
		return r.EntryID
	}
	if r.Entry != "" {
		return r.Entry
	}
	return r.Target
}

func primarySpan(r Record) Span {
	if r.Span.StartLine != 0 || r.Span.StartCol != 0 || r.Span.EndLine != 0 || r.Span.EndCol != 0 {
		return r.Span
	}
	if r.Line == 0 && r.Column == 0 {
		return Span{}
	}
	return Span{
		StartLine: r.Line,
		StartCol:  r.Column,
	}
}

func primaryStart(r Record) (int, int) {
	span := primarySpan(r)
	return span.StartLine, span.StartCol
}

func subjectDiscriminator(r Record) string {
	msg := strings.TrimSpace(r.Message)
	if subject := argumentSubject(msg); subject != "" {
		return subject
	}
	if subject := returnedValueSubject(msg); subject != "" {
		return subject
	}
	if strings.HasPrefix(msg, "cannot assign ") {
		return "assign " + trimSubjectSuffix(strings.TrimPrefix(msg, "cannot assign "))
	}
	if strings.HasPrefix(msg, "cannot return ") {
		return "return " + trimSubjectSuffix(strings.TrimPrefix(msg, "cannot return "))
	}
	if strings.HasPrefix(msg, "cannot pass ") {
		return "pass " + trimSubjectSuffix(strings.TrimPrefix(msg, "cannot pass "))
	}
	if i := strings.Index(msg, " has no member "); i >= 0 {
		return "member " + strings.TrimSpace(msg[:i])
	}
	if strings.HasPrefix(msg, "unknown value ") {
		return "unknown " + trimSubjectSuffix(strings.TrimPrefix(msg, "unknown value "))
	}
	if strings.HasPrefix(msg, "channel select ") {
		return "channel select"
	}
	if msg == "operand of `..` may be nil" {
		return "concat operand"
	}
	if label := primaryLabelSubject(r); label != "" {
		return "label " + label
	}
	return ""
}

func argumentSubject(msg string) string {
	i := strings.Index(msg, "argument ")
	if i < 0 {
		return ""
	}
	start := i + len("argument ")
	end := start
	for end < len(msg) && msg[end] >= '0' && msg[end] <= '9' {
		end++
	}
	if end == start {
		return ""
	}
	return "argument " + msg[start:end]
}

func returnedValueSubject(msg string) string {
	if !strings.HasPrefix(msg, "returned value ") {
		return ""
	}
	start := len("returned value ")
	end := start
	for end < len(msg) && msg[end] >= '0' && msg[end] <= '9' {
		end++
	}
	if end == start {
		return ""
	}
	return "returned value " + msg[start:end]
}

func trimSubjectSuffix(subject string) string {
	subject = strings.TrimSpace(subject)
	for _, marker := range []string{
		" because ",
		" comes from ",
		" satisfies ",
		" is ",
		" has ",
		" to ",
	} {
		if i := strings.Index(subject, marker); i >= 0 {
			return strings.TrimSpace(subject[:i])
		}
	}
	return subject
}

func primaryLabelSubject(r Record) string {
	line, col := primaryStart(r)
	for _, label := range r.Labels {
		if label.Message == "" {
			continue
		}
		if label.Span.StartLine == line && label.Span.StartCol == col {
			return strings.TrimSpace(label.Message)
		}
	}
	return ""
}
