local contract = {}

function contract.get(_id)
    return {
        with_context = function(self, _context)
            return self
        end,
        with_options = function(self, _options)
            return self
        end,
        open = function(self, _provider_id)
            return {}, nil
        end,
    }, nil
end

return contract
