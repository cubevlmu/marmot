package core

import (
	zero "marmot/onebot"
)

func IsGroupChat(z *zero.Ctx) bool {
	return z.Event.GroupID != 0
}

func IsBotAdmin(z *zero.Ctx) bool {
	return AppConfig.CheckIsAdmin(z.Event.Sender.ID)
}

func IsGroupAdmin(ctx *zero.Ctx) bool {
	return ctx.Event.Sender.Role == "owner" || ctx.Event.Sender.Role == "admin"
}

func IsGroupOwner(ctx *zero.Ctx) bool {
	return ctx.Event.Sender.Role == "owner"
}
