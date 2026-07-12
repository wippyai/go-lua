// Package circuit contains backend-neutral guarded circuit domains.
package circuit

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ClassID string
type GuardID string
type BindingID string

var (
	ErrInvalidPolicy      = errors.New("circuit: invalid binding policy")
	ErrInvalidCell        = errors.New("circuit: invalid guarded binding cell")
	ErrWidenRequired      = errors.New("circuit: guarded widening required")
	ErrCertificate        = errors.New("circuit: invalid exact-merge certificate")
	ErrWidenUnverified    = errors.New("circuit: guarded widening is not verified")
	ErrFiniteVerification = errors.New("circuit: finite-domain verification failed")
)

// BindingPartitionPolicy is a finite, versioned product of application,
// target, provenance, and alias classes. Source declaration order is ignored.
type BindingPartitionPolicy struct {
	version     uint32
	application []ClassID
	target      []ClassID
	provenance  []ClassID
	alias       []ClassID
	canonical   []byte
}

type PartitionInput struct {
	Application ClassID
	Target      ClassID
	Provenance  ClassID
	Alias       ClassID
}

type CellKey = PartitionInput

func NewBindingPartitionPolicy(version uint32, application, target, provenance, alias []ClassID) (BindingPartitionPolicy, error) {
	if version == 0 {
		return BindingPartitionPolicy{}, fmt.Errorf("%w: zero version", ErrInvalidPolicy)
	}
	p := BindingPartitionPolicy{version: version}
	var err error
	if p.application, err = normalizedClasses("application", application); err != nil {
		return BindingPartitionPolicy{}, err
	}
	if p.target, err = normalizedClasses("target", target); err != nil {
		return BindingPartitionPolicy{}, err
	}
	if p.provenance, err = normalizedClasses("provenance", provenance); err != nil {
		return BindingPartitionPolicy{}, err
	}
	if p.alias, err = normalizedClasses("alias", alias); err != nil {
		return BindingPartitionPolicy{}, err
	}
	p.canonical, _ = json.Marshal(struct {
		Schema                                 string `json:"schema"`
		Version                                uint32 `json:"version"`
		Application, Target, Provenance, Alias []ClassID
	}{"go-lua.circuit.binding-partition/v1", version, p.application, p.target, p.provenance, p.alias})
	return p, nil
}

func (p BindingPartitionPolicy) Version() uint32        { return p.version }
func (p BindingPartitionPolicy) CanonicalBytes() []byte { return append([]byte(nil), p.canonical...) }
func (p BindingPartitionPolicy) CellCount() uint64 {
	return uint64(len(p.application)) * uint64(len(p.target)) * uint64(len(p.provenance)) * uint64(len(p.alias))
}
func (p BindingPartitionPolicy) Partition(input PartitionInput) (CellKey, error) {
	if !containsClass(p.application, input.Application) || !containsClass(p.target, input.Target) ||
		!containsClass(p.provenance, input.Provenance) || !containsClass(p.alias, input.Alias) {
		return CellKey{}, fmt.Errorf("%w: unregistered partition %#v", ErrInvalidPolicy, input)
	}
	return input, nil
}

// PrecisionPolicy makes both finite guard carriers and the bounded precision
// choice explicit and versioned. Source declaration order is non-semantic.
type PrecisionPolicy struct {
	version           uint32
	maxDisjuncts      uint16
	widenedBinding    BindingID
	applicationGuards []GuardID
	provenanceGuards  []GuardID
	aliasGuards       []GuardID
	canonical         []byte
}

