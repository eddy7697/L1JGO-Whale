# Java 對齊交接：屬性強化／象牙塔／飾品卷軸（audit 1-8、1-9）

- 分支：`codex/java-enchant-scrolls`（工作樹 `E:\L1JGO-Whale-enchant-scrolls`，基於 master `26e6f23`）
- 提交：`0a79f88` feat: Java parity for attribute, ivory tower and accessory enchant scrolls；後續修正（tag 15 MR 併入飾品 m_def、防具 header grade 改送 Java `getGrade()`、機率門檻註解）與本文件同在其後一個提交（`git log codex/java-enchant-scrolls -2`）。
- Java 權威：`E:\L1JTW-Eddy\src\l1j\server\server`（`clientpackets/C_ItemUSe.java`、`model/item/action/Enchant.java`、`model/L1Attack.java`、`model/Instance/L1ItemInstance.java`、`model/L1EquipmentSlot.java`、`db/l1jtw_InnoDB.sql` armor.grade、`config/rates.properties` AttrEnchantChance=10）
- 未推送、未合併 master、未修改 audit 與 live checklist；由整合者統一收尾。

## 一、變更摘要

| 範圍 | 檔案 | 說明 |
|---|---|---|
| 分派 | `internal/handler/item.go`（卷軸區塊）、`internal/handler/context.go` | 41429-41432／49311／49312／49148 在一般 dai/zel 衝裝之前依 itemId 分派到 `ItemUseManager.EnchantSpecialScroll`（Java C_ItemUSe 對這些 itemId 皆 `readD()` 目標）。49148 YAML `use_type: del` 不再是「無處理」。只動卷軸區塊，料理／薪分支未碰。 |
| 卷軸邏輯 | `internal/system/item_enchant_scrolls.go`（新） | 三套流程 + `applySuccessEnchant`／`applyFailureEnchant`（對應 Java `SuccessEnchant`／`FailureEnchant`），提供 `EnchantSpecialScrollWithRoll` 供測試注入 1-100 亂數。 |
| ID／公式 | `internal/world/enchant_scrolls.go`（新） | 卷軸 ID、屬性種類、象牙塔白名單、名稱前綴 `$6115-$6126`、`WeaponAttrEnchantDamage`（Java `calcAttrEnchantDmg`）、`NpcAttrResist`。 |
| 戰鬥 | `internal/system/attr_enchant_damage.go`（新）、`combat.go`、`pvp.go` | 屬性附加傷害併入 `AttackerDmgMod`／`AttackerBowDmgMod`（Java 在 `calcPcDamage` 與 `_weaponAddDmg` 同階段相加）。涵蓋 PC→NPC 近戰／弓、PC→木人、PC→PC 近戰／弓。 |
| 飾品加成 | `internal/world/inventory.go`（`InvItem.Accessory`）、`internal/system/equip.go` | 新 `AccessoryBonus{AddHP,AddMP,HPR,MPR,AddSP,MDef,EarthMr,FireMr,WaterMr,WindMr}`；`calcEquipStats` 於裝備中併入角色屬性（Java `L1EquipmentSlot.setArmor` + `hpRegenPerTick`）。 |
| 顯示 | `internal/handler/item.go` | `buildViewName` 已鑑定武器加屬性前綴（置於 `+N` 之前，Java `getNumberedName` 順序）；status bytes 的 HP(14)/MP(32)/SP(17)/MR(15)/HPR(37)/MPR(38) 併入飾品累積值（Java tag 15 = `getMr()` 含飾品 m_def），新增元素抗性 tag 27 火／28 水／29 風／30 地（模板 + 飾品值，Java 同）；防具 header 第 4 位元組改送 `getGrade()`（非飾品 -1 → 0xFF，原 Go 固定送 0）。 |
| 資料 | `internal/data/item.go`、`data/yaml/armor_list.yaml` | 新增 `ItemInfo.Grade`（缺省 -1；武器／etcitem 固定 -1）；由 Java `armor.grade` 注入 176 筆 `grade:`（Java 178 筆，21133/21134 Go 無此物品）。 |
| 存檔 | `internal/persist/migrations/040_accessory_enchant.sql`、`item_repo.go`、`warehouse_repo.go`、`enterworld.go`、`system/warehouse.go`、`world/state.go` | `character_items` 與 `warehouse_items` 各新增 10 個 `acc_*` SMALLINT NOT NULL DEFAULT 0 欄位；登入載入、登出 `SaveInventory`、倉庫存入／取出／快取皆帶 `Accessory`。 |
| 設定 | `internal/config/config.go`、`config/server.toml` | `[enchant] attr_chance = 0.10`（Eddy AttrEnchantChance=10）。`config/server.local.toml` 為未追蹤本機檔，若有覆寫 `[enchant]` 區段需自行補 `attr_chance`，缺省回退 0.10。 |

