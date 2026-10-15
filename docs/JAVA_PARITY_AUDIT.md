# Java 功能同步盤點（L1JTW-Eddy → L1JGO-Whale）

本文件是 Go 版與 Java 權威版（`E:/L1JTW-Eddy`，`l1j.server.server` 套件）的功能對照總表，供多人共工時認領與追蹤。

- 盤點日期：2026-10-13。方式為唯讀交叉閱讀兩邊原始碼與資料，未實機驗證。
- 對照基準：Java 版行為即權威。Go 端刻意偏離的地方也列為差異，需要保留時請在「備註」寫明原因。
- 範圍排除（使用者決定）：**商城點數**（`custompoint/`、`.cpadd` `.cpquery` `.cpshopreload`、`S_CustomPointShopSellList`）不搬移；**自動練功機器人**（`AutoLvBot*`、`S_AutoSellList`、`S_MonsterExcludeList`、`S_SkillSelectList`）日後再處理。
- Go 檔案路徑均相對於 `internal/`，行號為盤點當日。

## 如何更新本表

1. 認領項目時，把「狀態」改為 `進行中` 並在「備註」填分支名稱（`codex/<任務>`）。
2. 完成並合併回 `master` 後，備註改為提交 SHA，狀態依下列規則擇一：
   - 行為可由單元／整合測試完整確認者，改為 `完成`。
   - 行為涉及客戶端顯示、封包 opcode、介面互動等需實機確認者，改為 `待驗證`，並在第八節「實機待驗證清單」補一行。
3. 實機驗證通過後，把第一至三節的狀態改為 `完成`，第八節該列的「結果」填通過日期；失敗則依「失敗時備案」修正，修正後重新驗證。
4. 確認 Go 行為刻意不同且要保留時，改為 `保留差異` 並寫原因。
5. 發現表上錯誤（例如其實已實作），直接修正並在最下方「修訂紀錄」補一行。

狀態值：`待辦`、`進行中`、`待驗證`、`完成`、`保留差異`、`低優先`。

### 對齊階段的實作原則

- **先實作，後實機驗證。** 需要客戶端確認的項目不因缺乏實機環境而停在 `待辦`；依 Java 行為實作、通過自動化測試並合併後標 `待驗證`，實機驗證集中另排。
- **有多個協定選項時，先依 Java 選一個預設，並在第八節記錄備案。** 例如 opcode 數值有疑義時，採 Java 值實作，備案寫明若客戶端拒絕時改用的替代值或原 Go 做法，實測失敗只需切換，不必重新設計。
- **影響客戶端相容性的項目標為高風險。** 封包 opcode 或格式猜錯可能導致客戶端斷線，這類項目在第八節「風險」欄標 `高`，實測時單獨執行，不與其他項目的結果混在一起。
- **行為取捨不屬於實機驗證。** 像武器衝裝率（1-10）、PK 扣業（1-21）這類「是否保留 Go 行為」的決策，需由使用者先拍板，再進入實作；不得以 `待驗證` 代替決策。

## 總覽

| 面向 | Java 數量 | 完整 | 部分 | 缺少 | 備註 |
|---|---|---|---|---|---|
| 客戶端封包 `C_*` | 88 | 50 | 23 | 4 | 另 6 個 Java 未接線、3 個 Java opcode > 255 永遠不會觸發 |
| GM 指令 | 59 | 55 | 4 | 0 | Java 另有 `crazy`/`save`/`system` 三個指令類別不存在 |
| 伺服器封包 `S_*` | 173 | 多數 | 約 10 | 約 12 | 另有 3 個送錯格式、2 個 opcode 數值需實機驗證 |
| 世界模型與計時器 | 約 100 | 約 55 | 約 25 | 約 20 | 整套缺：UB、屠龍、Boss 循環、地獄、村莊、盟屋稅 |
| 資料表 | 76 張 SQL | 多數已轉 YAML | 製作/寶箱/升級為測試資料 | 6 張無對應 | 見第五節 |
| 設定鍵 | 約 140 | 約 60 | — | — | 多數玩法開關寫死或不存在，見第六節 |

---

## 一、現有功能的缺陷（最高優先：會卡玩法或行為錯誤）

