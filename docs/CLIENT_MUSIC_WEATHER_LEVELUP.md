# 場景／戰鬥音樂、天氣與升級特效核對

核對日期：2026-10-13。Java 參考為 `E:\L1JTW-Eddy`（下列 Java 路徑均相對於 `src/l1j/server/server/`）；Go 基準為本機 `master` 的 `0a818f3`，已同步 origin/master，基準包含遠端內容。

## 核對結果

| 項目 | Java 行為與 Go 差異 | 處理／驗證界線 |
| --- | --- | --- |
| 場景背景音樂 | Java 登入、傳送以 `S_MapID` 傳地圖 ID／水底旗標，位置由角色外觀／移動封包提供。未找到場景 BGM 選曲或切換通知；Go 地圖封包格式一致。 | 加入 Java 地圖封包比對測試。尚未定位實際音樂相同的原因，需同一套客戶端連 Java／Go 比較。 |
| 打怪與 PvP 戰鬥音樂 | Java `L1Attack.actionPc()` 發攻擊動作，安全區對玩家揮刀亦呼叫 `attack.action()`；沒有另外發戰鬥音樂封包。Go 打怪及三類區域的近戰 PvP 都有傳攻擊動作給本人。 | 新增一般區／戰鬥區／安全區的端到端處理測試，驗證攻擊者與目標收到 Java 格式動作；保留既有打怪近／遠戰測試。這些不能證明客戶端已播放音樂。 |
| 新角色進世界下雨／下雪 | Java `L1World._weather = 4`；隨機 `Weather` 工具標示「沒用到」，src 中無呼叫。Go 原先首 tick 與遊戲換小時自行亂數，且未讀設定開關。 | 世界初始改為 4；預設關閉自動天氣，設定開關及 tick 間隔現在確實生效。GM 手動天氣保留。 |
| 升級發光 | Java `levelUp()` 直接送 `S_OwnCharStatus`，沒有先送 `S_Exp`，也沒有另發升級 GFX。Go 原先先以 `S_Exp` 通知新等級，再送完整狀態。 | 升級時取消前置 `S_Exp`，使完整狀態首次通知新等級，保留客戶端偵測升級的機會；單級、跨多級、未升級經驗更新均有測試。實際發光仍需遊戲內驗收。 |

## 音樂的依據與後續定位

- `serverpackets/S_MapID.java`：`C(206) + H(mapID) + C(underwater) + D(0) + D(0) + D(0)`，沒有 BGM ID。
- `model/L1Attack.java` 的 `actionPc()` 與 `serverpackets/S_AttackPacket.java`：一般近戰為 `C(30) + C(action) + D(attacker) + D(target) + H(damage) + C(heading) + D(0) + C(effect)`；遠攻由 `S_UseArrowSkill` 處理。
- `model/Instance/L1PcInstance.java` 的 `onAction()`：安全區阻擋傷害，仍發攻擊動作。因此不能用「安全區無傷害」解釋沒有 PvP 配樂。
- `serverpackets/S_Sound.java` 明確註解是播放 Sound 資料夾的 WAV；src 中呼叫處是道具／特定技能，沒有一般場景或戰鬥 BGM 切換。不應猜一個音效 ID，在每次攻擊時硬播。
- 搜查現有 Login38 原始碼未找到 BGM 切換補丁，但這不代表已驗證所有版本登入器／客戶端。

依上述伺服器行為，場景與戰鬥配樂由客戶端依遊戲狀態選擇是目前合理推論；尚不能斷言使用者遇到的問題一定出在音量設定、音樂檔案或登入器。這次沒有修改音樂檔或新增 BGM 封包。

實機對照應使用同一份客戶端、同一登入器及相同聲音設定，分別連 Java 與 Go，記錄：

1. 兩個明確地點的場景配樂，包括走路跨區與跨地圖傳送。
2. 打怪近戰／遠戰／被怪物攻擊；記錄進入與離開戰鬥後的配樂。
3. 對其他玩家揮刀：一般區、戰鬥區、安全區都測試，安全區維持零傷害。
4. 若同一客戶端連 Java 正常、Go 不正常，再比較同一操作的地圖、角色外觀、攻擊、角色設定封包；若兩邊都不正常，檢查客戶端音樂設定及資源。

本次沒有執行遊戲內音訊／畫面驗收，也沒有足夠資料確認實際使用中的客戶端資料夾。

## 雨雪畫面的呈現

伺服器透過 `S_Weather`（opcode 115）送一個天氣代碼，由客戶端繪製畫面上的雨／雪。Java 登入及死亡後重新開始都傳目前全世界天氣，不是創角時抽選；特定地圖是否顯示效果仍由客戶端處理。

| 代碼 | 意義／來源 |
| --- | --- |
| 4 | `L1World` 的初始無雨雪狀態 |
| 0 | `Weather.java`：無雪雨 |
| 1／2／3 | 小／中／大雪 |
| 16 | 停止下雨 |
| 17／18／19 | 小／中／大雨 |

`.weather` 現在也接受 4、16，便於恢復 Java 初始值與停雨。關閉自動天氣不會禁止 GM 手動改天氣；若 GM 設定下雨，新登入角色仍會繼承目前雨天，與 Java 相同。

隨附 `config/server.toml`、`config/server.docker.toml` 及程式預設均為 `weather_enabled = false`。自行維護的 `server.local.toml` 若仍有 `weather_enabled = true`，會明確啟用 Go 的可選隨機天氣，需改為 false 才是 Java 預設行為；本次未改動既有未版控的本機設定。若刻意啟用，`weather_interval_ticks` 現在控制真正的間隔（100 ticks、200 ms/tick 時為 20 秒），不再依遊戲換小時觸發。

## 升級特效的驗收

修正針對伺服器通知順序：讓 `S_OwnCharStatus` 首次把等級由舊值改成新值。原先前置 `S_Exp` 可能使客戶端等級已更新，導致後續完整狀態沒有等級差值。這是基於 Java 流程的原因判斷，尚未以客戶端實機確認。

請以正常打怪取得經驗跨級，確認本人看到升級光效，EXP、HP／MP 同時更新；再測一次跨多級及未升級的經驗增加。不額外指定未經 Java 證實的 GFX，也不擅自加入對旁觀者的升級特效廣播。

## 自動驗證

以下檢查已通過：

- `go build ./...`
- `go vet ./...`
- `go test ./internal/config ./internal/handler ./internal/system ./internal/world`
- `git diff --check`

測試覆蓋 Java 地圖封包格式、打怪攻擊通知、三類區域 PvP 動作、升級通知順序、預設無雨雪、自動天氣開關／間隔，以及 GM 的晴天／雪／雨／停雨代碼。未部署或重啟執行中的伺服器；音樂與升級光效的實際播放結果仍待實機驗收。
