package modules

import (
	"encoding/base64"
	"fmt"
	"github.com/goccy/go-json"
	"gorm.io/gorm"
	"io"
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"net/http"
)

type MojangProfile struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Properties []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"properties"`
}

type SkinProperties struct {
	Textures struct {
		Skin struct {
			URL string `json:"url"`
		} `json:"SKIN"`
	} `json:"textures"`
}

type CapeProperties struct {
	Textures struct {
		Skin struct {
			URL string `json:"url"`
		} `json:"CAPE"`
	} `json:"textures"`
}

type MojangResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type McSession struct {
	Username string `gorm:"primary_key"`
	Session  string
}

type McQuery struct {
	db *gorm.DB
}

func (m *McQuery) onMcSkin(args []string, ctx *zero.Ctx) {
	if len(args) != 1 {
		ctx.Send(core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text("需要提供玩家id")))
		return
	}

	s, e := m.getMCSkinURL(args[0])
	if e != nil {
		ctx.Send(core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text(fmt.Sprintf("请求失败 %v", e))))
		return
	}

	ctx.Send(core.MakeReply(message.Reply(ctx.Event.MessageID), message.Image(s)))
}

func (m *McQuery) onMcCape(args []string, ctx *zero.Ctx) {
	if len(args) != 1 {
		ctx.Send(core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text("需要提供玩家id")))
		return
	}

	s, e := m.getMCCapeURL(args[0])
	if e != nil {
		ctx.Send(core.MakeReply(message.Reply(ctx.Event.MessageID), message.Text(fmt.Sprintf("请求失败 %v", e))))
		return
	}

	ctx.Send(core.MakeReply(message.Reply(ctx.Event.MessageID), message.Image(s)))
}

func (m *McQuery) getMCTexture(username string) (string, error) {
	s, e := m.getSession(username)
	if !e {
		return "", fmt.Errorf("failed to find session")
	}
	url := fmt.Sprintf("https://sessionserver.mojang.com/session/minecraft/profile/%s", s)

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch profile: %s", body)
	}

	var profile MojangProfile
	err = json.Unmarshal(body, &profile)
	if err != nil {
		return "", err
	}

	return profile.Properties[0].Value, nil
}

func (m *McQuery) getMCSkinURL(username string) (string, error) {
	s, e := m.getMCTexture(username)
	if e != nil {
		return "", e
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}

	var skinData SkinProperties
	err = json.Unmarshal(decoded, &skinData)
	if err != nil {
		return "", err
	}

	skinURL := skinData.Textures.Skin.URL
	if skinURL == "" {
		return "", fmt.Errorf("skin URL not found")
	}

	return skinURL, nil
}

func (m *McQuery) getMCCapeURL(username string) (string, error) {
	s, e := m.getMCTexture(username)
	if e != nil {
		return "", e
	}

	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}

	var capeData CapeProperties
	err = json.Unmarshal(decoded, &capeData)
	if err != nil {
		return "", err
	}

	capeURL := capeData.Textures.Skin.URL
	if capeURL == "" {
		return "", fmt.Errorf("skin URL not found")
	}

	return capeURL, nil
}

func (m *McQuery) getSession(userId string) (string, bool) {
	usr := &McSession{}
	if e := m.db.
		Where("username = ?", userId).
		First(usr); e.Error == nil {
		return usr.Session, true
	}

	url := fmt.Sprintf("https://api.mojang.com/users/profiles/minecraft/%s", userId)

	resp, err := http.Get(url)
	if err != nil {
		core.LogWarn("[McQuery] Failed to get session info from %s", url)
		return "", false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		core.LogWarn("[McQuery] Failed to read session info from %s", url)
		return "", false
	}
	if resp.StatusCode != http.StatusOK {
		core.LogWarn("[McQuery] Failed to get session info from %s", url)
		return "", false
	}

	var result MojangResponse
	err = json.Unmarshal(body, &result)
	if err != nil {
		return "", false
	}

	ses := McSession{
		Username: userId,
		Session:  result.ID,
	}
	r := core.Common.Database.Insert(&ses)
	if r != nil {
		core.LogError("[McQuery] Failed to insert session info in database: %s", r)
		return "", false
	}

	return ses.Session, true
}

func (m *McQuery) Init(mgr *core.ModuleMgr) bool {
	m.db = core.Common.Database.Db
	if m.db == nil {
		core.LogError("[McQuery] bot database not init!")
		return false
	}
	err := m.db.AutoMigrate(&McSession{})
	if err != nil {
		core.LogError("[McQuery] bot database execute mirgate err: %v", err)
		return false
	}

	mgr.RegisterCmd().
		RegisterMember("McSkin", m.onMcSkin).
		RegisterMember("McCape", m.onMcCape)

	return true
}

func (m *McQuery) Stop(mgr *core.ModuleMgr) {

}

func (m *McQuery) Reload(mgr *core.ModuleMgr) {
	m.Init(mgr)
	m.Stop(mgr)
}

func init() {
	core.RegisterNamed("mcq", func() core.IModule {
		return &McQuery{}
	})
}
