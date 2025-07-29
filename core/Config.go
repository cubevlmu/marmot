package core

import (
	"errors"
	kyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	osyaml "gopkg.in/yaml.v3"
	"os"
)

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
