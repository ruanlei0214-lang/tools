// Command pickbuild 交互式挑选模块并构建。
//
// 在仓库根目录运行：
//
//	go run ./tools/pickbuild
//
// 它列出可用模块，让你按编号挑选，然后生成接线代码并执行 wails build。
// 适合临时组合，不用先去 modules.json 里加 profile；固定组合还是写成 profile
// 用 build.ps1 更省事。
//
// 模块发现与合法性校验都交给 genmodules，这里只负责交互。
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pickbuild:", err)
		os.Exit(1)
	}
}

func run() error {
	available, err := listModules()
	if err != nil {
		return err
	}
	if len(available) == 0 {
		return errors.New("没有发现任何模块")
	}

	selected, err := pick(available)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("已取消，没有构建。")
		return nil
	}

	fmt.Printf("\n构建模块：%s\n\n", strings.Join(selected, " "))
	if err := passThrough("go", "run", "./tools/genmodules", "-modules", strings.Join(selected, ",")); err != nil {
		return err
	}
	if err := passThrough("wails", "build"); err != nil {
		return err
	}
	if err := passThrough("go", "run", "./tools/packportable"); err != nil {
		return err
	}

	fmt.Printf("\n构建完成：build\\bin\\%s\\（模块：%s）\n", strings.TrimSuffix(outputName(), ".exe"), strings.Join(selected, " "))
	return nil
}

// outputName 从 wails.json 读产物文件名。
//
// 不在这儿写死：那个名字带版本号，改版本时漏改这里只会让提示指向一个不存在的文件。
// 读不到就退回一句不提名字的话——构建本身已经成功了，为一句提示报错没道理。
func outputName() string {
	raw, err := os.ReadFile("wails.json")
	if err != nil {
		return "（产物见 wails.json 的 outputfilename）"
	}
	var cfg struct {
		OutputFilename string `json:"outputfilename"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.OutputFilename == "" {
		return "（产物见 wails.json 的 outputfilename）"
	}
	return cfg.OutputFilename + ".exe"
}

func listModules() ([]string, error) {
	out, err := exec.Command("go", "run", "./tools/genmodules", "-list").Output()
	if err != nil {
		return nil, fmt.Errorf("取模块列表失败（请在仓库根目录运行）：%w", err)
	}

	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

// pick 返回空切片表示用户放弃构建。
func pick(available []string) ([]string, error) {
	fmt.Println("可编译的模块：")
	for i, name := range available {
		fmt.Printf("  %d) %s\n", i+1, name)
	}
	fmt.Print("\n输入编号选择（逗号或空格分隔），直接回车=全选，q=退出：")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return nil, fmt.Errorf("读取输入失败：%w", err)
	}

	switch line = strings.TrimSpace(line); strings.ToLower(line) {
	case "":
		return available, nil
	case "q":
		return nil, nil
	}
	return parseChoice(line, available)
}

func parseChoice(line string, available []string) ([]string, error) {
	fields := strings.FieldsFunc(line, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '、'
	})

	seen := make(map[string]bool, len(fields))
	selected := make([]string, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 1 || n > len(available) {
			return nil, fmt.Errorf("无效的编号 %q，可用范围 1-%d", f, len(available))
		}
		if name := available[n-1]; !seen[name] {
			seen[name] = true
			selected = append(selected, name)
		}
	}

	sort.Strings(selected)
	return selected, nil
}

func passThrough(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s 失败：%w", name, strings.Join(args, " "), err)
	}
	return nil
}
