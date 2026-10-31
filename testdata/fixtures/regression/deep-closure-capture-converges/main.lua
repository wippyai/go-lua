local M = {}

function M.run()
    local result = nil
    local function level_0()
        local function level_1()
            local function level_2()
                local function level_3()
                    local function level_4()
                        local function level_5()
                            local function level_6()
                                local function level_7()
                                    local function level_8()
                                        local function level_9()
                                            local function level_10()
                                                local function level_11()
                                                    result = { count = 1 }
                                                end
                                                level_11()
                                            end
                                            level_10()
                                        end
                                        level_9()
                                    end
                                    level_8()
                                end
                                level_7()
                            end
                            level_6()
                        end
                        level_5()
                    end
                    level_4()
                end
                level_3()
            end
            level_2()
        end
        level_1()
    end
    level_0()
    return result
end

return M
