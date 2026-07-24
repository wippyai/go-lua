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

// Proof provenance of one published row, in the vocabulary the diagnostic
// evidence layer already uses. It answers the only question a speculative code
// generator may not get wrong: whether the row is a conclusion the checker
// derived, or a statement the source asserted and the checker could not
// discharge. A guard may be elided for the first and never for the second.
const (
	// NativeTrustProven is a conclusion the closure derived. A user claim that
	// the closure discharged against a value it had already derived publishes
	// that derived value and is proven; the claim added no authority.
	NativeTrustProven = "proven"
	// NativeTrustClaimed is an undischarged source assertion — a cast, a
	// declared type or a non-nil assertion the closure could not prove. The
	// encoding is carried in the value itself, so every later read, copy and
	// merge of the value stays claimed: trust never rises by propagation.
	NativeTrustClaimed = "claimed"
	// NativeTrustUnknown is a row with no proof content: the closure's opaque
	// value, or an unvalidated gradual boundary.
	NativeTrustUnknown = "unknown"
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
	// Trust is the row's proof provenance: proven, claimed or unknown. It is
	// empty outside the value lane, whose rows are the only ones that carry a
	// value encoding.
	Trust string
	// Established is the epoch of Term that this row's validity begins at. It
	// is empty when the row is not epoch-gated — when its term has no published
	// epoch, or its key is not anchored at one of that term's epochs. An empty
	// Established means the closure published no validity interval for the row,
	// never that the row is valid everywhere.
	Established string
	// Revoked is the next epoch published for Term after Established: the exact
	// operation at which this row stops holding. It is empty when Established
	// is the term's last published epoch, which is the closure's statement that
	// nothing in the analysed body revokes the row.
	Revoked string
	// Event is the artifact's occurrence kind of the operation named by
	// Revoked: what kind of program event ends the row's validity.
	Event string
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
	anchors.bindValidity(facts)
	index.facts = facts
}

// bindValidity joins every row to the epoch interval the closure published for
// its term. The epochs are read from the same published rows; nothing is
// recomputed, so a term the closure never versioned yields no interval at all
// rather than an interval that happens to look unbounded.
func (a *nativeAnchors) bindValidity(facts []NativeFact) {
	chains := make(map[string][]string)
	for _, fact := range facts {
		if fact.Lane != NativeLaneValues || !strings.HasPrefix(fact.Key, epochFactPrefix) {
			continue
		}
		rest := fact.Key[len(epochFactPrefix):]
		cut := strings.LastIndexByte(rest, '/')
		if cut < 0 {
			continue
		}
		term, epoch := rest[:cut], rest[cut+1:]
		chain := chains[term]
		// The rows arrive in key order, so a term's epochs arrive in operation
		// order and a repeated key is adjacent to its twin.
		if len(chain) != 0 && chain[len(chain)-1] == epoch {
			continue
		}
		chains[term] = append(chain, epoch)
	}
	for index := range facts {
		fact := &facts[index]
		if fact.Lane != NativeLaneValues || fact.Term == "" {
			continue
		}
		chain, versioned := chains[fact.Term]
		if !versioned {
			continue
		}
		// An epoch-gated key is exactly "<prefix>/<term>/<epoch>": the same
		// spelling the closure publishes its derived facts at. A key that does
		// not end that way is anchored somewhere else and carries no interval.
		marker := "/" + fact.Term + "/"
		cut := strings.LastIndex(fact.Key, marker)
		if cut < 0 {
			continue
		}
		established := fact.Key[cut+len(marker):]
		if strings.Contains(established, "/") {
			continue
		}
		for position, epoch := range chain {
			if epoch != established {
				continue
			}
			fact.Established = established
			if position+1 < len(chain) {
				fact.Revoked = chain[position+1]
				fact.Event = a.operations[fact.Revoked]
			}
			break
		}
	}
}

// nativeAnchors recovers the term and coordinate vocabulary of one artifact.
// Both sets are exactly what the equations carry, so a fact key is anchored by
// matching published data rather than by a per-family key grammar.
// operations maps every equation coordinate name to the artifact's occurrence
// kind at that coordinate, which is the event vocabulary a revocation is named
// in.
type nativeAnchors struct {
	terms      map[string]string
	operations map[string]string
	longest    int
}

func newNativeAnchors(artifact equation.Artifact) *nativeAnchors {
	anchors := &nativeAnchors{terms: make(map[string]string), operations: make(map[string]string)}
	for _, operation := range artifact.Equations {
		anchors.operations[operation.Target.Name] = operation.Occurrence.Kind
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
	row := NativeFact{Lane: lane, Key: fact.Key, Value: nativeFactValue(fact.Value), Trust: nativeFactTrust(lane, fact.Value)}
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
		if _, coordinate := a.operations[fact.Key[starts[segment]:end(segment)]]; coordinate {
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

// nativeFactTrust classifies a published value by the same predicates the
// closure itself uses to decide whether a value is authority. It reads the
// published encoding and nothing else: a claim the closure refused to
// discharge is carried as a claim refinement inside the value, which is why an
// assignment, a merge or a later read of that value cannot launder it into a
// proof.
//
// Only the value lane carries value encodings. Outcome and diagnostic rows are
// display and report projections whose spellings this vocabulary does not
// classify, so they are left unclassified rather than defaulted to proven.
func nativeFactTrust(lane string, value []byte) string {
	switch {
	case lane != NativeLaneValues:
		return ""
	case isClaimRefinement(value):
		return NativeTrustClaimed
	case isUnknownScalar(value) || isUnvalidatedAnyValue(value):
		return NativeTrustUnknown
	default:
		return NativeTrustProven
	}
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
