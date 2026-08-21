// Package vocabulary owns the identity algebra of the analyzer's semantic
// roles: the derivation that turns one declared role string into the global
// identity the engine binds under, and the resolution a surface reaches that
// identity through.
//
// The roles themselves are not authored here. A role is a row of the
// structural vocabulary's semantic-role category, contributed by the domain
// that owns it, so a domain adds a role by declaring one rather than by
// widening a struct in this package. What stays here is what no domain can
// own: the framing the identity is derived under, and the resolution every
// surface performs against the sealed declaration.
//
// It deliberately contains no mounted-program, Link, or runtime identity:
// those belong to the program and binding layers.
package vocabulary

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// SemanticFormat is the version of the global semantic vocabulary.  Changing
// the role list, framing domain, or interpretation of a role requires bumping
// this value.
const SemanticFormat uint64 = 7

// rolePrefix is the key namespace a semantic role row is declared under. A
// row's spelling is the role, and its key is the role inside this namespace,
// so a role names one row of one category and cannot collide with a member of
// another structural vocabulary that happens to render the same way.
const rolePrefix = "semantic/"

// RoleKey is the structural row key one role string is declared under. A
// surface names a role by this key and resolves the identity through the
// sealed table, so no surface derives an identity from text of its own.
func RoleKey(role string) schema.Key { return schema.Key(rolePrefix + role) }

// RuleSemantics is the closed identity tuple for one rule: its rule identity
// and operand form. Rule-admission evidence is an engine concern rather than
// a semantic role, so it is intentionally absent from this vocabulary.
type RuleSemantics struct {
	Rule    identity.SemanticKey
	Operand identity.SemanticKey
}

func (semantics RuleSemantics) Available() bool {
	return semantics.Rule.Available() && semantics.Operand.Available()
}

// TransformedRuleSemantics adds the transform form used by rules whose output
// is normalized before admission.
type TransformedRuleSemantics struct {
	RuleSemantics
	Transform identity.SemanticKey
}

func (semantics TransformedRuleSemantics) Available() bool {
	return semantics.RuleSemantics.Available() && semantics.Transform.Available()
}

// RoleSpecs is one contributor's semantic role declaration: one row per role,
// keyed inside the role namespace and spelled as the role itself. The ordinal
// is left to the aggregation, because no foreign spelling numbers this
// vocabulary: a member of it is only ever resolved by key.
func RoleSpecs(roles ...string) []structure.Spec {
	specs := make([]structure.Spec, 0, len(roles))
	for _, role := range roles {
		specs = append(specs, structure.Spec{
			Key:      RoleKey(role),
			Category: structure.CategorySemanticRole,
			Spelling: role,
			Accepted: true,
		})
	}
	return specs
}

// RuleRoleSpecs declares the two roles one rule is identified by: the rule
// itself and the operand form its occurrences read. Rule-admission evidence is
// deliberately not a semantic role.
func RuleRoleSpecs(role string) []structure.Spec {
	return RoleSpecs("rule/"+role, "operand/"+role)
}

// TransformedRuleRoleSpecs declares the three roles a rule whose output is
// normalized before admission is identified by.
func TransformedRuleRoleSpecs(role string) []structure.Spec {
	return append(RuleRoleSpecs(role), RoleSpecs("transform/"+role)...)
}

// Roles is the resolved semantic role vocabulary: the declared rows of the
// semantic-role category, each carrying the global identity its spelling
// derives. A surface holds a role by its declared key and reaches the identity
// here, so the identity a catalog is composed under and the identity a row is
// sealed under are one derivation.
//
// A Roles restricted to one entry's declared roles is what that entry's own
// hooks receive, so a hook that reaches for a role its entry never declared
// resolves nothing and the composition rejects it.
type Roles struct {
	keys map[schema.Key]identity.SemanticKey
}

