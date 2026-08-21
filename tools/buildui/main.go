// Command buildui 是构建工具的原生窗口。
//
// 在仓库根目录：
//
//	go run ./tools/buildui
//	go run github.com/akavel/rsrc@latest -manifest tools/buildui/app.manifest -o tools/buildui/rsrc.syso
//	go build -ldflags="-H windowsgui" -o tools/BuildUI.exe ./tools/buildui
//
// 双击 tools/BuildUI.exe 即可。必须能找到仓库根目录的 modules.json。
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
)

type app struct {
	root string

	mw           *walk.MainWindow
	combo        *walk.ComboBox
	profileRadio *walk.RadioButton
	modulesRadio *walk.RadioButton
	modBox       *walk.Composite
	logHost      *walk.Composite
	log          *LogView
	btnGen       *walk.PushButton
	btnBuild     *walk.PushButton
	btnDev       *walk.PushButton
	btnStop      *walk.PushButton
	btnRun       *walk.PushButton
	btnWriteback *walk.PushButton
	checks       []*walk.CheckBox

	mode  string
	busy  bool
	mu    sync.Mutex
	cmd   *exec.Cmd
	steps []step
}

type step struct {
	file string
	args []string
}

func main() {
	enrichPath()
	root, err := findRoot()
	if err != nil {
		walk.MsgBox(nil, "构建", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
	if err := os.Chdir(root); err != nil {
		walk.MsgBox(nil, "构建", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}

	a := &app{root: root, mode: "profile"}
	if err := a.buildWindow(); err != nil {
		walk.MsgBox(nil, "构建", err.Error(), walk.MsgBoxIconError)
		os.Exit(1)
	}
	a.bootstrap()
	a.mw.Run()
}

func (a *app) buildWindow() error {
	err := MainWindow{
		AssignTo: &a.mw,
		Title:    "C2 工具箱 · 构建",
		MinSize:  Size{Width: 720, Height: 520},
		Size:     Size{Width: 860, Height: 600},
		Layout:   VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
		Children: []Widget{
			Label{Text: "构建工具", Font: Font{Family: "Microsoft YaHei UI", PointSize: 12, Bold: true}},
			Label{Text: "选 profile 或勾选模块，点按钮即可生成接线 / 构建，不必再敲命令。"},
			Composite{
				Border: true,
				Layout: VBox{},
				Children: []Widget{
					Composite{
						Layout: HBox{},
						Children: []Widget{
							RadioButton{
								AssignTo: &a.profileRadio,
								Text:     "使用 profile",
								OnClicked: func() {
									a.mode = "profile"
									a.modulesRadio.SetChecked(false)
								},
							},
							ComboBox{AssignTo: &a.combo, MinSize: Size{Width: 200}},
							HSpacer{},
						},
					},
					Composite{
						Layout: HBox{Alignment: AlignHNearVCenter},
						Children: []Widget{
							RadioButton{
								AssignTo:  &a.modulesRadio,
								Text:      "自选模块",
								OnClicked: func() { a.mode = "modules"; a.profileRadio.SetChecked(false) },
							},
							Composite{AssignTo: &a.modBox, Layout: HBox{}},
							HSpacer{},
						},
					},
					Label{Text: "交付给客户的组合请用 profile；自选只适合临时试一下。"},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{AssignTo: &a.btnGen, Text: "仅生成接线", OnClicked: a.onGen},
					PushButton{AssignTo: &a.btnBuild, Text: "构建", OnClicked: a.onBuild},
					PushButton{AssignTo: &a.btnDev, Text: "开发模式", OnClicked: a.onDev},
					PushButton{AssignTo: &a.btnStop, Text: "停止", Enabled: false, OnClicked: a.stop},
					PushButton{AssignTo: &a.btnRun, Text: "运行软件", OnClicked: a.onRun},
					PushButton{AssignTo: &a.btnWriteback, Text: "回写配置", OnClicked: a.onWriteback},
					PushButton{Text: "打开产物目录", OnClicked: a.openOut},
					HSpacer{},
				},
			},
			Composite{
				AssignTo:      &a.logHost,
				StretchFactor: 1,
				Layout:        VBox{MarginsZero: true, SpacingZero: true},
			},
		},
	}.Create()
	if err != nil {
		return err
	}
	lv, err := NewLogView(a.logHost)
	if err != nil {
		return err
	}
	a.log = lv
	a.mw.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if !a.busy {
			return
		}
		if walk.MsgBox(a.mw, "构建", "还有命令在跑，关闭窗口会中止它。确定关闭？", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			*canceled = true
			return
		}
		a.stop()
	})
	return nil
}

func (a *app) bootstrap() {
	a.profileRadio.SetChecked(true)
	a.setBusy(false)
	profiles, err := loadProfiles(a.root)
	if err != nil {
		a.appendLog("读取 modules.json 失败：" + err.Error())
	} else {
		_ = a.combo.SetModel(profiles)
		idx := indexOf(profiles, "all")
		if idx < 0 && len(profiles) > 0 {
			idx = 0
		}
		if idx >= 0 {
			_ = a.combo.SetCurrentIndex(idx)
		}
	}

	if lookPath("go") == "" {
		a.appendLog("找不到 go，模块列表无法扫描。装好 Go 后重新打开本窗口。")
		return
	}
	a.appendLog("仓库：" + a.root)
	a.appendLog("正在扫描模块…")
	go a.scanModules()
}

func (a *app) scanModules() {
	cmd := exec.Command("go", "run", "./tools/genmodules", "-list")
	cmd.Dir = a.root
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		a.sync(func() { a.appendLog("扫描模块失败：" + strings.TrimSpace(string(out)+" "+err.Error())) })
		return
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		if n := strings.TrimSpace(line); n != "" {
			names = append(names, n)
		}
	}
	a.sync(func() { a.fillModules(names) })
}

