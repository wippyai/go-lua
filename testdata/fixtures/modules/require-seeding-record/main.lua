local provider = require("provider")
local answer: string = provider.answer -- expect-error: 42

return answer
