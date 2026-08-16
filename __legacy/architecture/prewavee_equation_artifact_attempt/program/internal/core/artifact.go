package core

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/internal/canonical"
)

// ArtifactDependency is an exact external semantic prerequisite of one
// Program artifact. It is intentionally just an artifact name and immutable
// identity: dependency semantics remain in Link and Rules, never in storage.
type ArtifactDependency struct {
	Name string
	ID   ContentID
}

// ArtifactEnvelope is the portable authority around one Program. Target is
// the ContentID of the sealed target Contract supplied by the consumer.
// Provenance is caller-owned immutable source provenance (for example a
// content-addressed source revision); it is not interpreted by Program.
type ArtifactEnvelope struct {
	Target       ContentID
	Dependencies []ArtifactDependency
	Provenance   string
	// Equations is an optional derived cache section.  It is deliberately
	// outside Program's ContentID: the authored Program is complete without
	// it, and a miss always rebuilds exactly the same body equations.
	Equations *ArtifactEquationCache
}

// ArtifactSemanticKey identifies one independently versioned producer.  It
// is a fixed-width semantic key, never a string registry name or a dense
// process-local slot.
type ArtifactSemanticKey struct {
	ID      ContentID
	Version uint64
}

// ArtifactEquationCache is the sole portable derived section for one
// context-parametric body-equation inventory.  It contains topology and
// declaration identities only.  In particular it contains no Rule closure,
// Factor value, State root, Candidate coordinate, or specialization result.
//
// Module binds this derived section to one Link-owned module environment.
// Bodies use existing Program Terms, Edges restate existing causal topology
// and exact local recurrence annotations, and Boundaries carry only the
// typed Factor/Rule dependency identities needed to validate static binding.
// Solver still creates its active graph and recurrence
// schedule per State generation.
type ArtifactEquationCache struct {
	Program  ContentID
	Module   ContentID
	Engine   ArtifactSemanticKey
	Factors  []ArtifactSemanticKey
	Rules    []ArtifactSemanticKey
	Bodies   []ArtifactEquationBody
	Boundary []ArtifactEquationBoundary
}

// ArtifactEquationBody is one Program Body-local equation row.
type ArtifactEquationBody struct {
	Body  Term
	Terms []Term
	Edges []ArtifactEquationEdge
}

// ArtifactEquationEdge is a direct typed restatement of one existing Program
// Edge.  Mu and MuDecisions are Seal-derived references and are validated
// against the decoded Program before the cache can be used.
type ArtifactEquationEdge struct {
	From        Term
	To          Term
	Decision    Term
	Truthy      bool
	Mu          Term
	MuDecisions []Term
}

// ArtifactEquationRead is one ordered input Factor read declared by a body-local
// Rule. Position addresses the Rule's input tuple; the same Factor may therefore
// occur at more than one position without losing its distinct dependency.
// Exact distinguishes a closed direct-key read from a dynamic Factor read.
// Key is meaningful only when Exact.  This signature is cache identity data,
// not a fact value, closure capture, or runtime specialization result.
type ArtifactEquationRead struct {
	Position int
	Factor   ArtifactSemanticKey
	Exact    bool
	Key      uint64
}

// ArtifactEquationBoundary is one body-local Rule boundary row. InputArity,
// Reads, and Writes are the complete portable Rule key contract. Reads retain
// the input coordinate and producer identity; a non-empty Writes vector is the
// sorted, duplicate-free direct output-key signature. An empty Writes vector
// denotes an ordinary generic output, never an implicit direct key.
type ArtifactEquationBoundary struct {
	Rule       ArtifactSemanticKey
	Output     ArtifactSemanticKey
	InputArity int
	// Activation is the lexical function/chunk activation owning At.  It is
	// intentionally distinct from EquationBody.Body, which names a lexical
	// nested block/loop/body equation row.
	Activation Term
	At         Term
	Reads      []ArtifactEquationRead
	Writes     []uint64
}

