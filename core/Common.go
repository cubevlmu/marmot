package core

import (
	"fmt"
	"marmot/utils"
	"os"
	"path/filepath"
)

type AppCommon struct {
	Logger   *Logger
	BotQQ    int64
	Database *DbCtx
}

var Common *AppCommon = nil

func InitCommon() {
	err := checkAppDir()
	if err != nil {
		fmt.Printf("[ERROR] Failed to init bot's data directory, err:%e\n", err)
		panic(err)
	}
	InitConfig()

	Common = &AppCommon{}
	Common.Logger = createLogger()
	Common.Database = newDbCtx("marmot_data.db")
}

func checkAppDir() error {
	if !utils.IsDirExists("bot") {
		err := os.Mkdir("bot", 0777)
		if err != nil {
			return err
		}
	}

	return nil
}

func GetSubDir(name string) (string, bool) {
	realPth := filepath.Join("bot", name)

	if !utils.IsDirExists(realPth) {
		err := os.Mkdir(realPth, 0777)
		if err != nil {
			return "", false
		}
	}

	r, err := filepath.Abs(realPth)
	if err != nil {
		fmt.Printf("[ERROR] Failed to get absolute path, err:%e\n", err)
		return "", false
	}

	return r, true
}

func IsSubDirFileExist(name string) bool {
	r := GetDataDir()
	if r == "" {
		return false
	}
	return utils.IsFileExists(filepath.Join(r, name))
}

func GetSubDirFilePath(name string) string {
	r := GetDataDir()
	if r == "" {
		return ""
	}
	return filepath.Join(r, name)
}

func GetDataDir() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("[ERROR] Failed to get working directory, err:%v\n", err)
		return ""
	}
	return filepath.Join(wd, "bot")
}
