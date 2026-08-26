// Package corpus is the full-corpus differential driver of the relational
// engine cut: it walks the frozen fixture corpus, has one observation process
// answer each fixture on both engines, and catalogues where the two answers
// part.
//
// The package holds the same fence the parity harness holds: it links no
// analyzer. One foreign binary per fixture compiles that fixture once and
// solves it on both engines; this package only frames, compares and reports
// what that binary wrote to its stdout. A driver that cannot link an engine
// cannot quietly answer for one.
package corpus

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Protocol identifies the envelope grammar one observation process writes.
const Protocol = "w5-corpus/v1"

// SolveReady is the phase boundary emitted by an observation process after
// it has compiled and sealed its fixture, immediately before either engine is
// asked to solve.  The corpus driver starts its analysis budget only after
// seeing this line.  Compilation therefore remains governed by the process
// watchdog, not by the per-fixture solve bound.
const SolveReady = "corpus.Phase=solve"

// SolvePermit is written by the driver after it has started the solve timer.
// A probe waits for this acknowledgement before asking either engine to
// solve, making the phase boundary a handshake rather than a best-effort
// timestamp observed through a pipe.
const SolvePermit = "corpus.PhaseAck=solve"

// ErrMalformed refuses an envelope that does not read as this protocol.
var ErrMalformed = errors.New("corpus probe: malformed envelope")

// ErrDuplicate refuses two rows published at one logical address.
var ErrDuplicate = errors.New("corpus probe: duplicate row address")

// ErrDigest refuses an envelope whose content does not match its digest.
var ErrDigest = errors.New("corpus probe: digest mismatch")

// The two engine sides one fixture is answered on.
const (
	// SideOld is the engine the oracle answers with today.
	SideOld = "old"
	// SideNew is the relational engine under construction.
	SideNew = "new"
)

// Status is how one side ended for one fixture. It is a closed vocabulary:
// a side that produced no rows says why, and the driver never has to infer
// the difference between "answered nothing" and "could not be asked".
type Status string

const (
	// StatusSolved is a side that solved the fixture and published rows.
	StatusSolved Status = "solved"
	// StatusRefused is a side that was asked and declined to answer. Two
	// sides refusing identically are at parity: a refusal is an answer.
	StatusRefused Status = "refused"
	// StatusUncompiled is a fixture that did not reach either engine,
	// because it did not compile. Both sides carry it together: neither
	// engine was asked, so the two cannot have disagreed, and the fixture is
	// counted as unreached rather than as agreement.
	StatusUncompiled Status = "uncompiled"
	// StatusUnconstructed is the new engine's side when the production
	// constructor that would carry this fixture into it does not yet exist.
	// It is a recorded divergence class, never a skip.
	StatusUnconstructed Status = "constructor-unavailable"
	// StatusError is a side that failed while answering.
	StatusError Status = "error"
)

// Row is one published answer, addressed the way the catalogue is read: which
// family published it and which query site it answers.
//
// The remaining columns are independent published faces of that one answer.
// They are strings at this process boundary by design: the observation process
// owns their typed meaning, and the driver compares them without ever
// reconstructing a domain value it would then be answering for.
type Row struct {
	Family     string
	Site       string
	Value      string
	Outcome    string
	Diagnostic string
	Lineage    string
}

func (row Row) valid() bool {
	if row.Family == "" || row.Site == "" {
		return false
	}
	for _, column := range row.columns() {
		if strings.ContainsAny(column, "\x00\r\n") {
			return false
		}
	}
	return true
}

func (row Row) columns() [6]string {
	return [6]string{row.Family, row.Site, row.Value, row.Outcome, row.Diagnostic, row.Lineage}
}

// Address is a row's logical identity: the family and the site it answers.
func (row Row) Address() string { return row.Family + "\x00" + row.Site }

func compareRow(left, right Row) int {
	leftColumns, rightColumns := left.columns(), right.columns()
	for index := range leftColumns {
		if leftColumns[index] < rightColumns[index] {
			return -1
		}
		if leftColumns[index] > rightColumns[index] {
			return 1
		}
	}
	return 0
}