func (a *app) fillModules(names []string) {
	for _, c := range a.checks {
		c.Dispose()
	}
	a.checks = nil
	for _, name := range names {
		cb, err := walk.NewCheckBox(a.modBox)
		if err != nil {
			a.appendLog("创建模块勾选失败：" + err.Error())
			return
		}
		_ = cb.SetText(name)
		cb.SetChecked(true)
		a.checks = append(a.checks, cb)
	}
	if len(names) == 0 {
		a.appendLog("没有发现任何模块。")
		return
	}
	a.appendLog("模块：" + strings.Join(names, "、"))
	a.appendLog("选好方式后点「构建」即可。")
}

func (a *app) onGen() {
	st, err := a.genStep()
	if err != nil {
		walk.MsgBox(a.mw, "构建", err.Error(), walk.MsgBoxIconError)
		return
	}
	a.startPipeline([]step{st})
}

func (a *app) onBuild() {
	wails := lookPath("wails")
	if wails == "" {
		walk.MsgBox(a.mw, "构建", "找不到 wails。请先执行：\ngo install github.com/wailsapp/wails/v2/cmd/wails@latest", walk.MsgBoxIconError)
		return
	}
	st, err := a.genStep()
	if err != nil {
		walk.MsgBox(a.mw, "构建", err.Error(), walk.MsgBoxIconError)
		return
	}
	goBin := lookPath("go")
	if goBin == "" {
		walk.MsgBox(a.mw, "构建", "找不到 go。请确认已安装 Go 并加入 PATH。", walk.MsgBoxIconError)
		return
	}
	a.appendLog("构建完成后产物：build\\bin\\" + strings.TrimSuffix(outputName(a.root), ".exe") + "\\")
	a.startPipeline([]step{
		st,
		{file: wails, args: []string{"build"}},
		{file: goBin, args: []string{"run", "./tools/packportable"}},
	})
}

func (a *app) onDev() {
	wails := lookPath("wails")
	if wails == "" {
		walk.MsgBox(a.mw, "构建", "找不到 wails。请先执行：\ngo install github.com/wailsapp/wails/v2/cmd/wails@latest", walk.MsgBoxIconError)
		return
	}
	st, err := a.genStep()
	if err != nil {
		walk.MsgBox(a.mw, "构建", err.Error(), walk.MsgBoxIconError)
		return
	}
	a.appendLog("开发模式会一直跑着，要停就点「停止」。浏览器调试入口一般是 http://localhost:34115")
	a.startPipeline([]step{st, {file: wails, args: []string{"dev"}}})
}

