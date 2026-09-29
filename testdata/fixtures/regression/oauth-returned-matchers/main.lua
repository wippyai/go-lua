local test = require("test")
local oauth_repo = require("oauth_repo")

-- From kickside.oauth.persist:oauth_repo_test:11-20.
local function expect(value: any): any
    return {
        to_be_nil = function() test.is_nil(value) end,
        not_to_be_nil = function() test.not_nil(value) end,
        to_equal = function(other: any) end,
        not_to_equal = function(other: any) end,
        to_be_true = function() end,
        to_match = function(pattern: string) end,
    }
end

local function check_profile(id: string)
    -- From oauth_repo_test:506-514.
    local connection, err = oauth_repo.get_connection(id)
    expect(err).to_be_nil()
    local update_data = {
        user_profile = {
            provider_user_id = connection.user_profile.provider_user_id,
            username = connection.user_profile.username,
            avatar_url = connection.user_profile.avatar_url,
        },
    }
    return update_data
end

local function check_credentials(id: string)
    -- From oauth_repo_test:589.
    local connection, err = oauth_repo.get_connection(id)
    expect(err).to_be_nil()
    return connection.client_credentials.client_id
end

local function check_provider(id: string)
    -- From oauth_repo_test:995.
    local connection, err = oauth_repo.get_connection(id)
    expect(err).to_be_nil()
    expect(connection.provider_specific.complex_nested).not_to_be_nil()
    return connection.provider_specific.complex_nested.array[4]
end

local function check_not_nil(id: string)
    local connection, err = oauth_repo.get_connection(id)
    expect(connection).not_to_be_nil()
    return connection.user_profile.provider_user_id
end

return { profile = check_profile, credentials = check_credentials, provider = check_provider, not_nil = check_not_nil }
