package lower_test

// sourceCase is test-only authored source evidence. It has no production
// registry, schema, claim, or lowering role.
type sourceCase struct {
	ID     string
	Form   string
	Source string
	Line   int
}
