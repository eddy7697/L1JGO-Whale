package handler

import (
	"time"

	"github.com/l1jgo/server/internal/net"
	"github.com/l1jgo/server/internal/persist"
	"github.com/l1jgo/server/internal/world"
)

type innDoor struct {
	x, y  int32
	mapID int16
}

// Java Dungeon identifies these seven hotel entrances by their source tile.
// The destination in dungeon/portal_list is only the default room, not the hall.
var innDoorNPCs = map[innDoor]int32{
	{32600, 32931, 0}: 70012,
	{32632, 32761, 4}: 70019,
	{33437, 32789, 4}: 70031,
	{34068, 32254, 4}: 70065,
	{32628, 33167, 4}: 70070,
	{33116, 33379, 4}: 70075,
	{33605, 33275, 4}: 70084,
}

// Java getback_restart sends these room/hall maps outside on login. This also
// prevents a saved location from reopening a room without its rental identity.
var innRestartLocations = map[int32][3]int32{
	70012: {32599, 32931, 0},
	70019: {32631, 32761, 4},
	70031: {33437, 32790, 4},
	70065: {34067, 32254, 4},
	70070: {32627, 33167, 4},
	70075: {33115, 33379, 4},
	70084: {33604, 33276, 4},
}

func innNPCForMap(mapID int16) int32 {
	for npcID, coords := range innRoomCoords {
		if int16(coords[2]) == mapID || int16(coords[5]) == mapID {
			return npcID
		}
	}
	return 0
}

func resetInnLocationOnLogin(ch *persist.CharacterRow) bool {
	loc, ok := innRestartLocations[innNPCForMap(ch.MapID)]
	if !ok {
		return false
	}
	ch.X, ch.Y, ch.MapID = loc[0], loc[1], int16(loc[2])
	return true
}

// validInnKey accepts shared/traded keys, not just the original renter's key.
// Revalidate the current rental to reject keys from returned or re-rented rooms.
func validInnKey(player *world.PlayerInfo, npcID int32, deps *Deps, now time.Time) *world.InvItem {
	if player == nil || player.Dead || player.Inv == nil {
		return nil
	}
	for _, item := range player.Inv.Items {
		if item.ItemID != innKeyItemID || item.Count <= 0 || item.InnNpcID != npcID || item.InnKeyID <= 0 || item.InnDueTime <= now.Unix() {
			continue
		}
		for i := int32(0); i < 16; i++ {
			room := deps.InnRooms[npcID][i]
			if room != nil && room.KeyID == item.InnKeyID && room.LodgerID != 0 && room.Hall == item.InnHall && now.Before(room.DueTime) {
				return item
			}
		}
	}
	return nil
}

func enterInnWithKey(sess *net.Session, player *world.PlayerInfo, npcID int32, dungeon bool, deps *Deps) bool {
	coords, ok := innRoomCoords[npcID]
	if !ok {
		return false
	}
	key := validInnKey(player, npcID, deps, time.Now())
	if key == nil {
		return false
	}
	index := 0
	if key.InnHall {
		index = 3
	}
	cancelTradeIfActive(player, deps)
	if dungeon {
		applyDungeonTeleportEffect(player)
	}
	// Reuse Go's instance isolation for Java's innKeyId visibility boundary.
	// Aden/Pirate Island have NPC actions but no Dungeon/getback_restart rows;
	// their existing entry behavior is outside the seven doorway fixes.
	showID := player.ShowID
	if _, supported := innRestartLocations[npcID]; supported {
		showID = key.InnKeyID
	}
	teleportPlayerInShow(sess, player, coords[index], coords[index+1], int16(coords[index+2]), 6, showID, deps)
	return true
}

func tryInnPortal(sess *net.Session, player *world.PlayerInfo, x, y int32, deps *Deps) (isInn, entered bool) {
	npcID, ok := innDoorNPCs[innDoor{x, y, player.MapID}]
	if !ok {
		return false, false
	}
	return true, enterInnWithKey(sess, player, npcID, true, deps)
}
