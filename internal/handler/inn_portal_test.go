package handler

import (
	"fmt"
	"testing"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/data"
	"github.com/l1jgo/server/internal/net/packet"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
	"go.uber.org/zap"
)

func innPortalFixture(t *testing.T, portals *data.PortalTable, door innDoor, npcID int32, hall bool) (*world.PlayerInfo, *world.InvItem, *Deps) {
	t.Helper()
	p := &world.PlayerInfo{CharID: 5101, Name: "guest", SessionID: 1, Session: newHandlerTestSession(t, 1),
		X: door.x - 1, Y: door.y, MapID: door.mapID, Inv: world.NewInventory(), Known: world.NewKnownEntities()}
	due := time.Now().Add(time.Hour)
	key := p.Inv.AddItem(innKeyItemID, 1, "inn key", 0, 0, false, 1)
	key.InnKeyID, key.InnNpcID, key.InnHall, key.InnDueTime = 90001, npcID, hall, due.Unix()
	ws := world.NewState()
	ws.AddPlayer(p)
	ws.AddNpc(&world.NpcInfo{ID: 91001, NpcID: npcID, X: door.x - 1, Y: door.y, MapID: door.mapID})
	deps := &Deps{World: ws, Portals: portals, Log: zap.NewNop(), Config: &config.Config{},
		InnRooms: map[int32]map[int32]*persist.InnRoom{npcID: {0: {NpcID: npcID, RoomNumber: 0, KeyID: key.InnKeyID, LodgerID: 9999, Hall: hall, DueTime: due}}}}
	return p, key, deps
}

func innPortalReader(x, y int32) *packet.Reader {
	w := packet.NewWriterWithOpcode(packet.C_OPCODE_ENTER_PORTAL)
	w.WriteH(uint16(x))
	w.WriteH(uint16(y))
	return packet.NewReader(w.Bytes())
}

func useInnEntry(route string, p *world.PlayerInfo, door innDoor, deps *Deps) {
	switch route {
	case "walk":
		HandleMove(p.Session, moveReader(uint16(p.X), uint16(p.Y), 2), deps)
	case "portal":
		HandleEnterPortal(p.Session, innPortalReader(door.x, door.y), deps)
	case "npc":
		HandleNpcAction(p.Session, npcActionReader(91001, "enter"), deps)
	}
}

