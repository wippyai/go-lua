package pack

import (
	"github.com/wippyai/go-lua/analysis/domain/static"
)

// Endpoint is one schema-issued scalar subject. It is deliberately opaque:
// an existing Link Value, Target formal, or result subject is named by the
// schema that issued it, never by a Pack-local ordinal supplied by a Rule.
type Endpoint struct {
	owner  *algebra
	index  uint32
	class  static.Class
	sealed bool
}

func newEndpoint(owner *algebra, index uint32, class static.Class) (Endpoint, bool) {
	if owner == nil || !owner.valid() || index == 0 || !owner.admits(class) {
		return Endpoint{}, false
	}
	endpoint := Endpoint{owner: owner, index: index, class: class, sealed: true}
	return endpoint, endpoint.valid()
}

func (endpoint Endpoint) valid() bool {
	return endpoint.sealed && endpoint.owner != nil && endpoint.index != 0
}

func (endpoint Endpoint) Class() (static.Class, bool) {
	if !endpoint.valid() {
		return static.Class{}, false
	}
	return endpoint.class, true
}

// Port is one schema-issued whole-Pack endpoint. It is deliberately only an
// owner-fenced dense handle: the Link occurrence that issued it remains cold
// in Schema and cannot become recurrent carrier state.
type Port struct {
	owner  *algebra
	index  uint32
	class  static.Class
	free   bool
	sealed bool
}

// newPort initializes one whole-Pack endpoint. Free means that this complete
// Pack relation may occur as a source expression; output eligibility is
// independently fixed by the relation target vector, never by this flag.
func newPort(owner *algebra, index uint32, class static.Class, free bool) (Port, bool) {
	if owner == nil || !owner.valid() || index == 0 || !owner.admits(class) {
		return Port{}, false
	}
	port := Port{owner: owner, index: index, class: class, free: free, sealed: true}
	return port, port.valid()
}

func (port Port) valid() bool {
	return port.sealed && port.owner != nil && port.index != 0
}

func (port Port) Class() (static.Class, bool) {
	if !port.valid() {
		return static.Class{}, false
	}
	return port.class, true
}

// TailKind distinguishes an interface-owned free Pack parameter from a
// case-local existential tail. Bound tails are alpha-normalized inside each
// Case before equality, hashing, join, or publication.
type TailKind uint8

const (
	TailInvalid TailKind = iota
	TailFree
	TailBound
)

// TailRef names one complete Pack tail, never a scalar element. For a Free
// tail, port is its immutable Link/schema port. For a Bound tail, index is a
// local alpha name whose identity has no meaning outside its Case.
type TailRef struct {
	owner  *algebra
	kind   TailKind
	index  uint32
	class  static.Class
	port   Port
	sealed bool
}

func freeTail(port Port) (TailRef, bool) {
	if !port.valid() || !port.free {
		return TailRef{}, false
	}
	tail := TailRef{owner: port.owner, kind: TailFree, index: port.index, class: port.class, port: port, sealed: true}
	return tail, tail.valid()
}

func boundTail(owner *algebra, index uint32, class static.Class) (TailRef, bool) {
	if owner == nil || !owner.valid() || index == 0 || !owner.admits(class) {
		return TailRef{}, false
	}
	tail := TailRef{owner: owner, kind: TailBound, index: index, class: class, sealed: true}
	return tail, tail.valid()
}

func (tail TailRef) valid() bool {
	if !tail.sealed || tail.owner == nil || tail.index == 0 {
		return false
	}
	switch tail.kind {
	case TailFree:
		return tail.port.valid() && tail.port.owner == tail.owner && tail.port.free && tail.port.index == tail.index && tail.owner.equalClass(tail.port.class, tail.class)
	case TailBound:
		return tail.port.owner == nil && tail.port.index == 0
	default:
		return false
	}
}

func (tail TailRef) Kind() TailKind {
	if !tail.valid() {
		return TailInvalid
	}
	return tail.kind
}

func (tail TailRef) Class() (static.Class, bool) {
	if !tail.valid() {
		return static.Class{}, false
	}
	return tail.class, true
}

// ScalarKind is the finite Pack scalar grammar. Every scalar either names one
// existing Link endpoint, derives one element from one complete tail, or is
// a class-constrained unknown. No Rule may manufacture a scalar identity.
type ScalarKind uint8

const (
	ScalarInvalid ScalarKind = iota
	ScalarEndpoint
	ScalarHead
	ScalarAny
)

