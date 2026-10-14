package data

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DropItemRate 為單一道具的掉落倍率（Java: drop_item 表）。
type DropItemRate struct {
	ItemID     int32   `yaml:"item_id"`
	DropRate   float64 `yaml:"drop_rate"`   // 掉落機率倍率
	DropAmount float64 `yaml:"drop_amount"` // 掉落數量倍率（套用於 min/max）
	Note       string  `yaml:"note"`
}

type dropItemFile struct {
	Items []DropItemRate `yaml:"items"`
}

// DropItemTable 依道具 ID 查詢掉落倍率；查無時機率與數量倍率皆為 1（Java: DropItemTable）。
type DropItemTable struct {
	items map[int32]DropItemRate
}

// NewDropItemTable 以給定資料建立表格（測試用）。
func NewDropItemTable(items []DropItemRate) *DropItemTable {
	t := &DropItemTable{items: make(map[int32]DropItemRate, len(items))}
	for _, it := range items {
		t.items[it.ItemID] = it
	}
	return t
}

// LoadDropItemTable 讀取 drop_item.yaml。
func LoadDropItemTable(path string) (*DropItemTable, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read drop_item: %w", err)
	}
	var f dropItemFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse drop_item: %w", err)
	}
	return NewDropItemTable(f.Items), nil
}

// DropRate 回傳道具掉落機率倍率，查無時為 1。
func (t *DropItemTable) DropRate(itemID int32) float64 {
	if t == nil {
		return 1
	}
	if it, ok := t.items[itemID]; ok {
		return it.DropRate
	}
	return 1
}

// DropAmount 回傳道具掉落數量倍率，查無時為 1。
func (t *DropItemTable) DropAmount(itemID int32) float64 {
	if t == nil {
		return 1
	}
	if it, ok := t.items[itemID]; ok {
		return it.DropAmount
	}
	return 1
}

// Count 回傳表中道具數。
func (t *DropItemTable) Count() int {
	if t == nil {
		return 0
	}
	return len(t.items)
}
