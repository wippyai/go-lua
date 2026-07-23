package interproc

import (
	"errors"
	"fmt"
	"sort"
)

// DemandKey is a canonical requested output identity.  It deliberately
// excludes entry values; the entry projection is a separate cache dimension.
type DemandKey string

func (d DemandKey) valid() bool { return d != "" }

// EntrySelector identifies one exact entry coordinate/path relation.  It is
// never a scalarized fallback or a request to capture the entire entry.
type EntrySelector string

func (s EntrySelector) valid() bool { return s != "" }

// ReadRole names every semantic route through which a body may inspect an
// entry.  The certificate records role plus selector so a provider or
// diagnostic access cannot hide behind an otherwise identical ordinary read.
type ReadRole string

const (
	ReadSemantic         ReadRole = "semantic"
	ReadGuard            ReadRole = "guard"
	ReadEntrySeed        ReadRole = "entry-seed"
	ReadCallEntry        ReadRole = "call-entry"
	ReadProviderCallback ReadRole = "provider-callback"
	ReadPublication      ReadRole = "publication"
	ReadAllocation       ReadRole = "allocation"
	ReadDiagnostic       ReadRole = "diagnostic"
)

func (r ReadRole) valid() bool {
	switch r {
	case ReadSemantic, ReadGuard, ReadEntrySeed, ReadCallEntry, ReadProviderCallback, ReadPublication, ReadAllocation, ReadDiagnostic:
		return true
	default:
		return false
	}
}

// ReadCertificateInputs is deliberately exhaustive.  A compiler must account
// for every category even when a particular category has no selectors; it may
// not replace an incomplete certificate with a full-entry projection.
type ReadCertificateInputs struct {
	Semantic         []EntrySelector
	Guards           []EntrySelector
	EntrySeeding     []EntrySelector
	CallEntry        []EntrySelector
	ProviderCallback []EntrySelector
	Publication      []EntrySelector
	Allocation       []EntrySelector
	Diagnostic       []EntrySelector
}

// CertificateRead is one exact role/selector pair retained in a projection
// certificate.  It is exposed for diagnostics and auditing, not mutation.
type CertificateRead struct {
	Role     ReadRole
	Selector EntrySelector
}

func (r CertificateRead) less(other CertificateRead) bool {
	if r.Role != other.Role {
		return r.Role < other.Role
	}
	return r.Selector < other.Selector
}

// ReadProjectionCertificate is a static, conservative proof surface for the
// entry selectors needed by one demanded body output.
type ReadProjectionCertificate struct {
	demand DemandKey
	reads  []CertificateRead
}

// NewReadProjectionCertificate constructs the immutable certificate.  Every
// supplied category is recorded, including guards and diagnostic reads.
func NewReadProjectionCertificate(demand DemandKey, inputs ReadCertificateInputs) (ReadProjectionCertificate, error) {
	if !demand.valid() {
		return ReadProjectionCertificate{}, fmt.Errorf("interproc: read certificate has no demand key")
	}
	reads := make([]CertificateRead, 0, len(inputs.Semantic)+len(inputs.Guards)+len(inputs.EntrySeeding)+len(inputs.CallEntry)+len(inputs.ProviderCallback)+len(inputs.Publication)+len(inputs.Allocation)+len(inputs.Diagnostic))
	appendRole := func(role ReadRole, selectors []EntrySelector) {
		for _, selector := range selectors {
			reads = append(reads, CertificateRead{Role: role, Selector: selector})
		}
	}
	appendRole(ReadSemantic, inputs.Semantic)
	appendRole(ReadGuard, inputs.Guards)
	appendRole(ReadEntrySeed, inputs.EntrySeeding)
	appendRole(ReadCallEntry, inputs.CallEntry)
	appendRole(ReadProviderCallback, inputs.ProviderCallback)
	appendRole(ReadPublication, inputs.Publication)
	appendRole(ReadAllocation, inputs.Allocation)
	appendRole(ReadDiagnostic, inputs.Diagnostic)
	return newReadProjectionCertificate(demand, reads)
}

func newReadProjectionCertificate(demand DemandKey, reads []CertificateRead) (ReadProjectionCertificate, error) {
	out := ReadProjectionCertificate{demand: demand, reads: append([]CertificateRead(nil), reads...)}
	sort.Slice(out.reads, func(i, j int) bool { return out.reads[i].less(out.reads[j]) })
	for index, read := range out.reads {
		if !demand.valid() || !read.Role.valid() || !read.Selector.valid() {
			return ReadProjectionCertificate{}, fmt.Errorf("interproc: malformed read certificate")
		}
		if index != 0 && !out.reads[index-1].less(read) {
			return ReadProjectionCertificate{}, fmt.Errorf("interproc: duplicate read certificate selector")
		}
	}
	return out, nil
}