func NewPrecisionPolicy(version uint32, maxDisjuncts uint16, widened BindingID, application, provenance, alias []GuardID) (PrecisionPolicy, error) {
	if version == 0 || maxDisjuncts == 0 || !validID(string(widened)) {
		return PrecisionPolicy{}, ErrInvalidPolicy
	}
	p := PrecisionPolicy{version: version, maxDisjuncts: maxDisjuncts, widenedBinding: widened}
	var err error
	if p.applicationGuards, err = normalizedGuards("application", application); err != nil {
		return PrecisionPolicy{}, err
	}
	if p.provenanceGuards, err = normalizedGuards("provenance", provenance); err != nil {
		return PrecisionPolicy{}, err
	}
	if p.aliasGuards, err = normalizedGuards("alias", alias); err != nil {
		return PrecisionPolicy{}, err
	}
	p.canonical, _ = json.Marshal(struct {
		Schema                         string    `json:"schema"`
		Version                        uint32    `json:"version"`
		MaxDisjuncts                   uint16    `json:"max_disjuncts"`
		WidenedBinding                 BindingID `json:"widened_binding"`
		Application, Provenance, Alias []GuardID
	}{"go-lua.circuit.binding-precision/v1", version, maxDisjuncts, widened, p.applicationGuards, p.provenanceGuards, p.aliasGuards})
	return p, nil
}
func (p PrecisionPolicy) Version() uint32           { return p.version }
func (p PrecisionPolicy) MaxDisjuncts() uint16      { return p.maxDisjuncts }
func (p PrecisionPolicy) WidenedBinding() BindingID { return p.widenedBinding }
func (p PrecisionPolicy) CanonicalBytes() []byte    { return append([]byte(nil), p.canonical...) }
func (p PrecisionPolicy) GuardVocabularySize() int {
	return len(p.applicationGuards) + len(p.provenanceGuards) + len(p.aliasGuards)
}

// GuardSet is a canonical finite disjunction. Empty is invalid: callers must
// name the explicit universal guard in their finite guard vocabulary.
type GuardSet struct{ ids []GuardID }

