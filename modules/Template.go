package modules

import (
	"github.com/derekparker/trie"
	lru "github.com/hashicorp/golang-lru"
	"gorm.io/gorm"
	"marmot/core"
	zero "marmot/onebot"
)

type Template struct {
	Id      int64 `gorm:"primaryKey"`
	Trigger string
	Content string
	Removed bool
}

type TemplateEngine struct {
	db      *gorm.DB
	buffer  *lru.Cache
	matcher *trie.Trie
}

func (t *TemplateEngine) OnMsg(ctx *zero.Ctx) {
	if ctx.Event.GroupID == 0 {
		return
	}
	r := t.tryMatch(ctx.Event.Message.String())
	if r == nil {
		return
	}
	ctx.Send(*r)
}

func (t *TemplateEngine) onAddCmd(args []string, ctx *zero.Ctx) {
	if len(args) != 2 {
		ctx.Send("错误的内容，触发器或者内容为空")
		return
	}
	trigger := args[0]
	content := args[1]

	r := t.addTemplate(trigger, content)
	if r == -1 {
		ctx.SendGroupMessage(ctx.Event.GroupID, "添加失败,触发器重复")
		return
	}

	ctx.SendGroupMessage(ctx.Event.GroupID, "添加成功!")
}

func (t *TemplateEngine) onRemoveCmd(args []string, ctx *zero.Ctx) {
	if len(args) != 1 {
		ctx.Send("格式: DelTemp [Trigger]")
		return
	}
	trigger := args[0]

	tp := &Template{}
	r := t.db.Where("trigger = ?", trigger).First(&tp)
	if r.Error != nil {
		ctx.Send("没有找到这一条模版")
		return
	}

	t.RemoveTemplateById(tp.Id)
	ctx.Send("模版删除成功!")
}

func (t *TemplateEngine) Init(mgr *core.ModuleMgr) bool {
	mgr.RegisterEvent(core.ETGroupMsg, t.OnMsg)
	mgr.RegisterCmd("AddTem", t.onAddCmd)
	mgr.RegisterCmd("DelTem", t.onRemoveCmd)

	return true
}

func (t *TemplateEngine) Stop(_ *core.ModuleMgr) {
	t.close()
}

func (t *TemplateEngine) Reload(_ *core.ModuleMgr) {
	t.buffer = nil
	t.matcher = nil

	cache, err := lru.New(core.AppConfig.MessageBufSize)
	if err != nil {
		core.LogError("[Template] (Reload) failed to create lru cache for TemplateEngine: %v", err)
		return
	}
	t.buffer = cache

	t.matcher = trie.New()
	t.loadTriggers()
}

func newTemplateEngine() *TemplateEngine {
	engine := &TemplateEngine{
		db: core.Common.Database.Db,
	}
	if engine.db == nil {
		core.LogError("[TemplateEngine] failed to connect to database")
		panic("[TemplateEngine] failed to connect to database")
	}
	err := engine.db.AutoMigrate(&Template{})
	if err != nil {
		core.LogError("[Template] failed to set auto-migrate: %v", err)
		return nil
	}

	cache, err := lru.New(core.AppConfig.MessageBufSize)
	if err != nil {
		core.LogError("[Template] failed to create lru cache for TemplateEngine: %v", err)
		return nil
	}
	engine.buffer = cache

	engine.matcher = trie.New()
	engine.loadTriggers()

	return engine
}

func (t *TemplateEngine) loadTriggers() {
	var triggers []string
	if err := t.db.Model(&Template{}).
		Where("removed = ?", false).
		Pluck("trigger", &triggers).Error; err != nil {
		core.LogError("[Template] failed to load Triggers: %v", err)
		return
	}
	var ids []int64
	if err := t.db.Model(&Template{}).Select("id").Pluck("id", &ids).Error; err != nil {
		core.LogError("[Template] failed to load ids: %v", err)
		return
	}

	for idx, trig := range triggers {
		t.matcher.Add(trig, ids[idx])
	}
}

func (t *TemplateEngine) tryMatch(str string) *string {
	val, ok := t.matcher.Find(str)
	if !ok {
		return nil
	}

	if tplRaw, ok := t.buffer.Get(str); ok {
		tpl := tplRaw.(*Template)
		return &tpl.Content
	}

	tpl := t.GetTemplateById(val.Meta().(int64))
	if tpl == nil {
		return nil
	}
	t.buffer.Add(str, tpl)
	return &tpl.Content
}

func (t *TemplateEngine) close() {
	t.buffer = nil
	t.matcher = nil
	t.db = nil
}

func (t *TemplateEngine) GetTemplateById(id int64) *Template {
	var template Template
	r := t.db.Where("id = ?", id).First(&template)
	if r.Error != nil {
		core.LogError("[Template] failed to find template by id: %v", r.Error)
		return nil
	}
	return &template
}

func (t *TemplateEngine) GetTemplateByName(name string) *Template {
	var template Template
	r := t.db.Where("trigger = ?", name).First(&template)
	if r.Error != nil {
		core.LogError("[Template] failed to find template by id: %v", r.Error)
		return nil
	}
	return &template
}

func (t *TemplateEngine) GetTemplateByTrigger(trigger string) *Template {
	var template Template
	r := t.db.Where("trigger = ?", trigger).First(&template)
	if r.Error != nil {
		core.LogError("[Template] failed to find template by trigger: %v", r.Error)
		return nil
	}
	return &template
}

func (t *TemplateEngine) addTemplate(trigger string, content string) int64 {
	tmp := Template{
		Trigger: trigger,
		Content: content,
	}

	var out Template
	rs := t.db.Where("trigger = ?", trigger).First(&out)
	if rs.Error == nil {
		return -1
	}

	r := core.Common.Database.Insert(&tmp)
	if r != nil {
		core.LogError("[Template] failed to insert template: %v", r.Error)
		return -1
	}

	t.buffer.Add(tmp.Id, tmp)
	t.matcher.Add(trigger, tmp.Id)

	return tmp.Id
}

func (t *TemplateEngine) RemoveTemplateById(id int64) {
	var out Template
	r := t.db.Where("id = ?", id).First(&out)
	if r.Error != nil {
		core.LogError("[Template] failed to find template by id: %v", r.Error)
	}

	t.buffer.Remove(out.Id)
	t.matcher.Remove(out.Trigger)
	t.remove(&out)
}

func (t *TemplateEngine) remove(tm *Template) {
	if tm == nil {
		return
	}
	tm.Removed = true
	tm.Content = ""
	tm.Trigger = ""

	r := core.Common.Database.Update(tm)
	if r != nil {
		core.LogError("[Template] failed to remove template, error %v", r.Error)
	}
}

func (t *TemplateEngine) Update(tm *Template) {
	var out Template
	r := t.db.Where("id = ?", tm.Id).First(&out)
	if r.Error != nil {
		core.LogError("[Template] failed to update template: %v", r.Error)
	}

	if out.Trigger != tm.Trigger {
		t.matcher.Remove(out.Trigger)
		t.matcher.Add(tm.Trigger, tm.Id)
	}

	rt := core.Common.Database.Update(&tm)
	if rt != nil {
		core.LogError("[Template] failed to update template, error %v", rt.Error)
	}
}

// register to global map
func init() {
	core.RegisterNamed("template", func() core.IModule {
		return newTemplateEngine()
	})
}
