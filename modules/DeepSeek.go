package modules

import (
	"bytes"
	"fmt"
	"github.com/goccy/go-json"
	"io"
	"marmot/core"
	zero "marmot/onebot"
	"marmot/onebot/message"
	"marmot/utils"
	"net/http"
	"strings"
)

const deepseekAPI = "https://api.deepseek.com/chat/completions"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

type AskTsk struct {
	prompt string
	user   int64
	group  int64
}

type DeepSeekConfig struct {
	ApiKey    string `koanf:"api_key" yaml:"api_key"`
	QueueSize int    `koanf:"queue_size" yaml:"queue_size"`
	Interval  int    `koanf:"interval" yaml:"interval"`
	Prompt    string `koanf:"prompt" yaml:"prompt"`
	Model     string `koanf:"model" yaml:"model"`
}

func (d DeepSeekConfig) CreateDefaultConfig() interface{} {
	return &DeepSeekConfig{
		ApiKey:    "enter-your-api-key",
		QueueSize: 100,
		Interval:  5,
		Prompt:    "You are a helpful assistant.",
		Model:     "deepseek-chat",
	}
}

type DeepSeekAI struct {
	config   *DeepSeekConfig
	reqQueue *utils.RingQueue[AskTsk]
	ctx      *zero.Ctx
	msgTmp   []message.Segment
}

func (s *DeepSeekAI) request(prompt string) (string, error) {
	reqBody := ChatRequest{
		Model: s.config.Model,
		Messages: []Message{
			{Role: "system", Content: s.config.Prompt},
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", deepseekAPI, bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.ApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error: %s", body)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no message returned")
}

func (s *DeepSeekAI) queueListner() {
	for {
		r := s.reqQueue.WaitDequeue()
		rq, e := s.request(r.prompt)
		if s.ctx == nil {
			s.ctx = zero.GetBot(core.Common.BotQQ)
		}
		if e != nil {
			s.ctx.SendGroupMessage(r.group, fmt.Sprintf("[Deepseek] 请求deepseek失败，错误信息 %v", e))
			return
		}

		s.msgTmp[0] = message.At(r.user)
		s.msgTmp[1] = message.Text(" " + rq)

		s.ctx.SendGroupMessage(r.group, s.msgTmp)
	}
}

func (s *DeepSeekAI) Init(mgr *core.ModuleMgr) bool {
	cfg := core.GetSubDirFilePath("deepseek.yml")
	s.config = &DeepSeekConfig{}
	r := core.InitCustomConfig(s.config, cfg)
	if r != nil {
		core.LogWarn("[DeepSeek] init config error: %v. Using default instead", r)
		s.config = s.config.CreateDefaultConfig().(*DeepSeekConfig)
	}

	mgr.RegisterCmd("deepseek", s.onCmd)
	// start service
	go s.queueListner()

	return true
}

func (s *DeepSeekAI) Stop(_ *core.ModuleMgr) {
	s.msgTmp = nil
	s.config = nil
	s.reqQueue = nil
	s.ctx = nil
}

func (s *DeepSeekAI) Reload(mgr *core.ModuleMgr) {
	s.Init(mgr)
	s.Stop(mgr)
}

func (s *DeepSeekAI) onCmd(args []string, ctx *zero.Ctx) {
	txt := strings.Join(args, " ")
	tsk := AskTsk{
		prompt: txt,
		user:   ctx.Event.UserID,
		group:  ctx.Event.GroupID,
	}
	r := s.reqQueue.Enqueue(tsk)
	if r != nil {
		ctx.Send(fmt.Sprintf("[Deepseek] 添加到队列失败 %v", r))
	} else {
		ctx.Send("[Deepseek] 添加到队列成功 请耐心等待")
	}
}

func init() {
	core.RegisterNamed("deepseek", func() core.IModule {
		return &DeepSeekAI{
			reqQueue: utils.NewRingQueue[AskTsk](100),
			msgTmp:   make([]message.Segment, 2),
		}
	})
}
