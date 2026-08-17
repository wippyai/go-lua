// Package contract owns library contract INSTANCES: the serialized,
// mount-bound artifacts whose kinds the library surface declares.
//
// The split is deliberate. The declaration table
// (analysis/schema/library) owns the closed catalog of contract KINDS - the
// member-form algebra, the codec identity, the addressing form and the
// validation law-set reference a kind's instances are checked under. It owns no
// instance, because an instance is mount-time data and its mount identity is
// Link-local. This package owns the other half: the instance envelope, the
// codec that serializes it, and the admission that checks one authored instance
// against the sealed kind it claims to be published under.
//
// A member is addressed by the path of exported values from the contract root,
// never by a dotted global name. The distinction is the whole point of the
// surface it serves: under name addressing `local f = string.len` loses the
// contract that made f callable and `string.len = print` inherits one it was
// never given, while an export path is walked once at mount and the contract
// binds to the value it reached. An alias of that value keeps the contract; a
// slot rebound to another value does not acquire it.
//
// One name survives, and it is stated rather than hidden: Root is the authored
// selector a project uses to choose which mount an instance is bound to. A name
// may select a mount during project construction; it may never address a
// member. Nothing downstream of the mount reads Root.
//
// A member payload's ENCODING belongs to the form's owner, not to this package.
// An instance therefore states, per member, whether its payload encoding is
// resolved - the body is present and framed - or deferred, meaning the owning
// format has not landed and the member carries its address and nothing else. A
// deferred member is an honest half of a contract: it says which value the
// contract attaches to and admits that what it says about that value is not yet
// serializable. An empty body under a resolved encoding is not that; it is a
// payload that claims to exist and does not, and admission rejects it.
package contract

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// StepKind is how one export path step reaches the next value. Both spellings
// address a value: an export key reaches a value the contract publishes
// directly, and a metatable key reaches one published through the metatable of
// the value the path has reached so far.
type StepKind uint8

const (
	StepInvalid StepKind = iota
	// StepExport reaches the value published under one key of the current
	// value's own export graph.
	StepExport
	// StepMetatable reaches the value published under one key of the current
	// value's metatable. It is how a contract addresses a member that is
	// reached through __index rather than through a direct export.
	StepMetatable
	stepKindLimit
)

func (kind StepKind) Available() bool { return kind > StepInvalid && kind < stepKindLimit }

// Step is one hop of an export path.
type Step struct {
	Kind StepKind
	Key  string
}

func (step Step) Available() bool { return step.Kind.Available() && step.Key != "" }

// Path is the address of one contract member: the ordered steps from the
// contract root to the value the member describes. The empty path is the
// contract root itself, which is an addressable member - a library's aggregate
// export is a value the contract has something to say about.
//
// A path is not a name. Its steps are keys of the contract's OWN export graph,
// walked once from the root the mount resolved, so the value a path reaches is
// fixed at mount. A dotted global name is resolved through the mutable global
// environment at every use, which is why the surface refuses it.
type Path struct {
	steps []Step
}

// Root is the path of the contract root.
func Root() Path { return Path{} }

// NewPath builds one export path. A step that does not address anything leaves
// the path unavailable rather than silently short.
func NewPath(steps ...Step) Path { return Path{steps: append([]Step(nil), steps...)} }

// Export is the common path of one directly exported member, one hop from the
// contract root.
func Export(key string) Path { return NewPath(Step{Kind: StepExport, Key: key}) }

// Metatable is the path of one member published through a metatable key of the
// contract root.
func Metatable(key string) Path { return NewPath(Step{Kind: StepMetatable, Key: key}) }

// Available reports whether every step of this path addresses a value. The root
// path is available: it addresses the contract root.
func (path Path) Available() bool {
	for _, step := range path.steps {
		if !step.Available() {
			return false
		}
	}
	return true
}

// Len is the number of steps from the contract root.
func (path Path) Len() int { return len(path.steps) }

// At resolves one step by its position from the contract root.
func (path Path) At(position int) (Step, bool) {
	if position < 0 || position >= len(path.steps) {
		return Step{}, false
	}
	return path.steps[position], true
}

// Equal reports whether two paths address the same value from one root.
func (path Path) Equal(other Path) bool {
	if len(path.steps) != len(other.steps) {
		return false
	}
	for index, step := range path.steps {
		if step != other.steps[index] {
			return false
		}
	}
	return true
}

// Encoding states whether a member's payload body is present. It is the
// instance-level counterpart of the declaration surface's own resolution
// reference: a format that has not landed is declared deferred rather than
// written as an empty payload that looks resolved.
type Encoding uint8

const (
	EncodingInvalid Encoding = iota
	// EncodingResolved carries a framed payload body in the format the kind
	// declares for the member's form.
	EncodingResolved
	// EncodingDeferred carries no body. The member states which value the
	// contract attaches to; what it says about that value is owned by a format
	// that has not landed.
	EncodingDeferred
)

func (encoding Encoding) Available() bool {
	return encoding == EncodingResolved || encoding == EncodingDeferred
}

// Member is one authored row of a contract instance: which member form it is,
// which exported value it attaches to, the payload format that form is
// serialized in, and the payload itself when the format has landed.
//
// Payload restates the kind's declared format identity for the form. The
// restatement is not redundancy: it makes an instance self-describing, so a
// reader that resolved a different kind than the writer published under
// rejects the decode instead of reading a member as a shape it is not.
type Member struct {
	Form     library.Form
	Path     Path
	Payload  identity.ContentID
	Encoding Encoding
	Body     []byte
}

