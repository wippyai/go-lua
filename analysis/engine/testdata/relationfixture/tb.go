package testfixture

// TB is the narrow failure surface shared by fixture tests and standalone
// probe processes. *testing.T satisfies it without making the fixture depend
// on testing.TB's unimplementable private method.
type TB interface {
	Helper()
	Fatal(...any)
	Fatalf(string, ...any)
}