// Scalar is an immutable Pack expression. Source literals already enter Pack
// as canonical Link scalar endpoints, so this carrier retains no duplicate
// literal identity or payload table. Its class is supplied by Static's sealed
// ClassSet; Pack never decodes a type.
type Scalar struct {
	owner    *algebra
	kind     ScalarKind
	endpoint Endpoint
	class    static.Class
	tail     TailRef
	offset   Offset
	sealed   bool
}

func endpointScalar(endpoint Endpoint) (Scalar, bool) {
	value := Scalar{owner: endpoint.owner, kind: ScalarEndpoint, endpoint: endpoint, class: endpoint.class, sealed: endpoint.valid()}
	return value, value.valid()
}

func headScalar(tail TailRef, offset Offset) (Scalar, bool) {
	if !tail.valid() || !offset.valid() || offset.owner != tail.owner {
		return Scalar{}, false
	}
	class, ok := tail.owner.joinClass(tail.class, tail.owner.classes.Nil())
	value := Scalar{owner: tail.owner, kind: ScalarHead, tail: tail, offset: offset, class: class, sealed: ok}
	return value, value.valid()
}

func anyScalar(owner *algebra, class static.Class) (Scalar, bool) {
	value := Scalar{owner: owner, kind: ScalarAny, class: class, sealed: owner != nil && owner.admits(class)}
	return value, value.valid()
}

func (scalar Scalar) valid() bool {
	if !scalar.sealed || scalar.owner == nil {
		return false
	}
	switch scalar.kind {
	case ScalarEndpoint:
		return scalar.endpoint.valid() && scalar.endpoint.owner == scalar.owner && scalar.owner.equalClass(scalar.endpoint.class, scalar.class) && !scalar.tail.valid() && !scalar.offset.valid()
	case ScalarHead:
		headClass, ok := scalar.owner.joinClass(scalar.tail.class, scalar.owner.classes.Nil())
		return ok && !scalar.endpoint.valid() && scalar.tail.valid() && scalar.tail.owner == scalar.owner && scalar.owner.equalClass(headClass, scalar.class) && scalar.offset.valid()
	case ScalarAny:
		return !scalar.endpoint.valid() && !scalar.tail.valid() && !scalar.offset.valid()
	default:
		return false
	}
}

func (scalar Scalar) Kind() ScalarKind {
	if !scalar.valid() {
		return ScalarInvalid
	}
	return scalar.kind
}

func (scalar Scalar) Class() (static.Class, bool) {
	if !scalar.valid() {
		return static.Class{}, false
	}
	return scalar.class, true
}

// Endpoint returns the existing schema-issued scalar identity when this is an
// exact endpoint expression.  Head and class-only scalars deliberately do not
// fabricate one: their Pack-local provenance remains in the Pack relation.
func (scalar Scalar) Endpoint() (Endpoint, bool) {
	if !scalar.valid() || scalar.kind != ScalarEndpoint {
		return Endpoint{}, false
	}
	return scalar.endpoint, true
}

// Rest is the unbounded middle of an Open term. Tail is exact drop of one
// shared tail; AnyTail deliberately carries an independent unknown sequence.
type RestKind uint8

const (
	RestInvalid RestKind = iota
	RestTail
	RestAny
)

type Rest struct {
	owner  *algebra
	kind   RestKind
	tail   TailRef
	offset Offset
	class  static.Class
	sealed bool
}

func tailRest(tail TailRef, offset Offset) (Rest, bool) {
	value := Rest{owner: tail.owner, kind: RestTail, tail: tail, offset: offset, class: tail.class, sealed: tail.valid() && offset.valid() && offset.owner == tail.owner}
	return value, value.valid()
}

func anyRest(owner *algebra, class static.Class) (Rest, bool) {
	value := Rest{owner: owner, kind: RestAny, class: class, sealed: owner != nil && owner.admits(class)}
	return value, value.valid()
}

func (rest Rest) valid() bool {
	if !rest.sealed || rest.owner == nil {
		return false
	}
	switch rest.kind {
	case RestTail:
		return rest.tail.valid() && rest.tail.owner == rest.owner && rest.owner.equalClass(rest.tail.class, rest.class) && rest.offset.valid()
	case RestAny:
		return !rest.tail.valid() && !rest.offset.valid()
	default:
		return false
	}
}

func (rest Rest) Kind() RestKind {
	if !rest.valid() {
		return RestInvalid
	}
	return rest.kind
}

func (rest Rest) Class() (static.Class, bool) {
	if !rest.valid() {
		return static.Class{}, false
	}
	return rest.class, true
}

