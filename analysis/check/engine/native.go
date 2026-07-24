package engine

import (
	"encoding/base64"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

// Publication lanes of one engine evaluation. They are named after the output
// closure channels they project, so a consumer that reads a row always knows
// which channel closed it.
const (
	NativeLaneValues      = "values"
	NativeLaneOutcomes    = "outcomes"
	NativeLaneDiagnostics = "diagnostics"
)

// NativeFactIndex is the read-only projection of every fact closed by one
// engine evaluation, in the form a native code generator consumes: the
// published row, the family it belongs to, the equation coordinate it is
// anchored at, and the source binding it concerns.
//
// It is a projection of the same closure that produces PlacementPlan and it
// performs no second analysis: a conclusion the closure did not publish is
// absent here, never defaulted. The projection is built on first read so a
// checking run that never consumes native facts pays only one pointer.
type NativeFactIndex struct {
	artifact    equation.Artifact
	values      []equation.Fact
	outcomes    []equation.Fact
	diagnostics []equation.Fact
	once        sync.Once
	facts       []NativeFact
}

// NativeFact is one published fact row. Every field is recovered from the
// published key, the published value, or the artifact the closure was
// evaluated from; none of them is inferred from a key spelling.
type NativeFact struct {
	// Lane is the output closure channel that carried the fact.
	Lane string
	// Family is the first segment of the fact key: the published fact family.
	Family string
	// Key is the published fact key, verbatim.
	Key string
	// Value is the published fact value. A value that is not valid UTF-8 is
	// rendered as "base64:" followed by its canonical RawURL encoding, so a
	// row is always comparable as text without losing content.
	Value string
	// Term is the closed term the key anchors on, when the key carries one of
	// the artifact's operand terms. It is empty otherwise.
	Term string
	// Subject is the source display name published for Term. It is empty when
	// the closure published no display binding for that term.
	Subject string
	// Occurrence is the equation coordinate the fact is anchored at, when the
	// key carries one of the artifact's equation target names.
	Occurrence string
}

// NativeValuePrefixBase64 marks a value rendering that carries the fact's raw
// bytes rather than its text.
const NativeValuePrefixBase64 = "base64:"

func publishedNativeFacts(artifact equation.Artifact, values, outcomes, diagnostics []equation.Fact) *NativeFactIndex {
	return &NativeFactIndex{artifact: artifact, values: values, outcomes: outcomes, diagnostics: diagnostics}
}

// Facts returns every published row in a deterministic order: lane, then key,
// then value. Two evaluations of the same program yield the same slice.
func (index *NativeFactIndex) Facts() []NativeFact {
	if index == nil {
		return nil
	}
	index.once.Do(index.build)
	return index.facts
}

func (index *NativeFactIndex) build() {
	anchors := newNativeAnchors(index.artifact)
	total := len(index.values) + len(index.outcomes) + len(index.diagnostics)
	facts := make([]NativeFact, 0, total)
	for _, lane := range []struct {
		name  string
		facts []equation.Fact
	}{
		{NativeLaneValues, index.values},
		{NativeLaneOutcomes, index.outcomes},
		{NativeLaneDiagnostics, index.diagnostics},
	} {
		for _, fact := range lane.facts {
			facts = append(facts, anchors.project(lane.name, fact))
		}
	}
	sort.Slice(facts, func(i, j int) bool {
		if facts[i].Lane != facts[j].Lane {
			return facts[i].Lane < facts[j].Lane
		}
		if facts[i].Key != facts[j].Key {
			return facts[i].Key < facts[j].Key
		}
		return facts[i].Value < facts[j].Value
	})
	index.facts = facts
}

// nativeAnchors recovers the term and coordinate vocabulary of one artifact.
// Both sets are exactly what the equations carry, so a fact key is anchored by
// matching published data rather than by a per-family key grammar.
type nativeAnchors struct {
	terms      map[string]string
	operations map[string]bool
	longest    int
}

func newNativeAnchors(artifact equation.Artifact) *nativeAnchors {
	anchors := &nativeAnchors{terms: make(map[string]string), operations: make(map[string]bool)}
	for _, operation := range artifact.Equations {
		anchors.operations[operation.Target.Name] = true
		byRole := make(map[string][]byte, len(operation.Operands))
		for _, operand := range operation.Operands {
			if operand.Term.Entry || len(operand.Term.Encoding) == 0 {
				continue
			}
			byRole[operand.Role] = operand.Term.Encoding
			term := string(operand.Term.Encoding)
			if _, known := anchors.terms[term]; !known {
				anchors.terms[term] = ""
			}
			if segments := strings.Count(term, "/") + 1; segments > anchors.longest {
				anchors.longest = segments
			}
		}
		// An operand role carrying the "-display" infix holds the source
		// spelling of the operand at the same role without it. The pairing is
		// self-validating: a display role whose subject role is absent from the
		// same operation names nothing.
		for role, display := range byRole {
			if !strings.Contains(role, "-display") {
				continue
			}
			if subject, found := byRole[strings.Replace(role, "-display", "", 1)]; found {
				anchors.name(string(subject), string(display))
			}
		}
	}
	// A binding's own write display is its name, so it wins over a spelling
	// recovered from a use.
	artifactDisplayBindings(artifact, func(target, display []byte, _ equation.Coordinate) {
		anchors.terms[string(target)] = string(display)
	})
	return anchors
}

// name records a source spelling recovered from a use. Disagreeing spellings
// resolve to the smallest so the projection never depends on iteration order.
func (a *nativeAnchors) name(term, display string) {
	if display == "" {
		return
	}
	if current, known := a.terms[term]; !known || current == "" || display < current {
		a.terms[term] = display
	}
}

func (a *nativeAnchors) project(lane string, fact equation.Fact) NativeFact {
	row := NativeFact{Lane: lane, Key: fact.Key, Value: nativeFactValue(fact.Value)}
	// Segment boundaries stay as offsets into the key, so every candidate run
	// below is a substring of the published key rather than a rebuilt string.
	starts := make([]int, 1, 8)
	for index := 0; index < len(fact.Key); index++ {
		if fact.Key[index] == '/' {
			starts = append(starts, index+1)
		}
	}
	end := func(segment int) int {
		if segment+1 < len(starts) {
			return starts[segment+1] - 1
		}
		return len(fact.Key)
	}
	row.Family = fact.Key[:end(0)]
	for segment := len(starts) - 1; segment >= 0; segment-- {
		if a.operations[fact.Key[starts[segment]:end(segment)]] {
			row.Occurrence = fact.Key[starts[segment]:end(segment)]
			break
		}
	}
	// The longest segment-aligned run that the artifact published as a term is
	// the subject of the key. Longest wins so a term never loses to one of its
	// own prefixes; the leftmost of equal-length runs wins so the choice does
	// not depend on iteration order.
	best := 0
	for first := 0; first < len(starts); first++ {
		last := first + a.longest
		if last > len(starts) {
			last = len(starts)
		}
		for count := last - first; count > best; count-- {
			candidate := fact.Key[starts[first]:end(first+count-1)]
			if display, known := a.terms[candidate]; known {
				best, row.Term, row.Subject = count, candidate, display
				break
			}
		}
	}
	return row
}

func nativeFactValue(value []byte) string {
	if utf8.Valid(value) {
		return string(value)
	}
	return NativeValuePrefixBase64 + base64.RawURLEncoding.EncodeToString(value)
}

// artifactDisplayBindings yields every source display binding the artifact
// carries. publishedValues selects the dependency-latest value for each
// display; the native fact index uses the same bindings to name the source
// subject a published fact concerns. Hidden front terms are not source
// bindings and are excluded from both.
func artifactDisplayBindings(artifact equation.Artifact, visit func(target, display []byte, coordinate equation.Coordinate)) {
	for _, operation := range artifact.Equations {
		var target, display []byte
		switch operation.Occurrence.Kind {
		case "environment-write", "claim":
			operands, err := artifactOperandsByRole(operation.Operands, "target", "display")
			if err != nil {
				continue
			}
			target, display = operands["target"], operands["display"]
		case "expression":
			operands, err := artifactOperandsByRole(operation.Operands, "result", "display")
			if err != nil || !strings.HasPrefix(string(operands["result"]), "path/") {
				continue
			}
			target, display = operands["result"], operands["display"]
		default:
			continue
		}
		if strings.HasPrefix(string(display), "front/hidden/") {
			continue
		}
		visit(target, display, operation.Target)
	}
}
