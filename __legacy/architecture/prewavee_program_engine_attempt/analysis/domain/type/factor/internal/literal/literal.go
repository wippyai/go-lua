// Package literal admits the finite set of Program literal labels used by the
// Type Factor. Link Value is the only occurrence identity after Link Seal.
package literal

import (
	"errors"
	"fmt"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/semantic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
)

type Entry struct {
	Value  link.Value
	Handle typedomain.Handle
}

type Set struct{ entries []Entry }

func Admit(source *link.Link, table *typedomain.Table) (*Set, error) {
	if source == nil || table == nil || table.Sealed() || !source.ContentID().Available() {
		return nil, errors.New("typedomain: invalid literal admission authority")
	}
	result := &Set{}
	for shardIndex := 0; shardIndex < source.ShardCount(); shardIndex++ {
		shard, ok := source.ShardAt(shardIndex)
		if !ok {
			return nil, errors.New("typedomain: malformed Link shard sequence")
		}
		p, ok := source.Program(shard)
		if !ok || p == nil {
			return nil, fmt.Errorf("typedomain: missing Program for shard %d", shard)
		}
		if err := result.admitProgram(source, shard, p, table); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func Declare(solver *engine.Solver, source *link.Link, table *typedomain.Table, universe *origin.Universe, factor *engine.Factor[link.Value, carrier.Value], set *Set) bool {
	if solver == nil || source == nil || table == nil || !table.Sealed() || universe == nil || factor == nil || set == nil {
		return false
	}
	for index := 0; index < set.Count(); index++ {
		entry, ok := set.At(index)
		if !ok {
			return false
		}
		shard, term, ok := source.ValueOrigin(entry.Value)
		if !ok {
			return false
		}
		value, ok := entry.Carrier(table, universe)
		if !ok || !declare(solver, source, factor, shard, term, entry.Value, value) {
			return false
		}
	}
	return true
}

// Carrier projects one admitted literal into the Factor's canonical terminal
// value. Literals carry Data only: source provenance belongs to the later
// Cell/Read transfer slice, not to expression evaluation. Admission records
// only a cold Handle; the hot value is constructed after both authorities
// have sealed.
func (entry Entry) Carrier(table *typedomain.Table, universe *origin.Universe) (carrier.Value, bool) {
	if table == nil || !table.Sealed() || !table.Valid(entry.Handle) {
		return carrier.Value{}, false
	}
	pack, ok := table.Closed(entry.Handle)
	if !ok {
		return carrier.Value{}, false
	}
	return carrier.New(table, universe, pack, origin.Empty())
}

func declare(solver *engine.Solver, source *link.Link, factor *engine.Factor[link.Value, carrier.Value], shard link.Shard, term program.Term, value link.Value, terminal carrier.Value) bool {
	rule, ok := engine.DeclareRule(solver, factor, semantic.Literal(source, value), func(binding *engine.RuleBinding) bool {
		return binding.At(shard, term)
	}, func(access engine.Access[link.Value, carrier.Value]) bool {
		return access.Set(value, terminal)
	})
	return ok && engine.WriteExact(rule, value)
}

func (set *Set) Count() int {
	if set == nil {
		return 0
	}
	return len(set.entries)
}
func (set *Set) At(index int) (Entry, bool) {
	if set == nil || index < 0 || index >= len(set.entries) {
		return Entry{}, false
	}
	return set.entries[index], true
}

func (set *Set) admitProgram(source *link.Link, shard link.Shard, p *program.Program, table *typedomain.Table) error {
	if err := set.admitNil(source, shard, p, table); err != nil {
		return err
	}
	if err := set.admitBool(source, shard, p, table); err != nil {
		return err
	}
	if err := set.admitInteger(source, shard, p, table); err != nil {
		return err
	}
	if err := set.admitFloat(source, shard, p, table); err != nil {
		return err
	}
	return set.admitString(source, shard, p, table)
}

func (set *Set) append(source *link.Link, shard link.Shard, term program.Term, handle typedomain.Handle, table *typedomain.Table) error {
	value, ok := source.ValueOf(shard, term)
	if !ok || !table.Valid(handle) {
		return errors.New("typedomain: invalid admitted literal")
	}
	set.entries = append(set.entries, Entry{Value: value, Handle: handle})
	return nil
}

func (set *Set) derive(source *link.Link, shard link.Shard, term program.Term, value typ.Type, table *typedomain.Table) error {
	handle, err := table.DeriveClosed(value)
	if err != nil {
		return fmt.Errorf("typedomain: admit literal term %d: %w", term, err)
	}
	return set.append(source, shard, term, handle, table)
}

func (set *Set) admitNil(source *link.Link, shard link.Shard, p *program.Program, table *typedomain.Table) error {
	for index := 0; index < p.NilCount(); index++ {
		term, ok := p.NilAt(index)
		if !ok {
			return errors.New("typedomain: malformed Nil sequence")
		}
		if err := set.append(source, shard, term, table.Nil(), table); err != nil {
			return err
		}
	}
	return nil
}
func (set *Set) admitBool(source *link.Link, shard link.Shard, p *program.Program, table *typedomain.Table) error {
	for index := 0; index < p.BoolCount(); index++ {
		term, ok := p.BoolAt(index)
		if !ok {
			return errors.New("typedomain: malformed Bool sequence")
		}
		_, value, ok := p.Bool(term)
		if !ok {
			return errors.New("typedomain: malformed Bool row")
		}
		if err := set.derive(source, shard, term, typ.LiteralBool(value), table); err != nil {
			return err
		}
	}
	return nil
}
func (set *Set) admitInteger(source *link.Link, shard link.Shard, p *program.Program, table *typedomain.Table) error {
	for index := 0; index < p.IntegerCount(); index++ {
		term, ok := p.IntegerAt(index)
		if !ok {
			return errors.New("typedomain: malformed Integer sequence")
		}
		_, value, ok := p.Integer(term)
		if !ok {
			return errors.New("typedomain: malformed Integer row")
		}
		if err := set.derive(source, shard, term, typ.LiteralInt(value), table); err != nil {
			return err
		}
	}
	return nil
}
func (set *Set) admitFloat(source *link.Link, shard link.Shard, p *program.Program, table *typedomain.Table) error {
	for index := 0; index < p.FloatCount(); index++ {
		term, ok := p.FloatAt(index)
		if !ok {
			return errors.New("typedomain: malformed Float sequence")
		}
		_, value, ok := p.Float(term)
		if !ok {
			return errors.New("typedomain: malformed Float row")
		}
		if err := set.derive(source, shard, term, typ.LiteralNumber(value), table); err != nil {
			return err
		}
	}
	return nil
}
func (set *Set) admitString(source *link.Link, shard link.Shard, p *program.Program, table *typedomain.Table) error {
	for index := 0; index < p.StringCount(); index++ {
		term, ok := p.StringAt(index)
		if !ok {
			return errors.New("typedomain: malformed String sequence")
		}
		_, value, ok := p.String(term)
		if !ok {
			return errors.New("typedomain: malformed String row")
		}
		if err := set.derive(source, shard, term, typ.LiteralString(value), table); err != nil {
			return err
		}
	}
	return nil
}
