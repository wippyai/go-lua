package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
)

// The rule-delegation payload format. It is the second payload this package
// owns, and it owns it for the same reason it owns the export path: the payload
// is nothing but an identity. A member whose result selection is driven by an
// arbitrary caller literal cannot be enumerated as contract data, so the
// contract names the rule that owns the computation instead of pretending to
// carry it, and naming is all this format does. A form whose payload carries a
// TYPE stays with the layer that owns the type.
//
// The identity written is the authored key of the rule surface entry. A key is
// a construction input everywhere else in the declaration table, and it is one
// here too: the reader resolves it against the sealed rule surface, so the
// delegation names an entry and never restates what that entry does.
const (
	delegationDomain  = "analysis/library/contract/rule-delegation"
	delegationVersion = 1
)

// EncodeRuleDelegation writes one rule reference as a member payload body. The
// result is a complete framed stream: a payload is decodable on its own, so a
// reader that holds a member and its declared format never needs the enclosing
// instance to interpret it.
func EncodeRuleDelegation(rule schema.Key) ([]byte, error) {
	if !rule.Available() {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, delegationDomain, delegationVersion); err != nil {
		return nil, err
	}
	if err := writer.String(string(rule)); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodeRuleDelegation reads one rule-delegation payload body.
func DecodeRuleDelegation(data []byte) (schema.Key, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return "", ErrMalformed
	}
	if err := reader.Header(delegationDomain, delegationVersion); err != nil {
		return "", ErrMalformed
	}
	key, err := reader.String(maxKey)
	if err != nil {
		return "", ErrMalformed
	}
	if err := reader.Finish(); err != nil {
		return "", ErrMalformed
	}
	rule := schema.Key(key)
	if !rule.Available() {
		return "", ErrMalformed
	}
	return rule, nil
}

// RuleDelegationEntryID derives the declaration-table identity one delegation
// names. It states once which surface a delegation resolves against, so a reader
// resolving a member never restates the surface the reference belongs to.
func RuleDelegationEntryID(rule schema.Key) schema.EntryID {
	return schema.NewEntryID(schema.SurfaceKindRule, rule)
}