| # | 項目 | Java 行為 | Go 現況 | 狀態 | 備註 |
|---|---|---|---|---|---|
| 1-1 | 攻城守護塔不重生 | `L1WarSpawn.SpawnTower` 於攻城結束與奪城後刪除舊塔並重生（亞丁含 4 座子塔） | 已補 `CastleSystem.SpawnTower`：攻城結束與王冠奪城時刪除戰爭區內所有守護塔（含屍體）後重生；亞丁生成 81189 與 81190-81193。順帶把亞丁主塔的可攻擊條件由「4 座子塔全毀」改為 Java 的「至少 3 座」 | 待驗證 | 2026-10-14 合併，單元測試涵蓋肯特／亞丁；客戶端顯示見第八節 V-1 |
| 1-2 | 攻城排程在啟動時停住 | `WarTimeController.checkWarTime`：結束時間已過且不在戰爭中 → 下次攻城 = 現在 + WarInterval | `TickWar` 新增過期分支，推到 `now + WarInterval` 並寫回 DB；結束時間以 WarTime 推算 | 完成 | Java 此分支把結束時間設成與開始相同（實際永遠開不了戰），Go 不照抄。Java 的 `WarUpdate` 在推進時同時重設稅率 10%／寶庫歸零，Go 只推進時間，是否照抄需使用者拍板 |
| 1-3 | 攻城間隔 | `WarInterval` 預設 4 天、`WarTime` 預設 2 小時 | 新增 `[gameplay] war_interval`（預設 `"96h"`）與 `war_time`（預設 `"2h"`），零值回退 Java 預設 | 完成 | Go 設定以 Duration 字串表示，`4d` 須寫成 `96h` |
| 1-4 | 攻城結束清理 | 刪王冠、守護塔並重生塔；開戰與奪城趕人用 `getGetBackLoc` 隨機回城點且排除 GM | 攻城結束刪除戰爭區王冠與塔後重生；`castles.yaml` 新增各城 `getback` 清單（抄自 `L1TownLocation` GETBACK_LOC_*，狄亞得用 Java 固定 3 點），趕人隨機取一、heading 5、`IsGM()` 者不趕 | 待驗證 | 2026-10-14 合併；回城點見第八節 V-2 |
| 1-5 | 模擬戰自動休戰 | 240 分鐘後自動結束 | 新增 `ActiveWar.EndTime` 與 `SimWarTickSystem`；`[gameplay] sim_war_time` 預設 `"240m"`，嚴格超過期限的第一個 tick 自動休戰，不影響攻城戰 | 完成 | `2000759`；可替換時鐘測試確認 240 分鐘邊界、僅結束一次及攻城隔離；客戶端通知併入 V-10 |
| 1-6 | 血盟戰同意流程 | 宣戰／投降／休戰需對方盟主 `S_Message_YN` 217/221/222 同意；檢查 Lv ≤ 15（232）、戰區內（477）、已在戰爭中（234/236） | 已接 217/221/222 同意流程與 Java 檢查順序；專用 pending 綁雙方角色／連線／血盟／盟主資格、訊息類型及原模擬戰，拒絕過期、重複與錯類型回應；攻城仍直接生效 | 待驗證 | `2000759`、`0b7cb61`、`81ef17f`；38 個戰爭測試通過，見獨立清單 V-10。保留 Go 攻城單一攻方投降只移除該攻方的既有語意；共同攻方互送模擬戰投降／休戰不得影響攻城戰 |
| 1-7 | 寵物指定攻擊目標 | `C_SelectTarget`（opcode 223）回傳目標 | 已註冊／解碼 223，寵物所有權與攻擊／警戒模式檢查後，指定怪物、玩家、他人寵物或召喚物；AI 逐次重驗地圖／ShowID／安區與目標狀態；`S_SelectTarget` 改 Java opcode 54 + D + 三個 C(0) | 待驗證 | `6dd2163`；封包、四類目標、保護／反射／死亡與比賽回歸通過；見獨立清單 V-11。保留既有簡化戰鬥數值；非怪物 NPC、食物、全域 NonPvP 與召喚物反擊寵物仍未擴張 |
| 1-8 | 屬性強化卷軸 | 41429-41432 走 `scrollOfEnchantWeaponAttr` | `data/yaml/etcitem_list.yaml:32228` 標 `use_type: dai`，被當一般武器卷軸；屬性值只有存檔欄位 | 進行中 | 第二批：`codex/java-parity-items-inn` 統一整合 |
| 1-9 | 象牙塔與飾品卷軸 | 49312 等象牙塔卷軸、49148 飾品卷軸各有獨立流程 | 49312 標 `dai`；49148 標 `del` 無處理 | 進行中 | 第二批：`codex/java-parity-items-inn` 統一整合 |
| 1-10 | 武器衝裝成功率 | `(100+3C)/3`，+9 以上 `/6`；排除象牙塔裝備、試煉之劍、幻象武器 40128 | `scripts/item/enchant.lua` 改回 Java 固定值 `(100+3C)/3`、+9 以上 `/6`；`weapon_chance`／`armor_chance` 預設改為 0.10 對應 Eddy `rates.properties` 的 10 | 完成 | 2026-10-14 使用者拍板對齊；`scripting/enchant_rate_test.go` |
| 1-11 | 致盲藥水封包 | `S_CurseBlind`（op 47 + writeH） | 致盲藥水改走 `handler.SendCurseBlind`（op 47 + H type=1），與技能版共用 `BuildCurseBlind` | 待驗證 | 2026-10-14 合併；見第八節 V-4 |
| 1-12 | 伊娃的祝福（水底呼吸）圖示 | `S_SkillIconBlessOfEva`（op 126：D objectId + H time） | 新增 `S_OPCODE_BLESSOFEVA = 126` 與 `BuildSkillIconBlessOfEva`，水底呼吸藥水改送 op 126 | 待驗證 | 2026-10-14 合併；見第八節 V-5 |
| 1-13 | 等級上限道具提示 | `MSG_LEVEL_OVER`（PacketBox 12：C 0 + C level） | `checkLevelRestriction` 超過 `max_level` 時改送 `BuildPacketBoxLevelOver(max)`；低於下限仍送 318 | 待驗證 | 2026-10-14 合併；Java 註明 level 0-49 以外客戶端不顯示；見第八節 V-6 |
| 1-14 | 旅館房門傳送 | `Dungeon` 傳送前 `checkInnKey`，分房間／會議室 | `data/yaml/portal_list.yaml:41-48` 等旅館門口傳送點不檢查鑰匙 | 進行中 | 第二批：`codex/java-parity-items-inn` 統一整合 |
| 1-15 | 高等級經驗懲罰 | `ExpTable.getPenaltyRate`（50 級以上） | 新增 `ExpPenaltyRateLikeJava`（`system/calc_stat.go`）：50 級起依 Java `Lv50Exp`…`Lv99Exp` 預設除數表折減，`[rates] high_level_exp_divisors` 可覆寫；在 `awardNpcExpLikeJava` 與伺服器倍率、料理／藥水加成一同套用 | 完成 | 單元測試 `TestExpPenaltyRateLikeJava`、`TestHighLevelExpPenaltyLikeJava` |
| 1-16 | 組隊經驗分配 | `CalcExp` 隊伍加成與分配；善惡依仇恨分配 | 新增 `distributeNpcExpLikeJava`（`system/calc_exp.go`）：非隊員依仇恨領走；隊伍份額乘王族隊長 5.9%、每位可見隊員 4% 後依等級平方分給 killer 與可見隊員；善惡依玩家本人仇恨比例分配；寵物／召喚獸仇恨另記於 `NpcInfo.CompanionHate`（`AddCompanionHate`），寵物依自身仇恨份額取得經驗（乘 `exp_rate` 與 Go 額外的 `pet_exp_rate`），召喚獸份額依 Java 不發放；移除寵物擊殺時重複的整份經驗 | 完成 | 單元測試 `TestNpcExpSplitsByHateAndLawfulLikeJava`、`TestPartyExpDistributionLikeJava`、`TestPetAndSummonHateSharesLikeJava`。Go 的 `knownsObject` 以 `GetNearbyPlayersInShow` 近似 |
| 1-17 | 屬性衍生公式 | `CalcStat.calcAc`（DEX/等級 → AC）、`calcStatMr`（WIS → MR） | 新增 `CalcAcLikeJava`、`CalcStatMrLikeJava`、`CalcBaseMrLikeJava`（職業值 + 精神 + 等級/2）與 `ResetBaseAC`／`ResetBaseMR`（`system/calc_stat.go`，以 `PlayerInfo.BaseAC`／`BaseMR` 差值疊加）；進入世界、升級、GM `.level`、洗點、51 級配點、萬能藥、裝備與 buff 改變精神時重算，MR 改變時送 `S_SPMR` | 待驗證 | 第八節 V-8。公式由 `TestCalcAcLikeJava` 等測試鎖定；客戶端顯示與戰鬥平衡需實機確認 |
| 1-18 | 掉落倍率 | 乘全域倍率 × 地圖 `drop_rate`；`drop_item` 表單品倍率 | `GiveDrops` 機率改為 chance × `drop_rate` × 地圖 `drop_rate` × 單品 `drop_rate`，數量 min/max 乘單品 `drop_amount`；新增 `data/yaml/drop_item.yaml`（自 Java `drop_item` 轉出 76 筆，目前皆 1）與 `data.DropItemTable`，`sqlconv dropitem` 可重轉 | 完成 | 單元測試 `TestJavaGiveDropsAppliesMapAndItemRates`。保留差異：Java 查無地圖回 0，Go `map_list.yaml` 仍有缺漏地圖故查無時視為 1；`MonsterAmount` 屬生怪數量不在本項 |
| 1-19 | 生怪資料不一致 | `spawnlist` | `spawn_list.yaml` 與 Java DB 不符（例：45005 青蛙 Java 93 筆、Go 0 筆） | 待辦 | 需整批核對 |
| 1-20 | 料理製作 | 24 道、Lv1–3；營火 gfx 5943 檢查（1160）；90/5/5 成功／特級／失敗 | 料理分支已補製作路由、24 道配方及營火，待本批整合 | 進行中 | `codex/java-cooking-parity`；食用效果沿用既有實作 |
| 1-21 | PK 扣業 | 無此行為 | `system/pvp.go` 已移除 PK 扣 100 karma | 完成 | 2026-10-14 使用者拍板對齊；`pk_parity_test.go` |
| 1-22 | PK 懲罰（地獄） | `L1PcHellMonitor`：PK 達 10 次關進地圖 666 倒數；`C_Restart` 依 hellTime 重生 | 新增 `system/pk_hell.go`：PK 達門檻 `BeginHell(true)` 依 Java 公式設秒數（≤門檻 300 秒，之後每次 +300）、傳送 666 (32701,32777) 並送 552；`PvPSystem.Update` 每秒倒數、死亡不倒數，歸零傳回火田村回城點；重生（`death.go`）與進入世界續算並送 637；`hell_time` 欄位持久化（migration 038） | 待驗證 | 第八節 V-9。倒數／傳送／門檻由 `pk_parity_test.go` 鎖定；紅字訊息與地獄地圖落點需實機確認 |
| 1-23 | 登入與在線狀態 | `C_AuthLogin` 在 DB online=1 時回 7、不踢舊；357 只用於 DB 離線但記憶體仍在線的例外路徑；啟動重設與在線上限 | 已補啟動重設、帳號持有者、原子登記、斷線重試、上限及選角保護，詳見 `docs/LOGIN_ONLINE_STATE.md` | 完成 | `89f2e10`；已通過 build／vet／完整測試與 Claude 審查，合併回本機 `master` |
| 1-24 | 盟屋拍賣結算 | 訊息 524/527/528；無人得標延後 5 天；更新血盟 has_house；重設 onSale/tax_deadline | `auction_sys.go:207-331` 不發訊息、只延到隔天午夜、`clan.HasHouse` 只改記憶體 | 待辦 | |
| 1-25 | 龍騎士覺醒 | 安塔瑞斯 +127 MaxHP/AC-12；法利昂 MR+30 四屬性+30；巴拉卡斯六屬性+5；變身 6894；每週期扣 8 MP 扣完解除；再施放解除 | `skill_buff.go:1291-1312`、`buffs.lua` 參照 yiwei 版，以上皆無 | 待辦 | |
| 1-26 | 地面物品擁有者保護 | `L1ItemOwnerTimer` | `GroundItem.OwnerTicks` 由 `TickGroundItems` 倒數並清除 `OwnerID`；撿取時他人遇保護回 623；釣魚背包滿／超重改為停止釣魚並把魚丟地上保護 10 秒（Java `successFishing`）；玩家手動丟棄不再設擁有者（Java 亦無） | 完成 | `pk_parity_test.go`。Java 只有釣魚使用此計時器 |
| 1-27 | 隱身後攻擊延遲 | `L1PcInvisDelay` 3 秒內不能攻擊／撿物／施法 | `PlayerInfo.InvisDelayTicks`（15 tick）：無所遁形術揭露自己、隱身中脫下隱身斗篷時啟動；期間近戰／遠攻／撿物忽略，施法僅允許 Java `CAST_WITH_INVIS` 清單（`system/invis_delay.go`）；撿物另補 Java 的隱身中不可撿取 | 完成 | `pk_parity_test.go`。2026-10-14 使用者拍板改為 Java 行為：隱身中近戰／遠攻／攻擊封包直接忽略且不解除隱身（`combat.go`），施法同 Java `isCastableStatus` 只允許 `CAST_WITH_INVIS` 清單、允許的技能不解除隱身（`skill.go`） |

