# L1JGO-Whale 專案記憶

更新日期：2026-10-13（Asia/Taipei）。範圍為「盤點遊戲行為差異」對話的修正與調查交接，不代表已全面驗證所有遊戲行為。以下路徑、Git 狀態與版本是當時紀錄；接手時須重新確認。

## 對照基準與使用者要求

- 遊戲行為以本機 `E:\L1JTW-Eddy` 的 Java 實作為本次對照基準，不能只依 Go 現有行為推定正確。
- 最初要求只盤點，之後已明確授權開始修正，並同步到 `E:\L1JGO-Whale` 供重新啟動伺服器測試。
- 使用者確認：木乃伊詛咒由怪物施放；加速問題使用加速藥水與勇敢藥水，期間沒有重登。效果結束後必須還原，不應將重登視為正常修復方式。
- 開發遵循使用者提供的 AGENTS.md：專屬分支與工作樹、保留既有變更、驗證後合併本機 master、最後清理本任務工作樹。遠端推送與部署另依授權。

## 已完成的 Java 行為修正

提交：`f1fcea90438b07263009289d2de06612f1cd89d0`
訊息：`fix: align monster loot and status effects with Eddy Java`
狀態：已合併本機 `master`，並於 2026-10-13 同步到 `E:\L1JGO-Whale`。原任務工作樹 `java-gameplay-parity` 已封存。

| 項目 | 已實作行為 |
| --- | --- |
| 怪物掉落訊息 | 單人送系統訊息 143「怪物給你物品」，隊伍送 813 得主訊息，取代只有「獲得 XX」的文字。 |
| 掉落分配 | 依有效攻擊者仇恨選得主；自動分配隊伍拆分數量並保留餘數。背包滿／超重改落地並保留物品屬性；滿格仍可收入既有堆疊。掉落倍率 0 停止掉落，金幣倍率只調整數量。 |
| 怪物詛咒 | 木乃伊詛咒依 Eddy 延遲 8 秒後麻痺 16 秒；不再只有狀態圖示。 |
| 毒咒與致盲 | 毒咒每 3 秒扣 5 HP；致盲送視野封包；解毒藥清除實際毒狀態。 |
| 減益成功率 | 改用技能骰數／等級公式、MR 與對應狀態抗性；移除人為最低成功率，最高保留 90%。 |
| 加速／勇敢移除 | 自然到期及明確移除都還原伺服器速度與客戶端速度封包，保留裝備常駐加速及其他仍有效效果。 |
| 到期時間 | 增加實際時間截止檢查，避免主迴圈延遲拖長效果；存檔共用截止判斷。 |
| 藥水互斥與時長 | 綠水抵消緩速後不追加加速；有加速裝備時不建立藥水計時器、不消耗綠水；勇水不再誤移除力量提升，補齊血之渴望等互斥；祝福藍水／慎重藥水分別為 1200／600 秒。 |

主要入口：

- `internal/system/monster_loot.go`、`item_use.go`、`input.go`：掉落與藥水。
- `internal/system/npc_ai.go`、`skill_status.go`、`poison.go`、`skill_probability.go`：怪物減益與成功率。
- `internal/system/skill_speed.go`、`skill_buff.go`、`internal/world/state.go`、`internal/handler/buff_persist.go`：移除與到期。
- `internal/system/java_gameplay_*_test.go`、`internal/handler/buff_persist_expiry_test.go`：相關回歸。
- `CHANGELOG_NEXT.md` 的 2026-10-13 三筆紀錄包含詳細摘要。

當時驗證：獨立工作樹中 `go build ./...`、`go vet ./...`、全套 Go 測試通過；同步後再通過 build、vet 與 `TestJava|TestBuffSnapshot` 相關測試。真實客戶端效果仍需驗收，不能以單元測試通過宣稱全面對齊。

## 同步時保留的本機工作

- 使用者啟動目錄：`E:\L1JGO-Whale`，當時分支 `codex/gm-identity-commands` 已指向 `f1fcea9`。
- 保留未提交的 `README.md`、`CHANGELOG_NEXT.md` 與未追蹤的 `AGENTS.md`；不要覆蓋或把這些既有內容一併提交。
- 同步備份：`E:\L1JGO-Whale\.git\codex-backups\java-gameplay-sync-20261013-110751-8febe01e`。
- 當時 master 所在工作樹：`C:\Users\vince\.codex\worktrees\world-light-flicker\L1JGO-Whale`。後續操作先重新查 `git worktree list` 與狀態，不假定此位置一直存在。