func (a *app) genStep() (step, error) {
	goBin := lookPath("go")
	if goBin == "" {
		return step{}, errors.New("找不到 go。请确认已安装 Go 并加入 PATH。")
	}
	if a.mode != "modules" {
		profile := strings.TrimSpace(a.combo.Text())
		if profile == "" {
			return step{}, errors.New("请选择一个 profile")
		}
		return step{file: goBin, args: []string{"run", "./tools/genmodules", "-profile", profile}}, nil
	}
	var mods []string
	for _, cb := range a.checks {
		if cb.Checked() {
			mods = append(mods, cb.Text())
		}
	}
	if len(mods) == 0 {
		return step{}, errors.New("请至少勾选一个模块")
	}
	return step{file: goBin, args: []string{"run", "./tools/genmodules", "-modules", strings.Join(mods, ",")}}, nil
}

func (a *app) startPipeline(steps []step) {
	if a.busy {
		return
	}
	a.steps = steps
	a.setBusy(true)
	a.startNext()
}

func (a *app) startNext() {
	if len(a.steps) == 0 {
		a.setBusy(false)
		a.appendLog("全部完成。")
		return
	}
	st := a.steps[0]
	a.steps = a.steps[1:]
	a.appendLog("> " + st.file + " " + strings.Join(st.args, " "))

	cmd := exec.Command(st.file, st.args...)
	cmd.Dir = a.root
	hideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		a.appendLog("启动失败：" + err.Error())
		a.steps = nil
		a.setBusy(false)
		return
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		a.appendLog("启动失败：" + err.Error())
		a.steps = nil
		a.setBusy(false)
		return
	}
	a.mu.Lock()
	a.cmd = cmd
	a.mu.Unlock()
	go a.pump(cmd, stdout)
}

func (a *app) pump(cmd *exec.Cmd, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		a.sync(func() { a.appendLog(line) })
	}
	err := cmd.Wait()
	a.mu.Lock()
	a.cmd = nil
	a.mu.Unlock()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	a.sync(func() {
		a.appendLog(fmt.Sprintf("退出码 %d", code))
		if code != 0 {
			a.steps = nil
			a.setBusy(false)
			a.appendLog("已停止后续步骤。")
			return
		}
		a.startNext()
	})
}

func (a *app) stop() {
	a.steps = nil
	a.mu.Lock()
	cmd := a.cmd
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		a.setBusy(false)
		return
	}
	kill := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid))
	hideWindow(kill)
	if err := kill.Run(); err != nil {
		a.appendLog("中止失败：" + err.Error())
		return
	}
	a.appendLog("已中止当前命令。")
}

func (a *app) openOut() {
	dir := filepath.Join(a.root, "build", "bin")
	_ = os.MkdirAll(dir, 0o755)
	_ = exec.Command("explorer.exe", dir).Start()
}

// onRun 启动上一次构建出来的产物。
//
// 不接管它的输出也不 Wait：产物是给人用的，不是构建流程里的一步，
// 它的生命周期不该挂在构建窗口上——这个窗口关了，它得继续跑。
func (a *app) onRun() {
	exe, ok := outputPath(a.root)
	if !ok {
		walk.MsgBox(a.mw, "构建", "读不到 wails.json 的 outputfilename，不知道产物叫什么。", walk.MsgBoxIconError)
		return
	}
	if !fileExists(exe) {
		walk.MsgBox(a.mw, "构建", "还没有产物：\n"+exe+"\n\n先点「构建」。", walk.MsgBoxIconWarning)
		a.refreshRun()
		return
	}

	cmd := exec.Command(exe)
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		a.appendLog("启动失败：" + err.Error())
		return
	}
	_ = cmd.Process.Release()
	a.appendLog("已启动 " + exe)
}

