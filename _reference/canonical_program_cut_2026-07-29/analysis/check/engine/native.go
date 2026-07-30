package engine

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
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

var nativeGuardedElementDeopts = []string{"write.element", "write.length", "write.local", "meta.set", "call.opaque"}

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
			if family, declared := factkey.Lookup(fact.Key); declared && family.ID == factkey.FamilyNativeProjection {
				if projection, valid := front.DecodeNativeProjection(fact.Value); valid {
					row := nativeFactFromProjection(projection)
					if admitStructuralNativeFact(row) {
						facts = append(facts, row)
					}
				}
				continue
			}
			if family, declared := factkey.Lookup(fact.Key); declared && family.ID == factkey.FamilyHeapAllocationDisplay {
				// Allocation displays are typed kernel input, not a native
				// contract family. Their consumer publishes the source-facing
				// alias row at the guarded allocation coordinate.
				continue
			}
			row := anchors.project(lane.name, fact)
			if admitStructuralNativeFact(row) {
				facts = append(facts, row)
			}
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
	facts = deduplicateNativeFacts(facts)
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

// admitStructuralNativeFact is the decision consumer for structural eval and
// throw publications. Their exact closed payload determines whether the row
// crosses the native boundary; malformed or widened spellings fail closed.
func admitStructuralNativeFact(row NativeFact) bool {
	switch row.Family {
	case "throw_template":
		return row.Value == claimAssertThrowTemplateValue
	case "eval_node":
		value, ok := strings.CutPrefix(row.Value, "operation=")
		return ok && projectedEvalNodeOperation(value)
	default:
		return true
	}
}

func nativeFactFromProjection(projection front.NativeProjection) NativeFact {
	family, _ := factkey.Head(projection.Key)
	row := NativeFact{
		Lane: NativeLaneValues, Family: family,
		Key: projection.Key, Value: projection.Value,
		Term: projection.Term, Subject: projection.Subject,
		Occurrence: projection.Occurrence, Trust: NativeTrustProven,
		Revocations: make([]NativeRevocation, 0, len(projection.Revocations)),
	}
	for _, revocation := range projection.Revocations {
		row.Revocations = append(row.Revocations, NativeRevocation{
			Established: revocation.Established,
			Revoked:     revocation.Revoked,
			Event:       revocation.Event,
		})
	}
	if len(row.Revocations) != 0 {
		row.Established = row.Revocations[0].Established
		row.Revoked = row.Revocations[0].Revoked
		row.Event = row.Revocations[0].Event
	}
	return row
}

