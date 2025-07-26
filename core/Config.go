package core

import (
	"errors"
	"fmt"
	kyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	osyaml "gopkg.in/yaml.v3"
	"os"
)

type GlobalConfig struct {
	WsUrl            string   `koanf:"ws_url" yaml:"ws_url"`
	RecordLog        bool     `koanf:"record_log" yaml:"record_log"`
	AutoCleanOldLogs bool     `koanf:"auto_clean_old_logs" yaml:"auto_clean_old_logs"`
	MaxLogFiles      int      `koanf:"max_log_files" yaml:"max_log_files"`
	CleanUpAmount    int      `koanf:"clean_up_amount" yaml:"clean_up_amount"`
	CmdPrefix        string   `koanf:"cmd_prefix" yaml:"cmd_prefix"`
	AdminQQ          []int64  `koanf:"admin" yaml:"admin"`
	DbQueueSize      int      `koanf:"db_queue_size" yaml:"db_queue_size"`
	MessageBufSize   int      `koanf:"message_buf_size" yaml:"message_buf_size"`
	Modules          []string `koanf:"modules" yaml:"modules"`
}

var AppConfig *GlobalConfig = nil

func createDefaultConfig() {
	AppConfig = &GlobalConfig{
		WsUrl:            "ws://127.0.0.1:8080",
		RecordLog:        true,
		AutoCleanOldLogs: true,
		MaxLogFiles:      100,
		DbQueueSize:      100,
		CleanUpAmount:    10,
		CmdPrefix:        ".",
		AdminQQ:          []int64{},
		MessageBufSize:   100,
		Modules:          []string{""},
	}
	err := saveConfigToFile(GetSubDirFilePath("config.yaml"))
	if err != nil {
		fmt.Printf("save default config to file failed, err:%v\n", err)
	}
}

func saveConfigToFile(path string) error {
	if AppConfig == nil {
		return errors.New("global config is nil")
	}
	data, err := osyaml.Marshal(AppConfig)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func loadConfigFromFile(path string) error {
	k := koanf.New(GetDataDir())
	if AppConfig == nil {
		AppConfig = &GlobalConfig{}
	}
	if err := k.Load(file.Provider(path), kyaml.Parser()); err != nil {
		return err
	}
	return k.Unmarshal("", AppConfig)
}

func (c *GlobalConfig) CheckIsAdmin(id int64) bool {
	for _, s := range c.AdminQQ {
		if s == id {
			return true
		}
	}
	return false
}

func InitConfig() {
	if !IsSubDirFileExist("config.yaml") {
		createDefaultConfig()
		return
	}

	err := loadConfigFromFile(GetSubDirFilePath("config.yaml"))
	if err != nil {
		fmt.Printf("load config.yaml failed, err:%v\n", err)
		createDefaultConfig()
	}
}

type IConfig interface {
	CreateDefaultConfig() interface{}
}

func SaveCustomConfigToFile[T any](path string, config *T) error {
	if config == nil {
		return errors.New("config object is nil")
	}
	data, err := osyaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func LoadCustomConfigFromFile[T any](path string, config *T) error {
	k := koanf.New(GetDataDir())
	if config == nil {
		return errors.New("config object is nil")
	}
	if err := k.Load(file.Provider(path), kyaml.Parser()); err != nil {
		return err
	}
	return k.Unmarshal("", config)
}

func InitCustomConfig[T IConfig](config *T, path string) error {
	if config == nil {
		return errors.New("config object is nil")
	}

	r := LoadCustomConfigFromFile[T](path, config)
	if r != nil {
		r := *config
		config = r.CreateDefaultConfig().(*T)
		rs := SaveCustomConfigToFile[T](path, config)
		if rs != nil {
			LogWarn("[Config] failed to save default config to file: %v", rs)
		}
	}

	return nil
}
