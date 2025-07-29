package modules

import (
	"encoding/gob"
	"fmt"
	"github.com/RomiChan/syncx"
	"github.com/cloudflare/ahocorasick"
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"marmot/utils"
	"os"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

const (
	TwentyNineFiftyNineFiftyNine = 29*24*60*60 + 23*60*60 + 59*60
)

type BlockCfg struct {
	Triggers  []string      `koanf:"triggers" yaml:"triggers"`
	GroupIds  []int64       `koanf:"group_ids" yaml:"group_ids"`
	AutoReply bool          `koanf:"auto_reply" yaml:"auto_reply"`
	ReplyMsg  string        `koanf:"reply_msg" yaml:"reply_msg"`
	BanUser   bool          `koanf:"ban_user" yaml:"ban_user"`
	BanRule   map[int]int64 `koanf:"ban_rule" yaml:"ban_rule"`
	BanMsg    string        `koanf:"ban_msg" yaml:"ban_msg"`
}

func (b BlockCfg) CreateDefaultConfig() interface{} {
	return &BlockCfg{
		Triggers:  []string{},
		GroupIds:  []int64{},
		AutoReply: false,
		ReplyMsg:  "%s 触发了违禁词 %s bot自动撤回",
		BanUser:   false,
		BanRule: map[int]int64{
			1: 1 * 60,
			2: 10 * 60,
			3: 100 * 60,
		},
		BanMsg: "第 %v 次触发群违规词汇，被禁言 %s",
	}
}

type FilterEngine struct {
	config   *BlockCfg
	tempLock bool
	matcher  *ahocorasick.Matcher
	banMap   *syncx.Map[int64, *atomic.Int32]
}

func (m *FilterEngine) OnReqStop(_ []string, ctx *zero.Ctx) {
	m.tempLock = !m.tempLock
	ctx.Send(fmt.Sprintf("消息审查模式状态: %v 操作人: %s", m.tempLock, ctx.Event.Sender.Name()))
	return
}

func (m *FilterEngine) Init(mgr *core.ModuleMgr) bool {
	path := core.GetSubDirFilePath("filter.yaml")
	m.config = &BlockCfg{}
	r := core.InitCustomConfig[BlockCfg](m.config, path)
	if r != nil {
		core.LogWarn("FilterEngine init config error: %v Using default instead.", r)
		m.config = m.config.CreateDefaultConfig().(*BlockCfg)
	}

	mgr.RegisterCmd().
		RegisterGroupAdmin("SwitchBlock", m.OnReqStop)
	mgr.RegisterEvent(core.ETGroupMsg, m.OnMsg)

	m.matcher = ahocorasick.NewStringMatcher(m.config.Triggers)

	fmt.Printf("banrule: %v \n", m.config.BanUser)

	if m.config.BanUser {
		m.banMap = new(syncx.Map[int64, *atomic.Int32])
		bPath := core.GetSubDirFilePath("banUserHistory.dat")

		if utils.IsFileExists(bPath) {
			file, err := os.Open(bPath)
			if err != nil {
				core.LogError("failed to open ban history file: %v", err)
				return false
			}
			defer file.Close()

			decoder := gob.NewDecoder(file)
			if err := decoder.Decode(&m.banMap); err != nil {
				core.LogError("[FilterEngine] failed to decode ban history: %v", err)
				m.banMap = new(syncx.Map[int64, *atomic.Int32])
			}
		}
	}

	return true
}

func saveBanMap(path string, banMap *syncx.Map[int64, *atomic.Int32]) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(banMap)
}

func (m *FilterEngine) Stop(_ *core.ModuleMgr) {
	if m.config.BanUser {
		r := saveBanMap(core.GetSubDirFilePath("banUserHistory.dat"), m.banMap)
		if r != nil {
			core.LogError("[FilterEngine] failed to save ban history file: %v", r)
		}
	}

	m.banMap = nil
	m.matcher = nil
	m.tempLock = false
	m.config = nil
}

