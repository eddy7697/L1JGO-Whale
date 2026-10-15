# 血盟戰同意流程與模擬戰自動休戰交接（audit 1-5、1-6）

- 分支：`codex/java-war-consent`，worktree `E:\L1JGO-Whale-war-consent`，基準 `master` cfb990e。
- Java 權威：`E:\L1JTW-Eddy` 的 `l1j.server.server` 套件（`C_War`、`C_Attr` case 217/221/222、`L1War`）。`docs/l1j-fg/` 下的反編譯版不是權威，未採用其盟徽／新手保護／攻城期間暫停等額外檢查。
- 本交接不修改 `docs/JAVA_PARITY_AUDIT.md`，由整合者統一更新。

## 變更摘要

| 檔案 | 內容 |
|---|---|
| `internal/system/war_sys.go` | 重寫 `C_War` 請求流程為 Java 順序；血盟戰改為對方盟主 `S_Message_YN` 同意；新增專用 pending 請求紀錄（`warConsentRequest`）、`HandleWarConsent`、`TickSimWar`、`SimWarTickSystem`、可替換時鐘 |
| `internal/handler/context.go` | `WarManager` 介面新增 `HandleWarConsent`、`TickSimWar`；`ActiveWar` 新增 `EndTime` |
| `internal/handler/attr.go` | `C_Attr` case 217/221/222 轉交 `deps.War.HandleWarConsent` |
| `internal/config/config.go`、`config/server.toml` | 新增 `[gameplay] sim_war_time`（預設 `"240m"`，零值回退 240 分鐘） |
| `cmd/l1jgo/main.go` | 註冊 `system.NewSimWarTickSystem(deps.War)`（PostUpdate 階段，與攻城 tick 同處） |
| `internal/system/war_consent_test.go` | 37 個測試：同意流程、拒絕、離線、換盟（含換到另一個非零血盟）、換盟主、重複請求、錯 mode、過期回覆、原戰已結束又開新戰、pending 覆蓋保護、無 pending 偽造回應、通用 TempID 被覆寫、計時邊界、攻城回歸 |

### 1-6 血盟戰同意流程（Java `C_War` → `S_Message_YN` → `C_Attr`）

請求端（`requestWar`）檢查順序與訊息 ID 依 Java：

1. 非王族 478 → 無血盟 272 → 非盟主 478 → 指定自己血盟靜默 → 找不到對方血盟靜默（Java 亦靜默，原 Go 送 218 已移除）。
2. 自己已在任一戰爭：宣戰 234；投降／休戰若不在戰爭中靜默。
3. 自己有城堡：宣戰 474；投降／休戰靜默。
4. 對方無城堡且君主等級 ≤ 15：232。對方有城堡且等級 < 25：475。
5. 對方有城堡 → 攻城戰流程（見下）；否則血盟戰流程：
   - 對方已在戰爭：宣戰 236（帶對方盟名）；投降／休戰須同一場戰爭否則靜默。
   - 對方盟主不在線：218（帶對方盟名）。
   - 設定對方盟主 `TempID = 請求者 CharID`，送 `S_Message_YN`：宣戰 217（盟名、角色名）、投降 221（盟名）、休戰 222（盟名）。**此時不成立戰爭。**

   - **Go 不使用通用 `TempID`**（Java 作法），改在 `WarSystem.pending`（key = 對方盟主 CharID）記錄 `warConsentRequest`：訊息類型、雙方 CharID／SessionID／ClanID，221/222 另綁定原 `ActiveWar` 實例。避免被寵物改名（325）等其他對話覆寫或誤用。
   - 同一接收者已有**有效**請求時：同一請求者同類型重送 → 重送對話框、保留原綁定；不同請求者或不同類型 → 忽略並寫 log，不覆蓋。舊請求已失效（請求者離線／換盟／原戰爭已結束）則由新請求取代。

回應端（`HandleWarConsent`，`C_Attr` 217/221/222）：

- 無 pending、或回應 mode 與 pending 類型不符（例如發 221 回 222）→ 靜默且不消耗請求。
- mode 相符即消耗（刪除）pending，重複回覆只生效一次。
- 消耗後再核對：回應者 session 相符；雙方仍以同一 session 在線；雙方 `ClanID` 與請求時相同（換到另一個非零血盟也視為失效）；雙方仍是各自血盟的盟主；221/222 的原戰爭仍在清單中且雙方仍參與。任一不符 → 靜默，不改動任何戰爭。
- 221/222 同意時結束的是**綁定的原戰爭實例**，不再以「回應者目前參與的戰爭」查找，原戰已結束又開新戰時不會誤套。
- `removeWar` 會清除綁定該戰爭的 pending；`TickSimWar` 每 tick 清除失效 pending。
- 拒絕：217 → 請求者收 236（帶回應方盟名）；221/222 → 237。
- 同意 217：建立 `WarType=2` 戰爭（攻方 = 請求者血盟、守方 = 回應者血盟），`EndTime = now + sim_war_time`，雙方線上盟員各收 `S_War(1, 攻, 守)`。
- 同意 221/222：找回應者參與的模擬戰，結束並送 `S_War(2|3, 請求者盟, 回應者盟)` 給請求者盟員；回應者盟員另收 `S_War(4, 回應者盟, 請求者盟)`（Java `RequestSimWar`）。