const (
	artifactCodecDomain = "program/artifact"
	// v11 adds direct-key Rule write signatures. v10 adds direct-key Rule read
	// signatures. v9 adds the fixed typed
	// metamethod-candidate source sections. v8 carried
	// an exact ordered Rule read schema for each equation boundary:
	// input arity, positions, and Factor identities. It remains
	// cache-only: normal Seal still rebuilds every Program control projection,
	// and the engine independently rebuilds its active State graph.
	artifactCodecVersion = 11
	artifactMaxBytes     = 256 << 20

	// These are semantic reconstruction limits, not claimed heap-byte limits.
	// Scan counts them before Builder replay: every decoded row/pool/map/Seal
	// object is O(Events), and every copied source string is O(StringBytes).
	artifactMaxEvents              = 32 << 20
	artifactMaxStringBytes         = 64 << 20
	artifactMaxEquationBytes       = 128 << 20
	artifactEquationVectorOverhead = 32
	// Fixed conservative reconstruction weights keep artifact admission
	// canonical across host architectures and future implementations. They
	// cover a 64-bit Go backing element plus headroom, never wire width.
	artifactEquationSemanticBytes = 48
	artifactEquationReadBytes     = 80
	artifactEquationWriteBytes    = 16
	artifactEquationTermBytes     = 8
	artifactEquationBodyBytes     = 64
	artifactEquationEdgeBytes     = 64
	artifactEquationBoundaryBytes = 192
	// Solver.New copies the outer cache vector before it copies any nested
	// cache tree. Charge that backing array in aggregate preflight as well;
	// otherwise many empty sections bypass the only memory boundary.
	artifactEquationCacheBytes = 256
	// Factor schema validation uses a temporary producer-ID set. This fixed
	// charge covers its cold hash-table entry and prevents validation itself
	// from exceeding the reconstruction budget.
	artifactEquationSetEntryBytes = 96
)

var (
	ErrArtifactUnavailable = errors.New("program artifact: unavailable Program")
	ErrArtifactTarget      = errors.New("program artifact: target identity mismatch")
	ErrArtifactCanonical   = errors.New("program artifact: noncanonical encoding")
	ErrArtifactLimit       = errors.New("program artifact: resource limit")
)

// EncodeArtifact produces the only Program persistence representation. Its
// Program payload contains exclusively authored rows. The one optional
// derived equation section is validation-only topology; Seal-derived causal,
// boundary, index, and candidate projections remain absent from Program
// persistence authority.
func EncodeArtifact(p *Program, envelope ArtifactEnvelope) ([]byte, error) {
	if p == nil || !p.ContentID().Available() || !envelope.Target.Available() {
		return nil, ErrArtifactUnavailable
	}
	// EncodeArtifact consumes an owned, already-canonical dependency slice.
	// The only public caller creates that one defensive copy before entering
	// this internal boundary; accepting and sorting another caller-owned slice
	// here would reintroduce metadata amplification.
	if err := validateArtifactEnvelope(envelope); err != nil {
		return nil, err
	}
	if !ArtifactEquationCacheFits(envelope.Equations) {
		return nil, ErrArtifactLimit
	}
	if envelope.Equations != nil && !validateArtifactEquationCache(p, *envelope.Equations) {
		return nil, ErrArtifactUnavailable
	}
	return encodeArtifactBounded(p, envelope, artifactMaxBytes)
}

// encodeArtifactBounded is the one all-or-nothing encode transaction. The
// limit parameter exists for codec-law tests; public persistence always uses
// artifactMaxBytes. It never hands partial bytes to its caller.
func encodeArtifactBounded(p *Program, envelope ArtifactEnvelope, limit int) ([]byte, error) {
	dst := newArtifactBuffer(limit)
	if err := encodeArtifact(dst, p, envelope); err != nil {
		return nil, err
	}
	data := dst.Bytes()
	return data, nil
}

func artifactMeasureAllowed(measure canonical.StreamMeasure) bool {
	return measure.Events <= artifactMaxEvents && measure.StringBytes <= artifactMaxStringBytes
}

// ArtifactEquationCacheFits performs the exact preflight used before a
// caller-owned cache is copied and before a decoded cache vector is made.
// It charges each typed backing array plus a conservative allocation header;
// scalar cache rows are charged by their concrete in-memory width. It does
// not validate semantic contents.
func ArtifactEquationCacheFits(cache *ArtifactEquationCache) bool {
	var budget artifactEquationBudget
	return artifactEquationCacheFits(&budget, cache)
}