## 二、Java 行為依據與對齊細節

### 1-8 武器屬性強化卷軸 41429(風→8)/41430(地→1)/41431(水→4)/41432(火→2)
Java `Enchant.scrollOfEnchantWeaponAttr`：
1. 目標為空／非武器（type2≠1）／封印（bless≥128）→ 79，不消耗。
2. `safe_enchant < 0` → 1453，不消耗。
3. 同屬性且目前等級 ≥ 3 → 1453，不消耗。
4. `rnd = 1..100`，`ATTR_ENCHANT_CHANCE >= rnd` 成功：同屬性 +1，不同屬性改為新屬性 1 級；訊息 1410（%0 = log name）；寫回 kind/level 欄位。失敗 1411。
5. 成功或失敗都 `removeItem(scroll, 1)`。
- Go 一致；名稱／狀態更新用 `SendItemStatusUpdate` + `SendItemNameUpdate`，存檔走既有 `attr_enchant_kind/level`。
- 戰鬥效果（`L1Attack.calcAttrEnchantDmg`）：等級 1/2/3 → 1/3/5；PC 目標取對應屬性抗性，NPC 目標 `weak_attr == kind`（等值比較，非位元）→ resist -50；`resistFloor = int(0.32*|resist|)` 保號，係數 `1 - resistFloor/32`，傷害取整。Go 原先只有存檔欄位，戰鬥未接，本次補上。

### 1-9 象牙塔卷軸 49312（武器）／49311（防具）
Java `scrollOfEnchantWeaponIvoryTower`／`scrollOfEnchantArmorIvoryTower`：
- 類別不符／封印／非白名單 → 79，不消耗。
- 武器白名單 7,35,48,73,105,120,147,156,174,175,224；防具白名單 20028,20082,20126,20173,20206,20232,21138,21051-21056,21140,21141（Go 無 21138/21140/21141 物品，保留 ID 無害）。
- `enchantLevel < safe_enchant` → 先消耗卷軸再 `SuccessEnchant(+1)`（訊息 161，`$245`/`$252` + `$247`）；否則 79 不消耗。
- Java `scrollOfEnchantWeaponIvoryTower` 在 null 檢查前先取 `getItem()` 會 NPE，Go 直接回 79。
- 備註：Go 資料中象牙塔防具（修練者系列）`safe_enchant: 0`，表示 +0 起即無法用此卷軸；與 Java DB 一致，屬資料現況非程式差異。

### 1-9 飾品強化卷軸 49148
Java `scrollOfEnchantAccessory`：
- 目標為空／封印／非防具／type 不在 8-12（amulet, ring, belt, ring2, earring）／grade == -1 → 79，不消耗；強化等級 <0 或 ≥10 → 79。
- 成功門檻 `(50+lvl)/(lvl+1)+35`（整數除法；+0 門檻 85 … +9 門檻 40），`rnd(1..100) < chance` 嚴格小於才成功，故實際機率為門檻 −1 個百分點（+0 實際 84%、+9 實際 39%）。Go 照抄此判定。
- 成功：grade 0 上等四屬性 +1（+6 獎勵 HPR/MPR +1）；grade 1 中等 HP+2（獎勵 MR+1）；grade 2 初等 MP+1（獎勵 SP+1）；grade 3 特等台版未實裝、只 +1；其他 grade → 79 不消耗（Java 同，回傳前未刪卷軸）。之後 `SuccessEnchant(+1)`、`S_ItemStatus`、`saveEnchantAccessory`、消耗卷軸。
- 失敗：`FailureEnchant`（訊息 164、`$252`，負值 `$246`；物品整組刪除）後消耗卷軸。
- Go 裝備中：Java 直接 `pc.addMaxHp/addMaxMp/addSp/addMr/addFire…`；Go 由 `RecalcEquipStats` 讀取 `InvItem.Accessory` 差值套用，失敗時先 `UnequipSlot` 再移除。