## 二、整套未實作的系統

| # | 系統 | Java 位置 | Go 現況 | 狀態 | 備註 |
|---|---|---|---|---|---|
| 2-1 | 精靈屬性 | `C_NPCAction` fire/water/air/earth/init；`MSG_ELF`（PacketBox 15）；`S_DelSkill`（op 160） | 已接 NPC 選擇／重置：僅精靈且須先清除舊屬性；重置移除 129–176 中屬性技能、還原單屬性防禦（147）；新增 migration 039 的 `elf_attr`，屬性與魔法清單原子保存，登入、選角、斷線、定時與 GM 存檔皆保留 | 待驗證 | `6219364`；NPC 路由／四屬性／重置／失敗不改狀態回歸及 PostgreSQL 往返通過；客戶端訊息、技能視窗與召喚見獨立清單 V-12 |
| 2-2 | 無限大戰 UB | `L1UltimateBattle`、`L1UbSpawn`、`L1UbPattern`、`UbTimeController`、`ub_*` 表、`C_ExitGhost`、`L1PcGhostMonitor`、NPC 動作 par/info/sco | 無 | 待辦 | |
| 2-3 | 屠龍副本 | `L1DragonSlayer`：龍之鑰匙 → 龍門 → 地圖 1005/1011 三階段 → 隱藏之谷；`L1DragonPortalInstance` | `dragondoor_sys.go` 是另一套（雷歐門衛開橋）；`npc_list.yaml` 有 13 筆 `L1DragonPortal` 無處理 | 待辦 | |
| 2-4 | Boss 循環生成 | `L1BossCycle`、`L1BossSpawn`、`BossCycle.xml`、`spawnlist_boss` | 無 | 待辦 | |
| 2-5 | 限時生成 | `spawnlist_time`、`L1SpawnTime`；`near_spawn`；min/max 重生隨機；`spawn_type`、`delay_interval` | `data/npc.go:68-90` 只有單一 `respawn_delay` | 待辦 | |
| 2-6 | 村莊系統 | `HomeTownTimeController`、`L1TownLocation`、`town` 表；NPC 動作 set/clear/ask/get/townscore；村莊稅；`TOWN_TELEPORT`（PacketBox 176）；`C_SendLocation` 0x30 | 只有 GM `.hometown daily/monthly`（`gmcommand_admin.go:463`）；`world/gametime.go` 無 onDayChanged listener；`shop.go:81-100` 無村莊稅 | 待辦 | |
| 2-7 | 盟屋稅 | `HouseTaxTimeController` 欠稅收回送拍（10 萬底價、5 天）；管家 pay/payfee/expel/rem | `house_repo.go:69` `UpdateTaxDeadline` 從未被呼叫；管家只有 name/tel/upgrade/hall/agsell | 待辦 | |
| 2-8 | 裝備觸發魔法 | `L1EquipSpell`、`equip_spell_def`、`weapon_spell`、`armor_spell`、`spell_scroll`、`character_item_spell` | 無 | 待辦 | |
| 2-9 | 限時道具 | `ItemExpireTimeController`（`delete_day`/`unit_type`）、`L1EquipmentTimer`（`max_use_time`）、封印 `can_seal` | `InvItem` 無 deleteTime/remainingTime；封印只有顯示旗標（`handler/item.go:632`） | 待辦 | |
| 2-10 | 業／友好度 | `L1Karma`：NPC karma、擊殺增減、依業等級主動攻擊、NPC 兌換 | 只有欄位與 `S_Karma`（`pk.go:154`） | 待辦 | |
| 2-11 | 寵物保管所 | NPC 動作 depositnpc/withdrawnpc；`C_Result` type 12 領取寵物 | type 12 被強化商店佔用（`poweritem.go:72`）；`companion_pack.go:417` `sendPetList` 無呼叫 | 待辦 | |
| 2-12 | 經驗回復與贖罪 | NPC 動作 exp（YN 738）、pk 扣 PK 次數 | `npcaction.go:214` 只送頁面 | 待辦 | |
| 2-13 | 賽狗與變身賽跑 | `L1BugBearRace`、`L1PolyRace`、`S_Race`、`race_ticket`、YN 1256 | 無 | 低優先 | |
| 2-14 | 傭兵 | `C_HireSoldier`、NPC 動作 employ/arrange | `mercenary.go:11` stub | 低優先 | Java 本身也近乎空殼 |
| 2-15 | 公告 | `Announcements`（登入）、`AnnouncementsCycle`（循環） | 只有 GM `.marquee` | 待辦 | |
| 2-16 | NPC 依現實時間喊話 | `NpcChatTimeController`；`L1NpcChatTimer` 解除隱身時機 | `data/npcchat.go:15` `ChatTimingGameTime` 標「保留」；`ChatTimingHide` 無呼叫 | 待辦 | |
| 2-17 | 反作弊 | `AcceleratorChecker`（移動/攻擊/施法速度，用 `SprTable`）、`BadNamesList`、`L1ItemCheck`、`L1PcDeleteTimer`（死亡 10 分鐘斷線） | `movement.go:81-85` 只記錄時間；`createchar.go:40-44` 只檢查空字串；`item_ground.go:60-80` 只夾數量 | 待辦 | |
| 2-18 | 戰鬥特化 | `FightIsActive`：依 lawful/10000 給 AC/MR 或 Dmg/SP；`S_Fight`（PacketBox 0x72） | 無 | 待辦 | |
| 2-19 | 控制戒指通知 | `S_Ability`（op 36）傳送／變形／召喚控制 | 無 36 常數 | 待辦 | 傳送控制戒指選點介面可能出不來 |
| 2-20 | 頻道開關 | `C_LoginToServerOK`（opcode 26）全體／密語／交易／血盟／組隊開關 | 未註冊；無對應旗標 | 待辦 | |
| 2-21 | GM 傳送到玩家 | `C_CallPlayer`（opcode 185） | 未註冊；`.recall` 方向相反 | 待辦 | |
| 2-22 | 精靈森林守護者 | `L1GuardianInstance` 主動攻擊非精靈；精靈空手採集 40505/40506/40507/40519/40503 | `combat.go:1090` 只能被打，無 AI | 待辦 | |
| 2-23 | 護送任務 | `L1FollowerInstance`（調查員、安迪亞、卡米特、蜥蜴人、羅伊、女間諜） | `world/follower.go`、`companion_ai.go:442` 有框架，`AddFollower` 無呼叫 | 待辦 | |
| 2-24 | 其他小系統 | 精靈之石生成 `ElementalStoneGenerator`；娃娃產道具 `ItemMakeByDoll` 與 regist/sleep/sustain/blind 抗性；底比斯追擊 `L1Chaser`；NPC 自然回復 hpr；寵物飽食度 `L1PetFood`；便箋 `LetterTable`；家具持久化 `spawnlist_furniture`；血盟推薦持久化；神力／媽祖／戰鬥藥水 47000-47006；`magicdmgmodifier`；龍之血痕圖示；三段加速圖示 0x3c；登入 buff 子碼 21；`S_SignboardPack`；`S_NoSee`；`S_RuneSlot` 登入時發送；道具／技能音效 `S_Sound`；記錄表 log_chat/log_enchant/log_delete_item | 無 | 待辦 | 可拆成多個小任務 |

