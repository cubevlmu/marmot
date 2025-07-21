package core

import (
	zero "github.com/cubevlmu/CZeroBot"
	"strconv"
)

func IsGroupChat(z *zero.Ctx) bool {
	return z.Event.GroupID != 0
}

func SenderIsBotAdmin(z *zero.Ctx) bool {
	return AppConfig.CheckIsAdmin(strconv.FormatInt(z.Event.Sender.ID, 10))
}
