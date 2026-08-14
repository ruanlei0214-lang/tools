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

type config struct {
	Profiles map[string][]string `json:"profiles"`
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
func generate(label string, wanted []string) error {
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

	fmt.Printf("profile %q：启用 %d 个模块 [%s]\n", label, len(selected), strings.Join(selected, " "))
	return nil
}

func splitModules(s string) []string {
	var names []string
	for _, part := range strings.Split(s, ",") {
		if name := strings.TrimSpace(part); name != "" {
			names = append(names, name)
		}
	}
	return names
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

func resolve(wanted, available []string) ([]string, error) {
	for _, name := range wanted {
		if name == "*" {
			return available, nil
		}
	}

	have := make(map[string]bool, len(available))
	for _, name := range available {
		have[name] = true
	}
	selected := make([]string, 0, len(wanted))
	for _, name := range wanted {
		if !have[name] {
			return nil, fmt.Errorf("指定的模块 %q 不存在，现有模块：%s", name, strings.Join(available, "、"))
		}
		selected = append(selected, name)
	}
	sort.Strings(selected)
	return selected, nil
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

func renderGo(profile string, modules []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by tools/genmodules; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// profile: %s\n\n", profile)
	b.WriteString("package modules\n\nimport (\n\t\"embedtools/internal/module\"\n")
	for _, name := range modules {
		fmt.Fprintf(&b, "\t\"embedtools/internal/modules/%s\"\n", name)
	}
	b.WriteString(")\n\n// All 返回当前 profile 启用的模块。\nfunc All() []module.Module {\n\treturn []module.Module{\n")
	for _, name := range modules {
		fmt.Fprintf(&b, "\t\t%s.New(),\n", name)
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

func renderTS(profile string, modules []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by tools/genmodules; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// profile: %s\n\n", profile)
	for _, name := range modules {
		fmt.Fprintf(&b, "import %s from '../modules/%s/module'\n", ident(name), name)
	}
	b.WriteString("\nexport const enabledModules = [")
	for i, name := range modules {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(ident(name))
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