## 三、部分實作的封包與指令

### 客戶端封包

| Java 封包 | Go 位置 | 缺少 | 狀態 |
|---|---|---|---|
| `C_Attr` | `handler/attr.go` | 738（經驗回復）、1256（寵物競速）；223 聯盟為 stub。217/221/222 盟戰同意已接，見 1-6 | 待辦 |
| `C_Chat` | `handler/chat.go:24` | 頻道 13（聯合血盟）、14（聊天組隊）、17（王族公告）；聯盟頻道 Go 用 15、Java 用 13 | 待辦 |
| `C_Door` | `handler/door.go:11` | 鑰匙門 6006/6007（消耗 40163/40313，5 秒自動關）、水晶洞窟 5001-5010、本盟小屋判斷（`door.go:31` TODO） | 待辦 |
| `C_GiveItem` | `handler/pet_tame.go:14` | 交給一般 NPC／守衛／傳送師（`onGetItem`）、負重 942、娃娃 1181；馴服是消耗而非轉移物品 | 待辦 |
| `C_Mail` | `handler/mail.go:18` | 0x11 讀血盟信、0x21 寄血盟信、0x01 開血盟信箱 | 待辦 |
| `C_SendLocation` | `handler/windows.go:12` | 0x0b 傳座標給玩家（`S_SendLocation` 0x6f）、0x2e 盟徽開關、0x30 村莊傳送；龍門無 8-12 點禁用（1643）；0x22/0x27 書籤 TODO | 待辦 |
| `C_RestartMenu` | `handler/clan.go:131` | data 6 生存吶喊（ALT+0） | 待辦 |
| `C_Result` | `handler/shop.go:14` | type 12 領取寵物 | 待辦 |
| `C_Title` | `clan_sys.go:688` | GM 改任何人封號、訊息 196/201 | 待辦 |
| `C_EmblemUpload` | `clan_sys.go:783` | Java 允許階級 4 與 10，Go 只允許君主 | 待辦 |
| `C_PledgeContent` | `handler/clan.go:102` | 設定後無回應封包（`HTML_PLEDGE_REALEASE_ANNOUNCE` 168） | 待辦 |
| `C_AddBuddy` | `handler/buddy.go:27` | 重複好友訊息 1052 | 低優先 |
| `C_WarePassword` | `handler/warehouse.go:177` | 密碼變更／取消的系統訊息；血盟倉庫階級只擋階級 2 | 低優先 |
| `C_NPCAction` | `handler/npcaction.go:29` | 見第二節各系統；另有萊拉 `contract1*`、各城稅率頁 `pandora6` 等約 28 個動作掉到 `unhandled NPC action`（`npcaction.go:392`） | 待辦 |