## 尚未結案：Qwer 擊殺後整個遊戲關閉

使用者重新啟動後回報；確認是整個遊戲視窗關閉，角色為 Qwer，不是返回登入／選角。下列時間均為 2026-10-13 台灣時間；Docker log 為 UTC，需加 8 小時。

| 時間 | 已確認證據 |
| --- | --- |
| 11:10:19 | 啟動 `E:\lin-golang\lin380clean\TW13081901.bin`，PID 15768。 |
| 11:11:07～11:11:55 | 啟用 Home 輔助前已成功擊殺五隻怪物：狼人、蜥蜴人、卡司特等。 |
| 11:12:09.824 | 按 Home 啟用 helper。 |
| 11:12:10.137 | NotificationHook 開始觀察掉落／擊殺，ring 位址 `0x013A0000`。 |
| 11:13:22.167 | Home 後第一次擊殺：妖魔法師 `npc45121`，擊殺日誌經驗 145。 |
| 11:13:22.298／.360 | SpriteIndex 載入 157643 個 sprite，SpriteArtwork 回報 frame／bar／experience／gold 均已讀取。 |
| 11:13:23 | Windows Application Error 1000：遊戲原生程序存取違規 `0xC0000005`。 |
| 11:13:27.947 | 伺服器 session 2 才出現 `read frame header: EOF`。 |
| 11:13:28.043 | 登入器記錄遊戲退出。亮度／時鐘等讀取失敗位於崩潰後，不能當作初始原因。 |

Windows 事件：

- 程序與故障模組：`TW13081901.bin`，版本 `13.8.19.1`。
- 故障位移 `0x00029fca`；基底 `0x00400000`，對應 VA `0x00429fca`。
- 報告 ID：`f00490eb-1f91-43f4-8c00-e6eaeb7bbfb4`。
- 原 WER 暫存 dump 已不存在，這次歷史事故沒有可用 call stack。磁碟遊戲執行檔疑似打包，不能把直接反組譯該 RVA 的結果當成可靠現場指令。

當時 `l1jgo-whale-l1jgo-1` 仍正常執行，restart_count=0、oom_killed=false；未見 server panic、kick 或 queue full。這能確認客戶端確實崩潰，但不能排除伺服器資料觸發客戶端問題。

### 封包與假設的界線

- 現有 server TX 只記 opcode／長度，不記完整 payload、子碼、訊息 ID 或物品 ID。
- 擊殺附近可見：113(len8)、250(len8)、34(len16)、15(len48)、250(len8)、250(len24)、71(len24)、24(len24)、250(len8)、250(len8)、71(len24)。
- 250/190 ItemBoard 與 250/192 ShowDrop 是原有路徑，此次 Java 行為修正沒有新增這兩種通知。它們與本機 Eddy Java 的結構一致：
  - ItemBoard：`[250][190][H gfx][S name]`。
  - ShowDrop：`[250][192][C type][D amount]`。
  - 系統訊息 143：`[71][H 143][C 2][S monster][S item]`。
  - 多位元組為 little-endian；字串轉客戶端編碼並以 NUL 結尾；補零到 8-byte 邊界。
- 格式對照不等於證明事故中的實際 payload 正確。日誌經驗 145 也不能直接當作最終通知 amount。
- 「Home 前正常、Home 後首次擊殺崩潰」與提示素材首次載入的時間相關，優先排查通知 hook／提示繪製；尚未證實根因，不要宣稱已修復或確定是登入器造成。

Login38 排查入口：

- `src/Login38.Aux/Notifications/NotificationHookCave.cs`：掛鉤 `0x0053938E`，處理 190／192，固定複製 72 bytes，返回 `0x00541596`；ring 為 16 個 80-byte slot。
- `NotificationTask.cs`：PickupToast（撿取道具提示）與 GainDrift（金幣經驗值提示）兩項都關閉才移除原本通知 hook。
- `src/Login38.App/Notifications/OverlayTask.cs`：首次有通知資料才載入 SpriteArtwork。
- 固定長度複製與 hook／繪圖路徑都只是待查線索，不能僅由程式碼與時間相關性定罪。

