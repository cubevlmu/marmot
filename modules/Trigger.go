package modules

import (
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"sync"
)

type TriggerItem struct {
	GroupJoinMsg  string `koanf:"group_join_msg" yaml:"group_join_msg"`
	GroupLeaveMsg string `koanf:"group_quit_msg" yaml:"group_quit_msg"`
}

type TriggerConfig struct {
	ActionGroups map[int64]*TriggerItem `koanf:"action_groups" yaml:"action_groups"`
}

func (t TriggerConfig) CreateDefaultConfig() interface{} {
	return &TriggerConfig{
		ActionGroups: make(map[int64]*TriggerItem),
	}
}

type Trigger struct {
	cfg *TriggerConfig
	mtx *sync.Mutex
}

func (t *Trigger) Init(mgr *core.ModuleMgr) bool {
	t.cfg = &TriggerConfig{
		ActionGroups: make(map[int64]*TriggerItem),
	}
	pth := core.GetSubDirFilePath("trigger.yml")
	r := core.InitCustomConfig[TriggerConfig](t.cfg, pth)
	if r != nil {
		core.LogError("[Trigger] Failed to load trigger.yml error: %v", r)
		return false
	}

	mgr.RegisterEvent(core.ETGroupJoin, t.onGroupJoin)
	mgr.RegisterEvent(core.ETGroupQuit, t.onGroupQuit)
	mgr.RegisterCmd().
		RegisterGroupAdmin("SetGroupTrigger", t.onSetGroupTrigger).
		RegisterGroupAdmin("DelGroupTrigger", t.onDelGroupTrigger)

	return true
}

func (t *Trigger) Stop(_ *core.ModuleMgr) {
	r := core.SaveCustomConfigToFile(core.GetSubDirFilePath("trigger.yml"), t.cfg)
	if r != nil {
		core.LogError("[Trigger] Failed to save trigger.yml error: %v", r)
	}
	t.cfg = nil
}

func (t *Trigger) Reload(mgr *core.ModuleMgr) {
	t.Init(mgr)
	t.Stop(mgr)
}

func (t *Trigger) isGroupInTrigger(id int64) bool {
	for groupId := range t.cfg.ActionGroups {
		if id == groupId {
			return true
		}
	}
	return false
}

func (t *Trigger) onGroupJoin(ctx *zero.Ctx) {
	id := ctx.Event.GroupID
	if !t.isGroupInTrigger(id) {
		return
	}
	item, ok := t.cfg.ActionGroups[id]
	if !ok {
		core.LogError("[Trigger] Failed to get group info for id %v", id)
		return
	}
	if len(item.GroupJoinMsg) == 0 {
		return
	}
	ctx.SendGroupMessage(ctx.Event.GroupID, core.MakeReply(message.At(ctx.Event.UserID), message.Text(item.GroupJoinMsg)))
}

func (t *Trigger) onGroupQuit(ctx *zero.Ctx) {
	id := ctx.Event.GroupID
	if !t.isGroupInTrigger(id) {
		return
	}
	item := t.cfg.ActionGroups[id]
	if len(item.GroupLeaveMsg) == 0 {
		return
	}
	ctx.SendGroupMessage(ctx.Event.GroupID, core.MakeReply(message.At(ctx.Event.Sender.ID), message.Text(item.GroupLeaveMsg)))
}

func (t *Trigger) onSetGroupTrigger(args []string, ctx *zero.Ctx) {
	if len(args) != 2 {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("使用方法 SetGroupTrigger [welcome/leave] [msg]"))
		return
	}
	t.mtx.Lock()
	id := ctx.Event.GroupID
	item, ok := t.cfg.ActionGroups[id]
	if !ok {
		item = &TriggerItem{}
	}

	if args[0] == "welcome" {
		item.GroupJoinMsg = args[1]
		ctx.SendGroupMessage(id, core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text("设定入群消息成功!")))
	} else if args[0] == "leave" {
		item.GroupLeaveMsg = args[1]
		ctx.SendGroupMessage(id, core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text("设定离群消息成功!")))
	} else {
		ctx.SendGroupMessage(id, message.Text("错误的种类 welcome 来设置入群消息 leave 设置离群消息"))
	}

	t.cfg.ActionGroups[id] = item
	r := core.SaveCustomConfigToFile(core.GetSubDirFilePath("trigger.yml"), t.cfg)
	if r != nil {
		core.LogError("[Trigger] Failed to save trigger.yml error: %v", r)
	}
	t.mtx.Unlock()
}

func (t *Trigger) onDelGroupTrigger(args []string, ctx *zero.Ctx) {
	if len(args) != 1 {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("使用方法 DelGroupTrigger [welcome/leave]"))
		return
	}

	t.mtx.Lock()
	id := ctx.Event.GroupID
	item, ok := t.cfg.ActionGroups[id]
	if !ok {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.Text("没有本群的记录！不需要删除"))
		return
	}

	if args[0] == "welcome" {
		item.GroupJoinMsg = ""
		ctx.SendGroupMessage(id, core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text("删除入群消息成功!")))
	} else if args[0] == "leave" {
		item.GroupLeaveMsg = ""
		ctx.SendGroupMessage(id, core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text("删除离群消息成功!")))
	} else {
		ctx.SendGroupMessage(id, message.Text("错误的种类 welcome 来删除入群消息 leave 设置删除消息"))
	}

	t.cfg.ActionGroups[id] = item
	r := core.SaveCustomConfigToFile(core.GetSubDirFilePath("trigger.yml"), t.cfg)
	if r != nil {
		core.LogError("[Trigger] Failed to save trigger.yml error: %v", r)
	}
	t.mtx.Unlock()
}

func init() {
	core.RegisterNamed("trigger", func() core.IModule {
		return &Trigger{
			mtx: &sync.Mutex{},
		}
	})
}
