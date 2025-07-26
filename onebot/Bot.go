package onebot

import (
	"github.com/goccy/go-json"
	"hash/crc64"
	"marmot/onebot/message"
	"marmot/utils"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FloatTech/ttl"
	"github.com/tidwall/gjson"
)

// Config is config of zero bot
type Config struct {
	NickName        []string      `json:"nickname"`           // 机器人名称
	CommandPrefix   string        `json:"command_prefix"`     // 触发命令
	SuperUsers      []int64       `json:"super_users"`        // 超级用户
	RingLen         uint          `json:"ring_len"`           // 事件环长度 (默认关闭)
	Latency         time.Duration `json:"latency"`            // 事件处理延迟 (延迟 latency 再处理事件，在 ring 模式下不可低于 1ms)
	MaxProcessTime  time.Duration `json:"max_process_time"`   // 事件最大处理时间 (默认4min)
	MarkMessage     bool          `json:"mark_message"`       // 自动标记消息为已读
	KeepAtMeMessage bool          `json:"keep_at_me_message"` // 是否保留at me的原始消息
	AddSpaceAfterAt bool          `json:"at_space"`           // 是否在At消息后没有空格时自动添加空格
	Driver          Driver        `json:"-"`                  // 通信驱动
}

// APICallers 所有的APICaller列表， 通过self-ID映射
var APICallers callerMap

// APICaller is the interface of CallAPI
type APICaller interface {
	CallAPI(request APIRequest) (APIResponse, error)
}

// Driver 与OneBot通信的驱动，使用driver.DefaultWebSocketDriver
type Driver interface {
	Connect()
	Listen(func([]byte, APICaller))
}

// BotConfig 运行中bot的配置，是Run函数的参数的拷贝
var BotConfig Config

var (
	evring    *eventRing // evring 事件环
	isrunning uintptr
)

func runinit(op *Config) {
	if op.MaxProcessTime == 0 {
		op.MaxProcessTime = time.Minute * 4
	}
	BotConfig = *op
	if op.RingLen == 0 {
		return
	}
	evring = newring(op.RingLen)
	evring.loop(op.Latency, op.MaxProcessTime, processEventAsync)
}

func (op *Config) directlink(b []byte, c APICaller) {
	go func() {
		if op.Latency != 0 {
			time.Sleep(op.Latency)
		}
		processEventAsync(b, c, op.MaxProcessTime)
	}()
}

// Run 主函数初始化
func Run(op *Config) {
	if !atomic.CompareAndSwapUintptr(&isrunning, 0, 1) {
		LogWarn("[bot] ignored duplicated Run")
	}
	runinit(op)
	linkf := op.directlink
	if op.RingLen != 0 {
		linkf = evring.processEvent
	}
	op.Driver.Connect()
	go op.Driver.Listen(linkf)
}

var _handler func(ctx *Ctx)

// RunAndBlock 主函数初始化并阻
func RunAndBlock(op *Config, handler func(ctx *Ctx)) {
	if handler == nil {
		LogError("[bot] Handler is nil!")
		return
	}
	_handler = handler
	if !atomic.CompareAndSwapUintptr(&isrunning, 0, 1) {
		LogWarn("[bot] ignored calling duplicated RunAndBlock")
	}
	runinit(op)
	linkf := op.directlink
	if op.RingLen != 0 {
		linkf = evring.processEvent
	}
	op.Driver.Connect()
	op.Driver.Listen(linkf)
}

var (
	triggeredMessages   = ttl.NewCache[int64, []message.ID](time.Minute * 5)
	triggeredMessagesMu = sync.Mutex{}
)

type messageLogger struct {
	msgid  message.ID
	caller APICaller
}

// CallAPI 记录被触发的回复消息
func (m *messageLogger) CallAPI(request APIRequest) (rsp APIResponse, err error) {
	noLog := false
	b, ok := request.Params["__zerobot_no_log_mseeage_id__"].(bool)
	if ok {
		noLog = b
		delete(request.Params, "__zerobot_no_log_mseeage_id__")
	}
	rsp, err = m.caller.CallAPI(request)
	if err != nil {
		return
	}
	id := rsp.Data.Get("message_id")
	if id.Exists() && !noLog {
		mid := m.msgid.ID()
		triggeredMessagesMu.Lock()
		defer triggeredMessagesMu.Unlock()
		triggeredMessages.Set(mid,
			append(
				triggeredMessages.Get(mid),
				message.NewMessageIDFromString(id.String()),
			),
		)
	}
	return
}

// GetTriggeredMessages 获取被 id 消息触发的回复消息 id
func GetTriggeredMessages(id message.ID) []message.ID {
	triggeredMessagesMu.Lock()
	defer triggeredMessagesMu.Unlock()
	return triggeredMessages.Get(id.ID())
}