### 伺服器封包

| Java 封包 | Go 現況 | 狀態 |
|---|---|---|
| `S_Disconnect` | 只有 GM 踢人（`gmcommand_admin.go:247`）；封鎖帳號、加速器、刪角計時器未送 | 待辦 |
| `S_ChatPacket` | 缺 14、17；聯盟頻道號不一致 | 待辦 |
| `S_Message_YN` | 缺 738/1256；217/221/222 已接（1-6，V-10） | 待辦 |
| `S_PacketBox` 子碼 | 缺 21、168、176；12（LEVEL_OVER）、15（ELF）已接（V-6、V-12）；80（WAR_IS_GOING_ALL）改逐城送 | 待辦 |
| `S_OwnCharStatus2`（155）、`S_NpcChangeShape`（230） | Go 以完整版替代，影響小 | 保留差異 |
| `S_Exp`（Go 113，每次擊殺送） | Java 無此封包（opcode 無效並已註解） | 待辦 | 需確認 3.81 客戶端是否接受 113 |

### GM 指令

| 指令 | Go 位置 | 缺少 | 狀態 |
|---|---|---|---|
| `.recall` | `gmcommand.go:897-911` | `all` 參數、略過 GM、傳到 GM 面前 2 格 | 待辦 |
| `.item` | `gmcommand.go:289-364` | 第 4 參數鑑定、第 5/6 參數武器屬性與等級、飾品依 grade 加成；`ItemCreateOptions`（`system/item_create.go:16`）無對應欄位；Go 另限制不可堆疊最多 10 個 | 待辦 |
| `.cleaning` | `gmcommand_admin.go:414-461` | 便箋 49016-49025 信件資料、家具 41383-41400 FurnitureSpawn 刪除 | 待辦 |
| `.weather` | `gmcommand.go:1327` | 允許 16 與其他數值 | 低優先 |
| `.who` / `.loc` | `gmcommand.go:873` / `:1150` | `S_WhoAmount`；原始 tile 值 | 低優先 |

