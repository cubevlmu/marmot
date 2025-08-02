package core

import (
	zero "marmot/onebot"
	"strings"
)

type IModule interface {
	Init(mgr *ModuleMgr) bool
	Stop(mgr *ModuleMgr)
	Reload(mgr *ModuleMgr)
}

var registry = make(map[string]func() IModule)

func RegisterNamed(name string, initFunc func() IModule) {
	name = strings.ToLower(strings.TrimSpace(name))
	registry[name] = initFunc
}

func createModule(name string) IModule {
	name = strings.ToLower(strings.TrimSpace(name))
	if f, ok := registry[name]; ok {
		return f()
	}
	return nil
}

type EventType int

const (
	ETUnknown EventType = iota
	ETGroupMsg
	ETPrivateMsg
	ETGroupQuit
	ETGroupJoin
	ETGroupRequestJoin
)

type EventHandler func(ctx *zero.Ctx)
type Event struct {
	Type    EventType
	Handler EventHandler
	Moduel  *IModule
}

type ModuleMgr struct {
	loadedModules map[string]IModule
	events        map[EventType][]Event
	cmd           *CmdMgr
}

var sharedInstance *ModuleMgr

func GetModuleMgr() *ModuleMgr {
	return sharedInstance
}

func NewModuleMgr() *ModuleMgr {
	sharedInstance = &ModuleMgr{
		loadedModules: make(map[string]IModule),
		events:        make(map[EventType][]Event),
		cmd:           newCmdMgr(),
	}
	return sharedInstance
}

func (m *ModuleMgr) RegisterCmd() *CmdMgr {
	return m.cmd
}

func (m *ModuleMgr) RegisterEvent(tp EventType, handler EventHandler) bool {
	arr, ok := m.events[tp]
	if !ok {
		m.events[tp] = make([]Event, 0)
		arr = m.events[tp]
	}
	for _, event := range arr {
		if &event.Handler == &handler {
			return false
		}
	}
	m.events[tp] = append(arr, Event{
		Type:    tp,
		Handler: handler,
	})
	return true
}

func (m *ModuleMgr) UnloadAll() {
	LogInfo("[Bot] Unloading all modules...")
	for _, module := range m.loadedModules {
		module.Stop(m)
	}
	m.events = nil
	m.loadedModules = nil
}

func (m *ModuleMgr) HandleEvent(c *zero.Ctx) {
	var msgType EventType
	if c.Event.PostType == "message" && c.Event.MessageType == "group" {
		if strings.HasPrefix(c.Event.RawMessage, AppConfig.CmdPrefix) {
			m.cmd.OnCmd(c)
			return
		} else {
			msgType = ETGroupMsg
		}
	} else if c.Event.PostType == "notice" {
		if c.Event.DetailType == "group_decrease" {
			msgType = ETGroupQuit
		} else if c.Event.DetailType == "group_increase" {
			msgType = ETGroupJoin
		}
	} else if c.Event.PostType == "message" && c.Event.MessageType == "private" {
		msgType = ETPrivateMsg
	} else {
		msgType = ETUnknown
	}

	r, ok := m.events[msgType]
	if ok {
		for _, event := range r {
			go event.Handler(c)
		}
	}
}

func (m *ModuleMgr) GetModule(key string) *IModule {
	key = strings.ToLower(strings.TrimSpace(key))
	if f, ok := m.loadedModules[key]; ok {
		return &f
	}
	return nil
}

func (m *ModuleMgr) LoadAll() {
	count := 1
	for _, module := range AppConfig.Modules {
		LogDebug("[Bot] loading module %v/%v : %s", count, len(AppConfig.Modules), module)
		r := createModule(module)
		if r == nil {
			LogWarn("[Bot] failed to load module : %s , not found or invalid key", module)
			continue
		}
		if !r.Init(m) {
			LogError("[Bot] failed to load module : %s , init failed", module)
			continue
		}
		m.loadedModules[module] = r
		count++
	}
	LogInfo("[Bot] loaded %d modules", count-1)
}

func (m *ModuleMgr) ReloadAll() {
	for _, module := range m.loadedModules {
		module.Reload(m)
	}
	LogInfo("[Bot] reloaded %d modules", len(m.loadedModules))
}

func (m *ModuleMgr) ListAll() []string {
	result := make([]string, len(m.loadedModules))
	idx := 0
	for i := range m.loadedModules {
		result[idx] = i
		idx++
	}
	return result
}
