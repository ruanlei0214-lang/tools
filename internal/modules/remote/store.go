package remote

import (
	"embedtools/internal/module"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// 现场改出来的配置存在 exe 旁边，不进 %APPDATA%。现场整夹拷走时配置跟着走；
// 文件名都带 remote- 前缀，和 board / netcfg 的现场文件不会撞。
//
// 几份文件分开存，对应编译进产物的出厂默认：逐份「恢复默认」不牵连另外几份，
// 某一份坏掉时另两份也照常可用。合成一份之后，点位表里少个逗号会把连接参数一起废掉。
const (
	deviceFileName   = "remote-config.json"
	ioFileName       = "remote-io.json"
	registerFileName = "remote-register.json"
	ioFlowFileName   = "remote-io-flow.json"
)

// errNoOverride 表示这一份现场配置还不存在，该用编译进产物的出厂默认。
// 它不是故障——干净的一台机器上第一次打开程序时三份都不存在，是正常状态。
var errNoOverride = errors.New("没有现场配置")

func configDir() (string, error) {
	return module.DataDir()
}

func storePath(name string) (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// readStore 读一份现场配置，连同它的完整路径一起返回——告警要指出是哪个文件出的问题。
// 文件不存在返回 errNoOverride，调用方据此退回出厂默认。
func readStore(name string) ([]byte, string, error) {
	path, err := storePath(name)
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, path, errNoOverride
		}
		return nil, path, fmt.Errorf("读取 %s 失败：%v", path, err)
	}
	return raw, path, nil
}

// writeStore 整份写回。先写 .tmp 再改名：直接往目标文件上写的话，进程在写一半时挂掉
// 就留下半份 JSON，下次打开整份配置都读不出来。改名在同一目录内是原子的。
func writeStore(name string, raw []byte) error {
	path, err := storePath(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("无法创建 %s：%w", filepath.Dir(path), err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("写入 %s 失败：%w", path, err)
	}
	return nil
}

// removeStore 删掉一份现场配置，加载随之退回出厂默认——「恢复默认」就是这么实现的，
// 不另走一条「把默认值写进文件」的路径。文件本来就不存在算成功。
func removeStore(name string) error {
	path, err := storePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除 %s 失败：%w", path, err)
	}
	return nil
}