// CanonicalRows validates and copies rows into their one deterministic order.
//
// One address may not be published twice, even under different values. That
// keeps row ordinals from becoming a hidden authority: the comparison
// addresses a row by what it answers, never by where it happened to land in a
// scan.
func CanonicalRows(rows []Row) ([]Row, error) {
	ordered := append([]Row(nil), rows...)
	seen := make(map[string]struct{}, len(ordered))
	for _, row := range ordered {
		if !row.valid() {
			return nil, fmt.Errorf("%w: row has an empty address or a control byte", ErrMalformed)
		}
		if _, held := seen[row.Address()]; held {
			return nil, fmt.Errorf("%w: %s/%s", ErrDuplicate, row.Family, row.Site)
		}
		seen[row.Address()] = struct{}{}
	}
	sort.Slice(ordered, func(left, right int) bool { return compareRow(ordered[left], ordered[right]) < 0 })
	return ordered, nil
}

// Answer is one side's whole outcome for one fixture: how it ended, why, and
// the rows it published when it solved.
type Answer struct {
	Side   string
	Status Status
	Detail string
	Rows   []Row
}

// Envelope is one observation process's complete answer: one fixture, both
// sides, sealed under one digest.
type Envelope struct {
	Fixture string
	Answers []Answer
	Digest  string
}

// Seal validates one envelope and computes its digest. Exactly the two named
// sides may appear, once each, in old-then-new order, so a comparison can
// never be handed one side twice or the sides transposed.
func Seal(fixture string, answers []Answer) (Envelope, error) {
	if !validLabel(fixture) {
		return Envelope{}, fmt.Errorf("%w: fixture label %q", ErrMalformed, fixture)
	}
	if len(answers) != 2 || answers[0].Side != SideOld || answers[1].Side != SideNew {
		return Envelope{}, fmt.Errorf("%w: envelope must carry %s then %s", ErrMalformed, SideOld, SideNew)
	}
	sealed := make([]Answer, 0, len(answers))
	for _, answer := range answers {
		if !validStatus(answer.Status) {
			return Envelope{}, fmt.Errorf("%w: status %q", ErrMalformed, answer.Status)
		}
		if answer.Status == StatusSolved {
			rows, err := CanonicalRows(answer.Rows)
			if err != nil {
				return Envelope{}, err
			}
			answer.Rows = rows
		} else {
			if len(answer.Rows) != 0 {
				return Envelope{}, fmt.Errorf("%w: %s published rows under status %s", ErrMalformed, answer.Side, answer.Status)
			}
			answer.Rows = nil
		}
		sealed = append(sealed, answer)
	}
	envelope := Envelope{Fixture: fixture, Answers: sealed}
	envelope.Digest = digest(envelope)
	return envelope, nil
}

// Side returns the named side's answer.
func (envelope Envelope) Side(name string) (Answer, bool) {
	for _, answer := range envelope.Answers {
		if answer.Side == name {
			return answer, true
		}
	}
	return Answer{}, false
}

func validStatus(status Status) bool {
	switch status {
	case StatusSolved, StatusRefused, StatusUncompiled, StatusUnconstructed, StatusError:
		return true
	}
	return false
}

func validLabel(value string) bool {
	return value != "" && !strings.ContainsAny(value, "\x00\r\n= ")
}

