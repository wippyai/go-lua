package grammarproof

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program/internal/grammarproof/astcodec"
)

// ObserveSource parses one accepted source through the public parser and
// returns every concrete compiler-AST value and its exported field states.
// It is cold proof support only. The returned observations are never consumed
// by Program construction and cannot define a requirement denominator.
func ObserveSource(text, name string) ([]ASTOccurrence, error) {
	statements, err := parse.ParseString(text, name)
	if err != nil {
		return nil, err
	}
	return astcodec.Observe(statements), nil
}

// RequireObservedState verifies an exact constructor-field-state source witness.
// It is intentionally a verifier, not a discovery API: callers must derive
// their required rows independently from the parser schema.
func RequireObservedState(occurrences []ASTOccurrence, constructor, field string, state FieldState) error {
	for _, occurrence := range occurrences {
		if occurrence.Type != constructor {
			continue
		}
		for _, observed := range occurrence.Fields {
			if observed.Name == field && observed.State == state {
				return nil
			}
		}
	}
	return fmt.Errorf("grammar proof source has no ast.%s.%s state %d", constructor, field, state)
}