### 其他部分實作

| 項目 | Go 現況 | 缺少 | 狀態 |
|---|---|---|---|
| 寵物／召喚物戰鬥剩餘對齊 | 1-7 已接手動指定怪物、玩家、寵物與召喚物；保留既有簡化傷害 | 非怪物 NPC 的手動目標、完整命中／傷害公式、召喚物反擊寵物 AI；食物與全域 NonPvP 仍分別見第五／六節 | 待辦 |
| 釣魚 | `fishing_sys.go:28-126` | 掉落 7 種（Java 23 種、分母 215）；每次 2×RATE_XP 經驗；訊息 1517；閃爍鱗片 47104 停止；負重停止並掉地 | 待辦 |
| 地面物品清除 | 每件物品各自 TTL | 全服 10 秒預告與完成廣播（166）；排除精靈之石 | 低優先 |
| Getback | `scripts/world/respawn.lua:4-129` | 死亡重生僅 6 張地圖、回家約 40 張；Java `getback` 398 筆含區域矩形、職業專屬村莊、3 點隨機；`getback_restart` 215 筆全缺 | 待辦 |
| 魔法娃娃 | `data/doll.go:82`、`system/doll.go` | `make_itemid`、regist_stone/sleep/sustain/blind | 待辦 |
| 任務 NPC | `npc_action_list.yaml` 固定 HTML | `L1QuestInstance` 依任務步驟切換 HTML | 待辦 |
| 稅金 | `shop.go:81-176` | 村莊稅 `_taxRatesTown` | 待辦 |
| 召喚設定 | `skill_summon.go` 硬編碼 `summonRingTable` | `summon_monster` 資料表 | 低優先 |
| 血盟推薦 | `clanmatching.go:36` 記憶體 | 持久化 repo（022 已建表） | 待辦 |
| 王冠奪城 | `castle_sys.go:542-629` | 奪城後刪舊塔生新塔 | 待辦 | 與 1-1 一併處理 |