### 保留的既有行為
- 一般衝裝公式（1-10，Lua `scripts/item/enchant.lua`）未動。
- `applySuccessEnchant` 沿用 Go 現有一般衝裝成功路徑的 GFX 2583（Java `SuccessEnchant` 無此特效）。**整合者決定：本批新增流程依 Java，由整合者在整合分支移除 `item_enchant_scrolls.go` 的 `sendEffectOnPlayer(sess, player.CharID, 2583)` 一行；一般 dai/zel 卷軸的既有 GFX 行為不動。** 本分支未改此行以避免重複修改。
- Java `SuccessEnchant` 對防具另送 `S_OwnCharAttrDef`；Go 的 `RecalcEquipStats` 送狀態／能力／魔法狀態封包替代。

## 三、驗證證據

在 `E:\L1JGO-Whale-enchant-scrolls`（提交 `0a79f88`）：

| 項目 | 結果 |
|---|---|
| `go build ./...` | 通過 |
| `go vet ./...` | 通過 |
| `go test ./internal/system/ ./internal/handler/ ./internal/world/ ./internal/data/ ./internal/persist/ ./internal/config/` | 全部 ok（system 90s） |
| `go test ./...` | 通過（exit 0，於 `0a79f88` 執行；修正提交後重跑 handler／system 受影響測試通過） |
| Docker/PostgreSQL 往返（整合者執行） | 通過：040 對舊資料零預設、10 個飾品欄位、武器屬性、四種倉庫、鑰匙及租約；容器已清理 |
| `git diff --check` | 通過（僅 autocrlf 警告） |

新增測試（`internal/system/item_enchant_scrolls_test.go`、`internal/handler/item_enchant_scrolls_test.go`）：
- 屬性卷軸：非武器 79 不消耗；safe<0 1453；成功 roll=10 設 kind/level 並消耗、1410、名稱前綴 `$6121`；失敗 roll=11 消耗不改值、1411；同屬性 2→3 後再用 1453 不消耗；不同屬性在 3 級重設為新屬性 1 級；封印 79。
- 象牙塔：非白名單武器 79 不消耗；象牙塔短劍 +5→+6 必成功並消耗、+6 再用 79 不消耗；武器卷軸不可用於防具；防具卷軸依 safe_enchant 與白名單。
- 飾品：門檻表 +0..+9 = 85,60,52,48,45,44,43,42,41,40（roll 85 失敗、84 成功）；非飾品／grade -1／武器／+10／封印皆 79 不消耗；初等 +1 MP+1、+6 獎勵 SP+1；中等 HP+2 MR+1；上等四屬性+HPR/MPR；失敗飾品消失且卷軸消耗、164；裝備中成功 MaxMP 立即 +1；裝備中失敗解除裝備並清除加成。
- 複製路徑：倉庫 row→cache→InvItem 往返保留 `Accessory`；既有丟地拾取與交易測試加入 `Accessory` 斷言（兩者傳遞同一 `*InvItem`）。
- 戰鬥：`WeaponAttrEnchantDamage` 對照表（含 -50 → 7、抗 100 → 0、-100 → 10）；NPC `weak_attr` 等值比較；PvP 取目標抗性；PvP 近戰／遠程 context 含附加傷害。
- 顯示／資料：名稱 `$6122 +7 長劍`、未鑑定或無屬性無前綴；status bytes 含 14/32/17/15/37/38/27-30；防具 header grade 位元組（20282 → 2、20028 → 0xFF）；YAML grade 載入（20282=2、20252=1、20250=0、20251=-1、武器/etc=-1）。

## 四、DB／整合測試需求（由整合者統一跑 Docker）