func NewGuardSet(ids ...GuardID) (GuardSet, error) {
	out := append([]GuardID(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i, id := range out {
		if !validID(string(id)) || i > 0 && out[i-1] == id {
			return GuardSet{}, fmt.Errorf("%w: invalid or duplicate guard %q", ErrInvalidCell, id)
		}
	}
	if len(out) == 0 {
		return GuardSet{}, fmt.Errorf("%w: empty guard set", ErrInvalidCell)
	}
	return GuardSet{ids: out}, nil
}
func (g GuardSet) IDs() []GuardID            { return append([]GuardID(nil), g.ids...) }
func (g GuardSet) equal(other GuardSet) bool { return slicesEqual(g.ids, other.ids) }

// Disjunct preserves all three guard families beside one binding.
type Disjunct struct {
	application GuardSet
	provenance  GuardSet
	alias       GuardSet
	binding     BindingID
}

func NewDisjunct(application, provenance, alias GuardSet, binding BindingID) (Disjunct, error) {
	if len(application.ids) == 0 || len(provenance.ids) == 0 || len(alias.ids) == 0 || !validID(string(binding)) {
		return Disjunct{}, fmt.Errorf("%w: incomplete disjunct", ErrInvalidCell)
	}
	return Disjunct{application: cloneGuard(application), provenance: cloneGuard(provenance), alias: cloneGuard(alias), binding: binding}, nil
}
func (d Disjunct) ApplicationGuards() GuardSet { return cloneGuard(d.application) }
func (d Disjunct) ProvenanceGuards() GuardSet  { return cloneGuard(d.provenance) }
func (d Disjunct) AliasGuards() GuardSet       { return cloneGuard(d.alias) }
func (d Disjunct) Binding() BindingID          { return d.binding }

// Cell is one finite partition cell containing correlated disjuncts.
type Cell struct {
	key       CellKey
	disjuncts []Disjunct
	loss      bool
	policy    uint32
}

func (c Cell) Key() CellKey           { return c.key }
func (c Cell) Disjuncts() []Disjunct  { return cloneDisjuncts(c.disjuncts) }
func (c Cell) PrecisionLost() bool    { return c.loss }
func (c Cell) PolicyVersion() uint32  { return c.policy }
func (c Cell) CanonicalBytes() []byte { out, _ := json.Marshal(cellWire(c)); return out }

// Rank is the bounded ascent measure. Partition rises until MaxDisjuncts+1;
// after widening, GuardAtoms can only grow within the finite guard vocabulary.
type Rank struct {
	Partition  uint32
	GuardAtoms uint32
}

func (c Cell) rank(max uint16) Rank {
	partition := uint32(len(c.disjuncts))
	if c.loss {
		partition = uint32(max) + 1
	}
	guards := map[string]struct{}{}
	for _, d := range c.disjuncts {
		for _, id := range d.application.ids {
			guards["a:"+string(id)] = struct{}{}
		}
		for _, id := range d.provenance.ids {
			guards["p:"+string(id)] = struct{}{}
		}
		for _, id := range d.alias.ids {
			guards["l:"+string(id)] = struct{}{}
		}
	}
	return Rank{Partition: partition, GuardAtoms: uint32(len(guards))}
}

type MergeClaim struct{ left, right, merged Disjunct }

func NewMergeClaim(left, right, merged Disjunct) MergeClaim {
	return MergeClaim{left: left, right: right, merged: merged}
}
func (c MergeClaim) Left() Disjunct   { return cloneDisjunct(c.left) }
func (c MergeClaim) Right() Disjunct  { return cloneDisjunct(c.right) }
func (c MergeClaim) Merged() Disjunct { return cloneDisjunct(c.merged) }

// authoritySeal is deliberately non-zero-sized so distinct authorities cannot
// be coalesced to the same implementation-defined zero-size address.
type authoritySeal struct{ nonce byte }
type ExactMergeVerifier func(MergeClaim) bool
type ExactMergeAuthority struct {
	seal   *authoritySeal
	verify ExactMergeVerifier
}

func NewExactMergeAuthority(verifier ExactMergeVerifier) (*ExactMergeAuthority, error) {
	if verifier == nil {
		return nil, fmt.Errorf("%w: nil verifier", ErrCertificate)
	}
	return &ExactMergeAuthority{seal: &authoritySeal{nonce: 1}, verify: verifier}, nil
}

type ExactMergeCertificate struct {
	seal   *authoritySeal
	claim  MergeClaim
	digest [sha256.Size]byte
}

func (a *ExactMergeAuthority) Certify(claim MergeClaim) (ExactMergeCertificate, error) {
	if a == nil || a.seal == nil || a.verify == nil || !claimGuardUnionExact(claim) || !a.verify(claim) {
		return ExactMergeCertificate{}, ErrCertificate
	}
	owned := MergeClaim{cloneDisjunct(claim.left), cloneDisjunct(claim.right), cloneDisjunct(claim.merged)}
	return ExactMergeCertificate{seal: a.seal, claim: owned, digest: sha256.Sum256(claimBytes(owned))}, nil
}

type WidenVerifier func(widened BindingID, members []BindingID) bool

type Domain struct {
	partition   BindingPartitionPolicy
	precision   PrecisionPolicy
	authority   *ExactMergeAuthority
	verifyWiden WidenVerifier
}

func NewBindingDomain(partition BindingPartitionPolicy, precision PrecisionPolicy, authority *ExactMergeAuthority, widen WidenVerifier) (*Domain, error) {
	if partition.version == 0 || precision.version == 0 || precision.maxDisjuncts == 0 || !validID(string(precision.widenedBinding)) || precision.GuardVocabularySize() == 0 {
		return nil, ErrInvalidPolicy
	}
	precision.applicationGuards = append([]GuardID(nil), precision.applicationGuards...)
	precision.provenanceGuards = append([]GuardID(nil), precision.provenanceGuards...)
	precision.aliasGuards = append([]GuardID(nil), precision.aliasGuards...)
	precision.canonical = append([]byte(nil), precision.canonical...)
	return &Domain{partition: partition, precision: precision, authority: authority, verifyWiden: widen}, nil
}

// CertifyExactMerge validates the complete claim against this domain's finite
// guard vocabulary before invoking its semantic exactness authority.
func (d *Domain) CertifyExactMerge(claim MergeClaim) (ExactMergeCertificate, error) {
	if d == nil || d.authority == nil || !d.validDisjunct(claim.left) || !d.validDisjunct(claim.right) || !d.validDisjunct(claim.merged) {
		return ExactMergeCertificate{}, ErrCertificate
	}
	return d.authority.Certify(claim)
}

// Rank returns the mechanically bounded ascent rank for a cell in this exact
// finite policy universe.
func (d *Domain) Rank(cell Cell) (Rank, error) {
	if !d.validCell(cell) {
		return Rank{}, ErrInvalidCell
	}
	rank := cell.rank(d.precision.maxDisjuncts)
	if rank.GuardAtoms > uint32(d.precision.GuardVocabularySize()) {
		return Rank{}, fmt.Errorf("%w: guard rank exceeds vocabulary", ErrInvalidCell)
	}
	return rank, nil
}

// Equal is the canonical guarded-cell equality owned by the binding domain.
func (d *Domain) Equal(left, right Cell) bool {
	return d.validCell(left) && d.validCell(right) && bytes.Equal(left.CanonicalBytes(), right.CanonicalBytes())
}

// LessOrEq is the guarded-cell order. Exact right-hand cells require literal
// disjunct inclusion. A precision-loss RHS additionally requires guard
// inclusion and a fresh call to the configured binding upper-bound verifier;
// the widened spelling is never treated as Top by name alone.
func (d *Domain) LessOrEq(left, right Cell) bool {
	if !d.validCell(left) || !d.validCell(right) || left.key != right.key {
		return false
	}
	if d.Equal(left, right) {
		return true
	}
	if !right.loss {
		if left.loss {
			return false
		}
		for _, candidate := range left.disjuncts {
			if findDisjunct(right.disjuncts, candidate) < 0 {
				return false
			}
		}
		return true
	}
	widened := right.disjuncts[0]
	bindings := make([]BindingID, 0, len(left.disjuncts))
	for _, candidate := range left.disjuncts {
		if !guardSetSubset(candidate.application, widened.application) ||
			!guardSetSubset(candidate.provenance, widened.provenance) ||
			!guardSetSubset(candidate.alias, widened.alias) {
			return false
		}
		bindings = append(bindings, candidate.binding)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i] < bindings[j] })
	bindings = dedupBindings(bindings)
	return d.verifyWiden != nil && d.verifyWiden(widened.binding, append([]BindingID(nil), bindings...))
}

