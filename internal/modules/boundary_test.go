package modules

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const goModulePrefix = "embedtools/internal/modules/"

// 匹配 import ... from '../xxx/...' 里的第一段相对路径。
var tsImportRe = regexp.MustCompile(`from\s+['"]\.\./([^/'"]+)/`)

// 模块之间必须互不引用：删掉任意一个模块，其余模块都应当照常编译。
// 共享逻辑要么下沉到 internal/module，要么就留在各自模块里重复一份。

func TestBackendModulesAreIndependent(t *testing.T) {
	for _, name := range subdirs(t, ".") {
		for _, file := range goFiles(t, name) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
			if err != nil {
				t.Fatalf("解析 %s 失败: %v", file, err)
			}
			for _, imp := range f.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				if err != nil || !strings.HasPrefix(path, goModulePrefix) {
					continue
				}
				other := strings.TrimPrefix(path, goModulePrefix)
				if other != name && !strings.HasPrefix(other, name+"/") {
					t.Errorf("%s 引用了模块 %s，模块之间不允许互相引用", file, other)
				}
			}
		}
	}
}

func TestFrontendModulesAreIndependent(t *testing.T) {
	root := filepath.Join("..", "..", "frontend", "src", "modules")
	for _, name := range subdirs(t, root) {
		dir := filepath.Join(root, name)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", dir, err)
		}
		for _, e := range entries {
			ext := filepath.Ext(e.Name())
			if e.IsDir() || (ext != ".ts" && ext != ".vue") {
				continue
			}
			file := filepath.Join(dir, e.Name())
			src, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("读取 %s 失败: %v", file, err)
			}
			for _, m := range tsImportRe.FindAllStringSubmatch(string(src), -1) {
				// ".." 是 ../../shell 这类指向共享层的路径，不是同级模块。
				if m[1] != ".." && m[1] != name {
					t.Errorf("%s 引用了模块 %s，模块之间不允许互相引用", file, m[1])
				}
			}
		}
	}
}

func subdirs(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		t.Fatalf("%s 下没有找到任何模块目录", dir)
	}
	return names
}

func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("扫描 %s 失败: %v", dir, err)
	}
	return files
}
