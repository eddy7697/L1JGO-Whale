package persist

import (
	"context"
	"fmt"
)

// BeginnerRow mirrors one row of the Java `beginner` table (Eddy Beginner.java).
// activate 'A' applies to every class; P/K/E/W/D/R/I are per-class codes.
type BeginnerRow struct {
	ID          int32
	ItemID      int32
	Count       int32
	ChargeCount int16
	EnchantLvl  int16
	ItemName    string
	Activate    string
	Bless       int16
}

// beginnerInsert is the character_items row Java writes for one beginner entry.
type beginnerInsert struct {
	CharID      int32
	ItemID      int32
	Count       int32
	EnchantLvl  int16
	ChargeCount int16
	Bless       int16
	Equipped    bool
	Identified  bool
	Durability  int16
}

// beginnerActivateCode reproduces the isCrown/isKnight/... chain in
// Beginner.GiveItem: class_type 0..6 → P K E W D R I, anything else → 'A'.
func beginnerActivateCode(classType int16) string {
	switch classType {
	case 0:
		return "P"
	case 1:
		return "K"
	case 2:
		return "E"
	case 3:
		return "W"
	case 4:
		return "D"
	case 5:
		return "R"
	case 6:
		return "I"
	default:
		return "A"
	}
}

// beginnerInsertValues copies Java's INSERT INTO character_items values:
// count/enchantlvl/charge_count from the table, bless hard-coded 1,
// is_equipped=0, is_id=0, durability=0. The table's bless column is ignored.
func beginnerInsertValues(charID int32, row BeginnerRow) beginnerInsert {
	return beginnerInsert{
		CharID:      charID,
		ItemID:      row.ItemID,
		Count:       row.Count,
		EnchantLvl:  row.EnchantLvl,
		ChargeCount: row.ChargeCount,
		Bless:       1,
		Equipped:    false,
		Identified:  false,
		Durability:  0,
	}
}

type BeginnerRepo struct{ db *DB }

func NewBeginnerRepo(db *DB) *BeginnerRepo { return &BeginnerRepo{db: db} }

// LoadForClass returns the rows Java selects: WHERE activate IN ('A', <class>).
func (r *BeginnerRepo) LoadForClass(ctx context.Context, classType int16) ([]BeginnerRow, error) {
	rows, err := r.db.Pool.Query(ctx,
		`SELECT id, item_id, count, charge_count, enchantlvl, item_name, activate, bless
		 FROM beginner WHERE activate IN ('A', $1) ORDER BY id`, beginnerActivateCode(classType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BeginnerRow
	for rows.Next() {
		var b BeginnerRow
		if err := rows.Scan(&b.ID, &b.ItemID, &b.Count, &b.ChargeCount, &b.EnchantLvl, &b.ItemName, &b.Activate, &b.Bless); err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// GiveItems is the Go counterpart of Beginner.GiveItem(pc): it inserts every
// matching beginner row into character_items for the new character. Like
// Java, a failing row is reported but does not stop the remaining rows.
// It returns the number of rows inserted and the collected per-row errors.
func (r *BeginnerRepo) GiveItems(ctx context.Context, charID int32, classType int16) (int, []error) {
	rows, err := r.LoadForClass(ctx, classType)
	if err != nil {
		return 0, []error{fmt.Errorf("load beginner: %w", err)}
	}
	var errs []error
	inserted := 0
	for _, row := range rows {
		v := beginnerInsertValues(charID, row)
		if _, err := r.db.Pool.Exec(ctx,
			`INSERT INTO character_items (char_id, item_id, count, enchant_lvl, bless, equipped, identified, equip_slot, obj_id, durability, charge_count)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 0, $8, $9)`,
			v.CharID, v.ItemID, v.Count, v.EnchantLvl, v.Bless, v.Equipped, v.Identified, v.Durability, v.ChargeCount,
		); err != nil {
			errs = append(errs, fmt.Errorf("beginner row %d (item %d): %w", row.ID, row.ItemID, err))
			continue
		}
		inserted++
	}
	return inserted, errs
}