func (m *FilterEngine) Reload(mg *core.ModuleMgr) {
	// I'm so lazy to implement this standalone :(
	m.Stop(mg)
	m.Init(mg)
}

func cleanMessageBytes(input []byte) []byte {
	output := input[:0]

	for i := 0; i < len(input); {

		r, size := utf8.DecodeRune(input[i:])
		i += size

		// skip empty
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\u3000' {
			continue
		}

		// convert（rage：FF01 - FF5E）
		if r >= 0xFF01 && r <= 0xFF5E {
			r -= 0xFEE0
		}

		// keep chinese characters / english characters / numbers
		if !isAllowedRune(r) {
			continue
		}

		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], r)
		output = append(output, buf[:n]...)
	}

	return output
}

func isAllowedRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		isHanCharacter(r)
}

func isHanCharacter(r rune) bool {
	// （CJK Unified Ideographs）
	return unicode.Is(unicode.Han, r)
}

func extractPlainText(m message.Message) []byte {
	totalLen := 0
	for _, val := range m {
		if val.Type == "text" {
			totalLen += len(val.Data["text"])
		}
	}
	if totalLen == 0 {
		return []byte{}
	}

	buf := make([]byte, 0, totalLen)
	for _, val := range m {
		if val.Type == "text" {
			buf = append(buf, val.Data["text"]...)
		}
	}

	return cleanMessageBytes(buf)
}

func (m *FilterEngine) isWhitelistGroup(id int64) bool {
	for _, groupId := range m.config.GroupIds {
		if groupId == id {
			return true
		}
	}
	return false
}

func (m *FilterEngine) OnMsg(ctx *zero.Ctx) {
	if m.tempLock {
		return
	}
	if !m.isWhitelistGroup(ctx.Event.GroupID) {
		return
	}
	if ctx.Event.Sender.ID == core.Common.BotQQ {
		return
	}

	var a = ctx.Event.RawMessage
	var b = ctx.Event.Message.ExtractPlainText()
	core.LogDebug("MsgRaw %s MsgPlain %s", a, b)
	hints := m.matcher.MatchThreadSafe(extractPlainText(ctx.Event.Message))
	if len(hints) == 0 {
		return
	}
	ctx.DeleteMessage(ctx.Event.MessageID)

	if m.config.BanUser {
		r, ok := m.banMap.Load(ctx.Event.Sender.ID)
		if ok {
			r.Add(1)
			m.banMap.Store(ctx.Event.Sender.ID, r)
		} else {
			r = &atomic.Int32{}
			r.Store(1)
			m.banMap.Store(ctx.Event.Sender.ID, r)
		}

		rawVal := r.Load()
		tp, ok2 := m.config.BanRule[int(rawVal)]

		if ok2 && tp > 0 {
			ctx.SetGroupBan(ctx.Event.GroupID, ctx.Event.Sender.ID, tp)
			var msg = make([]message.Segment, 2)
			msg[0] = message.At(ctx.Event.Sender.ID)
			msg[1] = message.Text(fmt.Sprintf(m.config.BanMsg, rawVal, utils.FormatDuration(tp)))
			ctx.Send(msg)
		} else {
			ctx.SetGroupBan(ctx.Event.GroupID, ctx.Event.Sender.ID, TwentyNineFiftyNineFiftyNine)
			var msg = make([]message.Segment, 2)
			msg[0] = message.At(ctx.Event.Sender.ID)
			msg[1] = message.Text(fmt.Sprintf(m.config.BanMsg, rawVal, utils.FormatDuration(TwentyNineFiftyNineFiftyNine)))
			ctx.Send(msg)
		}
	}

	if !m.config.AutoReply {
		return
	}
	for _, idx := range hints {
		ctx.Send(fmt.Sprintf(m.config.ReplyMsg, ctx.Event.Sender.Name(), m.config.Triggers[idx]))
	}
}

func newMsgBlock() core.IModule {
	return &FilterEngine{
		config:   nil,
		tempLock: false,
	}
}

// register for current module
func init() {
	core.RegisterNamed("filter", newMsgBlock)
}
