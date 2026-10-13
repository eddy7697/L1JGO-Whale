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

本次沒有執行遊戲內音訊／畫面驗收。後續由使用者確認測試客戶端為 `E:\lin-golang\lin380clean`；檔案核對結果如下。

## Login38 與實際客戶端追加核對

追加核對日期：2026-10-13。使用者指定第 1、2 項同時比對 `E:\lin-golang\Login38`，並確認客戶端為 `E:\lin-golang\lin380clean`。

Login38 已 fetch `origin/main`，本次測試基準為 `da102a8`。其原工作樹有其他任務的未提交變更，因此測試在獨立分支／工作樹執行。另檢查目前工作檔及與 `v1.1.0` 的差異：設定檔解析、封包代理、傷害數字 cave、通知 cave 在這段版本間均未改動；測試結果不代表驗證過所有未提交變更或發行二進位。

| 檢查路徑（相對於 Login38） | 實際行為 | 能確認的範圍 |
| --- | --- | --- |
| `src/Login38.Core/Configuration/LineageConfigFile.cs`、`LineageConfigDocument.cs` | 已有 `lineage.cfg` 時只改 FullScreen `0x12`、WindowMode `0x1A`，其他記錄保留。不存在時建立包含三個顯示記錄的最小設定。 | 未發現登入器主動改音樂開關／音量；實際客戶端已有設定檔。本次沒有從未知 key 猜測聲音開關是否開啟。 |
| `src/Login38.Core/Net/PacketEncryptProxy.cs` | 僅在客戶端送出方向做 XOR；消耗握手 challenge 後，伺服器送往客戶端方向以 `key: 0` 原樣轉送，沒有按 opcode 過濾。 | 地圖、攻擊等伺服器資料不會被這個代理依種類吞掉；實際 socket 測試通過。 |
| `src/Login38.Aux/Toggles/DamageCave.cs` | 單體傷害數字補丁保存／恢復暫存器與旗標，執行被取代的原始比較，保留原本兩條分支目的地。 | 未發現此處直接跳過原本攻擊流程或控制 BGM；不能據此排除所有執行期補丁交互影響。 |
| `src/Login38.Aux/Notifications/NotificationHookCave.cs`、`src/Login38.Patching/Patches/SmoothRunPatch.cs` | 通知擴充處理自身通知種類；平滑移動補丁限定移動相關呼叫，保留攻擊動作。 | 相關 cave／補丁測試通過，未找到直接播放或禁止戰鬥配樂的程式碼。 |
| `src/Login38.App/Services/GameLaunchService.cs`、`src/Login38.Interop/GameProcessLauncher.cs` | 以選定的遊戲資料夾定位遊戲執行檔，且以該資料夾作為工作目錄啟動。 | 原始碼未發現因啟動工作目錄設定而必然找錯 Sound 的問題。 |

核對已包含 Java 核心，並非只比對登入器：前述 `S_MapID`、`S_AttackPacket`、`L1Attack.actionPc()` 與 `L1PcInstance.onAction()` 都直接讀取 `E:\L1JTW-Eddy` 原始碼。普通玩家外觀的 PC 旗標及一般 NPC 的角色類別欄位亦已檢視；這些檢查不能延伸為整個 Go 核心與 Java 所有行為完全一致。

### 實際客戶端的檔案檢查

- 資料夾內 `launcher.exe` 的 ProductVersion 為 `1.2.1-debugtoggle.20261013.1+da102a81154ea0a36f9fa8efd261323c6a872d29`；`TW13081901.bin` 為 `13,08,19,1`。這是磁碟檔案版本，檢查時未發現執行中的 launcher／遊戲程序。
- `Sound` 中有 142 個檔名含 music 的 MP3，無零位元組檔；SHA-256 有 141 組不同內容，只有 `music17.mp3` 與 `music42.mp3` 相同。可排除這批檔案全部被替換成同一首，但不能推定地圖選曲表或音樂資源已完整。
- 142 個檔案皆以 FFmpeg 完整解碼，無錯誤。客戶端使用的 `Mss32.dll`（6.6a）與 `mssmp3.asi` 也存在；FFmpeg 成功不等於遊戲的 Miles 播放流程正常。
- 現有 `lineage.cfg` 為 315 bytes，可依 Login38 的 TLV 格式完整解析；未改寫檔案，也未臆測未知設定 key 的含義。

### 本次新增驗證

在 Login38 的乾淨 `da102a8` 工作樹執行既有測試，共 104 項通過，無失敗或略過：

- `dotnet test tests/Login38.Core.Tests/Login38.Core.Tests.csproj --filter "FullyQualifiedName~LineageConfigDocumentTests|FullyQualifiedName~PacketEncryptProxyTests"`：35 項。
- `dotnet test tests/Login38.Aux.Tests/Login38.Aux.Tests.csproj --filter "FullyQualifiedName~DamageCaveTests|FullyQualifiedName~NotificationHookCaveTests|FullyQualifiedName~LowCpuDetourTests"`：45 項。
- `dotnet test tests/Login38.Patching.Tests/Login38.Patching.Tests.csproj --filter "FullyQualifiedName~SmoothRunPatchTests"`：24 項。

目前結論是「未找到可證明場景／戰鬥配樂故障的 Java 封包差異或 Login38 直接控制」，不是「音樂已修好」，也不是「已排除登入器」。尚缺同一套客戶端連 Java／Go 的實機播放對照，以及必要時的客戶端音訊呼叫追蹤。本次未新增配樂封包、修改未知音樂設定或替換登入器／客戶端檔案。

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
