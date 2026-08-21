// Command genmodules 按 modules.json 里的 profile 生成模块接线代码。
//
// 在仓库根目录运行：
//
//	go run ./tools/genmodules -profile all
//	go run ./tools/genmodules -modules remote,netcfg  // 临时组合，不读 modules.json
//	go run ./tools/genmodules -list                   // 只打印可用模块
//
// 它会扫描 internal/modules/ 与 frontend/src/modules/ 发现所有模块，按 profile
// 过滤后生成 internal/modules/modules_gen.go 和 frontend/src/shell/modules.gen.ts。
// 没被选中的模块不会被任何代码 import，因此不会进入构建产物。
//
// profile 条目可以是模块名，也可以带选项：
//
//	{"module": "remote", "tabs": ["command"]}
//
// tabs 是 remote 的构建期标签页白名单：裁掉的页签不进产物，现场配置改不回来，
// 和 profile 裁掉整个模块一个道理。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	backendRoot  = "internal/modules"
	frontendRoot = "frontend/src/modules"
	docRoot      = "doc"
	backendOut   = "internal/modules/modules_gen.go"
	frontendOut  = "frontend/src/shell/modules.gen.ts"
	configPath   = "modules.json"
)

var identRe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// profileEntry 是 profile 里的一个条目：模块名，或带选项的对象。
// 目前唯一的选项是 tabs——remote 的构建期标签页白名单。
type profileEntry struct {
	Module string   `json:"module"`
	Tabs   []string `json:"tabs"`
}

// UnmarshalJSON 接受两种写法："remote" 或 {"module":"remote","tabs":["command"]}。
func (e *profileEntry) UnmarshalJSON(raw []byte) error {
	var name string
	if json.Unmarshal(raw, &name) == nil {
		e.Module = name
		return nil
	}
	type alias profileEntry
	var a alias
	if err := json.Unmarshal(raw, &a); err != nil {
		return fmt.Errorf("profile 条目必须是模块名或 {\"module\":...,\"tabs\":[...]}：%s", raw)
	}
	*e = profileEntry(a)
	return nil
}

// tabKinds 是 remote 认识的标签页 kind。在这里钉一份而不是运行时才暴露：
// 白名单拼错一个字母（比如 "commnad"）会把整页裁成空白，构建时就该报出来。
var tabKinds = map[string]bool{"io": true, "register": true, "command": true}

type config struct {
	Profiles map[string][]profileEntry `json:"profiles"`
}

func main() {
	profile := flag.String("profile", "all", "modules.json 里的 profile 名称")
	modules := flag.String("modules", "", "直接指定模块，逗号分隔；给了它就不读 modules.json")
	list := flag.Bool("list", false, "打印所有可用模块，一行一个，然后退出")
	flag.Parse()

	if err := run(*profile, *modules, *list); err != nil {
		fmt.Fprintln(os.Stderr, "genmodules:", err)
		os.Exit(1)
	}
}

func run(profile, modules string, list bool) error {
	if list {
		return printModules()
	}
	if modules != "" {
		return generate("自选", splitModules(modules))
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	wanted, ok := cfg.Profiles[profile]
	if !ok {
		return fmt.Errorf("%s 里没有 profile %q，可选：%s", configPath, profile, strings.Join(profileNames(cfg), "、"))
	}
	return generate(profile, wanted)
}

// printModules 供 tools/pickbuild 之类的调用方取模块列表，
// 免得发现规则在两处各写一份。
func printModules() error {
	available, err := discover()
	if err != nil {
		return err
	}
	for _, name := range available {
		fmt.Println(name)
	}
	return nil
}

// generate 是两条入口的公共部分，label 只用来标注生成文件的来源。
func generate(label string, wanted []profileEntry) error {
	available, err := discover()
	if err != nil {
		return err
	}
	warnMissingDocs(available)

	selected, err := resolve(wanted, available)
	if err != nil {
		return err
	}

	if err := writeFile(backendOut, renderGo(label, selected)); err != nil {
		return err
	}
	if err := writeFile(frontendOut, renderTS(label, selected)); err != nil {
		return err
	}

	names := make([]string, 0, len(selected))
	for _, e := range selected {
		names = append(names, e.Module)
	}
	fmt.Printf("profile %q：启用 %d 个模块 [%s]\n", label, len(selected), strings.Join(names, " "))
	return nil
}

func splitModules(s string) []profileEntry {
	var out []profileEntry
	for _, part := range strings.Split(s, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out = append(out, profileEntry{Module: name})
		}
	}
	return out
}

func loadConfig() (*config, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败（请在仓库根目录运行）：%w", configPath, err)
	}
	// Windows 上的编辑器常写出带 BOM 的 UTF-8，encoding/json 不认。
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	var cfg config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", configPath, err)
	}
	if len(cfg.Profiles) == 0 {
		return nil, fmt.Errorf("%s 里没有定义任何 profile", configPath)
	}
	return &cfg, nil
}

