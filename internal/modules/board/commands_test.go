package board

import (
	"embedtools/internal/module"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateConfigDir 把清单文件指到一个临时目录，别动开发机上真正的那份。
func isolateConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(module.UseTempDataDir(dir))

	path, err := commandsPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, dir) {
		t.Fatalf("清单路径 %q 没有落在临时目录 %q 里，测试会污染真实配置", path, dir)
	}
	return path
}

func TestSaveAndLoadCommandsRoundTrip(t *testing.T) {
	path := isolateConfigDir(t)

	saved, err := saveCommands([]Command{
		{Name: "重启运行时", Command: "systemctl restart runtime"},
		{Name: "看进程", Command: "ps | grep runtime"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Path != path {
		t.Errorf("返回的路径 %q 与预期 %q 不符", saved.Path, path)
	}

	loaded := loadCommands()
	if loaded.Warning != "" {
		t.Errorf("刚写的清单读回来带告警：%s", loaded.Warning)
	}
	if len(loaded.Commands) != 2 {
		t.Fatalf("读回 %d 条，期望 2 条", len(loaded.Commands))
	}
	if loaded.Commands[0].Name != "重启运行时" || loaded.Commands[0].Command != "systemctl restart runtime" {
		t.Errorf("第一条内容不对：%+v", loaded.Commands[0])
	}
	if loaded.Path != path {
		t.Errorf("读回的路径 %q 与预期 %q 不符", loaded.Path, path)
	}
}

// 编号从 c1 往后递增，已有编号保留原样。
func TestSaveCommandsAssignsIDs(t *testing.T) {
	isolateConfigDir(t)

	saved, err := saveCommands([]Command{
		{Name: "一", Command: "echo 1"},
		{Name: "二", Command: "echo 2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Commands[0].ID != "c1" || saved.Commands[1].ID != "c2" {
		t.Fatalf("编号=%q/%q，期望 c1/c2", saved.Commands[0].ID, saved.Commands[1].ID)
	}

	// 再加一条，新编号接在最大的后面，老的不动。
	again, err := saveCommands(append(saved.Commands, Command{Name: "三", Command: "echo 3"}))
	if err != nil {
		t.Fatal(err)
	}
	if again.Commands[0].ID != "c1" || again.Commands[2].ID != "c3" {
		t.Fatalf("编号=%v", []string{again.Commands[0].ID, again.Commands[1].ID, again.Commands[2].ID})
	}
}

// 清单被人工编辑过就可能出现重复编号，重复的那个要重新发一个——
// 否则按编号取命令执行会命中错的那条。
func TestSaveCommandsFixesDuplicateIDs(t *testing.T) {
	isolateConfigDir(t)

	saved, err := saveCommands([]Command{
		{ID: "c1", Name: "一", Command: "echo 1"},
		{ID: "c1", Name: "二", Command: "echo 2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Commands[0].ID == saved.Commands[1].ID {
		t.Fatalf("编号还是重复的：%q", saved.Commands[0].ID)
	}
}

func TestSaveCommandsRejectsBlank(t *testing.T) {
	isolateConfigDir(t)

	cases := []struct {
		name string
		cmds []Command
		want string
	}{
		{"名称为空", []Command{{Name: "  ", Command: "ls"}}, "名称"},
		{"命令为空", []Command{{Name: "看看", Command: "  "}}, "命令"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := saveCommands(c.cmds); err == nil {
				t.Fatal("应当报错")
			} else if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err=%v，期望提到 %q", err, c.want)
			}
		})
	}
}

// 第一次用还没有现场文件，退回出厂默认，不该报警。
func TestLoadCommandsUsesFactoryWhenMissing(t *testing.T) {
	isolateConfigDir(t)

	list := loadCommands()
	if list.Warning != "" {
		t.Errorf("文件不存在不该有告警：%s", list.Warning)
	}
	factory, err := parseCommands(commandsJSON, "commands.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Commands) != len(factory) {
		t.Fatalf("应当是出厂默认 %d 条，实际 %d 条", len(factory), len(list.Commands))
	}
	if list.Commands[0].Name != factory[0].Name {
		t.Errorf("第一条应当是出厂的 %q，实际 %+v", factory[0].Name, list.Commands[0])
	}
}

func TestLoadCommandsPrefersRuntimeFile(t *testing.T) {
	isolateConfigDir(t)
	if _, err := saveCommands([]Command{{Name: "现场", Command: "echo 1"}}); err != nil {
		t.Fatal(err)
	}
	list := loadCommands()
	if list.Warning != "" {
		t.Fatal(list.Warning)
	}
	if len(list.Commands) != 1 || list.Commands[0].Name != "现场" {
		t.Fatalf("应当用现场那份：%+v", list.Commands)
	}
}

// 清单坏了退回空列表加告警，而且**不能覆盖那个坏文件**——
// 里面可能还有人工能救回来的命令。
func TestLoadCommandsKeepsBrokenFile(t *testing.T) {
	path := isolateConfigDir(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const broken = `[{"name":"半条"`
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	list := loadCommands()
	if list.Warning == "" {
		t.Error("坏文件应当带一条告警")
	}
	if !strings.Contains(list.Warning, path) {
		t.Errorf("告警里应当带上文件路径，好让人自己去看：%s", list.Warning)
	}
	factory, err := parseCommands(commandsJSON, "commands.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Commands) != len(factory) || list.Commands[0].Name != factory[0].Name {
		t.Fatalf("坏文件应当退回出厂默认：%+v", list.Commands)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != broken {
		t.Errorf("坏文件被改动了，原内容 %q 变成了 %q", broken, string(raw))
	}
}

// 成功之后临时文件不能留在那儿，否则配置目录里会慢慢堆出一堆 .tmp。
func TestSaveCommandsLeavesNoTempFile(t *testing.T) {
	path := isolateConfigDir(t)

	if _, err := saveCommands([]Command{{Name: "一", Command: "echo 1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("临时文件还在：%v", err)
	}
}

func TestApplyImportedCommandsTakesEffect(t *testing.T) {
	isolateConfigDir(t)
	saved, err := applyImportedCommands([]byte(`[
		{"name":"导入的","command":"echo imported"}
	]`), "import.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Commands) != 1 || saved.Commands[0].Name != "导入的" {
		t.Fatalf("导入后没有立即生效：%+v", saved.Commands)
	}
	loaded := loadCommands()
	if loaded.Commands[0].Name != "导入的" {
		t.Fatalf("没有落盘：%+v", loaded.Commands)
	}
}

func TestApplyImportedCommandsRejectsBadWithoutWriting(t *testing.T) {
	isolateConfigDir(t)
	if _, err := saveCommands([]Command{{Name: "现场", Command: "echo 1"}}); err != nil {
		t.Fatal(err)
	}
	_, err := applyImportedCommands([]byte(`[{"name":"","command":"ls"}]`), "bad.json")
	if err == nil {
		t.Fatal("空名称应当被拒")
	}
	list := loadCommands()
	if len(list.Commands) != 1 || list.Commands[0].Name != "现场" {
		t.Fatalf("拒收之后现场清单不该变：%+v", list.Commands)
	}
}

func TestExportImportNeedStartup(t *testing.T) {
	s := &Service{}
	if _, err := s.ExportCommands(); err == nil {
		t.Fatal("没 Startup 不该弹出导出框")
	}
	if _, err := s.ImportCommands(); err == nil {
		t.Fatal("没 Startup 不该弹出导入框")
	}
}