// Spec is the authored declaration of one contract instance.
type Spec struct {
	// Kind is the declared contract kind this instance is published under. It
	// is resolved against the sealed library surface; nothing here restates the
	// kind's own declaration.
	Kind schema.Key
	// Codec is the serialized format this instance claims to be written in. It
	// must be the format the kind declares, at the version the kind declares.
	Codec library.Codec
	// Root is the authored selector a project uses to choose the mount this
	// instance binds to. It is the one name in a contract, it is used once at
	// project construction, and no member address derives from it.
	Root string
	// Members are the authored member rows, in authored order.
	Members []Member
}

// Instance is one admitted contract instance. It is immutable once built.
type Instance struct {
	kind    schema.Key
	codec   library.Codec
	class   library.Class
	root    string
	members []Member
}

// New admits one authored instance against the sealed kind it claims. A
// rejected spec returns false rather than a partially usable instance: an
// instance that carried one member the kind cannot describe would be a contract
// whose reader has no ground to interpret that member.
func New(spec Spec, kind *library.Entry) (*Instance, bool) {
	if kind == nil || !kind.EntryAvailable() || kind.Key() != spec.Kind {
		return nil, false
	}
	// The addressing law is stated here as well as at the surface. A kind that
	// addressed by name could not be admitted into the table at all, so this is
	// the reader's own refusal to publish an instance under one if it ever were.
	if !kind.Addressing().ValueProvenance() {
		return nil, false
	}
	if !spec.Codec.Available() || spec.Codec != kind.Codec() {
		return nil, false
	}
	// A mount is selected by the root selector. An instance with none cannot be
	// bound to anything, so it is not an instance of a mount-bound contract.
	if spec.Root == "" {
		return nil, false
	}
	if len(spec.Members) == 0 {
		return nil, false
	}
	instance := &Instance{
		kind:    spec.Kind,
		codec:   spec.Codec,
		class:   kind.Class(),
		root:    spec.Root,
		members: make([]Member, 0, len(spec.Members)),
	}
	for _, member := range spec.Members {
		if !admissibleMember(member, kind) {
			return nil, false
		}
		// Two rows of one form over one address are one row written twice, and
		// a reader would have no ground to choose between them.
		for _, prior := range instance.members {
			if prior.Form == member.Form && prior.Path.Equal(member.Path) {
				return nil, false
			}
		}
		instance.members = append(instance.members, copyMember(member))
	}
	return instance, true
}

// admissibleMember states the per-row laws: the form is one the kind declares,
// the address is walkable, the payload format is the one the kind declared for
// that form, and the body agrees with the encoding it is published under.
func admissibleMember(member Member, kind *library.Entry) bool {
	if !member.Form.Available() || !kind.Declares(member.Form) {
		return false
	}
	if !member.Path.Available() {
		return false
	}
	declared, resolved := kind.Payload(member.Form)
	if !resolved || !member.Payload.Available() || member.Payload != declared {
		return false
	}
	switch member.Encoding {
	case EncodingResolved:
		return len(member.Body) != 0
	case EncodingDeferred:
		return len(member.Body) == 0
	default:
		return false
	}
}

func copyMember(member Member) Member {
	member.Path = NewPath(member.Path.steps...)
	member.Body = append([]byte(nil), member.Body...)
	return member
}

func (instance *Instance) Kind() schema.Key { return instance.kind }

func (instance *Instance) Codec() library.Codec { return instance.codec }

// Class is the contract algebra this instance's kind is published under. An
// instance never restates it: it is read from the kind at admission.
func (instance *Instance) Class() library.Class { return instance.class }

// Root is the authored mount selector.
func (instance *Instance) Root() string { return instance.root }

// Count is the number of authored member rows.
func (instance *Instance) Count() int { return len(instance.members) }

// At resolves one member row by its authored position.
func (instance *Instance) At(position int) (Member, bool) {
	if instance == nil || position < 0 || position >= len(instance.members) {
		return Member{}, false
	}
	return copyMember(instance.members[position]), true
}

// Members returns a copy of the authored rows, so a reader cannot rewrite an
// admitted instance through the slice it was handed.
func (instance *Instance) Members() []Member {
	if instance == nil {
		return nil
	}
	rows := make([]Member, 0, len(instance.members))
	for _, member := range instance.members {
		rows = append(rows, copyMember(member))
	}
	return rows
}

// Resolve returns the member of one form at one address.
func (instance *Instance) Resolve(form library.Form, path Path) (Member, bool) {
	if instance == nil {
		return Member{}, false
	}
	for _, member := range instance.members {
		if member.Form == form && member.Path.Equal(path) {
			return copyMember(member), true
		}
	}
	return Member{}, false
}

// Deferred reports how many member payload encodings this instance has not
// landed. It is the instance's own statement of how much of the contract is
// address and how much is content, so a consumer never has to infer it from an
// empty body.
func (instance *Instance) Deferred() int {
	if instance == nil {
		return 0
	}
	var count int
	for _, member := range instance.members {
		if member.Encoding == EncodingDeferred {
			count++
		}
	}
	return count
}
