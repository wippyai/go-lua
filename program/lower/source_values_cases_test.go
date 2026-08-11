package lower_test

// valuesSourceCases is the values vertical's complete atomic source witness
// set. Each case is consumed directly by the exact source-to-Program law.
var valuesSourceCases = []sourceCase{
	{ID: "values.case.nil", Form: "NilExpr", Source: "local x = nil", Line: 1},
	{ID: "values.case.false", Form: "FalseExpr", Source: "local x = false", Line: 1},
	{ID: "values.case.true", Form: "TrueExpr", Source: "local x = true", Line: 1},
	{ID: "values.case.number.integer", Form: "NumberExpr", Source: "local x = 1", Line: 1},
	{ID: "values.case.number.float", Form: "NumberExpr", Source: "local x = 1.5", Line: 1},
	{ID: "values.case.string", Form: "StringExpr", Source: "local x = 's'", Line: 1},
	{ID: "values.case.vararg.open", Form: "Comma3Expr", Source: "local function f(...)\n  return ...\nend", Line: 2},
	{ID: "values.case.vararg.scalar", Form: "Comma3Expr", Source: "local function f(...)\n  return (...)\nend", Line: 2},
	{ID: "values.case.identifier.read", Form: "IdentExpr", Source: "local x = 1\nlocal y = x", Line: 2},
	{ID: "values.case.attr.dot", Form: "AttrGetExpr", Source: "local t = { x = 1 }\nlocal y = t.x", Line: 2},
	{ID: "values.case.attr.index-exact", Form: "AttrGetExpr", Source: "local t = {}\nlocal y = t[1]", Line: 2},
	{ID: "values.case.attr.index-dynamic", Form: "AttrGetExpr", Source: "local t = {}\nlocal k = 1\nlocal y = t[k]", Line: 3},
	{ID: "values.case.assignment", Form: "AssignStmt", Source: "local t = {}\nlocal first = 1\nlocal second = 2\nt[first], t[second] = 10, 20", Line: 4},
	{ID: "values.case.values.return-list", Form: "ReturnStmt", Source: "return 1, 2", Line: 1},
	{ID: "values.case.table", Form: "TableExpr", Source: "local t = {}", Line: 1},
	{ID: "values.case.table-field.name", Form: "Field", Source: "local t = {\n  x = 1,\n}", Line: 2},
	{ID: "values.case.table-field.exact", Form: "Field", Source: "local t = {\n  [1] = 2,\n}", Line: 2},
	{ID: "values.case.table-field.dynamic", Form: "Field", Source: "local k = 1\nlocal t = {\n  [k] = 2,\n}", Line: 3},
	{ID: "values.case.table-field.list-scalar-final", Form: "Field", Source: "local t = {\n  1,\n}", Line: 2},
	{ID: "values.case.table-field.list", Form: "Field", Source: "local function f(...)\n  local t = {\n    ...,\n  }\nend", Line: 3},
	{ID: "values.case.table-field.list-prefix", Form: "Field", Source: "local function f(...)\n  local t = {\n    ...,\n    1,\n  }\nend", Line: 3},
}
