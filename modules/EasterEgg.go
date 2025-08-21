package modules

import (
	"io/fs"
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"os"
	"path/filepath"
	"strings"
)

type EggItem struct {
	path    string
	content []byte
}

type EasterEgg struct {
	eggs map[string]*EggItem
}

func (e *EasterEgg) Init(mgr *core.ModuleMgr) bool {
	r, b := core.GetSubDir("EasterEgg")
	if !b {
		core.LogError("[EasterEgg] subdir not found")
		return false
	}

	err := filepath.WalkDir(r, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".png") && !strings.HasSuffix(path, ".jpg") {
			return nil
		}
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		core.LogDebug("[EasterEgg] loaded %s -> %s", name, path)

		e.eggs[name] = &EggItem{
			path:    path,
			content: nil,
		}
		return nil
	})
	if err != nil {
		core.LogError("[EasterEgg] exception occured when walking sub-directory, err:", err)
		return false
	}

	mgr.RegisterEvent(core.ETGroupMsg, e.onMsg)

	return true
}

func (e *EasterEgg) Stop(_ *core.ModuleMgr) {
	e.eggs = nil
}

func (e *EasterEgg) Reload(mgr *core.ModuleMgr) {
	e.Init(mgr)
	e.Stop(mgr)
}

func (e *EasterEgg) onMsg(ctx *zero.Ctx) {
	txt := ctx.ExtractPlainText()
	if !strings.HasPrefix(txt, "!") {
		return
	}
	txt = txt[1:]
	r, ok := e.eggs[txt]
	if !ok {
		return
	}
	if r.content == nil {
		dat, err := os.ReadFile(r.path)
		if err != nil {
			core.LogError("[EasterEgg] read file %s failed, err:%s", r.path, err)
			return
		}
		r.content = dat
	}
	ctx.SendGroupMessage(ctx.Event.GroupID, core.MakeReply(message.Reply(ctx.Event.MessageID), message.ImageBytes(r.content)))
}

func init() {
	core.RegisterNamed("easter_egg", func() core.IModule {
		return &EasterEgg{
			eggs: make(map[string]*EggItem),
		}
	})
}
