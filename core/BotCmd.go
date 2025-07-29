package core

import (
	"fmt"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"marmot/utils"
	"strings"
	"time"
)

type CmdHandler func(args []string, ctx *zero.Ctx)
type CmdInfo struct {
	handler    CmdHandler
	permission byte
}

type CmdCall struct {
	c    *zero.Ctx
	time int64
}

type CmdMgr struct {
	reqMap map[int64]int64
	cmds   map[string]CmdInfo
	buf    *utils.RingQueue[CmdCall]
	dur    int64
	durTxt string
}

func newCmdMgr() *CmdMgr {
	tmd, err := time.ParseDuration(AppConfig.CmdCoolDown)
	if err != nil {
		tmd = time.Second * 5
	}

	mgr := &CmdMgr{
		reqMap: make(map[int64]int64),
		cmds:   make(map[string]CmdInfo),
		buf:    utils.NewRingQueue[CmdCall](100),
		dur:    tmd.Nanoseconds(),
		durTxt: fmt.Sprintf("抱歉，您发送的太快了 命令冷却时间:%s", AppConfig.CmdCoolDown),
	}

	go mgr.processor()

	return mgr
}

func (m *CmdMgr) processor() {
	for {
		t := m.buf.WaitDequeue()
		id := t.c.Event.Sender.ID
		lTime, ok := m.reqMap[id]
		if !ok {
			m.reqMap[id] = time.Now().UnixNano()
			go m.invokeCmd(t.c)
		} else {
			dur := t.time - lTime
			m.reqMap[id] = t.time
			if dur < m.dur {
				t.c.SendGroupMessage(t.c.Event.GroupID, MakeReply(message.Reply(t.c.Event.MessageID), message.Text(m.durTxt)))
				continue
			}

			go m.invokeCmd(t.c)
		}
	}
}

func (m *CmdMgr) Register(label string, handler CmdHandler, permission byte) {
	_, ok := m.cmds[label]
	if ok {
		LogError("[Bot] Duplicated cmd: %s", label)
		return
	}
	info := CmdInfo{handler: handler, permission: permission}
	m.cmds[label] = info
}

func (m *CmdMgr) RegisterMember(label string, handler CmdHandler) *CmdMgr {
	m.Register(label, handler, 0)
	return m
}

func (m *CmdMgr) RegisterGroupAdmin(label string, handler CmdHandler) *CmdMgr {
	m.Register(label, handler, 1)
	return m
}

func (m *CmdMgr) RegisterBotAdmin(label string, handler CmdHandler) *CmdMgr {
	m.Register(label, handler, 2)
	return m
}

func (m *CmdMgr) invokeCmd(c *zero.Ctx) {
	msg := c.ExtractPlainText()
	lb, arg := parseInputCmd(msg, AppConfig.CmdPrefix)
	cmd, ok := m.cmds[lb]
	if !ok {
		LogError("[Bot] Command not found: %s", msg)
		return
	}

	switch cmd.permission {
	case 2:
		if !IsBotAdmin(c) {
			c.SendGroupMessage(c.Event.GroupID, MakeReply(message.Reply(c.Event.MessageID), message.Text("很抱歉 您没有权限执行这条命令 只有管理员可以执行")))
			return
		}
		break
	case 1:
		if !IsGroupAdmin(c) && !IsBotAdmin(c) {
			c.SendGroupMessage(c.Event.GroupID, MakeReply(message.Reply(c.Event.MessageID), message.Text("很抱歉 您没有权限执行这条命令 只有管理员可以执行")))
			return
		}
		break
	default:
	case 0:
		break
	}
	cmd.handler(arg, c)
}

func (m *CmdMgr) OnCmd(c *zero.Ctx) {
	err := m.buf.Enqueue(CmdCall{c: c, time: time.Now().UnixNano()})
	if err != nil {
		c.SendGroupMessage(c.Event.GroupID, MakeReply(message.Reply(c.Event.MessageID), message.Text("命令无法被处理，内部错误")))
		LogError("[Bot] command enqueue failed %v", err)
	}
}

// parseInputCmd parses input into command and args, respecting quoted strings.
func parseInputCmd(input string, prefix string) (cmd string, args []string) {
	inputLen := len(input)
	args = make([]string, 0, 8)
	var b strings.Builder
	inQuotes := false
	escaped := false

	// TIPS avoid rune allocation
	for i := 0; i < inputLen; i++ {
		c := input[i]

		switch {
		case escaped:
			b.WriteByte(c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == '"':
			inQuotes = !inQuotes
		case c == ' ' || c == '\t':
			if inQuotes {
				b.WriteByte(c)
			} else if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}

	if len(args) == 0 {
		return "", nil
	}
	cmd = strings.TrimPrefix(args[0], prefix)
	return cmd, args[1:]
}