func (c ReadProjectionCertificate) DemandKey() DemandKey { return c.demand }
func (c ReadProjectionCertificate) Reads() []CertificateRead {
	return append([]CertificateRead(nil), c.reads...)
}
func (c ReadProjectionCertificate) Selectors() []EntrySelector {
	selectors := make([]EntrySelector, 0, len(c.reads))
	for _, read := range c.reads {
		if len(selectors) == 0 || selectors[len(selectors)-1] != read.Selector {
			selectors = append(selectors, read.Selector)
		}
	}
	// reads sort by role, so deduplicate through a deterministic selector sort.
	sort.Slice(selectors, func(i, j int) bool { return selectors[i] < selectors[j] })
	out := selectors[:0]
	for _, selector := range selectors {
		if len(out) == 0 || out[len(out)-1] != selector {
			out = append(out, selector)
		}
	}
	return append([]EntrySelector(nil), out...)
}
func (c ReadProjectionCertificate) Covers(role ReadRole, selector EntrySelector) bool {
	for _, read := range c.reads {
		if read.Role == role && read.Selector == selector {
			return true
		}
	}
	return false
}
func (c ReadProjectionCertificate) CanonicalBytes() []byte {
	if !c.demand.valid() {
		return nil
	}
	for index, read := range c.reads {
		if !read.Role.valid() || !read.Selector.valid() || index != 0 && !c.reads[index-1].less(read) {
			return nil
		}
	}
	out := appendText(nil, "interproc-read-projection-certificate/content-v1")
	out = appendText(out, string(c.demand))
	out = appendU64(out, uint64(len(c.reads)))
	for _, read := range c.reads {
		out = appendText(out, string(read.Role))
		out = appendText(out, string(read.Selector))
	}
	return out
}
func (c ReadProjectionCertificate) ContentID() ContentID {
	encoded := c.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

// ReadObservation is one runtime entry access reported by an existing bound
// kernel. It is audit evidence only and cannot influence evaluation.
type ReadObservation struct {
	Role     ReadRole
	Selector EntrySelector
}

// IncompleteReadCertificateError is a non-retryable correctness failure.  In
// particular callers must not turn it into a silent full-entry projection.
type IncompleteReadCertificateError struct {
	Demand   DemandKey
	Role     ReadRole
	Selector EntrySelector
}

func (e *IncompleteReadCertificateError) Error() string {
	return fmt.Sprintf("interproc: incomplete read certificate for demand %q: %s %q", e.Demand, e.Role, e.Selector)
}

func IsIncompleteReadCertificateError(err error) bool {
	var incomplete *IncompleteReadCertificateError
	return errors.As(err, &incomplete)
}

// VerifyReadAudit is the certificate equivalent of OperatorContract.VerifyAccess:
// every observed role/selector pair must be a declared subset.
func (c ReadProjectionCertificate) VerifyReadAudit(observed []ReadObservation) error {
	if c.CanonicalBytes() == nil {
		return fmt.Errorf("interproc: malformed read certificate")
	}
	for _, read := range observed {
		if !read.Role.valid() || !read.Selector.valid() || !c.Covers(read.Role, read.Selector) {
			return &IncompleteReadCertificateError{Demand: c.demand, Role: read.Role, Selector: read.Selector}
		}
	}
	return nil
}

// EntryValue is one caller-provided canonical encoding for an entry selector.
// The envelope never interprets the value or substitutes it into a symbolic
// tuple; the future VM binding retains the complete entry privately.
type EntryValue struct {
	Selector EntrySelector
	Encoding []byte
}

// EntryBinding is a complete normalized entry parameter represented as exact
// selector encodings. It contains no State pointer or callback.
type EntryBinding struct{ values []EntryValue }

func NewEntryBinding(values []EntryValue) (EntryBinding, error) {
	out := EntryBinding{values: append([]EntryValue(nil), values...)}
	sort.Slice(out.values, func(i, j int) bool { return out.values[i].Selector < out.values[j].Selector })
	for index, value := range out.values {
		if !value.Selector.valid() || value.Encoding == nil {
			return EntryBinding{}, fmt.Errorf("interproc: malformed entry binding")
		}
		if index != 0 && out.values[index-1].Selector == value.Selector {
			return EntryBinding{}, fmt.Errorf("interproc: duplicate entry selector %q", value.Selector)
		}
		out.values[index].Encoding = append([]byte(nil), value.Encoding...)
	}
	return out, nil
}

func (b EntryBinding) Values() []EntryValue {
	out := append([]EntryValue(nil), b.values...)
	for index := range out {
		out[index].Encoding = append([]byte(nil), out[index].Encoding...)
	}
	return out
}

// EntryProjection is the canonical cache-key payload selected by a complete
// certificate. Its bytes are retained for later byte-for-byte collision checks.
type EntryProjection struct{ values []EntryValue }

func (p EntryProjection) Values() []EntryValue { return EntryBinding{values: p.values}.Values() }
func (p EntryProjection) CanonicalBytes() []byte {
	for index, value := range p.values {
		if !value.Selector.valid() || value.Encoding == nil || index != 0 && p.values[index-1].Selector >= value.Selector {
			return nil
		}
	}
	out := appendText(nil, "interproc-entry-projection/content-v1")
	out = appendU64(out, uint64(len(p.values)))
	for _, value := range p.values {
		out = appendText(out, string(value.Selector))
		out = appendBytes(out, value.Encoding)
	}
	return out
}
func (p EntryProjection) ContentID() ContentID {
	encoded := p.CanonicalBytes()
	if encoded == nil {
		return ContentID{}
	}
	return contentID(encoded)
}

// Project selects exactly the certified entry coordinates. A missing selected
// coordinate is a named incomplete-certificate failure, never a full-entry
// fallback.
func (c ReadProjectionCertificate) Project(entry EntryBinding) (EntryProjection, error) {
	if c.CanonicalBytes() == nil {
		return EntryProjection{}, fmt.Errorf("interproc: malformed read certificate")
	}
	bySelector := make(map[EntrySelector]EntryValue, len(entry.values))
	for _, value := range entry.values {
		bySelector[value.Selector] = value
	}
	selectors := c.Selectors()
	projection := EntryProjection{values: make([]EntryValue, 0, len(selectors))}
	for _, selector := range selectors {
		value, ok := bySelector[selector]
		if !ok {
			return EntryProjection{}, &IncompleteReadCertificateError{Demand: c.demand, Selector: selector}
		}
		projection.values = append(projection.values, EntryValue{Selector: selector, Encoding: append([]byte(nil), value.Encoding...)})
	}
	return projection, nil
}