func loadInnPortals(t *testing.T) *data.PortalTable {
	t.Helper()
	portals, err := data.LoadPortalTable("../../data/yaml/portal_list.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return portals
}

func TestInnEntryRoutesRoomAndHallForEveryDoor(t *testing.T) {
	portals := loadInnPortals(t)
	for door, npcID := range innDoorNPCs {
		for _, hall := range []bool{false, true} {
			for _, route := range []string{"walk", "portal", "npc"} {
				t.Run(fmt.Sprintf("npc=%d/hall=%t/%s", npcID, hall, route), func(t *testing.T) {
					if portals.Get(door.x, door.y, door.mapID) == nil {
						t.Fatal("production doorway missing")
					}
					p, key, deps := innPortalFixture(t, portals, door, npcID, hall)
					useInnEntry(route, p, door, deps)
					index := 0
					if hall {
						index = 3
					}
					coords := innRoomCoords[npcID]
					if p.X != coords[index] || p.Y != coords[index+1] || p.MapID != int16(coords[index+2]) || p.Heading != 6 || p.ShowID != key.InnKeyID {
						t.Fatalf("wrong rental destination: (%d,%d,map=%d,heading=%d,show=%d)", p.X, p.Y, p.MapID, p.Heading, p.ShowID)
					}
					if key.Count != 1 || p.Inv.FindByObjectID(key.ObjectID) == nil {
						t.Fatal("entry consumed key")
					}
					if route != "npc" && (!p.AbsoluteBarrier || p.GetBuff(78).TicksLeft != 10) {
						t.Fatal("missing dungeon arrival protection")
					}
				})
			}
		}
	}
}

func TestInnEntryRejectsInvalidKeysOnEveryRoute(t *testing.T) {
	portals := loadInnPortals(t)
	door := innDoor{32600, 32931, 0}
	cases := map[string]func(*world.PlayerInfo, *world.InvItem, *Deps){
		"missing":   func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { p.Inv.RemoveItem(k.ObjectID, 1) },
		"wrong inn": func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { k.InnNpcID = 70019 },
		"expired key": func(p *world.PlayerInfo, k *world.InvItem, d *Deps) {
			k.InnDueTime = time.Now().Add(-time.Minute).Unix()
		},
		"expiry boundary": func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { k.InnDueTime = time.Now().Unix() },
		"expired rental": func(p *world.PlayerInfo, k *world.InvItem, d *Deps) {
			d.InnRooms[70012][0].DueTime = time.Now().Add(-time.Minute)
		},
		"returned rental": func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { d.InnRooms[70012][0].KeyID = 0 },
		"re-rented room":  func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { d.InnRooms[70012][0].KeyID++ },
		"wrong hall":      func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { k.InnHall = true },
		"impostor item":   func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { k.ItemID = 40308 },
		"zero count":      func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { k.Count = 0 },
		"zero key id":     func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { k.InnKeyID = 0; d.InnRooms[70012][0].KeyID = 0 },
		"no rooms":        func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { d.InnRooms = nil },
		"dead":            func(p *world.PlayerInfo, k *world.InvItem, d *Deps) { p.Dead = true },
	}
	for name, change := range cases {
		for _, route := range []string{"walk", "portal", "npc"} {
			t.Run(name+"/"+route, func(t *testing.T) {
				p, key, deps := innPortalFixture(t, portals, door, 70012, false)
				change(p, key, deps)
				useInnEntry(route, p, door, deps)
				if p.MapID != door.mapID || p.ShowID != 0 || p.AbsoluteBarrier || p.GetBuff(78) != nil {
					t.Fatal("invalid key entered rental or got arrival effect")
				}
				if route == "walk" && !p.Dead && (p.X != door.x || p.Y != door.y) {
					t.Fatal("Java dg=false should allow ordinary step")
				}
			})
		}
	}
}

func TestInnPortalPreservesDistanceAndNPCInstanceChecks(t *testing.T) {
	portals := loadInnPortals(t)
	door := innDoor{32600, 32931, 0}
	p, _, deps := innPortalFixture(t, portals, door, 70012, false)
	p.X = door.x - 10
	useInnEntry("portal", p, door, deps)
	if p.MapID != 0 {
		t.Fatal("remote portal packet entered inn")
	}
	p.X = door.x - 1
	deps.World.GetNpc(91001).ShowID = 7
	useInnEntry("npc", p, door, deps)
	if p.MapID != 0 {
		t.Fatal("NPC in another instance admitted guest")
	}
	deps.World.GetNpc(91001).ShowID = 0
	deps.World.GetNpc(91001).MapID = 4
	useInnEntry("npc", p, door, deps)
	if p.MapID != 0 {
		t.Fatal("NPC on another map admitted guest")
	}
}

func TestInnRentalVisibilityAndExitReset(t *testing.T) {
	door := innDoor{32600, 32931, 0}
	p, key, deps := innPortalFixture(t, loadInnPortals(t), door, 70012, false)
	same := addNpcActionShowIDPlayer(deps.World, &world.PlayerInfo{CharID: 5102, SessionID: 2, Session: newHandlerTestSession(t, 2), X: 32745, Y: 32804, MapID: 16384, ShowID: key.InnKeyID})
	other := addNpcActionShowIDPlayer(deps.World, &world.PlayerInfo{CharID: 5103, SessionID: 3, Session: newHandlerTestSession(t, 3), X: 32745, Y: 32804, MapID: 16384, ShowID: key.InnKeyID + 1})
	outside := addNpcActionShowIDPlayer(deps.World, &world.PlayerInfo{CharID: 5104, SessionID: 4, Session: newHandlerTestSession(t, 4), X: 32599, Y: 32930, MapID: 0})
	pet := &world.PetInfo{ID: 5201, OwnerCharID: p.CharID, X: p.X, Y: p.Y, MapID: p.MapID, HP: 10, MaxHP: 10}
	deps.World.AddPet(pet)
	useInnEntry("portal", p, door, deps)
	if !hasNpcActionShowIDPutObjectPacket(drainHandlerTestPackets(same.Session), p.CharID) {
		t.Fatal("same rental cannot see guest")
	}
	if hasNpcActionShowIDPutObjectPacket(drainHandlerTestPackets(other.Session), p.CharID) {
		t.Fatal("different rental saw guest")
	}
	if pet.ShowID != key.InnKeyID || pet.MapID != p.MapID {
		t.Fatal("companion did not enter rental")
	}
	useInnEntry("portal", p, innDoor{32746, 32803, 16384}, deps)
	if p.MapID != 0 || p.ShowID != 0 || pet.ShowID != 0 || pet.MapID != 0 {
		t.Fatal("exit retained inn instance")
	}
	if !hasNpcActionShowIDPutObjectPacket(drainHandlerTestPackets(outside.Session), p.CharID) {
		t.Fatal("guest invisible outside")
	}
}

func TestInnLoginUsesJavaRestartLocations(t *testing.T) {
	for npcID, loc := range innRestartLocations {
		for _, index := range []int{2, 5} {
			ch := &persist.CharacterRow{MapID: int16(innRoomCoords[npcID][index]), X: 32745, Y: 32803}
			if !resetInnLocationOnLogin(ch) || ch.X != loc[0] || ch.Y != loc[1] || ch.MapID != int16(loc[2]) {
				t.Fatalf("wrong restart for NPC %d: %+v", npcID, ch)
			}
		}
	}
	ch := &persist.CharacterRow{MapID: 4, X: 100, Y: 100}
	if resetInnLocationOnLogin(ch) || ch.X != 100 || ch.Y != 100 || ch.MapID != 4 {
		t.Fatal("ordinary map changed")
	}
}