func (d *Domain) Singleton(key CellKey, disjunct Disjunct) (Cell, error) {
	if d == nil {
		return Cell{}, ErrInvalidPolicy
	}
	if _, err := d.partition.Partition(key); err != nil {
		return Cell{}, err
	}
	if !d.validDisjunct(disjunct) {
		return Cell{}, ErrInvalidCell
	}
	return Cell{key: key, disjuncts: []Disjunct{cloneDisjunct(disjunct)}, policy: d.precision.version}, nil
}

type OperationStats struct {
	InputDisjuncts, Deduplicated, CertifiedMerges, WidenedDisjuncts, OutputDisjuncts int
	PrecisionLost                                                                    bool
}

func (d *Domain) Join(left, right Cell, certificates ...ExactMergeCertificate) (Cell, OperationStats, error) {
	return d.combine(left, right, false, certificates)
}
func (d *Domain) Widen(left, right Cell, certificates ...ExactMergeCertificate) (Cell, OperationStats, error) {
	return d.combine(left, right, true, certificates)
}

func (d *Domain) combine(left, right Cell, allowWiden bool, certificates []ExactMergeCertificate) (Cell, OperationStats, error) {
	stats := OperationStats{InputDisjuncts: len(left.disjuncts) + len(right.disjuncts)}
	if d == nil || left.key != right.key || !d.validCell(left) || !d.validCell(right) {
		return Cell{}, stats, ErrInvalidCell
	}
	all := append(cloneDisjuncts(left.disjuncts), right.disjuncts...)
	all = normalizeDisjuncts(all)
	stats.Deduplicated = stats.InputDisjuncts - len(all)
	var err error
	all, stats.CertifiedMerges, err = d.applyCertificates(all, certificates)
	if err != nil {
		return Cell{}, stats, err
	}
	loss := left.loss || right.loss
	if loss || len(all) > int(d.precision.maxDisjuncts) {
		if !allowWiden {
			return Cell{}, stats, ErrWidenRequired
		}
		all, err = d.widenAll(all)
		if err != nil {
			return Cell{}, stats, err
		}
		stats.WidenedDisjuncts = stats.InputDisjuncts
		loss = true
	}
	out := Cell{key: left.key, disjuncts: all, loss: loss, policy: d.precision.version}
	stats.OutputDisjuncts, stats.PrecisionLost = len(all), loss
	return out, stats, nil
}