刻意保留／補強的差異：

- **同意 217 時加一道保險**：若等待期間任一方已與第三盟開戰，忽略並寫 log（Java 不檢查，會成立第二場）。
- **模擬戰封包只送雙方盟員**（Java 行為）；攻城戰仍沿用 Go 既有的全伺服器 `BroadcastWarPacket`，未動。
- 原 Go 的「宣戰需有盟徽（1041）」檢查已移除，Java 權威版無此檢查。
- Java 以通用 `TempID` 記住請求者並在回應時只查該 ID；Go 改用專用 pending 紀錄並核對雙方身分、血盟與原戰爭（見上）。正常操作流程與 Java 相同，只是在換盟／換盟主／重登／錯 mode 等邊界時拒絕套用。
- Java `S_Message_YN` 的 D 欄位為 `GameServer.getYesNoCount()`；Go 既有 `SendYesNoDialog` 只在 252 交易時帶序號，其餘為 0，沿用既有做法（需實機確認 217/221/222 對話框顯示，見下）。

### 攻城戰流程（順帶對齊，行為上仍立即生效）

- 非攻城時間：宣戰 476（原 Go 182）。
- 任一線上盟員在戰區內：477（原 Go 無此檢查）。
- 對方已有攻城戰：加入攻方並廣播；投降／休戰須同一場。無則建立新攻城戰。
- 投降／休戰保留 Go 既有語意：只移除該攻方，其他攻方續戰（Java `RequestCastleWar` 任一攻方投降會刪整場戰爭，屬 Java 怪異行為，未照抄；整合者可視需要另立項目）。

### 1-5 模擬戰 240 分鐘自動休戰

- `ActiveWar.EndTime` 在 `startSimWar` 設定；`TickSimWar` 每 tick 檢查 `WarType==2 && EndTime.Before(now)`，到期呼叫 `endSimWar(w, 3, 攻方, 守方)`，等同 Java `SimWarTimer` 的 `CeaseWar(_param1, _param2)`。
- 邊界：Java 條件為 `before`，故恰好 240 分不觸發、240 分 + 1 秒觸發（測試 `TestSimWarAutoCeaseAt240Minutes`）。
- 攻城戰 `EndTime` 為零值，不受此 tick 影響（`TestSimWarTickDoesNotTouchCastleWar`）。
- 伺服器重啟後戰爭清單本來就不持久化（Go 既有），計時隨之消失，與 Java 一致。

## 驗證結果

- `go build ./...` 通過；`go vet ./...` 通過。
- `go test ./internal/system/ ./internal/handler/ ./internal/config/ -count=1` 全部通過（system 87s、handler 4.4s、config 0.5s）；`-run 'SimWar|CastleWar'` 37 個測試通過。
- 無 DB 需求，未使用 Docker。

## 實機待驗證（建議登記為第八節 V-10）

| 編號 | 驗證步驟 | 預期結果 | 失敗時備案 | 風險 |
|---|---|---|---|---|
| V-10（1-5、1-6） | 1. 兩個無城堡血盟，盟主皆為 16 級以上王族並在線。2. A 盟主對 B 盟宣戰。3. B 盟主先按「否」，再由 A 重新宣戰後按「是」。4. 戰爭中 A 盟主對 B 投降，B 按「是」。5. 重新開戰後把 `sim_war_time` 設為 `"2m"` 重啟測試，等待到期。6. 以 15 級王族、對方盟主離線各試一次宣戰。 | 步驟 2 不立刻開戰，B 盟主畫面跳出「A 血盟的 X 想與你的血盟開戰，是否接受？」；按「否」A 收到「B 血盟拒絕你的宣戰」(236)；按「是」雙方盟員收到宣戰訊息且血盟成員名字變敵對色。步驟 4 B 盟主跳出「A 血盟希望投降」(221)，按「是」後雙方收到結束訊息。步驟 5 兩分鐘後雙方自動收到戰爭結束訊息，伺服器 log 有「血盟戰時間到，自動休戰」。步驟 6 分別顯示 232、218。 | 若對話框不出現：抓包確認 `S_OPCODE_YES_NO`(219) 後 H type 為 217/221/222，必要時比照 252 改帶遞增序號；若按「是」無反應：確認 `C_Attr` 送來的 mode 與 pending 類型一致（伺服器 log 會記「戰爭回應類型與待回應請求不符」），且回應者未在對話期間重登或換盟；若封包僅盟員可見有誤：改回 `BroadcastWarPacket` 全伺服器廣播。 | 中 |

## 給整合者

- 與 `codex/java-parity-companions-elf` 的衝突點僅 `internal/handler/context.go`（我改 `WarManager` 介面與 `ActiveWar`，位於第 692-722 行附近）與 `cmd/l1jgo/main.go`（我在 `NewCastleWarTickSystem` 註冊後加一行），區塊不同，應可自動合併。
- 提交順序：`2000759`（初版，依賴通用 TempID）→ 第二個提交補強為專用 pending 紀錄；整合時取分支最新提交即可。
- 未合併 `master`、未清理 worktree、未推送遠端，依協作約定交由整合者處理。