// NewRoles resolves one admitted structural inventory into the semantic role
// vocabulary. A row of the semantic-role category whose spelling derives no
// identity leaves the vocabulary unresolved rather than short one role.
func NewRoles(entries []*structure.Entry) (Roles, bool) {
	keys := make(map[schema.Key]identity.SemanticKey, len(entries))
	for _, entry := range entries {
		if entry == nil {
			return Roles{}, false
		}
		if entry.Category() != structure.CategorySemanticRole {
			continue
		}
		semantic, ok := Key(entry.Spelling())
		if !ok {
			return Roles{}, false
		}
		if _, declared := keys[entry.Key()]; declared {
			return Roles{}, false
		}
		keys[entry.Key()] = semantic
	}
	return Roles{keys: keys}, len(keys) != 0
}

// Available reports whether this vocabulary resolves anything.
func (roles Roles) Available() bool { return len(roles.keys) != 0 }

// Count is the number of roles this vocabulary resolves.
func (roles Roles) Count() int { return len(roles.keys) }

// Key resolves one declared role's global identity by the row key it is
// declared under.
func (roles Roles) Key(key schema.Key) (identity.SemanticKey, bool) {
	semantic, ok := roles.keys[key]
	return semantic, ok && semantic.Available()
}

// Restrict narrows this vocabulary to exactly the named roles. A name this
// vocabulary does not resolve leaves the restriction unavailable, so an entry
// declaring a role no domain contributed is rejected where it is declared
// rather than where its hook reads.
func (roles Roles) Restrict(keys ...schema.Key) (Roles, bool) {
	narrowed := make(map[schema.Key]identity.SemanticKey, len(keys))
	for _, key := range keys {
		semantic, ok := roles.Key(key)
		if !ok {
			return Roles{}, false
		}
		narrowed[key] = semantic
	}
	return Roles{keys: narrowed}, len(narrowed) != 0
}

// Rule resolves the two roles one rule is identified by. The rule names the
// role once and receives its whole identity tuple, so the two forms cannot be
// resolved against two different roles.
func (roles Roles) Rule(role string) (RuleSemantics, bool) {
	rule, ruleOK := roles.Key(RoleKey("rule/" + role))
	operand, operandOK := roles.Key(RoleKey("operand/" + role))
	semantics := RuleSemantics{Rule: rule, Operand: operand}
	return semantics, ruleOK && operandOK && semantics.Available()
}

// Transformed resolves the three roles a rule whose output is normalized before
// admission is identified by.
func (roles Roles) Transformed(role string) (TransformedRuleSemantics, bool) {
	base, baseOK := roles.Rule(role)
	transform, transformOK := roles.Key(RoleKey("transform/" + role))
	semantics := TransformedRuleSemantics{RuleSemantics: base, Transform: transform}
	return semantics, baseOK && transformOK && semantics.Available()
}

// Key derives one global semantic role.  The framing and domain are part of
// the stable preimage and intentionally match the historical analysis key
// derivation.
func Key(role string) (identity.SemanticKey, bool) {
	if role == "" {
		return identity.SemanticKey{}, false
	}
	hash := sha256.New()
	var version [8]byte
	binary.BigEndian.PutUint64(version[:], SemanticFormat)
	if !writeFramedHash(hash, []byte("analysis/global-schema")) || !writeFramedHash(hash, version[:]) || !writeFramedHash(hash, []byte(role)) {
		return identity.SemanticKey{}, false
	}
	var digest [32]byte
	if sum := hash.Sum(digest[:0]); len(sum) != len(digest) {
		return identity.SemanticKey{}, false
	}
	return identity.NewSemanticKey(digest, SemanticFormat)
}

func writeFramedHash(hash interface{ Write([]byte) (int, error) }, value []byte) bool {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	first, firstErr := hash.Write(size[:])
	second, secondErr := hash.Write(value)
	return firstErr == nil && secondErr == nil && first == len(size) && second == len(value)
}