// ArtifactEquationCachesFit applies one aggregate portable reconstruction
// bound to a caller-supplied cache collection before any owner makes a
// defensive copy. It is used by Solver.New as well as artifact persistence.
func ArtifactEquationCachesFit(caches []ArtifactEquationCache) bool {
	var budget artifactEquationBudget
	if !budget.reserve(uint64(len(caches)), artifactEquationCacheBytes) {
		return false
	}
	for index := range caches {
		if !artifactEquationCacheFits(&budget, &caches[index]) {
			return false
		}
	}
	return true
}

func artifactEquationCacheFits(budget *artifactEquationBudget, cache *ArtifactEquationCache) bool {
	if cache == nil {
		return true
	}
	if !budget.reserve(uint64(len(cache.Factors)), artifactEquationSemanticBytes) ||
		!budget.reserve(uint64(len(cache.Factors)), artifactEquationSetEntryBytes) ||
		!budget.reserve(uint64(len(cache.Rules)), artifactEquationSemanticBytes) ||
		!budget.reserve(uint64(len(cache.Rules)), artifactEquationSetEntryBytes) ||
		!budget.reserve(uint64(len(cache.Bodies)), artifactEquationBodyBytes) ||
		!budget.reserve(uint64(len(cache.Boundary)), artifactEquationBoundaryBytes) {
		return false
	}
	for _, body := range cache.Bodies {
		if !budget.reserve(uint64(len(body.Terms)), artifactEquationTermBytes) ||
			!budget.reserve(uint64(len(body.Edges)), artifactEquationEdgeBytes) {
			return false
		}
		for _, edge := range body.Edges {
			if !budget.reserve(uint64(len(edge.MuDecisions)), artifactEquationTermBytes) {
				return false
			}
		}
	}
	for _, boundary := range cache.Boundary {
		if !budget.reserve(uint64(len(boundary.Reads)), artifactEquationReadBytes) ||
			!budget.reserve(uint64(len(boundary.Writes)), artifactEquationWriteBytes) {
			return false
		}
	}
	return true
}

type artifactEquationBudget struct{ bytes uint64 }

func (budget *artifactEquationBudget) reserve(count, width uint64) bool {
	if budget == nil {
		return false
	}
	if count == 0 {
		return true
	}
	if width == 0 || count > (^uint64(0)-artifactEquationVectorOverhead)/width {
		return false
	}
	need := artifactEquationVectorOverhead + count*width
	if budget.bytes > artifactMaxEquationBytes || need > artifactMaxEquationBytes-budget.bytes {
		return false
	}
	budget.bytes += need
	return true
}

// artifactBuffer is the authority-preserving encode sink. Writer writes
// through both io.Writer and io.StringWriter, so both paths check the same
// hard limit before any backing buffer growth or byte publication.
type artifactBuffer struct {
	data  bytes.Buffer
	limit int
}

func newArtifactBuffer(limit int) *artifactBuffer { return &artifactBuffer{limit: limit} }

func (b *artifactBuffer) Write(data []byte) (int, error) {
	if b == nil || b.limit < 0 || len(data) > b.limit-b.data.Len() {
		return 0, ErrArtifactLimit
	}
	return b.data.Write(data)
}

func (b *artifactBuffer) WriteString(value string) (int, error) {
	if b == nil || b.limit < 0 || len(value) > b.limit-b.data.Len() {
		return 0, ErrArtifactLimit
	}
	return b.data.WriteString(value)
}

func (b *artifactBuffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.data.Bytes()
}

// DecodeArtifact verifies the target identity, reconstructs a private Builder
// draft from authored rows, performs exactly one normal Seal, then requires
// byte-for-byte canonical re-encoding. Callers never receive an unsealed
// Program or a decoded derived projection.
func DecodeArtifact(data []byte, target ContentID) (*Program, ArtifactEnvelope, error) {
	if !target.Available() {
		return nil, ArtifactEnvelope{}, ErrArtifactTarget
	}
	if len(data) > artifactMaxBytes {
		return nil, ArtifactEnvelope{}, ErrArtifactLimit
	}
	p, envelope, err := decodeArtifact(data, target)
	if err != nil {
		if errors.Is(err, ErrArtifactTarget) {
			return nil, ArtifactEnvelope{}, err
		}
		return nil, ArtifactEnvelope{}, fmt.Errorf("%w: %w", ErrArtifactCanonical, err)
	}
	// decodeArtifact has already completed the sole semantic transaction:
	// target, authored replay, complete cache validation, and claimed Program
	// identity. Re-encode the same owned values only to enforce canonical wire
	// bytes; routing this through EncodeArtifact would validate the cache twice.
	canonical, err := encodeArtifactBounded(p, envelope, artifactMaxBytes)
	if err != nil {
		return nil, ArtifactEnvelope{}, err
	}
	if !bytes.Equal(data, canonical) {
		return nil, ArtifactEnvelope{}, ErrArtifactCanonical
	}
	return p, envelope, nil
}

