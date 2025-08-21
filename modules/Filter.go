package modules

import (
	"bufio"
	"fmt"
	"github.com/RomiChan/syncx"
	"github.com/cloudflare/ahocorasick"
	"gorm.io/gorm"
	"io/fs"
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"marmot/utils"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

const (
	TwentyNineFiftyNineFiftyNine = 29*24*60*60 + 23*60*60 + 59*60
)

type BlockCfg struct {
	GroupIds []int64       `koanf:"group_ids" yaml:"group_ids"`
	BanUser  bool          `koanf:"ban_user" yaml:"ban_user"`
	BanRule  map[int]int64 `koanf:"ban_rule" yaml:"ban_rule"`
	BanMsg   string        `koanf:"ban_msg" yaml:"ban_msg"`
}

func (b BlockCfg) CreateDefaultConfig() interface{} {
	return &BlockCfg{
		GroupIds: []int64{},
		BanUser:  false,
		BanRule: map[int]int64{
			1: 1 * 60,
			2: 10 * 60,
			3: 100 * 60,
		},
		BanMsg: "第 %v 次触发群违规词汇，被禁言 %s",
	}
}

type BanHistoryItem struct {
	Id    int64 `gorm:"primaryKey"`
	Times int32
}

type FilterEngine struct {
	config    *BlockCfg
	db        *gorm.DB
	plainText []string
	tempLock  bool
	matcher   *ahocorasick.Matcher
	regexList []*regexp.Regexp
	banMap    *syncx.Map[int64, *atomic.Int32]
}

func isRegexLine(line string) bool {
	const special = `.^$*+?{}[]()|\`
	return strings.ContainsAny(line, special)
}

func (m *FilterEngine) loadRules() error {
	pth, r := core.GetSubDir("rules")
	if !r {
		return fmt.Errorf("failed to setup rules's directory at %s", pth)
	}

	err := filepath.WalkDir(pth, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".txt") {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			if isRegexLine(line) {
				pattern := line[1 : len(line)-1]
				re, err := regexp.Compile(pattern)
				if err != nil {
					return err
				}
				core.LogDebug("[Filter] loaded regx rule %s", pattern)
				m.regexList = append(m.regexList, re)
			} else {
				core.LogDebug("[Filter] loaded normal rule %s", line)
				m.plainText = append(m.plainText, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(m.plainText) > 0 {
		m.matcher = ahocorasick.NewStringMatcher(m.plainText)
	}
	return nil
}

func (m *FilterEngine) OnReqStop(_ []string, ctx *zero.Ctx) {
	m.tempLock = !m.tempLock
	ctx.Send(fmt.Sprintf("消息审查模式状态: %v 操作人: %s", m.tempLock, ctx.Event.Sender.Name()))
	return
}

func (m *FilterEngine) readBanData(id int64) int32 {
	r, ok := m.banMap.Load(id)
	if ok {
		return r.Load()
	} else {
		var iRel BanHistoryItem
		rT := m.db.Where("id = ?", id).First(&iRel)
		if rT.Error != nil {
			r = &atomic.Int32{}
			r.Store(0)
			m.banMap.Store(id, r)
			err := core.Common.Database.Insert(&BanHistoryItem{
				Id:    id,
				Times: 0,
			})
			if err != nil {
				core.LogError("[Filter] failed to insert ban history item: %v", err)
			}
			return 0
		}

		r = &atomic.Int32{}
		r.Store(iRel.Times)
		m.banMap.Store(id, r)
		return iRel.Times
	}
}

func (m *FilterEngine) updateBanData(id int64, times int32) {
	rT, ok := m.banMap.Load(id)
	if !ok {
		core.LogError("[Filter] invalid operation 'updateBanData'")
		return
	}
	rT.Store(times)
	m.banMap.Store(id, rT)

	err := core.Common.Database.Update(&BanHistoryItem{
		Id:    id,
		Times: times,
	})
	if err != nil {
		core.LogError("[Filter] failed to update ban history item: %v", err)
	}
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

	err := m.loadRules()
	if err != nil {
		core.LogWarn("FilterEngine load rules error: %v", err)
		return false
	}

	if m.config.BanUser {
		m.banMap = new(syncx.Map[int64, *atomic.Int32])
		m.db = core.Common.Database.Db

		err := m.db.AutoMigrate(&BanHistoryItem{})
		if err != nil {
			core.LogError("[Filter] Database auto-migrate error: %v", err)
			return false
		}
	}

	return true
}

func (m *FilterEngine) Stop(_ *core.ModuleMgr) {
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

	txt := extractPlainText(ctx.Event.Message)
	isMatched := false
	for _, r := range m.regexList {
		if r.Match(txt) {
			isMatched = true
			break
		}
	}
	if !isMatched {
		hints := m.matcher.MatchThreadSafe(txt)
		if len(hints) == 0 {
			return
		}
	}
	ctx.DeleteMessage(ctx.Event.MessageID)

	if m.config.BanUser {
		rawVal := m.readBanData(ctx.Event.Sender.ID)
		rawVal += 1
		m.updateBanData(ctx.Event.Sender.ID, rawVal)

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

}

func newMsgBlock() core.IModule {
	return &FilterEngine{
		config:    nil,
		tempLock:  false,
		plainText: make([]string, 0),
		regexList: make([]*regexp.Regexp, 0),
	}
}

// register for current module
func init() {
	core.RegisterNamed("filter", newMsgBlock)
}
