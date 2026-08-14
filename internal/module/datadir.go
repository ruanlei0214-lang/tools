package module

import (
	"fmt"
	"os"
	"path/filepath"
)

// 现场改出来的文件（remote 的三份配置、board 的按钮清单、netcfg 记住的地址）
// 和 WebView2 缓存都落在 exe 所在目录，不进 %APPDATA%。绿色版整夹拷走，
// 配置和第二次打开的加速缓存跟着走。
//
// 三个模块都要找这个目录，所以放在契约包里，而不是让哪个模块去引用另一个。
//
// dataDirFn 是包级变量而不是直接调 os.Executable：测试要把文件落在 t.TempDir() 里，
// 不能往测试二进制旁边写。
var dataDirFn = exeDir

func exeDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法定位程序目录：%w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe), nil
}

// DataDir 返回现场文件该落的目录，也就是正在跑的这个 exe 所在的目录。
func DataDir() (string, error) {
	return dataDirFn()
}

// UseTempDataDir 把数据目录指到 dir，返回恢复函数。只给测试用。
func UseTempDataDir(dir string) func() {
	old := dataDirFn
	dataDirFn = func() (string, error) { return dir, nil }
	return func() { dataDirFn = old }
}