func validateArtifactEnvelope(envelope ArtifactEnvelope) error {
	if !envelope.Target.Available() ||
		!artifactEnvelopeFits(envelope.Provenance, len(envelope.Dependencies), artifactDependencyNameBytes(envelope.Dependencies)) {
		return ErrArtifactUnavailable
	}
	prior := ""
	for _, dependency := range envelope.Dependencies {
		if dependency.Name == "" || dependency.Name <= prior || !dependency.ID.Available() {
			return ErrArtifactUnavailable
		}
		prior = dependency.Name
	}
	return nil
}

func validArtifactSemanticKey(key ArtifactSemanticKey) bool {
	return key.ID.Available() && key.Version != 0
}

func orderedArtifactSemanticKeys(keys []ArtifactSemanticKey) bool {
	for index, key := range keys {
		if !validArtifactSemanticKey(key) {
			return false
		}
		if index != 0 && bytes.Compare(keys[index-1].ID[:], key.ID[:]) >= 0 {
			return false
		}
	}
	return true
}

func compareArtifactSemanticKey(left, right ArtifactSemanticKey) int {
	if order := bytes.Compare(left.ID[:], right.ID[:]); order != 0 {
		return order
	}
	if left.Version < right.Version {
		return -1
	}
	if left.Version > right.Version {
		return 1
	}
	return 0
}

func orderedArtifactEquationReads(reads []ArtifactEquationRead, inputArity int) bool {
	if inputArity <= 0 {
		return false
	}
	for index, read := range reads {
		if read.Position < 0 || read.Position >= inputArity || !validArtifactSemanticKey(read.Factor) || !read.Exact && read.Key != 0 {
			return false
		}
		if index == 0 {
			continue
		}
		prior := reads[index-1]
		if prior.Position > read.Position ||
			(prior.Position == read.Position && compareArtifactSemanticKey(prior.Factor, read.Factor) > 0) ||
			(prior.Position == read.Position && compareArtifactSemanticKey(prior.Factor, read.Factor) == 0 && (!prior.Exact || !read.Exact || prior.Key >= read.Key)) {
			return false
		}
	}
	return true
}

// validateArtifactEquationCache compares the entire derived body inventory to
// the sole Program authority. Engine later validates the Factor/Rule contract
// keys and ModuleKey, but Program never accepts a malformed, partial, or
// alternate topology as a cache that the engine may merely miss.
func validateArtifactEquationCache(p *Program, cache ArtifactEquationCache) bool {
	if p == nil || cache.Program != p.ContentID() || !cache.Module.Available() || !validArtifactSemanticKey(cache.Engine) ||
		!orderedArtifactSemanticKeys(cache.Rules) {
		return false
	}
	factors, ok := artifactSemanticIndex(cache.Factors)
	if !ok {
		return false
	}
	rules, ok := artifactSemanticIndex(cache.Rules)
	if !ok {
		return false
	}
	if !MatchesCanonicalArtifactEquationBodies(p, cache.Bodies) {
		return false
	}
	for index, boundary := range cache.Boundary {
		if !validArtifactSemanticKey(boundary.Rule) || !validArtifactSemanticKey(boundary.Output) ||
			boundary.Activation == 0 || boundary.At == 0 || !p.has(boundary.Activation, tagBody) || !p.Valid(boundary.At) ||
			(index != 0 && compareArtifactBoundary(cache.Boundary[index-1], boundary) >= 0) ||
			(index != 0 && cache.Boundary[index-1].Rule.ID == boundary.Rule.ID) ||
			!orderedArtifactEquationReads(boundary.Reads, boundary.InputArity) ||
			!orderedArtifactEquationWrites(boundary.Writes) {
			return false
		}
		if rule, present := rules[boundary.Rule.ID]; !present || rule != boundary.Rule {
			return false
		}
		if output, present := factors[boundary.Output.ID]; !present || output != boundary.Output {
			return false
		}
		for _, read := range boundary.Reads {
			if factor, present := factors[read.Factor.ID]; !present || factor != read.Factor {
				return false
			}
		}
		activation, ok := p.Activation(boundary.At)
		if !ok || activation != boundary.Activation {
			return false
		}
	}
	return true
}

