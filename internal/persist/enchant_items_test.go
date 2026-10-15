package persist

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/l1jgo/server/internal/world"
)

func TestEnchantItemsPostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("L1JGO_TEST_POINTS_DSN")
	if dsn == "" {
		t.Skip("run scripts/test_java_items.ps1 for isolated PostgreSQL verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("codex_items_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	applyMigration := func(file string) {
		t.Helper()
		migration, err := migrations.ReadFile("migrations/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, strings.Split(string(migration), "-- +goose Down")[0]); err != nil {
			t.Fatalf("migration %s: %v", file, err)
		}
	}
	for _, file := range []string{"001_phase1.sql", "002_items.sql", "005_warehouse.sql", "008_item_objid.sql", "010_durability.sql", "021_attr_enchant.sql", "025_inns.sql", "028_charge_count.sql", "029_warehouse_item_state.sql", "030_item_last_used.sql"} {
		applyMigration(file)
	}
	// Seed legacy rows before 040 to verify the upgrade's zero defaults.
	if _, err = pool.Exec(ctx, `INSERT INTO accounts(name,password_hash) VALUES('fixture','unused');
		INSERT INTO characters(id,account_name,name,class_type,sex,class_id,str,dex,con,wis,cha,intel,hp,mp,max_hp,max_mp)
		VALUES(1,'fixture','fixture',0,0,0,12,12,12,12,12,12,20,10,20,10);
		INSERT INTO character_items(char_id,item_id) VALUES(1,20280);
		INSERT INTO warehouse_items(account_name,char_name,wh_type,item_id) VALUES('fixture','fixture',3,20280)`); err != nil {
		t.Fatal(err)
	}
	applyMigration("040_accessory_enchant.sql")
	db := &DB{Pool: pool}
	items, warehouse := NewItemRepo(db), NewWarehouseRepo(db)
	legacy, err := items.LoadByCharID(ctx, 1)
	if err != nil || len(legacy) != 1 || !legacy[0].Accessory.IsZero() {
		t.Fatalf("legacy inventory defaults: %+v, %v", legacy, err)
	}
	legacyWH, err := warehouse.Load(ctx, "fixture", 3)
	if err != nil || len(legacyWH) != 1 || !legacyWH[0].Accessory.IsZero() {
		t.Fatalf("legacy warehouse defaults: %+v, %v", legacyWH, err)
	}
	if _, err = warehouse.Withdraw(ctx, legacyWH[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	bonus := world.AccessoryBonus{AddHP: 50, AddMP: 25, HPR: 2, MPR: 3, AddSP: 1, MDef: 7, EarthMr: 8, FireMr: 9, WaterMr: 10, WindMr: 11}
	accessory := &world.InvItem{ObjectID: 10001, ItemID: 20280, Count: 1, EnchantLvl: 5, Identified: true, Equipped: true, Accessory: bonus}
	weapon := &world.InvItem{ObjectID: 10002, ItemID: 1, Count: 1, EnchantLvl: 6, AttrEnchantKind: 2, AttrEnchantLevel: 3, Equipped: true}
	key := &world.InvItem{ObjectID: 10003, ItemID: 40312, Count: 1, InnNpcID: 70012, InnKeyID: 90001, InnHall: true, InnDueTime: time.Now().Add(time.Hour).Unix()}
	inv := &world.Inventory{Items: []*world.InvItem{accessory, weapon, key}}
	equip := &world.Equipment{}
	equip.Set(world.SlotRing1, accessory)
	equip.Set(world.SlotWeapon, weapon)
	assertInventory := func() {
		t.Helper()
		rows, err := items.LoadByCharID(ctx, 1)
		if err != nil || len(rows) != 3 {
			t.Fatalf("load inventory: %+v, %v", rows, err)
		}
		byID := make(map[int32]ItemRow)
		for _, row := range rows {
			byID[row.ObjID] = row
		}
		acc := byID[accessory.ObjectID]
		if acc.Accessory != accessory.Accessory || acc.EnchantLvl != int16(accessory.EnchantLvl) || !acc.Equipped || acc.EquipSlot != int16(world.SlotRing1) {
			t.Fatalf("accessory state lost: %+v", acc)
		}
		weap := byID[weapon.ObjectID]
		if weap.AttrEnchantKind != int16(weapon.AttrEnchantKind) || weap.AttrEnchantLevel != int16(weapon.AttrEnchantLevel) || weap.EnchantLvl != 6 || !weap.Equipped || weap.EquipSlot != int16(world.SlotWeapon) {
			t.Fatalf("weapon state lost: %+v", weap)
		}
		gotKey := byID[key.ObjectID]
		if gotKey.InnKeyID != key.InnKeyID || gotKey.InnNpcID != key.InnNpcID || gotKey.InnHall != key.InnHall || gotKey.InnDueTime != key.InnDueTime {
			t.Fatalf("rental key state lost: %+v", gotKey)
		}
	}
	if err = items.SaveInventory(ctx, 1, inv, equip); err != nil {
		t.Fatal(err)
	}
	assertInventory()
	// A second save must replace old bonuses and elemental state, including zero.
	accessory.Accessory = world.AccessoryBonus{}
	accessory.EnchantLvl = 0
	weapon.AttrEnchantKind, weapon.AttrEnchantLevel = 8, 1
	if err = items.SaveInventory(ctx, 1, inv, equip); err != nil {
		t.Fatal(err)
	}
	assertInventory()
	for _, whType := range []int16{3, 4, 5, 6} {
		want := WarehouseItem{AccountName: "fixture", CharName: "fixture", WhType: whType, ItemObjID: 20000 + int32(whType), ItemID: 20280, Count: 1, EnchantLvl: 5, Identified: true, AttrEnchantKind: 4, AttrEnchantLevel: 2, Accessory: bonus, InnNpcID: key.InnNpcID, InnKeyID: key.InnKeyID, InnHall: key.InnHall, InnDueTime: key.InnDueTime}
		want.ID, err = warehouse.Deposit(ctx, want)
		if err != nil {
			t.Fatal(err)
		}
		loaded, err := warehouse.Load(ctx, "fixture", whType)
		if err != nil || len(loaded) != 1 || loaded[0] != want {
			t.Fatalf("warehouse %d: %+v, %v; want %+v", whType, loaded, err, want)
		}
		if whType == 6 {
			loaded, err = warehouse.LoadByCharName(ctx, "fixture", whType)
			if err != nil || len(loaded) != 1 || loaded[0] != want {
				t.Fatalf("character warehouse: %+v, %v", loaded, err)
			}
		}
	}
	inn := NewInnRepo(db)
	if err = inn.EnsureRooms(ctx, key.InnNpcID); err != nil {
		t.Fatal(err)
	}
	room := &InnRoom{NpcID: key.InnNpcID, RoomNumber: 3, KeyID: key.InnKeyID, LodgerID: 1, Hall: key.InnHall, DueTime: time.Unix(key.InnDueTime, 0)}
	if err = inn.UpdateRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	rooms, err := inn.LoadAll(ctx)
	if err != nil || len(rooms) != 16 {
		t.Fatalf("load rooms: %d, %v", len(rooms), err)
	}
	for _, got := range rooms {
		if got.RoomNumber == room.RoomNumber && (got.KeyID != room.KeyID || got.LodgerID != room.LodgerID || got.Hall != room.Hall || !got.DueTime.Equal(room.DueTime)) {
			t.Fatalf("rental state lost: %+v", got)
		}
	}
}
