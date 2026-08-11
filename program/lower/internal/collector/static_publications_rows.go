package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	programstatic "github.com/wippyai/go-lua/program/static"
)

func (rows *staticRows) TypePublication(term, assign keyspace.Term, pair uint32, target keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypePublication, len(rows.publications)); err != nil {
		return err
	}
	if assign == 0 || target == 0 {
		return errors.New("program/lower/collector: incomplete TypePublication")
	}
	rows.publications = append(rows.publications, programstatic.Publication{Assign: assign, Pair: pair, Target: target})
	return nil
}
