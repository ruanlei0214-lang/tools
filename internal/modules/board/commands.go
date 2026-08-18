package board

import (
	"bytes"
	"embedtools/internal/module"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// 按钮清单存在 exe 旁边，不进 %APPDATA%。现场整夹拷走时清单跟着走。
// 出厂默认是编译进产物的 config/commands.json；第一次打开还没有现场文件时用它。
const commandsFileName = "board-commands.json"

// errNoOverride 表示现场清单还不存在，该用出厂默认。第一次打开是正常状态。
var errNoOverride = errors.New("没有现场清单")

// Command 是界面上的一个指令按钮。
type Command struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
}

// CommandList 是清单本体加上界面要显示的两条元信息。
type CommandList struct {
	Commands []Command `json:"commands"`
	// Path 是清单文件的完整路径，显示在界面上供人手动拷贝。
	Path string `json:"path"`
	// Warning 非空表示现场清单不可用，当前这些值来自出厂默认。
	Warning string `json:"warning"`
}

// CommandFileResult 是一次导入的结果。取消选文件时 Canceled 为真，清单没动。
type CommandFileResult struct {
	List     CommandList `json:"list"`
	Path     string      `json:"path"`
	Canceled bool        `json:"canceled"`
}

func commandsPath() (string, error) {
	dir, err := module.DataDir()
	if err != nil {
		return "", fmt.Errorf("无法定位程序目录: %w", err)
	}
	return filepath.Join(dir, commandsFileName), nil
}

// loadCommands 现场清单优先，没有或坏掉就退回出厂默认。
//
// 坏文件不会被覆盖：里面可能还有人工能救回来的命令。告警里带上路径，好让人自己去看。
func loadCommands() CommandList {
	raw, path, err := readCommandsStore()
	if err == nil {
		cmds, perr := parseCommands(raw, path)
		if perr == nil {
			return CommandList{Commands: cmds, Path: path}
		}
		return builtinCommands(path, fmt.Sprintf("%v，已退回出厂默认。这个文件没有被改动，可以打开它人工修好。", perr))
	}
	if errors.Is(err, errNoOverride) {
		return builtinCommands(path, "")
	}
	return builtinCommands(path, err.Error())
}

func readCommandsStore() ([]byte, string, error) {
	path, err := commandsPath()
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

func builtinCommands(path, warn string) CommandList {
	cmds, err := parseCommands(commandsJSON, "commands.json")
	if err != nil {
		if warn == "" {
			return CommandList{Commands: []Command{}, Path: path, Warning: fmt.Sprintf("出厂默认 commands.json 不可用：%v", err)}
		}
		return CommandList{Commands: []Command{}, Path: path, Warning: fmt.Sprintf("%s；出厂默认也不可用：%v", warn, err)}
	}
	return CommandList{Commands: cmds, Path: path, Warning: warn}
}

func parseCommands(raw []byte, label string) ([]Command, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	var cmds []Command
	if err := json.Unmarshal(raw, &cmds); err != nil {
		return nil, fmt.Errorf("%s 不是合法的按钮清单（%v）", label, err)
	}
	normalized, err := normalizeCommands(cmds)
	if err != nil {
		return nil, fmt.Errorf("%s：%v", label, err)
	}
	return normalized, nil
}

func commandsBytes() ([]byte, error) {
	return json.MarshalIndent(loadCommands().Commands, "", "  ")
}

func applyImportedCommands(raw []byte, label string) (CommandList, error) {
	cmds, err := parseCommands(raw, label)
	if err != nil {
		return CommandList{}, err
	}
	return saveCommands(cmds)
}

// saveCommands 校验并整份写回。前端每次改动都送整份清单过来，
// 增、删、改因此共用这一条路径——三个方法各写一遍校验和落盘只会让它们慢慢长歪。
//
// 写失败要往上抛，不像 netcfg 记地址那样只记日志：那份丢了只是下次退回默认地址，
// 而这份是用户一条条攒出来的，丢了没法自动恢复；而且保存是他按下的动作，成没成得告诉他。
func saveCommands(cmds []Command) (CommandList, error) {
	normalized, err := normalizeCommands(cmds)
	if err != nil {
		return CommandList{}, err
	}
	path, err := commandsPath()
	if err != nil {
		return CommandList{}, err
	}

	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return CommandList{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return CommandList{}, fmt.Errorf("无法创建 %s: %w", filepath.Dir(path), err)
	}

	// 先写临时文件再改名。直接往目标文件上写的话，进程在写一半时挂掉就留下半份 JSON，
	// 下次打开整个清单都读不出来。改名在同一目录内是原子的。
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return CommandList{}, fmt.Errorf("写入 %s 失败: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return CommandList{}, fmt.Errorf("写入 %s 失败: %w", path, err)
	}
	return CommandList{Commands: normalized, Path: path}, nil
}

// normalizeCommands 校验每一条并给新加的补编号。
func normalizeCommands(cmds []Command) ([]Command, error) {
	out := make([]Command, 0, len(cmds))

	// 先扫一遍已有编号，新编号从最大的那个往后接。用递增编号而不是时间戳：
	// 编号只要在这份清单里唯一就够了，而递增的在测试里和人工翻文件时都好读。
	next := 1
	used := make(map[string]bool, len(cmds))
	for _, c := range cmds {
		id := strings.TrimSpace(c.ID)
		if id == "" || used[id] {
			continue
		}
		used[id] = true
		if n, err := strconv.Atoi(strings.TrimPrefix(id, "c")); err == nil && n >= next {
			next = n + 1
		}
	}

	seen := make(map[string]bool, len(cmds))
	for i, c := range cmds {
		c.Name = strings.TrimSpace(c.Name)
		c.Command = strings.TrimSpace(c.Command)
		if c.Name == "" {
			return nil, fmt.Errorf("第 %d 个按钮没有填名称", i+1)
		}
		if c.Command == "" {
			return nil, fmt.Errorf("按钮 %q 没有填命令", c.Name)
		}

		// 编号为空或撞了（清单被人工编辑过就可能撞），重新发一个。
		c.ID = strings.TrimSpace(c.ID)
		if c.ID == "" || seen[c.ID] {
			c.ID = "c" + strconv.Itoa(next)
			next++
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, nil
}
