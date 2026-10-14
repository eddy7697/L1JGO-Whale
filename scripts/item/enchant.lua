-- Enchant scroll formula (Java ref: Enchant.java)
-- ctx.scroll_bless: 0=blessed, 1=normal, 2=cursed  (對齊 Java/Go DB 慣例)
-- ctx.enchant_lvl:  current enchant level of target equipment
-- ctx.safe_enchant: safe enchant level from item template
-- ctx.category:     1=weapon, 2=armor
-- ctx.weapon_chance: config rate (0.0-1.0, default 0.10 = Eddy EnchantChanceWeapon 10)
-- ctx.armor_chance:  config rate (0.0-1.0, default 0.10 = Eddy EnchantChanceArmor 10)
--
-- Returns: { result = "success"/"nochange"/"break"/"minus", amount = N }
--   success:  +amount enchant levels
--   nochange: intense light but nothing happens
--   break:    equipment destroyed (normal scroll +9+ / cursed scroll <= -7)
--   minus:    -amount enchant levels (cursed scroll)

-- Java RandomELevel: blessed scrolls can give +1/+2/+3 at low enchant levels.
-- At +0~2: 31%(+1), 44%(+2), 24%(+3)
-- At +3~5: 50%(+1), 50%(+2)
-- At +6+ or normal scroll: always +1
local function random_e_level(enchant_lvl, scroll_bless)
    -- 祝福卷軸（bless=0）才會給 +1/+2/+3 變動量
    if scroll_bless == 0 then
        if enchant_lvl <= 2 then
            local j = math.random(1, 100)
            if j <= 31 then return 1
            elseif j <= 76 then return 2
            else return 3
            end
        elseif enchant_lvl >= 3 and enchant_lvl <= 5 then
            if math.random(1, 100) <= 50 then return 2
            else return 1
            end
        end
    end
    return 1
end

function calc_enchant(ctx)
    -- Cursed scroll (bless == 2): always -1, break at <= -7
    if ctx.scroll_bless == 2 then
        if ctx.enchant_lvl <= -7 then
            return { result = "break", amount = 0 }
        end
        return { result = "minus", amount = 1 }
    end

    -- Below safe enchant: always succeed
    if ctx.enchant_lvl < ctx.safe_enchant then
        local amount = random_e_level(ctx.enchant_lvl, ctx.scroll_bless)
        return { result = "success", amount = amount }
    end

    -- At or above safe enchant: level-dependent formula
    -- Config values are 0.0-1.0 floats; convert to Java-style integer (0-100)
    local rnd = math.random(1, 100)
    local chance
    local e_lvl = ctx.enchant_lvl

    if ctx.category == 1 then
        -- WEAPON formula (Java: Enchant.java scrollOfEnchantWeapon)
        -- Java 為固定值，不隨目前強化等級變化：
        --   +8 以下 (100 + 3C) / 3，+9 以上 (100 + 3C) / 6
        local wc = math.floor(ctx.weapon_chance * 100 + 0.5) -- Eddy rates.properties = 10
        if ctx.enchant_lvl >= 9 then
            chance = math.floor((100 + 3 * wc) / 6)
        else
            chance = math.floor((100 + 3 * wc) / 3)
        end
    else
        -- ARMOR formula (Java: Enchant.java lines 156-182)
        -- Eddy rates.properties EnchantChanceArmor = 10
        local ac = math.floor(ctx.armor_chance * 100 + 0.5)
        -- Bone/black mithril correction (safe_enchant == 0)
        if ctx.safe_enchant == 0 then
            e_lvl = ctx.enchant_lvl + 2
        end
        if e_lvl <= 0 then e_lvl = 1 end -- safety
        if ctx.enchant_lvl >= 9 then
            chance = math.floor((100 + e_lvl * ac) / (e_lvl * 2))
        else
            chance = math.floor((100 + e_lvl * ac) / e_lvl)
        end
    end

    -- Three-outcome system at +9+ (Java: success / nochange / break)
    if rnd < chance then
        -- Success
        local amount = random_e_level(ctx.enchant_lvl, ctx.scroll_bless)
        return { result = "success", amount = amount }
    elseif ctx.enchant_lvl >= 9 and rnd < (chance * 2) then
        -- No-change zone (only at +9+): intense light, nothing happens
        return { result = "nochange", amount = 0 }
    else
        -- Failed: equipment breaks (normal and blessed alike)
        return { result = "break", amount = 0 }
    end
end