// discover 返回前后端两半都齐备的模块名。任何一半缺失都直接报错，
// 因为那样构建一定会失败，早报比晚报好。
func discover() ([]string, error) {
	backend, err := subdirs(backendRoot)
	if err != nil {
		return nil, err
	}
	frontend, err := subdirs(frontendRoot)
	if err != nil {
		return nil, err
	}

	hasFrontend := make(map[string]bool, len(frontend))
	for _, name := range frontend {
		hasFrontend[name] = true
	}

	var names []string
	for _, name := range backend {
		if !isGoPackage(filepath.Join(backendRoot, name)) {
			continue
		}
		if !hasFrontend[name] {
			return nil, fmt.Errorf("模块 %s 只有后端，缺少 %s/%s/", name, frontendRoot, name)
		}
		if err := checkPackageName(name); err != nil {
			return nil, err
		}
		delete(hasFrontend, name)
		names = append(names, name)
	}
	for name := range hasFrontend {
		return nil, fmt.Errorf("模块 %s 只有前端，缺少 %s/%s/", name, backendRoot, name)
	}

	sort.Strings(names)
	return names, nil
}

func resolve(wanted []profileEntry, available []string) ([]profileEntry, error) {
	for _, e := range wanted {
		if e.Module == "*" {
			out := make([]profileEntry, 0, len(available))
			for _, name := range available {
				out = append(out, profileEntry{Module: name})
			}
			return out, nil
		}
	}

	have := make(map[string]bool, len(available))
	for _, name := range available {
		have[name] = true
	}
	selected := make([]profileEntry, 0, len(wanted))
	for _, e := range wanted {
		if !have[e.Module] {
			return nil, fmt.Errorf("指定的模块 %q 不存在，现有模块：%s", e.Module, strings.Join(available, "、"))
		}
		if err := checkEntry(e); err != nil {
			return nil, err
		}
		selected = append(selected, e)
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].Module < selected[j].Module })
	return selected, nil
}

// checkEntry 校验条目选项。tabs 目前只有 remote 消费：别的模块的 New 不收参数，
// 给了也编译不过，不如在这里早报。kind 拼错会把整页裁成空白，也在这里拦。
func checkEntry(e profileEntry) error {
	if len(e.Tabs) == 0 {
		return nil
	}
	if e.Module != "remote" {
		return fmt.Errorf("模块 %q 不支持 tabs 选项（目前只有 remote 的标签页可以在构建期裁剪）", e.Module)
	}
	for _, t := range e.Tabs {
		if !tabKinds[t] {
			return fmt.Errorf("remote 的 tabs 里 %q 不认识，可选：io、register、command", t)
		}
	}
	return nil
}

// checkPackageName 确保目录名与 Go 包名一致，生成的 import 才是对的。
func checkPackageName(name string) error {
	dir := filepath.Join(backendRoot, name)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return err
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, parser.PackageClauseOnly)
		if err != nil {
			return fmt.Errorf("解析 %s 失败：%w", file, err)
		}
		if f.Name.Name != name {
			return fmt.Errorf("%s 的包名是 %q，与目录名 %q 不一致", file, f.Name.Name, name)
		}
	}
	return nil
}

// warnMissingDocs 只提醒，不阻断构建。文档缺失不影响程序正确性，
// 但放着不管迟早会全都没有。
func warnMissingDocs(modules []string) {
	for _, name := range modules {
		if _, err := os.Stat(filepath.Join(docRoot, name+".md")); err != nil {
			fmt.Fprintf(os.Stderr, "genmodules: 警告：模块 %s 缺少文档 %s/%s.md\n", name, docRoot, name)
		}
	}
}

func isGoPackage(dir string) bool {
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	return err == nil && len(files) > 0
}

func subdirs(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败：%w", root, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

func profileNames(cfg *config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for name := range cfg.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func renderGo(profile string, modules []profileEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by tools/genmodules; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// profile: %s\n\n", profile)
	b.WriteString("package modules\n\nimport (\n\t\"embedtools/internal/module\"\n")
	for _, e := range modules {
		fmt.Fprintf(&b, "\t\"embedtools/internal/modules/%s\"\n", e.Module)
	}
	b.WriteString(")\n\n// All 返回当前 profile 启用的模块。\nfunc All() []module.Module {\n\treturn []module.Module{\n")
	for _, e := range modules {
		fmt.Fprintf(&b, "\t\t%s,\n", constructor(e))
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

// constructor 渲染模块的构造调用。带 tabs 的条目生成白名单参数，
// 由模块的 New 接收（目前只有 remote）。
func constructor(e profileEntry) string {
	if len(e.Tabs) == 0 {
		return e.Module + ".New()"
	}
	quoted := make([]string, 0, len(e.Tabs))
	for _, t := range e.Tabs {
		quoted = append(quoted, strconv.Quote(t))
	}
	return fmt.Sprintf("%s.New(%s)", e.Module, strings.Join(quoted, ", "))
}

func renderTS(profile string, modules []profileEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by tools/genmodules; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// profile: %s\n\n", profile)
	for _, e := range modules {
		fmt.Fprintf(&b, "import %s from '../modules/%s/module'\n", ident(e.Module), e.Module)
	}
	b.WriteString("\nexport const enabledModules = [")
	for i, e := range modules {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(ident(e.Module))
	}
	b.WriteString("]\n")
	return b.String()
}

func ident(name string) string {
	return "m_" + identRe.ReplaceAllString(name, "_")
}

func writeFile(path, content string) error {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", path, err)
	}
	return nil
}
