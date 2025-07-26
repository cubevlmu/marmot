package main

import (
	"marmot/core"
	_ "marmot/modules"
	"marmot/onebot"
	zero "marmot/onebot"
	"os"
)

func main() {
	// init basic services
	core.InitCommon()
	onebot.SetLogger(core.NewZBLogger())

	// init module manager
	mMgr := core.NewModuleMgr()
	if mMgr == nil {
		core.LogError("Failed to init moduleMgr")
		os.Exit(1)
	}
	//zero.OnMessage().Handle()
	mMgr.LoadAll()

	// reg shutdown hook to cleanup & save data
	core.RegisterShutdownHook(func() {
		mMgr.UnloadAll()
	})
	core.StartHookWatch()

	// run bot engine's loop
	zero.RunAndBlock(&zero.Config{
		NickName: []string{"bot"},
		Driver: onebot.NewWebSocketServer(16, core.AppConfig.WsUrl, "", func(id int64) {
			core.Common.BotQQ = id
			core.LogInfo("Bot id : %v", id)
		}),
	}, mMgr.HandleEvent)
}