## Login38 協同與本對話交付的除錯版

使用者已授權協同 Login38，並請對方編譯加入除錯模式的版本。本對話已聯絡其既有對話「確認登入器解析度與即時 HD」：

- 專案：`E:\lin-golang\Login38`。
- 對話 ID：`01a1314b-6598-77e2-98ca-c6a60dc39bdf`，host `local`。
- 原事故使用的登入器：檔案 1.2.0.0，產品 `1.2.0+da102a81154ea0a36f9fa8efd261323c6a872d29`。
- 本對話交付版本：`1.2.1-killdiag.20261013.1`，不是對其他對話後續版本的最新性保證。
- ZIP：`E:\lin-golang\Login38\artifacts\kill-debug-20261013-1.zip`。
- 解壓產物、繁中說明與 manifest：同位置的 `kill-debug-20261013-1\`。
- launcher SHA-256：`54DC136F0FFB9B18C0843EF4BCA664F3143BF93FB257ACBCA215B28DF2F6616B`。

除錯版保留既有 1200×900 與即時畫質開關，新增有界診斷日誌、hook／ring／解析狀態、獨立原生 crash collector 與配對 symbols。協同檢查也補強捕捉器取得鎖後再次檢查上限，避免多執行緒造成陣列越界。

驗證：2,123 項測試通過、11 項因既有資料不足略過；原生顯示測試通過。原生捕捉實測涵蓋未啟用（0 dump）、可處理 AV（1 dump，exit 0）、未處理 AV（1 dump，exit 0xC0000005）。本對話另獨立核對 dump exception/context、套件必要檔案及 manifest 雜湊。

交付時沒有覆蓋實際遊戲安裝目錄、玩家設定，也沒有為這輪診斷修改或重啟 Go 伺服器。Login38 套件包含其工作樹變更；不要將除錯套件交付等同於 Go 提交、Login38 已提交合併、已部署或閃退已修復。

## 接續實測

1. 確認最新診斷版本及說明；使用本次套件時，解壓到原遊戲目錄外，執行 `00-prepare.cmd`，來源填 `E:\lin-golang\lin380clean`，建立獨立完整 client 副本（需數 GB 空間）。
2. 先跑 `01-normal.cmd`，用 Qwer 與相同怪物，先不按 Home 擊殺一次，再啟用 Home 後擊殺；確認登入器顯示「崩潰捕捉已就緒」。
3. 分別用 `02-no-overlay.cmd`（保留 hook／解析，停用 WPF 提示與素材載入）與 `03-no-hook.cmd`（停用通知 hook 與提示）對照。每次切換前關閉遊戲及登入器，維持解析度、畫質與其他輔助設定一致。
4. 紀錄實測時間、角色、怪物與結果；讀取套件 `diagnostics\日期-時間-模式-腳本PID\`，`last-run.txt` 指向最近紀錄。主要檔案為 `kill-diagnostics.log`、`game-PID-native.log`、`game-PID-firstchance-N.dmp`、`windows-events.txt`，並保留 `build-manifest.txt` 與 `symbols\`。
5. first-chance 例外可能被遊戲處理；有 dump 不等於 fatal，必須對照 GAME_EXIT 與 Windows 事件。預設 Home 後才啟用，一次最多保存三個不同例外位置；缺 dump 也不等於沒有崩潰。
6. 若仍需 server 證據，只補同次擊殺 250/190、250/192 的實際長度、欄位與發送順序，避免擴大側錄登入帳密。重新建置／部署前遵循專案規則與任務授權。

歷史證據位置（暫存檔可能被清理）：

- `C:\Users\vince\AppData\Local\Temp\l1jgo-kill-crash-evidence-20261013.txt`：精簡時間線與 Windows 事件。
- `C:\Users\vince\AppData\Local\Temp\l1jgo-kill-disconnect-20261013.log`：完整 Docker 快照，接收封包可能含登入資料，不應公開或複製進版本庫。
- `E:\lin-golang\lin380clean\launcher_debug.log`、`launcher_startup_timing.log`：原登入器紀錄，後續可能追加或輪替。
- dump 可能包含私人程序記憶體；只保存必要交接資訊，不把 dump 或完整原始封包加入專案記憶／Git。