func (rest Rest) Tail() (TailRef, Offset, bool) {
	if !rest.valid() || rest.kind != RestTail {
		return TailRef{}, Offset{}, false
	}
	return rest.tail, rest.offset, true
}

// TermKind is the complete whole-Pack term grammar. A Closed term may be
// empty. Open preserves both ends around an arbitrary finite middle. AnyPack
// is the one all-unknown whole-Pack term and is not represented as a fake tail.
type TermKind uint8

const (
	TermInvalid TermKind = iota
	TermClosed
	TermOpen
	TermAny
)

type Term struct {
	owner  *algebra
	kind   TermKind
	prefix []Scalar
	rest   Rest
	suffix []Scalar
	sealed bool
}

func closedTerm(owner *algebra, values []Scalar) (Term, bool) {
	term := Term{owner: owner, kind: TermClosed, prefix: cloneScalars(values)}
	term.sealed = validClosedTerm(term)
	return term, term.valid()
}

func openTerm(owner *algebra, prefix []Scalar, rest Rest, suffix []Scalar) (Term, bool) {
	term := Term{owner: owner, kind: TermOpen, prefix: cloneScalars(prefix), rest: rest, suffix: cloneScalars(suffix)}
	term.sealed = validOpenTerm(term)
	if term.valid() && len(term.prefix) == 0 && len(term.suffix) == 0 && term.rest.kind == RestAny && owner.equalClass(term.rest.class, owner.classes.AnyValue()) {
		return anyTerm(owner)
	}
	return term, term.valid()
}

func anyTerm(owner *algebra) (Term, bool) {
	if owner == nil || !owner.valid() {
		return Term{}, false
	}
	return Term{owner: owner, kind: TermAny, sealed: true}, true
}

func (term Term) valid() bool {
	return term.sealed && term.owner != nil
}

func validClosedTerm(term Term) bool {
	if term.owner == nil || !term.owner.valid() || len(term.suffix) != 0 || term.rest.valid() {
		return false
	}
	check := func(values []Scalar) bool {
		for _, value := range values {
			if !value.valid() || value.owner != term.owner {
				return false
			}
		}
		return true
	}
	return check(term.prefix)
}

func validOpenTerm(term Term) bool {
	if term.owner == nil || !term.owner.valid() || !term.rest.valid() || term.rest.owner != term.owner {
		return false
	}
	check := func(values []Scalar) bool {
		for _, value := range values {
			if !value.valid() || value.owner != term.owner {
				return false
			}
		}
		return true
	}
	return check(term.prefix) && check(term.suffix)
}

func (term Term) Kind() TermKind {
	if !term.valid() {
		return TermInvalid
	}
	return term.kind
}

// Equal reports structural equality under one sealed Pack algebra.  It is a
// read-only comparison; callers cannot use it to combine terms from another
// relation or mint a new carrier value.
func (term Term) Equal(other Term) bool {
	return term.valid() && other.valid() && term.owner == other.owner && equalTerm(term, other)
}

// FixedCount reports the exact prefix width for Closed/Open terms. AnyPack
// has no fixed scalar positions.
func (term Term) FixedCount() int {
	if !term.valid() || term.kind == TermAny {
		return 0
	}
	return len(term.prefix)
}

func (term Term) FixedAt(index int) (Scalar, bool) {
	if !term.valid() || term.kind == TermAny || index < 0 || index >= len(term.prefix) {
		return Scalar{}, false
	}
	scalar := term.prefix[index]
	return scalar, scalar.valid()
}

// Tail returns the open middle and the exact fixed suffix. Closed and AnyPack
// terms have no residual tail.
func (term Term) Tail() (Rest, []Scalar, bool) {
	if !term.valid() || term.kind != TermOpen || !term.rest.valid() {
		return Rest{}, nil, false
	}
	return term.rest, append([]Scalar(nil), term.suffix...), true
}

func (term Term) SuffixCount() int {
	if !term.valid() || term.kind != TermOpen {
		return 0
	}
	return len(term.suffix)
}

func (term Term) SuffixAt(index int) (Scalar, bool) {
	if !term.valid() || term.kind != TermOpen || index < 0 || index >= len(term.suffix) {
		return Scalar{}, false
	}
	scalar := term.suffix[index]
	return scalar, scalar.valid()
}

func cloneScalars(values []Scalar) []Scalar {
	if len(values) == 0 {
		return nil
	}
	return append([]Scalar(nil), values...)
}
