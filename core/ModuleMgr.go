package core

import (
	zero "github.com/cubevlmu/CZeroBot"
	"marmot/utils"
	"strings"
)

type IModule interface {
	Init(mgr *ModuleMgr)
	Stop(mgr *ModuleMgr)
	Reload(mgr *ModuleMgr)
	OnMsg(ctx *zero.Ctx)
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

type CmdHandler func(args []string, ctx *zero.Ctx) bool

type ModuleMgr struct {
	loadedModules map[string]IModule
	cmds          map[string]CmdHandler
}

func NewModuleMgr() *ModuleMgr {
	return &ModuleMgr{
		loadedModules: make(map[string]IModule),
		cmds:          make(map[string]CmdHandler),
	}
}

func (m *ModuleMgr) RegisterCmd(name string, handle CmdHandler) bool {
	if _, ok := m.cmds[name]; ok {
		return false
	}
	m.cmds[name] = handle
	return true
}

func (m *ModuleMgr) OnCmd(str string, ctx *zero.Ctx) bool {
	cmd, args := utils.ParseInputCmd(str, AppConfig.CmdPrefix)
	handler, ok := m.cmds[cmd]
	if !ok {
		return false
	}
	resultChan := make(chan bool)

	go func() {
		result := handler(args, ctx)
		resultChan <- result
	}()

	return <-resultChan
}

func (m *ModuleMgr) UnloadAll() {
	LogInfo("[ModuleMgr] Unloading all modules...")
	for _, module := range m.loadedModules {
		module.Stop(m)
	}
	m.cmds = nil
	m.loadedModules = nil
}

func (m *ModuleMgr) BroadcastMsg(z *zero.Ctx) {
	str := z.MessageString()
	if strings.HasPrefix(str, AppConfig.CmdPrefix) {
		r := m.OnCmd(str, z)
		if !r {
			LogError("[ModuleMgr] failed to run command, cmd not found or other error")
		}
		return
	}
	for _, module := range m.loadedModules {
		go module.OnMsg(z)
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
	for i, module := range AppConfig.Modules {
		LogDebug("[ModuleMgr] loading module %v/%v : %s", i, len(AppConfig.Modules), module)
		r := createModule(module)
		if r == nil {
			LogWarn("[ModuleMgr] failed to load module : %s , not found or invalid key", module)
			continue
		}
		r.Init(m)
		m.loadedModules[module] = r
		count++
	}
	LogInfo("[ModuleMgr] loaded %d modules", count)
}

func (m *ModuleMgr) ReloadAll() {
	for _, module := range m.loadedModules {
		module.Reload(m)
	}
	LogInfo("[ModuleMgr] reloaded %d modules", len(m.loadedModules))
}
