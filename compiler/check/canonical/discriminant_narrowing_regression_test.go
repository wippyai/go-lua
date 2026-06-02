package canonical_test

import (
	"testing"
)

func TestDiscriminantNarrowingRegression(t *testing.T) {
	cases := map[string]string{
		"ok-truthy-value": `
type Result<T> = {ok: true, value: T} | {ok: false, error: string}
type Greeting = {message: string, user_name: string}
local function use(g: Result<Greeting>)
    if g.ok then
        local msg: string = g.value.message
    end
end
return use
`,
		"not-ok-error": `
type Result<T> = {ok: true, value: T} | {ok: false, error: string}
type Greeting = {message: string, user_name: string}
local function use(g: Result<Greeting>)
    if not g.ok then
        local e: string = g.error
    end
end
return use
`,
		"eq-true-discriminant": `
type Result<T> = {ok: true, value: T} | {ok: false, error: string}
type Greeting = {message: string, user_name: string}
local function use(g: Result<Greeting>)
    if g.ok == true then
        local msg: string = g.value.message
    end
end
return use
`,
		"string-tag-generic": `
type Node<T> = {kind: "leaf", payload: T} | {kind: "branch", left: string}
local function use(n: Node<string>)
    if n.kind == "leaf" then
        local p: string = n.payload
    end
end
return use
`,
		"sibling-err-nil": `
local function getEmail(): (string?, string?)
    return "a", nil
end
local function use()
    local email, err = getEmail()
    if err == nil then
        local e: string = email
    end
end
return use
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			requireCanonicalClean(t, src)
		})
	}
}