func deduplicateNativeFacts(facts []NativeFact) []NativeFact {
	if len(facts) < 2 {
		return facts
	}
	out := facts[:1]
	for _, fact := range facts[1:] {
		previous := out[len(out)-1]
		if fact.Lane == previous.Lane && fact.Key == previous.Key && fact.Value == previous.Value {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func closedNativeKernelProjectionFacts(root front.Compilation, facts []equation.Fact) []equation.Fact {
	anchors := make(map[string]*nativeAnchors)
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		anchors[fmt.Sprintf("%x", compilation.BodyID())] = newNativeAnchors(compilation.DraftArtifact())
		for _, child := range compilation.NestedCompilations() {
			visit(child)
		}
	}
	visit(root)
	out := make([]equation.Fact, 0, len(facts))
	for index, fact := range facts {
		if family, declared := factkey.Lookup(fact.Key); declared && family.ID == factkey.FamilyNativeProjection {
			// Owning kernels already closed the typed projection payload.
			// Wrapping it again would expose native-projection as the semantic
			// family and hide the verdict encoded inside it.
			out = append(out, fact)
			continue
		}
		var owner *nativeAnchors
		for body, candidate := range anchors {
			if strings.Contains(fact.Key, "/"+body+"/") {
				owner = candidate
				break
			}
		}
		if owner == nil {
			owner = newNativeAnchors(root.DraftArtifact())
		}
		projection := nativeProjectionFromFact(owner.project(NativeLaneValues, fact))
		encoded, err := front.EncodeNativeProjection(projection)
		if err != nil {
			continue
		}
		key := factkey.BuildKey(
			factkey.NativeProjection,
			[]factkey.Part{
				factkey.OpaquePart(fmt.Sprintf("%x", root.BodyID())),
				factkey.OpaquePart(fmt.Sprintf("%08d", index)),
			},
			"published",
		)
		out = append(out, equation.Fact{Key: key.String(), Value: encoded, Guards: fact.Guards})
	}
	return out
}

func nativeProjectionFromFact(row NativeFact) front.NativeProjection {
	revocations := row.Revocations
	if len(revocations) == 0 && (row.Established != "" || row.Revoked != "" || row.Event != "") {
		revocations = []NativeRevocation{{
			Established: row.Established,
			Revoked:     row.Revoked,
			Event:       row.Event,
		}}
	}
	projection := front.NativeProjection{
		Key: row.Key, Value: row.Value, Term: row.Term, Subject: row.Subject,
		Occurrence:  row.Occurrence,
		Revocations: make([]front.NativeProjectionRevocation, 0, len(revocations)),
	}
	for _, revocation := range revocations {
		projection.Revocations = append(projection.Revocations, front.NativeProjectionRevocation{
			Established: revocation.Established,
			Revoked:     revocation.Revoked,
			Event:       revocation.Event,
		})
	}
	return projection
}

// A contract with several invalidators is one grant with a set of deopt
// points, not several grants. Normalize the value-closure representation
// before ordinary epoch binding so the comma-separated transport spelling is
// never mistaken for one synthetic deopt event.
func coalesceNativeContractRevocations(facts []NativeFact) []NativeFact {
	const marker = "/contract-revocation/"
	out := make([]NativeFact, 0, len(facts))
	for _, fact := range facts {
		key, events, found := fact.contractRevocations(marker)
		if !found {
			out = append(out, fact)
			continue
		}
		row := fact
		row.Key, row.Established, row.Revoked, row.Event = key, "contract", "", ""
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

func (fact NativeFact) contractRevocations(marker string) (string, []string, bool) {
	key := fact.Key
	index := strings.LastIndex(key, marker)
	if index < 0 {
		return "", nil, false
	}
	events := strings.Split(key[index+len(marker):], ",")
	return key[:index], events, len(events) != 0
}

// projectNativeContracts translates only contract vocabulary already witnessed
// by the closed native fact set. It is one-way and fail-closed: a row appears
// only when every fact it names was published by this evaluation.
func projectNativeContracts(facts []NativeFact) []NativeFact {
	freezePrefix := factkey.EffectFreeze.Key().String()
	type identityAnchor struct{ term, subject string }
	type memberRead struct{ identity, member, occurrence string }
	type sealedClaim struct {
		attributes []string
		row        NativeFact
	}

	identityAnchors := make(map[string]identityAnchor)
	identityTerms := make(map[string]map[string]bool)
	closed := make(map[string]NativeFact)
	attached := make(map[string]bool)
	attachReceivers := make(map[string]map[string]bool)
	callArguments := make(map[string][]string)
	var memberReads []memberRead
	initialMembers := make(map[string]map[string]bool)
	laterMembers := make(map[string]bool)
	children := make(map[string]map[string]bool)
	frozenTerms := make(map[string]bool)

	for _, fact := range facts {
		if fact.Lane != NativeLaneValues {
			continue
		}
		family, heapFact := factkey.Lookup(fact.Key)
		parsed, parsedHeapFact := family.ParseKey(fact.Key)
		switch {
		case heapFact && parsedHeapFact && family.ID == factkey.FamilyHeapTableIdentity:
			if fact.Term != "" && fact.Value != "" {
				identityAnchors[fact.Value] = identityAnchor{term: fact.Term, subject: fact.Subject}
				identityTerms[fact.Term] = nativeSetAdd(identityTerms[fact.Term], base64.RawURLEncoding.EncodeToString([]byte(fact.Value)))
			}
		case heapFact && parsedHeapFact && family.ID == factkey.FamilyHeapTableClosed && fact.Value == "closed":
			identity, occurrence := parsed.Subject.Encoded(), parsed.Occurrence
			if identity != "" {
				// Aliases can publish the same identity. Its earliest close names
				// the allocation's complete initial key set.
				if current, found := closed[identity]; !found || occurrence < current.Occurrence {
					closed[identity] = fact
				}
			}
		case heapFact && parsedHeapFact && family.ID == factkey.FamilyHeapMetaAttached && fact.Value == "attached":
			identity, occurrence := parsed.Subject.Encoded(), parsed.Occurrence
			if identity != "" {
				attached[identity] = true
				attachReceivers[occurrence] = nativeSetAdd(attachReceivers[occurrence], identity)
			}
		case heapFact && parsedHeapFact && family.ID == factkey.FamilyHeapMember:
			// Rows arrive in key order, so a member is read before the close of
			// its own allocation. The key set is resolved once every close is
			// known, below.
			member, ok := parsed.Qualifier(0)
			if ok {
				memberReads = append(memberReads, memberRead{
					identity: parsed.Subject.Encoded(), member: member.Encoded(), occurrence: parsed.Occurrence,
				})
			}
		case heapFact && parsedHeapFact && family.ID == factkey.FamilyHeapMemberIdentity:
			if fact.Value != "" {
				children[parsed.Subject.Encoded()] = nativeSetAdd(
					children[parsed.Subject.Encoded()], base64.RawURLEncoding.EncodeToString([]byte(fact.Value)),
				)
			}
		case heapFact && parsedHeapFact && family.ID == factkey.FamilyCallArgument:
			application, position := parsed.Subject.Spelling(), parsed.Occurrence
			if application != "" && position != "" && fact.Value != "" {
				callArguments[application] = append(callArguments[application], fact.Value)
			}
		case factkey.OwnsPrefix(freezePrefix, fact.Key):
			term, _, ok := nativeFreezeTerm(fact.Key, freezePrefix)
			if ok {
				frozenTerms[term] = true
			}
		}
	}

	// The members published at the close occurrence are the allocation's initial
	// key set. A member first published after the close changes that key set;
	// re-reads of a member that was already present do not.
	for _, read := range memberReads {
		if close, found := closed[read.identity]; found && read.occurrence == close.Occurrence {
			initialMembers[read.identity] = nativeSetAdd(initialMembers[read.identity], read.member)
		}
	}
	for _, read := range memberReads {
		close, found := closed[read.identity]
		if !found || read.occurrence <= close.Occurrence {
			continue
		}
		if !initialMembers[read.identity][read.member] {
			laterMembers[read.identity] = true
		}
	}

	// A metatable attach names its receiver and publishes the same call's
	// argument terms at the same coordinate, so the installed metatable is the
	// argument that is not the receiver. That table's key set is read through
	// `__index` dispatch, and so is the key set of every table reachable from it.
	// Dispatch ends at a mutation of the metatable itself, a class no attribute
	// of a sealed row establishes, so those allocations publish no seal at all.
	metatables := make(map[string]bool)
	for occurrence, receivers := range attachReceivers {
		for _, argument := range callArguments[occurrence] {
			for identity := range identityTerms[argument] {
				if !receivers[identity] {
					markNativeReachable(identity, children, metatables)
				}
			}
		}
	}

	rows := make([]NativeFact, 0)
	claims := make(map[string]sealedClaim)
	for identity, source := range closed {
		if metatables[identity] || laterMembers[identity] {
			continue
		}
		anchor, anchored := identityAnchors[nativeDecodedIdentity(identity)]
		if !anchored {
			continue
		}
		// An allocation that receives a metatable keeps the seal it holds before
		// the install: up to that point no metatable is attached. The complete
		// key set is not published for it, because an installed `__index` makes
		// an absent-key read observable.
		attributes := []string{"closed=true", "key_set=complete", "sealed=true"}
		if attached[identity] {
			attributes = []string{"sealed=true", "shape=pre_install"}
		}
		row := nativeSealedRow("sealed_table/"+identity+"/"+source.Occurrence, attributes,
			anchor.term, anchor.subject, source.Occurrence)
		claims[identity] = sealedClaim{attributes: attributes, row: row}
		rows = append(rows, row)
	}

	// The freeze fact is exact to its root term; ownership rows carry that fact
	// only through the published graph to reachable already-sealed tables.
	frozen := make(map[string]bool)
	for identity, anchor := range identityAnchors {
		if frozenTerms[anchor.term] {
			markNativeReachable(base64.RawURLEncoding.EncodeToString([]byte(identity)), children, frozen)
		}
	}
	for identity := range frozen {
		claim, found := claims[identity]
		if !found {
			continue
		}
		rows = append(rows, nativeSealedRow("sealed_table/"+identity+"/frozen/"+claim.row.Occurrence,
			append(append([]string(nil), claim.attributes...), "depth=deep", "frozen=true"),
			claim.row.Term, claim.row.Subject, claim.row.Occurrence))
	}

	// An index the value closure proved present publishes the same guarded-read
	// contract the WIR projection publishes for a declared array parameter, in
	// the same deopt vocabulary. The two substrates prove presence for disjoint
	// containers — one an allocation identity this evaluation closed, the other
	// an opaque-origin binding — so the family keeps one contract spelling.
	for _, fact := range facts {
		if fact.Lane != NativeLaneValues || factkey.DecodeTruthString(fact.Value) != factkey.TruthProven {
			continue
		}
		family, found := factkey.Lookup(fact.Key)
		parsed, ok := family.ParseKey(fact.Key)
		if !found || !ok || family.ID != factkey.FamilyHeapIndexPresence || !parsed.Subject.TaggedIdentity() {
			continue
		}
		indexRef, present := parsed.Qualifier(0)
		if !present {
			continue
		}
		identity, index, occurrence := parsed.Subject.Encoded(), indexRef.Encoded(), parsed.Occurrence
		anchor, anchored := identityAnchors[nativeDecodedIdentity(identity)]
		if !anchored {
			continue
		}
		rows = append(rows, NativeFact{
			Lane: NativeLaneValues, Family: "table_element",
			Key: factkey.NativeTableElement.Key().String() + identity + "/" + index + "/" + occurrence +
				"/contract-revocation/" + strings.Join(nativeGuardedElementDeopts, ","),
			Value: "presence=proven result_nilability=non_nil",
			Term:  anchor.term, Subject: anchor.subject, Occurrence: occurrence,
			Trust: NativeTrustProven,
		})
	}
	return coalesceNativeContractRevocations(rows)
}

// nativeSealedDeopts is the closed vocabulary of a sealed-table claim: every
// attribute a row may assert, and the deopt class that ends it. A published row
// carries exactly the classes of the attributes it asserts, so a consumer that
// acts on the row installs a guard for every way the row can stop holding.
var nativeSealedDeopts = []struct{ attribute, event string }{
	// A complete static key set ends at a field store.
	{"key_set=complete", "write.field"},
	// An absent metatable ends at a metatable install.
	{"sealed=true", "meta.set"},
	// A pre-install physical shape ends at the transition the install performs.
	{"shape=pre_install", "shape.transition"},
}

// nativeSealedRow publishes one sealed-table claim. The attributes are the whole
// content of the row, and they alone name the row's revocation set: the key
// carries the deopt classes in the transport spelling the contract coalescer
// splits, so a claim with several invalidators stays one row.
func nativeSealedRow(stem string, attributes []string, term, subject, occurrence string) NativeFact {
	sorted := append([]string(nil), attributes...)
	sort.Strings(sorted)
	key := stem
	if events := nativeSealedDeoptEvents(sorted); events != "" {
		key += "/contract-revocation/" + events
	}
	return NativeFact{
		Lane: NativeLaneValues, Family: "sealed_table",
		Key: key, Value: strings.Join(sorted, " "),
		Term: term, Subject: subject, Occurrence: occurrence,
		Trust: NativeTrustProven,
	}
}

func nativeSealedDeoptEvents(attributes []string) string {
	events := make([]string, 0, len(nativeSealedDeopts))
	for _, deopt := range nativeSealedDeopts {
		for _, attribute := range attributes {
			if attribute == deopt.attribute {
				events = append(events, deopt.event)
				break
			}
		}
	}
	return strings.Join(events, ",")
}

func nativeSetAdd(set map[string]bool, value string) map[string]bool {
	if set == nil {
		set = make(map[string]bool)
	}
	set[value] = true
	return set
}

// markNativeReachable closes a marked set over the published ownership graph:
// an identity carries its mark to every identity published as one of its member
// values.
func markNativeReachable(identity string, children map[string]map[string]bool, marked map[string]bool) {
	if marked[identity] {
		return
	}
	marked[identity] = true
	for child := range children[identity] {
		markNativeReachable(child, children, marked)
	}
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
		if fact.Lane != NativeLaneValues || !factkey.Epoch.Owns(fact.Key) {
			continue
		}
		rest := fact.Key[len(factkey.Epoch.Key().String()):]
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
			if operand.Term.Entry || len(operand.Term.Encoding) == 0 || operand.Role.InFamily(equation.RoleFamilyNative) {
				continue
			}
			byRole[operand.Role.Wire()] = operand.Term.Encoding
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
	projected := factkey.Project(fact.Key, a.terms, a.operations, a.longest)
	row.Family, row.Term, row.Occurrence = projected.Family, projected.Term, projected.Occurrence
	row.Subject = a.terms[row.Term]
	if family, declared := factkey.Lookup(fact.Key); declared && family.ID == factkey.FamilyNativeAliasDisjoint {
		if wire, valid := decodeNativeAliasDisjoint(fact.Value); valid {
			row.Value = wire.Content
			row.Subject = wire.Subject
			row.Revocations = make([]NativeRevocation, 0, len(wire.Events))
			for _, event := range wire.Events {
				row.Revocations = append(row.Revocations, NativeRevocation{
					Established: "contract", Revoked: "contract/" + event, Event: event,
				})
			}
			if len(row.Revocations) != 0 {
				row.Established = row.Revocations[0].Established
				row.Revoked = row.Revocations[0].Revoked
				row.Event = row.Revocations[0].Event
			}
		}
	}
	if family, declared := factkey.Lookup(fact.Key); declared && family.ID == factkey.FamilyNativeCaptureEpochRoot {
		if wire, valid := decodeNativeCaptureEpochRoot(fact.Value); valid {
			row.Value = wire.Content
			row.Term = ""
			row.Subject = wire.Subject
			row.Occurrence = wire.Occurrence
		}
	}
	if family, declared := factkey.Lookup(fact.Key); declared && family.ID == factkey.FamilyNativeCaptureTransport {
		if wire, valid := decodeNativeCaptureTransport(fact.Value); valid {
			row.Value = wire.Content
			row.Term = ""
			row.Subject = wire.Subject
			row.Occurrence = wire.Occurrence
			row.Established = wire.Established
			row.Revoked = wire.Revoked
			row.Event = wire.Event
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