// processEventAsync 从池中处理事件, 异步调用匹配 mather
func processEventAsync(response []byte, caller APICaller, maxwait time.Duration) {
	var event Event
	_ = json.Unmarshal(response, &event)
	event.RawEvent = gjson.Parse(utils.BytesToString(response))
	var msgid message.ID
	messageID, err := strconv.ParseInt(utils.BytesToString(event.RawMessageID), 10, 64)
	if err == nil {
		event.MessageID = messageID
		msgid = message.NewMessageIDFromInteger(messageID)
	} else if event.MessageType == "guild" {
		// 是 guild 消息，进行如下转换以适配非 guild 插件
		// MessageID 填为 string
		event.MessageID, _ = strconv.Unquote(utils.BytesToString(event.RawMessageID))
		// 伪造 GroupID
		crc := crc64.New(crc64.MakeTable(crc64.ISO))
		crc.Write(utils.StringToBytes(event.GuildID))
		crc.Write(utils.StringToBytes(event.ChannelID))
		r := int64(crc.Sum64() & 0x7fff_ffff_ffff_ffff) // 确保为正数
		if r <= 0xffff_ffff {
			r |= 0x1_0000_0000 // 确保不与正常号码重叠
		}
		event.GroupID = r
		// 伪造 UserID
		crc.Reset()
		crc.Write(utils.StringToBytes(event.TinyID))
		r = int64(crc.Sum64() & 0x7fff_ffff_ffff_ffff) // 确保为正数
		if r <= 0xffff_ffff {
			r |= 0x1_0000_0000 // 确保不与正常号码重叠
		}
		event.UserID = r
		if event.Sender != nil {
			event.Sender.ID = r
		}
		msgid = message.NewMessageIDFromString(event.MessageID.(string))
	}

	switch event.PostType { // process DetailType
	case "message", "message_sent":
		event.DetailType = event.MessageType
	case "notice":
		event.DetailType = event.NoticeType
		preprocessNoticeEvent(&event)
	case "request":
		event.DetailType = event.RequestType
	}
	if event.PostType == "message" {
		preprocessMessageEvent(&event)
	}
	ctx := &Ctx{
		Event:  &event,
		caller: &messageLogger{msgid: msgid, caller: caller},
	}
	//matcherLock.Lock()
	//if hasMatcherListChanged {
	//	matcherListForRanging = make([]*Matcher, len(matcherList))
	//	copy(matcherListForRanging, matcherList)
	//	hasMatcherListChanged = false
	//}
	//matcherLock.Unlock()
	//go match(ctx, matcherListForRanging, maxwait)
	go _handler(ctx)
}

//
//func match(ctx *Ctx, matchers []*Matcher, maxwait time.Duration) {
//	if BotConfig.MarkMessage && ctx.Event.MessageID != nil {
//		ctx.MarkThisMessageAsRead()
//	}
//	gorule := func(rule Rule) <-chan bool {
//		ch := make(chan bool, 1)
//		go func() {
//			defer func() {
//				close(ch)
//				if pa := recover(); pa != nil {
//					LogError("[bot] execute rule err: %v\n%v", pa, utils.BytesToString(debug.Stack()))
//				}
//			}()
//			ch <- rule(ctx)
//		}()
//		return ch
//	}
//	gohandler := func(h Handler) <-chan struct{} {
//		ch := make(chan struct{}, 1)
//		go func() {
//			defer func() {
//				close(ch)
//				if pa := recover(); pa != nil {
//					LogError("[bot] execute handler err: %v\n%v", pa, utils.BytesToString(debug.Stack()))
//				}
//			}()
//			h(ctx)
//			ch <- struct{}{}
//		}()
//		return ch
//	}
//	t := time.NewTimer(maxwait)
//	defer t.Stop()
//loop:
//	for _, matcher := range matchers {
//		if !matcher.Type(ctx) {
//			continue
//		}
//		for k := range ctx.State { // Clear State
//			delete(ctx.State, k)
//		}
//		m := matcher.copy()
//		ctx.ma = m
//
//		// pre handler
//		if m.Engine != nil {
//			for _, handler := range m.Engine.preHandler {
//				c := gorule(handler)
//				for {
//					select {
//					case ok := <-c:
//						if !ok { // 有 pre handler 未满足
//							if m.Break { // 阻断后续
//								break loop
//							}
//							continue loop
//						}
//					case <-t.C:
//						if m.NoTimeout { // 不设超时限制
//							t.Reset(maxwait)
//							continue
//						}
//						LogWarn("[bot] timeout occured when handling preHandler stage, halt")
//						break loop
//					}
//					break
//				}
//			}
//		}
//
//		for _, rule := range m.Rules {
//			c := gorule(rule)
//			for {
//				select {
//				case ok := <-c:
//					if !ok { // 有 Rule 的条件未满足
//						if m.Break { // 阻断后续
//							break loop
//						}
//						continue loop
//					}
//				case <-t.C:
//					if m.NoTimeout { // 不设超时限制
//						t.Reset(maxwait)
//						continue
//					}
//					LogWarn("[bot] timeout occured when handling rule stage, halt")
//					break loop
//				}
//				break
//			}
//		}
//
//		// mid handler
//		if m.Engine != nil {
//			for _, handler := range m.Engine.midHandler {
//				c := gorule(handler)
//				for {
//					select {
//					case ok := <-c:
//						if !ok { // 有 mid handler 未满足
//							if m.Break { // 阻断后续
//								break loop
//							}
//							continue loop
//						}
//					case <-t.C:
//						if m.NoTimeout { // 不设超时限制
//							t.Reset(maxwait)
//							continue
//						}
//						LogWarn("[bot] timeout occured when handling midHandler stage, halt")
//						break loop
//					}
//					break
//				}
//			}
//		}
//
//		if m.Handler != nil {
//			c := gohandler(m.Handler)
//			for {
//				select {
//				case <-c: // 处理事件
//				case <-t.C:
//					if m.NoTimeout { // 不设超时限制
//						t.Reset(maxwait)
//						continue
//					}
//					LogWarn("[bot] timeout occured when handling Handler stage, halt")
//					break loop
//				}
//				break
//			}
//		}
//		if matcher.Temp { // 临时 Matcher 删除
//			matcher.Delete()
//		}
//
//		if m.Engine != nil {
//			// post handler
//			for _, handler := range m.Engine.postHandler {
//				c := gohandler(handler)
//				for {
//					select {
//					case <-c:
//					case <-t.C:
//						if m.NoTimeout { // 不设超时限制
//							t.Reset(maxwait)
//							continue
//						}
//						LogWarn("[bot] timeout occured when handling postHandler stage, halt")
//						break loop
//					}
//					break
//				}
//			}
//		}
//
//		if m.Block { // 阻断后续
//			break loop
//		}
//	}
//}

