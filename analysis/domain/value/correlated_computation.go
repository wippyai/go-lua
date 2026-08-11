package value

import (
	"crypto/sha256"
	"encoding/binary"

	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programstatic "github.com/wippyai/go-lua/program/static"
)

// UnaryNot is Value's owner-fenced interpretation of one Program Unary term.
// Program owns the causal terms; Link supplies only shard-to-Program and
// Program-term-to-Value topology.
type UnaryNot struct {
	schema  *Schema
	shard   linkproject.Shard
	term    keyspace.Term
	content keyspace.ContentID
	result  Coordinate
	operand Coordinate
}

func (schema *Schema) UnaryNot(shard linkproject.Shard, term keyspace.Term) (UnaryNot, bool) {
	if schema == nil || schema.source == nil {
		return UnaryNot{}, false
	}
	p, programOK := schema.source.Project().Mounts().Program(shard)
	if !programOK || p == nil {
		return UnaryNot{}, false
	}
	_, op, operandTerm, unaryOK := p.Flow().Authored().Operators().Unaries().Get(term)
	if !p.Flow().Executable().Contains(term) || !unaryOK || op != flowkind.UnaryNot {
		return UnaryNot{}, false
	}
	resultValue, resultOK := schema.source.Boundary().Values().Of(shard, term)
	operandValue, operandOK := schema.source.Boundary().Values().Of(shard, operandTerm)
	result, resultCoordinateOK := schema.CoordinateFor(resultValue)
	operand, operandCoordinateOK := schema.CoordinateFor(operandValue)
	shardIndex, shardIndexOK := schema.source.Project().Mounts().Index(shard)
	content := unaryNotContent(schema.source.ContentID(), uint64(shardIndex+1), term)
	if !shardIndexOK || !resultOK || !operandOK || !resultCoordinateOK || !operandCoordinateOK || !content.Available() {
		return UnaryNot{}, false
	}
	return UnaryNot{schema: schema, shard: shard, term: term, content: content, result: result, operand: operand}, true
}

func (operand UnaryNot) valid() bool {
	if operand.schema == nil || !operand.content.Available() {
		return false
	}
	expected, ok := operand.schema.UnaryNot(operand.shard, operand.term)
	return ok && expected == operand
}

func (schema *Schema) OwnsUnaryNot(operand UnaryNot) bool {
	return schema != nil && operand.schema == schema && operand.valid()
}

func (operand UnaryNot) ID() (keyspace.ContentID, bool) {
	if !operand.valid() {
		return keyspace.ContentID{}, false
	}
	return operand.content, true
}

func (operand UnaryNot) Endpoints() (result, input Coordinate, ok bool) {
	if !operand.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	return operand.result, operand.operand, true
}

// SelectBranch is Value's one truth-conditioned interpretation of an existing
// Program Select. The branch ordinal is Value-local; Program Select supplies
// the actual causal operand geometry.
type SelectBranch struct {
	schema       *Schema
	shard        linkproject.Shard
	term         keyspace.Term
	content      keyspace.ContentID
	branch       uint8
	truthy       bool
	chosenIsLeft bool
	result       Coordinate
	left         Coordinate
	chosen       Coordinate
}

func (schema *Schema) SelectBranch(shard linkproject.Shard, term keyspace.Term, branch int) (SelectBranch, bool) {
	if schema == nil || schema.source == nil || branch < 0 || branch > 1 {
		return SelectBranch{}, false
	}
	p, programOK := schema.source.Project().Mounts().Program(shard)
	if !programOK || p == nil {
		return SelectBranch{}, false
	}
	_, op, leftTerm, rightTerm, selectOK := p.Flow().Authored().Operators().Selects().Get(term)
	if !p.Flow().Executable().Contains(term) || !selectOK {
		return SelectBranch{}, false
	}
	var chosenTerm keyspace.Term
	var truthy, chosenIsLeft bool
	switch op {
	case flowkind.SelectAnd:
		if branch == 0 {
			chosenTerm, truthy = rightTerm, true
		} else {
			chosenTerm, chosenIsLeft = leftTerm, true
		}
	case flowkind.SelectOr:
		if branch == 0 {
			chosenTerm, truthy, chosenIsLeft = leftTerm, true, true
		} else {
			chosenTerm = rightTerm
		}
	default:
		return SelectBranch{}, false
	}
	resultValue, resultOK := schema.source.Boundary().Values().Of(shard, term)
	leftValue, leftOK := schema.source.Boundary().Values().Of(shard, leftTerm)
	chosenValue, chosenOK := schema.source.Boundary().Values().Of(shard, chosenTerm)
	result, resultCoordinateOK := schema.CoordinateFor(resultValue)
	left, leftCoordinateOK := schema.CoordinateFor(leftValue)
	chosen, chosenCoordinateOK := schema.CoordinateFor(chosenValue)
	shardIndex, shardIndexOK := schema.source.Project().Mounts().Index(shard)
	content := selectBranchContent(schema.source.ContentID(), uint64(shardIndex+1), term, uint8(branch))
	if !shardIndexOK || !resultOK || !leftOK || !chosenOK || !resultCoordinateOK || !leftCoordinateOK || !chosenCoordinateOK || !content.Available() {
		return SelectBranch{}, false
	}
	return SelectBranch{schema: schema, shard: shard, term: term, content: content, branch: uint8(branch), truthy: truthy, chosenIsLeft: chosenIsLeft, result: result, left: left, chosen: chosen}, true
}