func digest(envelope Envelope) string {
	hash := sha256.New()
	writeField(hash, Protocol)
	writeField(hash, envelope.Fixture)
	for _, answer := range envelope.Answers {
		writeField(hash, answer.Side)
		writeField(hash, string(answer.Status))
		writeField(hash, answer.Detail)
		writeField(hash, strconv.Itoa(len(answer.Rows)))
		for _, row := range answer.Rows {
			for _, column := range row.columns() {
				writeField(hash, column)
			}
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func writeField(writer io.Writer, value string) {
	// Length-prefixing prevents concatenation ambiguity while keeping the
	// wire itself human-addressable.
	_, _ = io.WriteString(writer, strconv.Itoa(len(value)))
	_, _ = io.WriteString(writer, ":")
	_, _ = io.WriteString(writer, value)
	_, _ = io.WriteString(writer, "\n")
}

func encode(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }

func decode(value string) (string, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return string(decoded), err == nil && encode(string(decoded)) == value
}

var (
	answerFields = [...]string{"side", "status", "detail"}
	rowFields    = [...]string{"family", "site", "value", "outcome", "diagnostic", "lineage"}
)

// MarshalText writes the envelope as the lines the driver reads back.
func (envelope Envelope) MarshalText() ([]byte, error) {
	resealed, err := Seal(envelope.Fixture, envelope.Answers)
	if err != nil {
		return nil, err
	}
	if resealed.Digest != envelope.Digest {
		return nil, fmt.Errorf("%w: unsealed envelope", ErrDigest)
	}
	var builder strings.Builder
	fmt.Fprintf(&builder, "corpus.Protocol=%s\n", Protocol)
	fmt.Fprintf(&builder, "corpus.Fixture=%s\n", encode(envelope.Fixture))
	fmt.Fprintf(&builder, "corpus.SideCount=%d\n", len(envelope.Answers))
	for index, answer := range envelope.Answers {
		prefix := fmt.Sprintf("corpus.SideAt(%d).", index)
		values := [...]string{answer.Side, string(answer.Status), answer.Detail}
		for field, value := range values {
			fmt.Fprintf(&builder, "%s%s=%s\n", prefix, answerFields[field], encode(value))
		}
		fmt.Fprintf(&builder, "%sRowCount=%d\n", prefix, len(answer.Rows))
		for ordinal, row := range answer.Rows {
			columns := row.columns()
			for field, column := range columns {
				fmt.Fprintf(&builder, "%sRowAt(%d).%s=%s\n", prefix, ordinal, rowFields[field], encode(column))
			}
		}
	}
	fmt.Fprintf(&builder, "corpus.Digest=%s\n", envelope.Digest)
	return []byte(builder.String()), nil
}

// Write writes one complete envelope. The bytes are assembled before the
// writer is touched, so a refusal never leaves a partial answer that would
// read to the driver as a valid observation.
func (envelope Envelope) Write(writer io.Writer) error {
	if writer == nil {
		return fmt.Errorf("%w: nil writer", ErrMalformed)
	}
	text, err := envelope.MarshalText()
	if err != nil {
		return err
	}
	_, err = writer.Write(text)
	return err
}

// ParseText opens one envelope. Unknown fields, duplicated fields, sides out
// of order, non-canonical rows and a wrong digest all refuse; there is no
// defaulting path.
func ParseText(text string) (Envelope, error) {
	if text == "" {
		return Envelope{}, fmt.Errorf("%w: empty answer", ErrMalformed)
	}
	var (
		protocol   string
		fixture    string
		sideCount  = -1
		wireDigest string
		fields     = map[string]string{}
		rowCounts  = map[int]int{}
	)
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		at := strings.IndexByte(line, '=')
		if at <= 0 {
			return Envelope{}, fmt.Errorf("%w: line %q", ErrMalformed, line)
		}
		key, value := line[:at], line[at+1:]
		switch {
		case key == "corpus.Protocol":
			if protocol != "" {
				return Envelope{}, fmt.Errorf("%w: duplicate protocol", ErrMalformed)
			}
			protocol = value
		case key == "corpus.Fixture":
			if fixture != "" {
				return Envelope{}, fmt.Errorf("%w: duplicate fixture", ErrMalformed)
			}
			decoded, ok := decode(value)
			if !ok || !validLabel(decoded) {
				return Envelope{}, fmt.Errorf("%w: fixture", ErrMalformed)
			}
			fixture = decoded
		case key == "corpus.SideCount":
			if sideCount >= 0 {
				return Envelope{}, fmt.Errorf("%w: duplicate side count", ErrMalformed)
			}
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed != 2 {
				return Envelope{}, fmt.Errorf("%w: side count %q", ErrMalformed, value)
			}
			sideCount = parsed
		case key == "corpus.Digest":
			if wireDigest != "" {
				return Envelope{}, fmt.Errorf("%w: duplicate digest", ErrMalformed)
			}
			if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
				return Envelope{}, fmt.Errorf("%w: digest encoding", ErrMalformed)
			}
			if _, err := hex.DecodeString(value); err != nil {
				return Envelope{}, fmt.Errorf("%w: digest encoding", ErrMalformed)
			}
			wireDigest = value
		case strings.HasPrefix(key, "corpus.SideAt("):
			if err := readSideField(key, value, fields, rowCounts); err != nil {
				return Envelope{}, err
			}
		default:
			return Envelope{}, fmt.Errorf("%w: unknown field %s", ErrMalformed, key)
		}
	}
	if protocol != Protocol || fixture == "" || sideCount != 2 || wireDigest == "" {
		return Envelope{}, fmt.Errorf("%w: incomplete header", ErrMalformed)
	}

	answers := make([]Answer, sideCount)
	for index := range answers {
		values := make([]string, len(answerFields))
		for field, name := range answerFields {
			value, ok := fields[fmt.Sprintf("%d.%s", index, name)]
			if !ok {
				return Envelope{}, fmt.Errorf("%w: missing side %d field %s", ErrMalformed, index, name)
			}
			values[field] = value
		}
		count, ok := rowCounts[index]
		if !ok {
			return Envelope{}, fmt.Errorf("%w: side %d states no row count", ErrMalformed, index)
		}
		rows := make([]Row, count)
		for ordinal := range rows {
			columns := make([]string, len(rowFields))
			for field, name := range rowFields {
				column, ok := fields[fmt.Sprintf("%d.%d.%s", index, ordinal, name)]
				if !ok {
					return Envelope{}, fmt.Errorf("%w: missing side %d row %d field %s", ErrMalformed, index, ordinal, name)
				}
				columns[field] = column
			}
			rows[ordinal] = Row{
				Family: columns[0], Site: columns[1], Value: columns[2],
				Outcome: columns[3], Diagnostic: columns[4], Lineage: columns[5],
			}
		}
		canonical, err := CanonicalRows(rows)
		if err != nil {
			return Envelope{}, err
		}
		for ordinal := range rows {
			if rows[ordinal] != canonical[ordinal] {
				return Envelope{}, fmt.Errorf("%w: side %d rows are not canonical", ErrMalformed, index)
			}
		}
		answers[index] = Answer{Side: values[0], Status: Status(values[1]), Detail: values[2], Rows: rows}
	}

	sealed, err := Seal(fixture, answers)
	if err != nil {
		return Envelope{}, err
	}
	if sealed.Digest != wireDigest {
		return Envelope{}, fmt.Errorf("%w: got=%s want=%s", ErrDigest, wireDigest, sealed.Digest)
	}
	return sealed, nil
}

// readSideField admits one side-addressed line into the flat field table.
func readSideField(key, value string, fields map[string]string, rowCounts map[int]int) error {
	const prefix = "corpus.SideAt("
	rest := key[len(prefix):]
	close := strings.Index(rest, ").")
	if close <= 0 {
		return fmt.Errorf("%w: side key %s", ErrMalformed, key)
	}
	side, err := strconv.Atoi(rest[:close])
	if err != nil || side < 0 || side >= 2 {
		return fmt.Errorf("%w: side index in %s", ErrMalformed, key)
	}
	tail := rest[close+2:]

	if tail == "RowCount" {
		count, err := strconv.Atoi(value)
		if err != nil || count < 0 {
			return fmt.Errorf("%w: row count %q", ErrMalformed, value)
		}
		if _, held := rowCounts[side]; held {
			return fmt.Errorf("%w: duplicate field %s", ErrMalformed, key)
		}
		rowCounts[side] = count
		return nil
	}

	address := ""
	if strings.HasPrefix(tail, "RowAt(") {
		inner := tail[len("RowAt("):]
		end := strings.Index(inner, ").")
		if end <= 0 {
			return fmt.Errorf("%w: row key %s", ErrMalformed, key)
		}
		ordinal, err := strconv.Atoi(inner[:end])
		if err != nil || ordinal < 0 {
			return fmt.Errorf("%w: row index in %s", ErrMalformed, key)
		}
		field := inner[end+2:]
		if !known(field, rowFields[:]) {
			return fmt.Errorf("%w: unknown row field %s", ErrMalformed, key)
		}
		address = fmt.Sprintf("%d.%d.%s", side, ordinal, field)
	} else {
		if !known(tail, answerFields[:]) {
			return fmt.Errorf("%w: unknown side field %s", ErrMalformed, key)
		}
		address = fmt.Sprintf("%d.%s", side, tail)
	}

	decoded, ok := decode(value)
	if !ok {
		return fmt.Errorf("%w: value of %s", ErrMalformed, key)
	}
	if _, held := fields[address]; held {
		return fmt.Errorf("%w: duplicate field %s", ErrMalformed, key)
	}
	fields[address] = decoded
	return nil
}

func known(name string, vocabulary []string) bool {
	for _, candidate := range vocabulary {
		if name == candidate {
			return true
		}
	}
	return false
}