func (d *Domain) applyCertificates(all []Disjunct, certificates []ExactMergeCertificate) ([]Disjunct, int, error) {
	ordered := append([]ExactMergeCertificate(nil), certificates...)
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i].digest[:], ordered[j].digest[:]) < 0 })
	merges := 0
	for _, certificate := range ordered {
		if d.authority == nil || certificate.seal == nil || certificate.seal != d.authority.seal ||
			certificate.digest != sha256.Sum256(claimBytes(certificate.claim)) || !claimGuardUnionExact(certificate.claim) ||
			!d.validDisjunct(certificate.claim.left) || !d.validDisjunct(certificate.claim.right) || !d.validDisjunct(certificate.claim.merged) {
			return nil, merges, ErrCertificate
		}
		left := findDisjunct(all, certificate.claim.left)
		right := findDisjunct(all, certificate.claim.right)
		if left < 0 || right < 0 || left == right {
			return nil, merges, ErrCertificate
		}
		if left > right {
			left, right = right, left
		}
		all = append(all[:right], all[right+1:]...)
		all = append(all[:left], all[left+1:]...)
		all = append(all, cloneDisjunct(certificate.claim.merged))
		all = normalizeDisjuncts(all)
		merges++
	}
	return all, merges, nil
}

func (d *Domain) widenAll(all []Disjunct) ([]Disjunct, error) {
	bindings := make([]BindingID, 0, len(all))
	var app, provenance, alias GuardSet
	for index, disjunct := range all {
		bindings = append(bindings, disjunct.binding)
		if index == 0 {
			app, provenance, alias = disjunct.application, disjunct.provenance, disjunct.alias
			continue
		}
		app = unionGuard(app, disjunct.application)
		provenance = unionGuard(provenance, disjunct.provenance)
		alias = unionGuard(alias, disjunct.alias)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i] < bindings[j] })
	bindings = dedupBindings(bindings)
	if d.verifyWiden == nil || !d.verifyWiden(d.precision.widenedBinding, append([]BindingID(nil), bindings...)) {
		return nil, ErrWidenUnverified
	}
	widened, _ := NewDisjunct(app, provenance, alias, d.precision.widenedBinding)
	return []Disjunct{widened}, nil
}

// VerifyFiniteSamples exhaustively checks commutativity, idempotence, bound,
// and monotone bounded rank over every subset pair of a small finite sample.
func VerifyFiniteSamples(domain *Domain, key CellKey, samples []Disjunct) error {
	if domain == nil || len(samples) == 0 || len(samples) > 10 {
		return fmt.Errorf("%w: sample size", ErrFiniteVerification)
	}
	for _, sample := range samples {
		if !domain.validDisjunct(sample) {
			return fmt.Errorf("%w: sample outside guard vocabulary", ErrFiniteVerification)
		}
	}
	states := make([]Cell, 0, 1<<len(samples))
	seen := make(map[string]struct{})
	for mask := 1; mask < 1<<len(samples); mask++ {
		var parts []Disjunct
		for i := range samples {
			if mask&(1<<i) != 0 {
				parts = append(parts, samples[i])
			}
		}
		parts = normalizeDisjuncts(parts)
		if len(parts) > int(domain.precision.maxDisjuncts) {
			continue
		}
		cell := Cell{key: key, disjuncts: parts, policy: domain.precision.version}
		states = append(states, cell)
		seen[string(cell.CanonicalBytes())] = struct{}{}
	}
	// Close the sample carrier under widening so precision-loss states and all
	// their finite guard unions participate in the law/rank checks.
	for changed := true; changed; {
		changed = false
		snapshot := append([]Cell(nil), states...)
		for _, left := range snapshot {
			for _, right := range snapshot {
				widened, _, err := domain.Widen(left, right)
				if err != nil {
					return fmt.Errorf("%w: closure: %v", ErrFiniteVerification, err)
				}
				key := string(widened.CanonicalBytes())
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				states = append(states, widened)
				changed = true
				if len(states) > 4096 {
					return fmt.Errorf("%w: closure safety bound", ErrFiniteVerification)
				}
			}
		}
	}
	for _, left := range states {
		self, _, err := domain.Widen(left, left)
		if err != nil || !bytes.Equal(self.CanonicalBytes(), left.CanonicalBytes()) {
			return fmt.Errorf("%w: widening idempotence", ErrFiniteVerification)
		}
		for _, right := range states {
			forward, _, err := domain.Widen(left, right)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrFiniteVerification, err)
			}
			reverse, _, err := domain.Widen(right, left)
			if err != nil || !bytes.Equal(forward.CanonicalBytes(), reverse.CanonicalBytes()) {
				return fmt.Errorf("%w: widening order", ErrFiniteVerification)
			}
			if len(forward.disjuncts) > int(domain.precision.maxDisjuncts) {
				return fmt.Errorf("%w: disjunct bound", ErrFiniteVerification)
			}
			got, err := domain.Rank(forward)
			if err != nil {
				return fmt.Errorf("%w: output rank: %v", ErrFiniteVerification, err)
			}
			lrank, err := domain.Rank(left)
			if err != nil {
				return fmt.Errorf("%w: left rank: %v", ErrFiniteVerification, err)
			}
			rrank, err := domain.Rank(right)
			if err != nil {
				return fmt.Errorf("%w: right rank: %v", ErrFiniteVerification, err)
			}
			if rankLess(got, lrank) || rankLess(got, rrank) || got.Partition > uint32(domain.precision.maxDisjuncts)+1 || got.GuardAtoms > uint32(domain.precision.GuardVocabularySize()) {
				return fmt.Errorf("%w: rank regression", ErrFiniteVerification)
			}
		}
	}
	return nil
}

