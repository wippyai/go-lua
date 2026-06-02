package canonical_test

import "testing"

func TestAttrGetValueReturnNarrowsBySiblingErr(t *testing.T) {
	src := `
type User = {id: string, email: string}
local users: {[string]: User} = {}
local function get_email(id: string): (string?, string?)
    local u = users[id]
    if u then
        return u.email, nil
    end
    return nil, "not found"
end
local email, err = get_email("u1")
if err == nil then
    local e: string = email
end
`
	requireCanonicalClean(t, src)
}