func orderedArtifactEquationWrites(writes []uint64) bool {
	for index := 1; index < len(writes); index++ {
		if writes[index-1] >= writes[index] {
			return false
		}
	}
	return true
}

// CanonicalArtifactEquationBodies derives the complete, cacheable equation
// topology from one sealed Program.  It is the shared authority for artifact
// validation and engine emission; no cache reader reconstructs its own body
// or recurrence vocabulary.
func CanonicalArtifactEquationBodies(p *Program) ([]ArtifactEquationBody, bool) {
	if p == nil || !p.ContentID().Available() {
		return nil, false
	}
	bodies := make([]ArtifactEquationBody, 0, p.BodyCount())
	for index := 0; index < p.BodyCount(); index++ {
		row, ok := canonicalArtifactEquationBodyAt(p, index)
		if !ok {
			return nil, false
		}
		bodies = append(bodies, row)
	}
	return bodies, true
}

// MatchesCanonicalArtifactEquationBodies compares each stored body with one
// freshly derived Program row at a time. Decode and Solver admission therefore
// never construct a second whole-artifact topology merely to validate it.
func MatchesCanonicalArtifactEquationBodies(p *Program, bodies []ArtifactEquationBody) bool {
	if p == nil || len(bodies) != p.BodyCount() {
		return false
	}
	for index := range bodies {
		want, ok := canonicalArtifactEquationBodyAt(p, index)
		if !ok || !sameArtifactEquationBody(bodies[index], want) {
			return false
		}
	}
	return true
}

func canonicalArtifactEquationBodyAt(p *Program, index int) (ArtifactEquationBody, bool) {
	if p == nil || index < 0 || index >= p.BodyCount() {
		return ArtifactEquationBody{}, false
	}
	body, ok := p.BodyAt(index)
	if !ok || !p.has(body, tagBody) {
		return ArtifactEquationBody{}, false
	}
	terms := make([]Term, 0, 8)
	add := func(term Term, present bool) bool {
		if !present || term == 0 || !p.Valid(term) {
			return false
		}
		terms = append(terms, term)
		return true
	}
	if entry, ok := p.BodyEntry(body); !add(entry, ok) {
		return ArtifactEquationBody{}, false
	}
	if normal, ok := p.BodyNormalExit(body); !add(normal, ok) {
		return ArtifactEquationBody{}, false
	}
	if thrown, ok := p.BodyThrowExit(body); !add(thrown, ok) {
		return ArtifactEquationBody{}, false
	}
	if yielded, ok := p.BodyYieldExit(body); !add(yielded, ok) {
		return ArtifactEquationBody{}, false
	}
	if canceled, ok := p.BodyCancelExit(body); !add(canceled, ok) {
		return ArtifactEquationBody{}, false
	}
	if first, ok := p.BodyFirst(body); ok && !add(first, true) {
		return ArtifactEquationBody{}, false
	}
	if returned, ok := p.BodyReturnExit(body); ok && !add(returned, true) {
		return ArtifactEquationBody{}, false
	}
	count, ok := p.BodyEdgeCount(body)
	if !ok || count < 0 {
		return ArtifactEquationBody{}, false
	}
	row := ArtifactEquationBody{Body: body, Edges: make([]ArtifactEquationEdge, 0, count)}
	for edgeIndex := 0; edgeIndex < count; edgeIndex++ {
		edge, ok := p.BodyEdgeAt(body, edgeIndex)
		if !ok || edge.From() == 0 || edge.To() == 0 || !add(edge.From(), true) || !add(edge.To(), true) {
			return ArtifactEquationBody{}, false
		}
		encoded := ArtifactEquationEdge{From: edge.From(), To: edge.To()}
		if decision, truthy, present := edge.Decision(); present {
			encoded.Decision, encoded.Truthy = decision, truthy
		}
		if head, present := edge.Mu(); present {
			encoded.Mu = head
			decisionCount, valid := edge.MuDecisionCount()
			if !valid || decisionCount < 0 {
				return ArtifactEquationBody{}, false
			}
			encoded.MuDecisions = make([]Term, 0, decisionCount)
			for decisionIndex := 0; decisionIndex < decisionCount; decisionIndex++ {
				decision, valid := edge.MuDecisionAt(decisionIndex)
				if !valid {
					return ArtifactEquationBody{}, false
				}
				encoded.MuDecisions = append(encoded.MuDecisions, decision)
			}
		}
		row.Edges = append(row.Edges, encoded)
	}
	sortTerms(terms)
	row.Terms = compactArtifactTerms(terms)
	return row, true
}

