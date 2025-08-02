package onebot

import (
	"fmt"
	"marmot/onebot/message"
	"reflect"
	"sync"
	"unsafe"
)

type State map[string]interface{}

// Ctx represents the Context which hold the event.
type Ctx struct {
	Event  *Event
	caller APICaller
	State  State

	// lazy message
	once    sync.Once
	message string
}

// ExposeCaller as *T, maybe panic if misused
func ExposeCaller[T any](ctx *Ctx) *T {
	return (*T)(*(*unsafe.Pointer)(unsafe.Add(unsafe.Pointer(&ctx.caller), unsafe.Sizeof(uintptr(0)))))
}

type dec struct {
	index int
	key   string
}

type decoder []dec

var decoderCache = sync.Map{}

// Parse 将 Ctx.State 映射到结构体
func (ctx *Ctx) Parse(model interface{}) (err error) {
	var (
		rv       = reflect.ValueOf(model).Elem()
		t        = rv.Type()
		modelDec decoder
	)
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parse state error: %v", r)
		}
	}()
	d, ok := decoderCache.Load(t)
	if ok {
		modelDec = d.(decoder)
	} else {
		modelDec = decoder{}
		for i := 0; i < t.NumField(); i++ {
			t1 := t.Field(i)
			if key, ok := t1.Tag.Lookup("zero"); ok {
				modelDec = append(modelDec, dec{
					index: i,
					key:   key,
				})
			}
		}
		decoderCache.Store(t, modelDec)
	}
	for _, d := range modelDec { // decoder类型非小内存，无法被编译器优化为快速拷贝
		rv.Field(d.index).Set(reflect.ValueOf(ctx.State[d.key]))
	}
	return nil
}

// Send 快捷发送消息/合并转发
func (ctx *Ctx) Send(msg interface{}) message.ID {
	event := ctx.Event
	m, ok := msg.(message.Message)
	if !ok {
		var p *message.Message
		p, ok = msg.(*message.Message)
		if ok {
			m = *p
		}
	}
	if ok && len(m) > 0 && m[0].Type == "node" && event.DetailType != "guild" {
		if event.GroupID != 0 {
			return message.NewMessageIDFromInteger(ctx.SendGroupForwardMessage(event.GroupID, m).Get("message_id").Int())
		}
		return message.NewMessageIDFromInteger(ctx.SendPrivateForwardMessage(event.UserID, m).Get("message_id").Int())
	}
	if event.DetailType == "guild" {
		return message.NewMessageIDFromString(ctx.SendGuildChannelMessage(event.GuildID, event.ChannelID, msg))
	}
	if event.GroupID != 0 {
		return message.NewMessageIDFromInteger(ctx.SendGroupMessage(event.GroupID, msg))
	}
	return message.NewMessageIDFromInteger(ctx.SendPrivateMessage(event.UserID, msg))
}

func (ctx *Ctx) SendChain(msg ...message.Segment) message.ID {
	if len(msg) > 0 {
		newMsg := make(message.Message, 0, len(msg)*2)
		for i := 0; i < len(msg)-1; i++ {
			newMsg = append(newMsg, msg[i])
			if msg[i].Type != "at" {
				continue
			}
			if msg[i+1].Type != "text" ||
				(len(msg[i+1].Data["text"]) > 0 && msg[i+1].Data["text"][0] != ' ') {
				newMsg = append(newMsg, message.Text(" "))
			}
		}
		newMsg = append(newMsg, msg[len(msg)-1])
		msg = newMsg
	}
	return ctx.Send((message.Message)(msg))
}

// Echo 向自身分发虚拟事件
func (ctx *Ctx) Echo(response []byte) {
	if BotConfig.RingLen != 0 {
		evring.processEvent(response, ctx.caller)
	} else {
		processEventAsync(response, ctx.caller, BotConfig.MaxProcessTime)
	}
}

// ExtractPlainText 提取消息中的纯文本
func (ctx *Ctx) ExtractPlainText() string {
	if ctx == nil || ctx.Event == nil || ctx.Event.Message == nil {
		return ""
	}
	return ctx.Event.Message.ExtractPlainText()
}

// MessageString 字符串消息便于Regex
func (ctx *Ctx) MessageString() string {
	ctx.once.Do(func() {
		if ctx.Event != nil && ctx.Event.Message != nil {
			ctx.message = ctx.Event.Message.String()
		}
	})
	return ctx.message
}