// onWriteback 把绿色版目录里改过的配置写回源码出厂文件。
func (a *app) onWriteback() {
	exe, ok := outputPath(a.root)
	if !ok || !fileExists(exe) {
		walk.MsgBox(a.mw, "构建", "还没有绿色版目录，先点「构建」。", walk.MsgBoxIconWarning)
		return
	}
	msg := "把绿色版目录里改过的配置写回源码出厂文件？\n下次构建会带上这些改动。\n\n只动 remote / board 那几份，不动 netcfg 记住的地址。"
	if walk.MsgBox(a.mw, "回写配置", msg, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	goBin := lookPath("go")
	if goBin == "" {
		walk.MsgBox(a.mw, "构建", "找不到 go。请确认已安装 Go 并加入 PATH。", walk.MsgBoxIconError)
		return
	}
	a.startPipeline([]step{{file: goBin, args: []string{"run", "./tools/packportable", "-writeback"}}})
}

func (a *app) setBusy(on bool) {
	a.busy = on
	a.btnGen.SetEnabled(!on)
	a.btnBuild.SetEnabled(!on)
	a.btnDev.SetEnabled(!on)
	if a.btnWriteback != nil {
		a.btnWriteback.SetEnabled(!on)
	}
	a.btnStop.SetEnabled(on)
	a.combo.SetEnabled(!on)
	a.profileRadio.SetEnabled(!on)
	a.modulesRadio.SetEnabled(!on)
	for _, cb := range a.checks {
		cb.SetEnabled(!on)
	}
	a.refreshRun()
}

// refreshRun 让「运行软件」跟着产物走：没构建过就点不动。
// 构建期间也禁掉——产物跑着的时候 wails build 覆盖不了那个 exe。
func (a *app) refreshRun() {
	if a.btnRun == nil {
		return
	}
	exe, ok := outputPath(a.root)
	ready := !a.busy && ok && fileExists(exe)
	a.btnRun.SetEnabled(ready)
	if a.btnWriteback != nil {
		a.btnWriteback.SetEnabled(ready)
	}
	if ok {
		a.btnRun.SetToolTipText(exe)
		if a.btnWriteback != nil {
			a.btnWriteback.SetToolTipText("把 " + filepath.Dir(exe) + " 里的配置写回源码")
		}
	}
}

func (a *app) appendLog(s string) {
	if s == "" || a.log == nil {
		return
	}
	a.log.AppendLine(s)
}

func (a *app) sync(fn func()) {
	if a.mw == nil {
		fn()
		return
	}
	a.mw.Synchronize(fn)
}

func findRoot() (string, error) {
	var starts []string
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	seen := map[string]bool{}
	for _, start := range starts {
		dir := start
		for i := 0; i < 6; i++ {
			abs, _ := filepath.Abs(dir)
			if seen[abs] {
				break
			}
			seen[abs] = true
			if fileExists(filepath.Join(abs, "modules.json")) && fileExists(filepath.Join(abs, "wails.json")) {
				return abs, nil
			}
			parent := filepath.Dir(abs)
			if parent == abs {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("找不到仓库根目录（需要 modules.json 与 wails.json）。请把 BuildUI.exe 放在 tools 目录，或在仓库根目录运行。")
}

func loadProfiles(root string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "modules.json"))
	if err != nil {
		return nil, err
	}
	raw = bytesTrimBOM(raw)
	var cfg struct {
		// 这里只要 profile 的名字，条目内容不关心——条目可以是模块名，
		// 也可以是 {"module":...,"tabs":[...]} 这种带选项的对象。
		// 用 RawMessage 跳过条目解析，条目格式以后再加选项这里也不用动。
		Profiles map[string][]json.RawMessage `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(cfg.Profiles))
	if _, ok := cfg.Profiles["all"]; ok {
		names = append(names, "all")
	}
	for k := range cfg.Profiles {
		if k != "all" {
			names = append(names, k)
		}
	}
	return names, nil
}

// outputPath 是绿色版目录里那个 exe 的完整路径。
// wails.json 读不到或没写 outputfilename 时返回 false。
func outputPath(root string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(root, "wails.json"))
	if err != nil {
		return "", false
	}
	var cfg struct {
		OutputFilename string `json:"outputfilename"`
	}
	if json.Unmarshal(bytesTrimBOM(raw), &cfg) != nil || cfg.OutputFilename == "" {
		return "", false
	}
	return filepath.Join(root, "build", "bin", cfg.OutputFilename, cfg.OutputFilename+".exe"), true
}

func outputName(root string) string {
	p, ok := outputPath(root)
	if !ok {
		return "（见 wails.json 的 outputfilename）"
	}
	return filepath.Base(p)
}

func enrichPath() {
	home, _ := os.UserHomeDir()
	parts := []string{
		filepath.Join(home, "go", "bin"),
		`C:\Program Files\Go\bin`,
		os.Getenv("Path"),
	}
	os.Setenv("Path", strings.Join(parts, string(os.PathListSeparator)))
}

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	cmd.Env = append(os.Environ(),
		"FORCE_COLOR=1",
		"CLICOLOR_FORCE=1",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func bytesTrimBOM(raw []byte) []byte {
	if len(raw) >= 3 && raw[0] == 0xef && raw[1] == 0xbb && raw[2] == 0xbf {
		return raw[3:]
	}
	return raw
}
