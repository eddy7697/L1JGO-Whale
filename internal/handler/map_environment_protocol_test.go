package handler

import (
	"bytes"
	"testing"
)

func TestSceneMapPacketMatchesEddyJava(t *testing.T) {
	// Eddy S_MapID: C(206), H(map), C(underwater), D(0), D(0), D(0)。
	// 不含伺服器指定 BGM 的欄位；相同地圖中的位置由移動／外觀封包通知。
	for _, tc := range []struct {
		mapID uint16
		water bool
		want  []byte
	}{
		{0, false, []byte{206, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{4, false, []byte{206, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{2005, false, []byte{206, 0xd5, 7, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
		{63, true, []byte{206, 63, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}},
	} {
		sess := newHandlerTestSession(t, uint64(tc.mapID)+1)
		SendMapID(sess, tc.mapID, tc.water)
		packets := drainHandlerTestPackets(sess)
		if len(packets) != 1 || !bytes.Equal(packets[0], tc.want) {
			t.Fatalf("map=%d water=%v got=%x want=%x", tc.mapID, tc.water, packets, tc.want)
		}
	}
}