migration `040_accessory_enchant.sql`（goose）：
```
character_items / warehouse_items 各加：
acc_add_hp, acc_add_mp, acc_hpr, acc_mpr, acc_add_sp, acc_m_def,
acc_earth_mr, acc_fire_mr, acc_water_mr, acc_wind_mr  SMALLINT NOT NULL DEFAULT 0
```
建議往返案例：
1. 飾品 `Accessory{AddMP:2, AddSP:1}` + 武器 `attr_enchant_kind=8, level=2` → `SaveInventory` → `LoadByCharID` 欄位相等。
2. 倉庫 `Deposit` 帶 `Accessory` → `Load`／`LoadByCharName` 取回相等。
3. 舊資料列（migration 前）載入後 `Accessory` 全 0。

## 五、未涵蓋範圍

- Java `LogEnchantTable`（衝裝紀錄表）未實作，Go 僅寫 log。
- Go 無 `ring2` 類型資料；Java 的 ring2 僅存在於 type 映射。
- Java `L1CreateItem`（`.item` 第 5/6 參數指定屬性與等級）未在本批（audit 第三節 `.item` 另列）。
- `saveEnchantAccessory` 的逐欄位增量存檔：Go 以登出整批 `SaveInventory` 存，與其他欄位一致。
- 21133/21134（Java 有 grade）Go 無對應物品，未注入。

## 六、實機驗證步驟／預期／風險／備案

| # | 步驟 | 預期 | 風險 | 備案 |
|---|---|---|---|---|
| E-1 | `.item 41429` 取風卷軸，對長劍使用（可暫調 `attr_chance = 1.0`） | 訊息 1410，名稱變 `$6121 長劍`（風 1 級），卷軸 -1；重登仍保留 | 中：名稱字串代碼 `$6121-$6126` 依 Java，若客戶端顯示亂碼需核對字串表 | 改用純文字前綴或暫不顯示前綴 |
| E-2 | 同一把再用風卷軸至 3 級，第四次 | 第三次成功後第四次送 1453、卷軸不消耗 | 低 | — |
| E-3 | 風 3 級武器改用火卷軸 | 成功後變火 1 級 | 低 | — |
| E-4 | 對非武器／封印武器／天空之劍(65) 使用 | 79 或 1453，不消耗 | 低 | — |
| E-5 | 風 3 級武器打弱風 NPC（npc `weak_attr=8`）與普通 NPC | 浮動傷害約 +7 與 +5 | 中：若 Lua `dmg_mod` 有其他倍率，數值略有偏差 | 將附加傷害改為結果後相加 |
| E-6 | `.item 49312` 對象牙塔短劍(7) +5 與 +6 使用；對長劍使用 | +5→+6 並顯示 161 藍光；+6 與長劍回 79 不消耗 | 低 | — |
| E-7 | `.item 49311` 對修練者皮頭盔 +0 | 79（safe_enchant 0） | 低：資料現況 | 若希望可用，需改資料 safe_enchant |
| E-8 | `.item 49148` 對修練者銀戒指(20282) 使用數次 | 成功 161 銀光、戒指 +N、識別視窗 MP 增加；失敗 164 戒指消失 | 中：status bytes 新增 27-30 元素 tag 與防具 header grade 位元組（非飾品改送 0xFF）會影響所有防具識別視窗 | 若客戶端識別視窗異常：先將 header grade 改回 0 觀察，再移除 27-30 tag |
| E-9 | 裝備中戒指強化成功 | 角色 MaxMP 立即增加（狀態列）；重登後仍在 | 低 | — |
| E-10 | 裝備中戒指強化失敗 | 戒指從裝備欄與背包消失，MaxMP 回落 | 低 | — |
| E-11 | 強化過飾品存入個人倉庫再取出、丟地拾取、交易給他人並重登 | 加成與 +N 全部保留 | 低 | — |
| E-12 | 變形怪首領項鍊(20250) 強化至 +6 | 四屬性各 +6，+6 時 HPR/MPR +1 | 低 | — |

## 完整測試

`go test ./...` 於 `0a79f88` 全數通過（exit 0）；修正提交後 `go build ./...`、`go vet`、handler／system 受影響測試、`git diff --check` 通過。