## 四、已確認完整的範圍（不必重做）

客戶端封包 50 個（移動、攻擊、施法、交易、倉庫、佈告欄、書籤、好友、拒絕、隊伍、血盟基本操作、盟徽、城堡寶庫與稅率、拍賣、旅館、修理、魔法商店、個人商店、釣魚收竿、求婚、決鬥、洗點、快捷鍵）；GM 指令 55 個；NPC AI 與仇恨、怪物技能、怪物群組、變身與進階變身、套裝、武器特效、溶解、陷阱 6 種、毒 4 種、娃娃、寵物（含裝備與進化）、鬼屋、寵物競賽、拍賣骨架、路燈日夜、關機倒數、IP 封鎖、郵件、佈告欄、旅館、立方與地面效果、HP/MP 自然回復、負重。

## 五、內容資料需轉檔（引擎已有）

| 資料 | Go 檔案 | Go 數量 | Java 數量 | 狀態 |
|---|---|---|---|---|
| 製作配方 | `data/yaml/item_making_list.yaml` | 6（標「測試資料」） | `ItemMaking.xml` 366 + `SingleItemMaking.xml` 168 | 待辦 |
| 寶箱 | `data/yaml/item_box.yaml` | 約 3（測試資料） | `TreasureBox.xml` 117 | 待辦 |
| 物品升級 | `data/yaml/item_upgrade.yaml` | 2（測試資料） | — | 待辦 |
| 任務 | `data/yaml/quests.yaml` | 13 | `C_NPCAction` 內多條 | 待辦 |
| 料理配方 | `handler/cooking.go` | 8 | 24 | 待辦 |
| 回城點 | `scripts/world/respawn.lua` | 約 46 張地圖 | `getback` 398、`getback_restart` 215 | 待辦 |
| 生怪 | `data/yaml/spawn_list.yaml` | 與 Java 不一致 | `spawnlist` | 待辦 |
| 物品欄位 | `data/item.go:276` | 缺 `delete_day`、`unit_type`、`cant_delete`、`can_seal`、`max_use_time`、`magicdmgmodifier`、`save_at_once`、`grade` | — | 待辦 |
| NPC 欄位 | `data/npc.go` | 缺 `hpr`、`hprinterval` | — | 待辦 |
| 寵物欄位 | `migrations/016_pets.sql` | 缺 `food` | — | 待辦 |
| 無對應資料表 | — | `drop_item`、`spawnlist_boss`、`spawnlist_time`、`ub_*`、`race_ticket`、`letter`、`getback_restart` | — | 待辦 |

## 六、設定鍵缺口

Java `altsettings` / `rates` / `charsettings` / `fights` 約 140 鍵，Go `config/server.toml` 約 60 鍵。以下 Java 鍵在 Go 設定與程式碼中都找不到對應，部分可能寫死，需逐一決定是否開放：

`NonPvP`、`AutoLoot`、`NoviceProtectionIsActive`／`NoviceMaxLevel`／`ProtectionLevelRange`、`MaxClanMember`、`MaxPT`／`MaxChatPT`、`LootingRange`、`GDropItemTime`／`ItemDeletionTime`／`NpcDeletionTime`、`WarTime`／`WarInterval`、`HouseTaxInterval`、`ElixirMaxUses`／`ElixirStatCap`、`RateKarma`、`RateShopPurchasingPrice`／`RateShopSellingPrice`、`RateDropAdena` 與 `RateDropItems` 分離、`Lv50Exp`…`Lv99Exp`、各職業 `MaxHP`／`MaxMP`、`MaxHpMpValue`、`MaxDollCount`、`MaxPersonalInventoryItem`／`MaxPersonalWarehouseItem`／`MaxClanWarehouseItem`、`ClanAlliance`、`ReturnToNature`、`RevivalPotion`、`TalkingScrollQuest`、`GlobalChatLevel`／`WhisperChatLevel`、`WhoCommand`、`SpawnHomePoint*`、`ElementalStoneAmount`、`MagicStoneAttr`／`MagicStoneLevel`、`CreateChance*`、`AttrEnchantChance`、`FightIsActive`、`SimWarPenalty`、`InitBossSpawn`、`ShowDamageGfx`、`EnableHpMp32bit`。