func (operand SelectBranch) valid() bool {
	if operand.schema == nil || operand.branch > 1 || !operand.content.Available() {
		return false
	}
	expected, ok := operand.schema.SelectBranch(operand.shard, operand.term, int(operand.branch))
	return ok && expected == operand
}

func (schema *Schema) OwnsSelectBranch(operand SelectBranch) bool {
	return schema != nil && operand.schema == schema && operand.valid()
}

func (operand SelectBranch) ID() (keyspace.ContentID, bool) {
	if !operand.valid() {
		return keyspace.ContentID{}, false
	}
	return operand.content, true
}

func (operand SelectBranch) Endpoints() (result, left, chosen Coordinate, truthy, chosenIsLeft bool, ok bool) {
	if !operand.valid() {
		return Coordinate{}, Coordinate{}, Coordinate{}, false, false, false
	}
	return operand.result, operand.left, operand.chosen, operand.truthy, operand.chosenIsLeft, true
}

// ValueClaim is Value's identity-preserving interpretation of a Program
// ValueClaim. Program owns the closed claim grammar and static target.
type ValueClaim struct {
	schema  *Schema
	shard   linkproject.Shard
	term    keyspace.Term
	content keyspace.ContentID
	result  Coordinate
	operand Coordinate
	kind    flowkind.ValueClaimKind
	target  programstatic.StaticTypeRef
}

func (schema *Schema) ValueClaim(shard linkproject.Shard, term keyspace.Term) (ValueClaim, bool) {
	if schema == nil || schema.source == nil {
		return ValueClaim{}, false
	}
	p, programOK := schema.source.Project().Mounts().Program(shard)
	if !programOK || p == nil {
		return ValueClaim{}, false
	}
	_, operandTerm, kind, claimOK := p.Flow().Authored().Claims().Get(term)
	if !p.Flow().Executable().Contains(term) || !claimOK {
		return ValueClaim{}, false
	}
	var reference programstatic.StaticTypeRef
	switch kind {
	case flowkind.ValueClaimTypeAs, flowkind.ValueClaimTypeColonColon:
		target, targetOK := p.Static().Operands().Claims().Target(term)
		if !targetOK {
			return ValueClaim{}, false
		}
		var staticOK bool
		reference, staticOK = p.Static().StaticTypes().Ref(target)
		if !staticOK {
			return ValueClaim{}, false
		}
	case flowkind.ValueClaimNonNil:
		if target, targetOK := p.Static().Operands().Claims().Target(term); targetOK && target != 0 {
			return ValueClaim{}, false
		}
	default:
		return ValueClaim{}, false
	}
	resultValue, resultOK := schema.source.Boundary().Values().Of(shard, term)
	operandValue, operandOK := schema.source.Boundary().Values().Of(shard, operandTerm)
	result, resultCoordinateOK := schema.CoordinateFor(resultValue)
	operand, operandCoordinateOK := schema.CoordinateFor(operandValue)
	shardIndex, shardIndexOK := schema.source.Project().Mounts().Index(shard)
	content := valueClaimContent(schema.source.ContentID(), uint64(shardIndex+1), term)
	if !shardIndexOK || !resultOK || !operandOK || !resultCoordinateOK || !operandCoordinateOK || !content.Available() {
		return ValueClaim{}, false
	}
	return ValueClaim{schema: schema, shard: shard, term: term, content: content, result: result, operand: operand, kind: kind, target: reference}, true
}

func (claim ValueClaim) valid() bool {
	if claim.schema == nil || !claim.content.Available() {
		return false
	}
	expected, ok := claim.schema.ValueClaim(claim.shard, claim.term)
	return ok && expected == claim
}

func (schema *Schema) OwnsValueClaim(claim ValueClaim) bool {
	return schema != nil && claim.schema == schema && claim.valid()
}

func (claim ValueClaim) ID() (keyspace.ContentID, bool) {
	if !claim.valid() {
		return keyspace.ContentID{}, false
	}
	return claim.content, true
}

func (claim ValueClaim) Endpoints() (result, operand Coordinate, ok bool) {
	if !claim.valid() {
		return Coordinate{}, Coordinate{}, false
	}
	return claim.result, claim.operand, true
}

func (claim ValueClaim) Kind() (flowkind.ValueClaimKind, bool) {
	if !claim.valid() {
		return 0, false
	}
	return claim.kind, true
}

// StaticTarget exposes the Static-owned reference for typed claims.
func (claim ValueClaim) StaticTarget() (programstatic.StaticTypeRef, bool) {
	if !claim.valid() || claim.target.Term() == 0 {
		return programstatic.StaticTypeRef{}, false
	}
	return claim.target, true
}

func unaryNotContent(linkID keyspace.ContentID, shard uint64, term keyspace.Term) keyspace.ContentID {
	var payload [56]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("val-not!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shard))
	binary.BigEndian.PutUint64(payload[48:56], uint64(term))
	return sha256.Sum256(payload[:])
}

func selectBranchContent(linkID keyspace.ContentID, shard uint64, term keyspace.Term, branch uint8) keyspace.ContentID {
	var payload [64]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("val-sel!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shard))
	binary.BigEndian.PutUint64(payload[48:56], uint64(term))
	payload[56] = branch
	return sha256.Sum256(payload[:])
}

func valueClaimContent(linkID keyspace.ContentID, shard uint64, term keyspace.Term) keyspace.ContentID {
	var payload [56]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("val-clm!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shard))
	binary.BigEndian.PutUint64(payload[48:56], uint64(term))
	return sha256.Sum256(payload[:])
}
