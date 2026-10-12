package data

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// GMCommandConfig 對應 Java data/xml/GmCommands/GMCommands.xml。
type GMCommandConfig struct {
	Rooms    map[string]GMRoom
	ItemSets map[string][]GMSetItem
}

type GMRoom struct {
	Name  string `xml:"Name,attr"`
	X     int32  `xml:"LocX,attr"`
	Y     int32  `xml:"LocY,attr"`
	MapID int16  `xml:"MapId,attr"`
}

type GMSetItem struct {
	ID      int32 `xml:"Id,attr"`
	Amount  int32 `xml:"Amount,attr"`
	Enchant int8  `xml:"Enchant,attr"`
}

func LoadGMCommandConfig(path string) (*GMCommandConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Rooms []GMRoom `xml:"RoomList>Room"`
		Sets  []struct {
			Name  string      `xml:"Name,attr"`
			Items []GMSetItem `xml:"Item"`
		} `xml:"ItemSetList>ItemSet"`
	}
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("解析 GMCommands.xml: %w", err)
	}
	cfg := &GMCommandConfig{Rooms: make(map[string]GMRoom), ItemSets: make(map[string][]GMSetItem)}
	for _, r := range doc.Rooms {
		if r.Name == "" || r.X < 0 || r.X > 65535 || r.Y < 0 || r.Y > 65535 {
			return nil, fmt.Errorf("GM room 無效: %q", r.Name)
		}
		cfg.Rooms[strings.ToLower(r.Name)] = r
	}
	for _, set := range doc.Sets {
		if set.Name == "" || len(set.Items) == 0 {
			return nil, fmt.Errorf("GM itemset 無效: %q", set.Name)
		}
		for _, item := range set.Items {
			if item.ID <= 0 || item.Amount <= 0 {
				return nil, fmt.Errorf("GM itemset %s 含無效物品", set.Name)
			}
		}
		cfg.ItemSets[set.Name] = set.Items
	}
	return cfg, nil
}

// GMSpawnStore 將 .insert 生成點存於獨立 YAML，避免重寫大型靜態資料。
// 所有讀寫都在遊戲迴圈執行；先成功落盤，再更新記憶體並生成 NPC。
type GMSpawnStore struct {
	path   string
	base   []SpawnEntry
	custom []SpawnEntry
}

func LoadGMSpawnStore(base []SpawnEntry, path string) (*GMSpawnStore, error) {
	s := &GMSpawnStore{path: path, base: append([]SpawnEntry(nil), base...)}
	custom, err := LoadSpawnList(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		s.custom = custom
	}
	return s, nil
}

func (s *GMSpawnStore) Entries() []SpawnEntry {
	entries := append([]SpawnEntry(nil), s.base...)
	return append(entries, s.custom...)
}

// Lookup 與 Java 一樣優先使用 npc 表。舊 YAML 無 ID 時另以 1-based 項次查詢。
func (s *GMSpawnStore) Lookup(id int32) (entry SpawnEntry, found, legacy bool) {
	if s == nil || id <= 0 {
		return
	}
	entries := s.Entries()
	for _, src := range []string{"npc", "mob", ""} {
		for _, e := range entries {
			if e.ID == id && e.Source == src {
				return e, true, false
			}
		}
	}
	if int64(id) <= int64(len(s.base)) && s.base[id-1].ID == 0 {
		return s.base[id-1], true, true
	}
	return
}

func (s *GMSpawnStore) Insert(entry SpawnEntry) (SpawnEntry, error) {
	if s == nil || s.path == "" {
		return SpawnEntry{}, fmt.Errorf("GM 生成點儲存未設定")
	}
	if entry.Source != "npc" && entry.Source != "mob" {
		return SpawnEntry{}, fmt.Errorf("生成類型必須為 npc 或 mob")
	}
	if entry.NpcID <= 0 || entry.Count <= 0 {
		return SpawnEntry{}, fmt.Errorf("生成資料無效")
	}
	id := int32(len(s.base) + 1)
	for _, e := range s.Entries() {
		if e.ID >= id {
			if e.ID == 2147483647 {
				return SpawnEntry{}, fmt.Errorf("生成點 ID 已耗盡")
			}
			id = e.ID + 1
		}
	}
	entry.ID = id
	next := append(append([]SpawnEntry(nil), s.custom...), entry)
	raw, err := yaml.Marshal(spawnListFile{Spawns: next})
	if err != nil {
		return SpawnEntry{}, err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".gm-spawns-*.yaml")
	if err != nil {
		return SpawnEntry{}, err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return SpawnEntry{}, err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return SpawnEntry{}, err
	}
	s.custom = next
	return entry, nil
}

// FindByNameWithoutSpace 與 Java GM summon 的完整名稱比對一致。
func (t *NpcTable) FindByNameWithoutSpace(name string) *NpcTemplate {
	name = strings.ReplaceAll(name, " ", "")
	var found *NpcTemplate
	for _, n := range t.templates {
		if strings.EqualFold(strings.ReplaceAll(n.Name, " ", ""), name) && (found == nil || n.NpcID < found.NpcID) {
			found = n
		}
	}
	return found
}

// OriginalTile 為 GM .tile 公開只讀的地圖原始通行碼。
func (t *MapDataTable) OriginalTile(mapID int16, x, y int32) byte {
	return t.accessOriginalTile(mapID, x, y)
}

// ValidateGMReload 在替換目前運行的陷阱前拒絕會使 randomizePosition panic 的設定。
func (d *TrapData) ValidateGMReload() error {
	if d == nil {
		return fmt.Errorf("陷阱資料為空")
	}
	for i, sp := range d.Spawns {
		if sp.RndX < 0 || sp.RndY < 0 || sp.RndX > 65535 || sp.RndY > 65535 || sp.Count < 0 || sp.Count > 100000 {
			return fmt.Errorf("陷阱生成點 %d 的範圍或數量無效", i+1)
		}
		if d.Templates[sp.TrapID] == nil {
			return fmt.Errorf("陷阱生成點 %d 的範本 %d 不存在", i+1, sp.TrapID)
		}
	}
	return nil
}