## 七、尚未逐項比對

- `C_ItemUSe`（Java 5,425 行）只確認主分派路由，未逐分支比對。
- `C_NPCAction` 依 NPC ID 的任務分支，只確認 NPC 存在於 Go 程式或 YAML。
- `C_Trade` 系列、`C_UseSkill`、`C_LoginToServer`、`C_SkillBuy/OK`、`C_EnterPortal` 標「已實作」僅因處理器存在，未檢視內部邏輯。
- `S_Exp` 113 與 `S_SelectTarget` 54（本輪改採 Java 值，舊 Go 為 236）的 opcode 數值是否符合 3.81 客戶端，需實機驗證；寵物封包備案見 V-11。

## 八、實機待驗證清單

完整步驟、預期結果、風險、失敗時備案與結果欄已獨立移至 [JAVA_PARITY_LIVE_CHECKLIST.md](JAVA_PARITY_LIVE_CHECKLIST.md)。保留原 V-1～V-9 編號，新增本輪項目，第一至三節的既有引用仍適用。

需要客戶端確認的項目保持「待驗證」；實測通過後，同時更新本表狀態及獨立清單的結果。

## 建議順序

1. 第一節缺陷（攻城、盟戰、寵物目標、卷軸、封包格式、旅館鑰匙、經驗公式、掉落倍率）。
2. 第五節資料轉檔（製作、寶箱、回城點、生怪核對）。
3. 第二節系統，先做精靈屬性、盟屋稅、村莊、限時道具、業，再做 UB、屠龍、Boss 循環。

## 修訂紀錄

| 日期 | 內容 |
|---|---|
| 2026-10-13 | 初版盤點（唯讀稽核，基準 Java `E:/L1JTW-Eddy`、Go `master` d35ff80） |
| 2026-10-14 | 與 Claude Code 重讀登入實際呼叫鏈，修正 1-23 的正常重複登入描述，補登入／在線狀態實作與回歸。 |
| 2026-10-14 | 新增 `待驗證` 狀態、對齊階段實作原則與第八節「實機待驗證清單」；需實機確認的項目改為先實作合併、再集中驗證。 |
| 2026-10-14 | 封包格式 1-11～1-13：致盲藥水改送 `S_CurseBlind`（op 47）、水底呼吸圖示改送 `S_SkillIconBlessOfEva`（op 126）、等級上限改送 `MSG_LEVEL_OVER`（PacketBox 12）；皆 `待驗證`（第八節 V-4～V-6）。 |
| 2026-10-14 | 攻城叢集 1-1～1-4：守護塔刪除後重生（含亞丁子塔、主塔 3 座子塔門檻）、攻城結束刪王冠、趕人改隨機回城點並排除 GM、過期攻城時間往後推、`war_time`／`war_interval` 設定鍵。1-2、1-3 完成；1-1、1-4 待驗證（第八節 V-1～V-3）。 |
| 2026-10-14 | 第八節新增 V-7：更換 Login38 登入器後，啟用輔助情境的點擊移動與原生連續攻擊實機驗證。 |
| 2026-10-14 | PK 叢集 1-22、1-26、1-27：PK 地獄（門檻入獄、每秒倒數、重生／重登續算、火田村釋放、`hell_time` 持久化）、地面物品擁有者保護（釣魚 10 秒）、隱身解除 3 秒行動延遲。1-26、1-27 完成；1-22 待驗證（第八節 V-9）。1-27 另記 Java 隱身中忽略攻擊 vs Go 自動解除為待拍板差異。 |
| 2026-10-14 | 1-27 補齊：依使用者拍板移除 Go「攻擊／施法時自動解除隱身」，改為 Java 隱身中忽略攻擊、非 `CAST_WITH_INVIS` 技能靜默拒絕。 |
| 2026-10-14 | 1-21、1-10 依使用者拍板對齊 Java：PK 不再扣 karma；武器衝裝改回 Java 固定值公式，衝裝係數預設改為 Eddy 設定的 10。 |
| 2026-10-14 | 經驗／公式叢集 1-15～1-18：高等級經驗懲罰、Java 組隊／仇恨／寵物經驗分配、AC／MR 衍生公式、地圖與單品掉落倍率（新增 `drop_item.yaml`、`[rates] high_level_exp_divisors`）。1-15、1-16、1-18 完成；1-17 待驗證（第八節 V-8）。 |
| 2026-10-15 | 與 Claude 協作完成本輪 1-5、1-6、1-7、2-1：模擬戰計時／盟戰同意、寵物指定四類目標、精靈屬性選擇與重置及持久化。1-5 完成，其餘待客戶端驗證；實機清單獨立為 `JAVA_PARITY_LIVE_CHECKLIST.md`（保留 V-1～V-9，新增 V-10～V-12）。寵物完整戰鬥公式等剩餘差異另列待辦，不以實機通過視為已實作。 |
