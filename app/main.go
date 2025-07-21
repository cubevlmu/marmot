package main

import (
	zero "github.com/cubevlmu/CZeroBot"
	"github.com/cubevlmu/CZeroBot/driver"
	"github.com/cubevlmu/CZeroBot/log"
	"marmot/core"
	_ "marmot/modules"
	"os"
)

func main() {
	// init basic services
	core.InitCommon()
	log.SetDefaultLogger(core.NewZBLogger())

	// init module manager
	mMgr := core.NewModuleMgr()
	if mMgr == nil {
		core.LogError("Failed to init moduleMgr")
		os.Exit(1)
	}
	zero.OnMessage().Handle(mMgr.BroadcastMsg)
	mMgr.LoadAll()

	// reg shutdown hook to cleanup & save data
	core.RegisterShutdownHook(func() {
		mMgr.UnloadAll()
	})
	core.StartHookWatch()

	// run bot engine's loop
	zero.RunAndBlock(&zero.Config{
		NickName: []string{"bot"},
		Driver: []zero.Driver{
			// reverse websocket
			driver.NewWebSocketServer(16, core.AppConfig.WsUrl, "", func(id int64) {
				core.Common.BotQQ = id
				core.LogInfo("Bot id : %v", id)
			}),
		},
	}, nil)
}