func sortTerms(terms []Term) {
	sort.Slice(terms, func(left, right int) bool { return terms[left] < terms[right] })
}

func compactArtifactTerms(terms []Term) []Term {
	if len(terms) < 2 {
		return terms
	}
	write := 1
	for _, term := range terms[1:] {
		if terms[write-1] == term {
			continue
		}
		terms[write] = term
		write++
	}
	return terms[:write]
}

func sameArtifactEquationBody(left, right ArtifactEquationBody) bool {
	return left.Body == right.Body && sameArtifactTerms(left.Terms, right.Terms) &&
		sameArtifactEdges(left.Edges, right.Edges)
}

func sameArtifactTerms(left, right []Term) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameArtifactEdges(left, right []ArtifactEquationEdge) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].From != right[index].From || left[index].To != right[index].To ||
			left[index].Decision != right[index].Decision || left[index].Truthy != right[index].Truthy ||
			left[index].Mu != right[index].Mu || !sameArtifactTerms(left[index].MuDecisions, right[index].MuDecisions) {
			return false
		}
	}
	return true
}

// artifactSemanticIndex is the one cold uniqueness/membership structure for
// a producer vector. Factor declaration order remains untouched; callers use
// the index only for exact producer/version membership checks.
func artifactSemanticIndex(keys []ArtifactSemanticKey) (map[ContentID]ArtifactSemanticKey, bool) {
	seen := make(map[ContentID]ArtifactSemanticKey, len(keys))
	for _, key := range keys {
		if !validArtifactSemanticKey(key) {
			return nil, false
		}
		if _, duplicate := seen[key.ID]; duplicate {
			return nil, false
		}
		seen[key.ID] = key
	}
	return seen, true
}

func compareArtifactBoundary(left, right ArtifactEquationBoundary) int {
	if order := bytes.Compare(left.Rule.ID[:], right.Rule.ID[:]); order != 0 {
		return order
	}
	if left.Rule.Version < right.Rule.Version {
		return -1
	}
	if left.Rule.Version > right.Rule.Version {
		return 1
	}
	return 0
}

// ArtifactEnvelopeFits lets the public artifact package reject metadata before
// converting it into a second dependency slice. `nameBytes` is the exact sum
// of dependency name byte lengths; overflow must be reported as false by the
// caller rather than wrapped. It is a resource preflight only: semantic ID
// and duplicate checks remain in validateArtifactEnvelope.
func ArtifactEnvelopeFits(provenance string, dependencies int, nameBytes uint64) bool {
	return artifactEnvelopeFits(provenance, dependencies, nameBytes)
}

func artifactEnvelopeFits(provenance string, dependencies int, nameBytes uint64) bool {
	if dependencies < 0 || len(provenance) > artifactMaxBytes ||
		uint64(dependencies) > uint64(artifactMaxBytes)/artifactDependencyWireMin {
		return false
	}
	// Every dependency consumes its name bytes in addition to a strict 40-byte
	// minimum frame. This is intentionally a lower-bound preflight; the bounded
	// encoder remains the final exact wire authority.
	base := uint64(dependencies) * artifactDependencyWireMin
	return nameBytes <= uint64(artifactMaxBytes) && base <= uint64(artifactMaxBytes)-nameBytes
}

func artifactDependencyNameBytes(dependencies []ArtifactDependency) uint64 {
	var total uint64
	for _, dependency := range dependencies {
		width := uint64(len(dependency.Name))
		if width > uint64(artifactMaxBytes)-total {
			return uint64(artifactMaxBytes) + 1
		}
		total += width
	}
	return total
}