// preprocessMessageEvent 返回信息事件
func preprocessMessageEvent(e *Event) {
	msgs := message.ParseMessage(e.NativeMessage)

	if len(msgs) > 0 {
		filtered := make([]message.Segment, 0, len(msgs))
		// trim space after at and remove empty text segment
		for i := range msgs {
			if i < len(msgs)-1 && msgs[i].Type == "at" && msgs[i+1].Type == "text" {
				msgs[i+1].Data["text"] = strings.TrimLeft(msgs[i+1].Data["text"], " ")
			}
			if msgs[i].Type != "text" || msgs[i].Data["text"] != "" {
				filtered = append(filtered, msgs[i])
			}
		}
		e.Message = filtered
	}

	processAt := func() { // 处理是否at机器人
		e.IsToMe = false
		if len(e.Message) == 0 {
			return
		}
		for i, m := range e.Message {
			if m.Type == "at" {
				qq, _ := strconv.ParseInt(m.Data["qq"], 10, 64)
				if qq == e.SelfID {
					e.IsToMe = true
					if !BotConfig.KeepAtMeMessage {
						e.Message = append(e.Message[:i], e.Message[i+1:]...)
					}
					return
				}
			}
		}
		if len(e.Message) == 0 || e.Message[0].Type != "text" {
			return
		}
		first := e.Message[0]
		first.Data["text"] = strings.TrimLeft(first.Data["text"], " ") // Trim!
		text := first.Data["text"]
		for _, nickname := range BotConfig.NickName {
			if strings.HasPrefix(text, nickname) {
				e.IsToMe = true
				first.Data["text"] = text[len(nickname):]
				return
			}
		}
	}

	switch {
	case e.DetailType == "group":
		LogDebug("[bot] received message from group (%v) %v : %v", e.GroupID, e.Sender.String(), e.RawMessage)
		processAt()
	case e.DetailType == "guild" && e.SubType == "channel":
		LogDebug("[bot] received message from channel (%v)(%v-%v) %v : %v", e.GroupID, e.GuildID, e.ChannelID, e.Sender.String(), e.Message)
		processAt()
	default:
		e.IsToMe = true // 私聊也判断为at
		LogDebug("[bot] received DM message from %v : %v", e.Sender.String(), e.RawMessage)
	}
	if len(e.Message) > 0 && e.Message[0].Type == "text" { // Trim Again!
		e.Message[0].Data["text"] = strings.TrimLeft(e.Message[0].Data["text"], " ")
	}
}

// preprocessNoticeEvent 更新事件
func preprocessNoticeEvent(e *Event) {
	if e.SubType == "poke" || e.SubType == "lucky_king" {
		e.IsToMe = e.TargetID == e.SelfID
	} else {
		e.IsToMe = e.UserID == e.SelfID
	}
}

// GetBot 获取指定的bot (Ctx)实例
func GetBot(id int64) *Ctx {
	caller, ok := APICallers.Load(id)
	if !ok {
		return nil
	}
	return &Ctx{caller: caller}
}

// RangeBot 遍历所有bot (Ctx)实例
//
// 单次操作返回 true 则继续遍历，否则退出
func RangeBot(iter func(id int64, ctx *Ctx) bool) {
	APICallers.Range(func(key int64, value APICaller) bool {
		return iter(key, &Ctx{caller: value})
	})
}

// GetFirstSuperUser 在 qqs 中获得 SuperUsers 列表的首个 qq
//
// 找不到返回 -1
func (op *Config) GetFirstSuperUser(qqs ...int64) int64 {
	m := make(map[int64]struct{}, len(qqs)*4)
	for _, qq := range qqs {
		m[qq] = struct{}{}
	}
	for _, qq := range op.SuperUsers {
		if _, ok := m[qq]; ok {
			return qq
		}
	}
	return -1
}
