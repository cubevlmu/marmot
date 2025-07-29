package core

import "fmt"

type GlobalConfig struct {
	WsUrl            string   `koanf:"ws_url" yaml:"ws_url"`
	RecordLog        bool     `koanf:"record_log" yaml:"record_log"`
	AutoCleanOldLogs bool     `koanf:"auto_clean_old_logs" yaml:"auto_clean_old_logs"`
	MaxLogFiles      int      `koanf:"max_log_files" yaml:"max_log_files"`
	CleanUpAmount    int      `koanf:"clean_up_amount" yaml:"clean_up_amount"`
	CmdPrefix        string   `koanf:"cmd_prefix" yaml:"cmd_prefix"`
	AdminQQ          []int64  `koanf:"admin" yaml:"admin"`
	DbQueueSize      int      `koanf:"db_queue_size" yaml:"db_queue_size"`
	CmdQueueSize     int      `koanf:"cmd_queue_size" yaml:"cmd_queue_size"`
	CmdCoolDown      string   `koanf:"cmd_cooldown" yaml:"cmd_cooldown"`
	MessageBufSize   int      `koanf:"message_buf_size" yaml:"message_buf_size"`
	Modules          []string `koanf:"modules" yaml:"modules"`
}

func (c GlobalConfig) CreateDefaultConfig() interface{} {
	return &GlobalConfig{
		WsUrl:            "ws://127.0.0.1:8080",
		RecordLog:        true,
		AutoCleanOldLogs: true,
		MaxLogFiles:      100,
		DbQueueSize:      100,
		CleanUpAmount:    10,
		CmdQueueSize:     100,
		CmdCoolDown:      "5s",
		CmdPrefix:        ".",
		AdminQQ:          []int64{},
		MessageBufSize:   100,
		Modules:          []string{},
	}
}

var AppConfig *GlobalConfig = nil

func CheckIsAdmin(id int64) bool {
	for _, s := range AppConfig.AdminQQ {
		if s == id {
			return true
		}
	}
	return false
}

func InitConfig() {
	pth := GetSubDirFilePath("config.yml")
	AppConfig = &GlobalConfig{}
	r := InitCustomConfig(AppConfig, pth)
	if r != nil {
		fmt.Printf("failed to init bot config, error : %v\n", r)
		panic("failed to init bot config")
	}
}
