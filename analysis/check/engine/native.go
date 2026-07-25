package engine

import (
	"encoding/base64"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
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
	// encoding is carried in the value itself, or in the artifact's explicit
	// non-nil-assertion lineage. Every later copy of either stays claimed:
	// trust never rises by propagation.
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
	compilation *front.Compilation
	values      []equation.Fact
	outcomes    []equation.Fact
	diagnostics []equation.Fact
	derived     []NativeFact
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
	// Revoked. Contract validity can name a comma-separated set of deopt event
	// classes when no one concrete source operation supplies the interval.
	Event string
	// Revocations records every independently published deopt class for a
	// contract row. Ordinary epoch-gated rows retain the single interval above;
	// a contract can have several possible invalidators without duplicating the
	// fact it invalidates.
	Revocations []NativeRevocation
}

// NativeRevocation is one validity interval or contract deopt class attached
// to a native fact. Contract rows use Established="contract" and name their
// event explicitly; no observed source operation is fabricated for them.
type NativeRevocation struct {
	Established string
	Revoked     string
	Event       string
}

// HasRevocation reports whether this fact publishes the requested deopt event.
func (fact NativeFact) HasRevocation(event string) bool {
	if fact.Event == event {
		return true
	}
	for _, revocation := range fact.Revocations {
		if revocation.Event == event {
			return true
		}
	}
	return false
}

// NativeValuePrefixBase64 marks a value rendering that carries the fact's raw
// bytes rather than its text.
const NativeValuePrefixBase64 = "base64:"

func publishedNativeFacts(artifact equation.Artifact, values, outcomes, diagnostics []equation.Fact) *NativeFactIndex {
	return &NativeFactIndex{artifact: artifact, values: values, outcomes: outcomes, diagnostics: diagnostics}
}

// publishedNativeFactsForCompilation adds native rows which are a direct
// projection of the resolved WIR carried by every admitted lexical body.  It
// deliberately consumes no source spelling and no inferred fallback: absent
// operand representation leaves the row absent.
func publishedNativeFactsForCompilation(compilation front.Compilation, values, outcomes, diagnostics []equation.Fact) *NativeFactIndex {
	index := publishedNativeFacts(compilation.Artifact, values, outcomes, diagnostics)
	index.compilation = &compilation
	index.derived = append(numericNativeFacts(compilation), tableNativeFacts(compilation)...)
	index.derived = append(index.derived, nilabilityNativeFacts(compilation)...)
	index.derived = append(index.derived, aliasNativeFacts(compilation)...)
	index.derived = append(index.derived, branchNativeFacts(compilation, closedBranchCoordinates(values))...)
	index.derived = append(index.derived, frozenBodyNativeFacts(compilation)...)
	index.derived = append(index.derived, metatableNativeFacts(compilation)...)
	index.derived = append(index.derived, summaryNativeFacts(compilation)...)
	return index
}

