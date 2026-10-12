package handler

import "github.com/l1jgo/server/internal/net/packet"

// BuildLiquor 對齊 Java S_Liquor：角色 ID 後接三段加速狀態（8 套用、0 解除）。
func BuildLiquor(charID int32, liquorType byte) []byte {
	w := packet.NewWriterWithOpcode(packet.S_OPCODE_DRUNKEN)
	w.WriteD(charID)
	w.WriteC(liquorType)
	return w.Bytes()
}