func normalizedClasses(kind string, input []ClassID) ([]ClassID, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("%w: empty %s classes", ErrInvalidPolicy, kind)
	}
	out := append([]ClassID(nil), input...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i, id := range out {
		if !validID(string(id)) || i > 0 && out[i-1] == id {
			return nil, fmt.Errorf("%w: invalid/duplicate %s %q", ErrInvalidPolicy, kind, id)
		}
	}
	return out, nil
}
func normalizedGuards(kind string, input []GuardID) ([]GuardID, error) {
	if len(input) == 0 {
		return nil, fmt.Errorf("%w: empty %s guard vocabulary", ErrInvalidPolicy, kind)
	}
	out := append([]GuardID(nil), input...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i, id := range out {
		if !validID(string(id)) || i > 0 && out[i-1] == id {
			return nil, fmt.Errorf("%w: invalid/duplicate %s guard %q", ErrInvalidPolicy, kind, id)
		}
	}
	return out, nil
}
func containsClass(classes []ClassID, id ClassID) bool {
	i := sort.Search(len(classes), func(i int) bool { return classes[i] >= id })
	return i < len(classes) && classes[i] == id
}
func validID(id string) bool {
	if id == "" || strings.TrimSpace(id) != id {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' || r == ':') {
			return false
		}
	}
	return true
}
func cloneGuard(g GuardSet) GuardSet { return GuardSet{ids: append([]GuardID(nil), g.ids...)} }
func cloneDisjunct(d Disjunct) Disjunct {
	return Disjunct{cloneGuard(d.application), cloneGuard(d.provenance), cloneGuard(d.alias), d.binding}
}
func cloneDisjuncts(in []Disjunct) []Disjunct {
	out := make([]Disjunct, len(in))
	for i, d := range in {
		out[i] = cloneDisjunct(d)
	}
	return out
}
func validDisjunct(d Disjunct) bool {
	return len(d.application.ids) > 0 && len(d.provenance.ids) > 0 && len(d.alias.ids) > 0 && validID(string(d.binding))
}
func guardsInVocabulary(guards GuardSet, vocabulary []GuardID) bool {
	for _, guard := range guards.ids {
		i := sort.Search(len(vocabulary), func(i int) bool { return vocabulary[i] >= guard })
		if i == len(vocabulary) || vocabulary[i] != guard {
			return false
		}
	}
	return true
}
func (d *Domain) validDisjunct(disjunct Disjunct) bool {
	return d != nil && validDisjunct(disjunct) && guardsInVocabulary(disjunct.application, d.precision.applicationGuards) && guardsInVocabulary(disjunct.provenance, d.precision.provenanceGuards) && guardsInVocabulary(disjunct.alias, d.precision.aliasGuards)
}
func (d *Domain) validCellDisjuncts(disjuncts []Disjunct) bool {
	if len(disjuncts) == 0 || len(disjuncts) > int(d.precision.maxDisjuncts) {
		return false
	}
	for _, disjunct := range disjuncts {
		if !d.validDisjunct(disjunct) {
			return false
		}
	}
	return true
}
func (d *Domain) validCell(cell Cell) bool {
	if d == nil || cell.policy != d.precision.version {
		return false
	}
	if _, err := d.partition.Partition(cell.key); err != nil {
		return false
	}
	if !d.validCellDisjuncts(cell.disjuncts) {
		return false
	}
	if !slicesEqualDisjuncts(cell.disjuncts, normalizeDisjuncts(cell.disjuncts)) {
		return false
	}
	if cell.loss {
		return len(cell.disjuncts) == 1 && cell.disjuncts[0].binding == d.precision.widenedBinding
	}
	return true
}
func guardSetSubset(left, right GuardSet) bool {
	for _, id := range left.ids {
		i := sort.Search(len(right.ids), func(i int) bool { return right.ids[i] >= id })
		if i == len(right.ids) || right.ids[i] != id {
			return false
		}
	}
	return true
}
func slicesEqualDisjuncts(left, right []Disjunct) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !disjunctEqual(left[i], right[i]) {
			return false
		}
	}
	return true
}
func slicesEqual[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func disjunctEqual(a, b Disjunct) bool {
	return a.binding == b.binding && a.application.equal(b.application) && a.provenance.equal(b.provenance) && a.alias.equal(b.alias)
}
func disjunctKey(d Disjunct) string { b, _ := json.Marshal(disjunctWire(d)); return string(b) }
func normalizeDisjuncts(in []Disjunct) []Disjunct {
	out := cloneDisjuncts(in)
	sort.Slice(out, func(i, j int) bool { return disjunctKey(out[i]) < disjunctKey(out[j]) })
	n := 0
	for _, d := range out {
		if n == 0 || !disjunctEqual(out[n-1], d) {
			out[n] = d
			n++
		}
	}
	return out[:n]
}
func findDisjunct(all []Disjunct, want Disjunct) int {
	for i, d := range all {
		if disjunctEqual(d, want) {
			return i
		}
	}
	return -1
}
func unionGuard(a, b GuardSet) GuardSet {
	ids := append(a.IDs(), b.ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	n := 0
	for _, id := range ids {
		if n == 0 || ids[n-1] != id {
			ids[n] = id
			n++
		}
	}
	return GuardSet{ids: ids[:n]}
}
func claimGuardUnionExact(c MergeClaim) bool {
	return c.merged.application.equal(unionGuard(c.left.application, c.right.application)) && c.merged.provenance.equal(unionGuard(c.left.provenance, c.right.provenance)) && c.merged.alias.equal(unionGuard(c.left.alias, c.right.alias))
}
func claimBytes(c MergeClaim) []byte {
	b, _ := json.Marshal(struct{ Left, Right, Merged any }{disjunctWire(c.left), disjunctWire(c.right), disjunctWire(c.merged)})
	return b
}
func dedupBindings(in []BindingID) []BindingID {
	n := 0
	for _, id := range in {
		if n == 0 || in[n-1] != id {
			in[n] = id
			n++
		}
	}
	return in[:n]
}
func rankLess(a, b Rank) bool {
	return a.Partition < b.Partition || a.Partition == b.Partition && a.GuardAtoms < b.GuardAtoms
}
func disjunctWire(d Disjunct) any {
	return struct {
		Application, Provenance, Alias []GuardID
		Binding                        BindingID
	}{d.application.ids, d.provenance.ids, d.alias.ids, d.binding}
}
func cellWire(c Cell) any {
	ds := make([]any, len(c.disjuncts))
	for i, d := range c.disjuncts {
		ds[i] = disjunctWire(d)
	}
	return struct {
		Schema        string
		Key           CellKey
		Policy        uint32
		PrecisionLost bool
		Disjuncts     []any
	}{"go-lua.circuit.binding-cell/v1", c.key, c.policy, c.loss, ds}
}