// closedBranchCoordinates names every branch the value closure already
// partitioned. The WIR projection defers to those coordinates so one branch
// never carries two verdicts.
func closedBranchCoordinates(values []equation.Fact) map[string]bool {
	closed := make(map[string]bool)
	for _, fact := range values {
		body, occurrence, _, ok := nativeBranchProof(fact.Key, "branch-proof/")
		if ok {
			closed[body+"/"+occurrence] = true
		}
	}
	return closed
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
	total := len(index.values) + len(index.outcomes) + len(index.diagnostics) + len(index.derived)
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
	facts = append(facts, index.derived...)
	if index.compilation != nil {
		facts = append(facts, structuralNativeFacts(*index.compilation, facts)...)
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
	facts = coalesceNativeContractRevocations(facts)
	anchors.bindValidity(facts)
	// Contract rows are deliberately a publication projection, not another
	// analysis. The substrate rows above remain available verbatim; the rows
	// below only give already-closed evidence its native-contract vocabulary.
	facts = append(facts, projectNativeContracts(facts)...)
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

// A contract with several invalidators is one grant with a set of deopt
// points, not several grants. Normalize the value-closure representation
// before ordinary epoch binding so the comma-separated transport spelling is
// never mistaken for one synthetic deopt event.
func coalesceNativeContractRevocations(facts []NativeFact) []NativeFact {
	const marker = "/contract-revocation/"
	out := make([]NativeFact, 0, len(facts))
	for _, fact := range facts {
		index := strings.LastIndex(fact.Key, marker)
		if index < 0 {
			out = append(out, fact)
			continue
		}
		events := strings.Split(fact.Key[index+len(marker):], ",")
		if len(events) == 0 {
			out = append(out, fact)
			continue
		}
		row := fact
		row.Key, row.Established, row.Revoked, row.Event = fact.Key[:index], "contract", "", ""
		row.Revocations = make([]NativeRevocation, 0, len(events))
		valid := true
		for _, event := range events {
			if event == "" || strings.Contains(event, "/") {
				valid = false
				break
			}
			row.Revocations = append(row.Revocations, NativeRevocation{
				Established: "contract", Revoked: "contract/" + event, Event: event,
			})
		}
		if !valid {
			out = append(out, fact)
			continue
		}
		out = append(out, row)
	}
	return out
}

// projectNativeContracts translates only contract vocabulary already witnessed
// by the closed native fact set. It is one-way and fail-closed: a row appears
// only when every fact it names was published by this evaluation.
func projectNativeContracts(facts []NativeFact) []NativeFact {
	const (
		tableIdentityPrefix  = "heap/table-identity/"
		tableClosedPrefix    = "heap/table-closed/"
		memberPrefix         = "heap/member/"
		memberIdentityPrefix = "heap/member-identity/"
		metaAttachedPrefix   = "heap/meta-attached/"
		indexPresencePrefix  = "heap/index-presence/"
		freezePrefix         = "effect.freeze/"
		branchProofPrefix    = "branch-proof/"
	)
	type identityAnchor struct{ term, subject string }

	identityAnchors := make(map[string]identityAnchor)
	closed := make(map[string]NativeFact)
	attached := make(map[string]bool)
	initialMembers := make(map[string]map[string]bool)
	laterMembers := make(map[string]bool)
	children := make(map[string]map[string]bool)
	frozenTerms := make(map[string]bool)

	for _, fact := range facts {
		if fact.Lane != NativeLaneValues {
			continue
		}
		switch {
		case strings.HasPrefix(fact.Key, tableIdentityPrefix):
			if fact.Term != "" && fact.Value != "" {
				identityAnchors[fact.Value] = identityAnchor{term: fact.Term, subject: fact.Subject}
			}
		case strings.HasPrefix(fact.Key, tableClosedPrefix) && fact.Value == "closed":
			identity, occurrence, ok := nativeIdentityOccurrence(fact.Key, tableClosedPrefix)
			if ok {
				// Aliases can publish the same identity. Its earliest close names
				// the allocation's complete initial key set.
				if current, found := closed[identity]; !found || occurrence < current.Occurrence {
					closed[identity] = fact
				}
			}
		case strings.HasPrefix(fact.Key, metaAttachedPrefix) && fact.Value == "attached":
			identity, _, ok := nativeIdentityOccurrence(fact.Key, metaAttachedPrefix)
			if ok {
				attached[identity] = true
			}
		case strings.HasPrefix(fact.Key, memberPrefix):
			identity, member, occurrence, ok := nativeMemberOccurrence(fact.Key, memberPrefix)
			if !ok {
				continue
			}
			if close, found := closed[identity]; found && occurrence == close.Occurrence {
				initialMembers[identity] = nativeSetAdd(initialMembers[identity], member)
			}
		case strings.HasPrefix(fact.Key, memberIdentityPrefix):
			parent, _, _, ok := nativeMemberOccurrence(fact.Key, memberIdentityPrefix)
			if ok && fact.Value != "" {
				children[parent] = nativeSetAdd(children[parent], base64.RawURLEncoding.EncodeToString([]byte(fact.Value)))
			}
		case strings.HasPrefix(fact.Key, freezePrefix):
			term, _, ok := nativeFreezeTerm(fact.Key, freezePrefix)
			if ok {
				frozenTerms[term] = true
			}
		}
	}

	// A member first published after the close changes the key set. Re-reads of
	// a member that was already present do not invalidate the closed allocation.
	for _, fact := range facts {
		if fact.Lane != NativeLaneValues || !strings.HasPrefix(fact.Key, memberPrefix) {
			continue
		}
		identity, member, occurrence, ok := nativeMemberOccurrence(fact.Key, memberPrefix)
		if !ok {
			continue
		}
		close, found := closed[identity]
		if !found || occurrence <= close.Occurrence {
			continue
		}
		if !initialMembers[identity][member] {
			laterMembers[identity] = true
		}
	}

	rows := make([]NativeFact, 0)
	sealed := make(map[string]NativeFact)
	for identity, source := range closed {
		if attached[identity] || laterMembers[identity] {
			continue
		}
		anchor, anchored := identityAnchors[nativeDecodedIdentity(identity)]
		if !anchored {
			continue
		}
		row := NativeFact{
			Lane: NativeLaneValues, Family: "sealed_table",
			Key:   "sealed_table/" + identity + "/" + source.Occurrence,
			Value: "closed=true key_set=complete sealed=true",
			Term:  anchor.term, Subject: anchor.subject, Occurrence: source.Occurrence,
			Trust: NativeTrustProven,
		}
		sealed[identity] = row
		rows = append(rows, row)
	}

	// The freeze fact is exact to its root term; ownership rows carry that fact
	// only through the published graph to reachable already-sealed tables.
	frozen := make(map[string]bool)
	for identity, anchor := range identityAnchors {
		if frozenTerms[anchor.term] {
			markFrozenIdentity(base64.RawURLEncoding.EncodeToString([]byte(identity)), children, frozen)
		}
	}
	for identity := range frozen {
		row, found := sealed[identity]
		if !found {
			continue
		}
		row.Key = "sealed_table/" + identity + "/frozen/" + row.Occurrence
		row.Value = "closed=true depth=deep frozen=true sealed=true key_set=complete"
		rows = append(rows, row)
	}

	for _, fact := range facts {
		if fact.Lane != NativeLaneValues || !strings.HasPrefix(fact.Key, indexPresencePrefix) || fact.Value != "proven" {
			continue
		}
		identity, index, occurrence, ok := nativeIndexOccurrence(fact.Key, indexPresencePrefix)
		if !ok {
			continue
		}
		anchor, anchored := identityAnchors[nativeDecodedIdentity(identity)]
		if !anchored {
			continue
		}
		rows = append(rows, NativeFact{
			Lane: NativeLaneValues, Family: "table_element",
			Key:   "table_element/" + identity + "/" + index + "/" + occurrence,
			Value: "presence=proven result_nilability=non_nil",
			Term:  anchor.term, Subject: anchor.subject, Occurrence: occurrence,
			Trust: NativeTrustProven,
		})
	}
	for _, fact := range facts {
		if fact.Lane != NativeLaneValues || !strings.HasPrefix(fact.Key, branchProofPrefix) || fact.Value != "proven" {
			continue
		}
		body, occurrence, edge, ok := nativeBranchProof(fact.Key, branchProofPrefix)
		if !ok {
			continue
		}
		value := "partition=always_not_taken dead_arm=then dead_arm_reachable=false"
		if edge == "true" {
			value = "partition=always_taken dead_arm=else dead_arm_reachable=false"
		}
		rows = append(rows, NativeFact{
			Lane: NativeLaneValues, Family: "branch_partition",
			Key:   "branch_partition/" + body + "/" + occurrence,
			Value: value, Occurrence: occurrence, Trust: NativeTrustProven,
		})
	}
	return rows
}

func nativeSetAdd(set map[string]bool, value string) map[string]bool {
	if set == nil {
		set = make(map[string]bool)
	}
	set[value] = true
	return set
}

func markFrozenIdentity(identity string, children map[string]map[string]bool, frozen map[string]bool) {
	if frozen[identity] {
		return
	}
	frozen[identity] = true
	for child := range children[identity] {
		markFrozenIdentity(child, children, frozen)
	}
}

func nativeIdentityOccurrence(key, prefix string) (identity, occurrence string, ok bool) {
	rest, found := strings.CutPrefix(key, prefix)
	if !found {
		return "", "", false
	}
	identity, occurrence, found = strings.Cut(rest, "/")
	return identity, occurrence, found && identity != "" && occurrence != ""
}

func nativeMemberOccurrence(key, prefix string) (identity, member, occurrence string, ok bool) {
	rest, found := strings.CutPrefix(key, prefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func nativeIndexOccurrence(key, prefix string) (identity, index, occurrence string, ok bool) {
	return nativeMemberOccurrence(key, prefix)
}

func nativeFreezeTerm(key, prefix string) (term, occurrence string, ok bool) {
	rest, found := strings.CutPrefix(key, prefix)
	if !found {
		return "", "", false
	}
	term, occurrence, found = strings.Cut(rest, "/")
	if !found || term == "" || occurrence == "" {
		return "", "", false
	}
	return "path/" + term, occurrence, true
}

func nativeBranchProof(key, prefix string) (body, occurrence, edge string, ok bool) {
	rest, found := strings.CutPrefix(key, prefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || (parts[2] != "true" && parts[2] != "false") {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func nativeDecodedIdentity(encoded string) string {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return ""
	}
	return string(decoded)
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
		// Native entry contracts carry a deopt *class*, rather than pretending
		// that this compilation happened to execute a mutation at a particular
		// coordinate.  The descriptor is emitted from binder-owned topology and
		// reaches this projection through the ordinary value closure.  It is a
		// semantic validity interval: consumers may install a guard for exactly
		// this class, but cannot mistake it for an observed program event.
		if event, ok := nativeContractRevocation(fact.Key); ok {
			fact.Established, fact.Revoked, fact.Event = "contract", "contract/"+event, event
			continue
		}
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

func nativeContractRevocation(key string) (string, bool) {
	const marker = "/contract-revocation/"
	index := strings.LastIndex(key, marker)
	if index < 0 {
		return "", false
	}
	event := key[index+len(marker):]
	return event, event != "" && !strings.Contains(event, "/")
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
	claimed    map[string]struct{}
	longest    int
}

func newNativeAnchors(artifact equation.Artifact) *nativeAnchors {
	anchors := &nativeAnchors{terms: make(map[string]string), operations: make(map[string]string), claimed: claimedAssertionTerms(artifact)}
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
	if lane == NativeLaneValues {
		if _, claimed := a.claimed[row.Term]; claimed {
			row.Trust = NativeTrustClaimed
		}
	}
	return row
}

// claimedAssertionTerms follows only declared value flow: a non-nil assertion
// is source authority, and an environment write copies that authority from its
// recorded value term to its recorded target term. This is an artifact
// projection, not an inferred proof: an assertion stays claimed until a
// validating operation publishes a separate fact.
func claimedAssertionTerms(artifact equation.Artifact) map[string]struct{} {
	claimed := make(map[string]struct{})
	for changed := true; changed; {
		changed = false
		for _, operation := range artifact.Equations {
			operands, err := artifactOperandsByRole(operation.Operands, "target")
			if err != nil {
				continue
			}
			target := string(operands["target"])
			if operation.Occurrence.Kind == "claim" {
				kind, err := artifactOperandsByRole(operation.Operands, "kind")
				if err == nil && string(kind["kind"]) == "claim-kind/2" {
					if _, found := claimed[target]; !found {
						claimed[target] = struct{}{}
						changed = true
					}
				}
				continue
			}
			if operation.Occurrence.Kind != "environment-write" {
				continue
			}
			value, err := artifactOperandsByRole(operation.Operands, "value")
			if err != nil {
				continue
			}
			if _, inherited := claimed[string(value["value"])]; inherited {
				if _, found := claimed[target]; !found {
					claimed[target] = struct{}{}
					changed = true
				}
			}
		}
	}
	return claimed
}

// nativeFactTrust classifies a published value by the same predicates the
// closure itself uses to decide whether a value is authority. It reads the
// published encoding and nothing else; explicit non-nil assertion lineage is
// joined separately from the artifact coordinates that publish each row.
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
