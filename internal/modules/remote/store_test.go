package remote

import (
	"embedtools/internal/module"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useTempConfigDir 把现场配置指到临时目录。真实的 exe 目录不能碰：
// 跑一次测试就往测试二进制旁边落三个文件，之后手工验证看到的就不是干净状态了。
func useTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(module.UseTempDataDir(dir))
	return dir
}

func TestReadStoreMissingFile(t *testing.T) {
	useTempConfigDir(t)

	_, path, err := readStore(ioFileName)
	if !errors.Is(err, errNoOverride) {
		t.Fatalf("文件不存在应当回 errNoOverride，实际 %v", err)
	}
	// 路径照样要给出来：告警和界面都要显示它。
	if !strings.HasSuffix(path, ioFileName) {
		t.Fatalf("path=%q", path)
	}
}

func TestWriteStoreThenRead(t *testing.T) {
	dir := useTempConfigDir(t)

	if err := writeStore(ioFileName, []byte(`{"groups":[]}`)); err != nil {
		t.Fatal(err)
	}
	raw, path, err := readStore(ioFileName)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"groups":[]}` {
		t.Fatalf("raw=%q", raw)
	}
	if path != filepath.Join(dir, ioFileName) {
		t.Fatalf("path=%q", path)
	}
	// 临时文件不许留下：目录里多一个 .tmp，下次有人手工翻这个目录就得猜哪份才是真的。
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("写完之后 .tmp 还在")
	}
}

// 三份文件互不干扰：写了 IO 不能让寄存器那份跟着出现。
func TestWriteStoreIsPerFile(t *testing.T) {
	useTempConfigDir(t)

	if err := writeStore(ioFileName, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readStore(registerFileName); !errors.Is(err, errNoOverride) {
		t.Fatalf("寄存器那份不该存在，err=%v", err)
	}
	if _, _, err := readStore(deviceFileName); !errors.Is(err, errNoOverride) {
		t.Fatalf("连接那份不该存在，err=%v", err)
	}
}

func TestReadStoreErrorNamesTheFile(t *testing.T) {
	dir := useTempConfigDir(t)

	// 用目录冒充文件：ReadFile 会失败，但不是「不存在」。
	if err := os.MkdirAll(filepath.Join(dir, ioFileName), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := readStore(ioFileName)
	if err == nil || errors.Is(err, errNoOverride) {
		t.Fatalf("读目录应当报错，实际 %v", err)
	}
	if !strings.Contains(err.Error(), ioFileName) {
		t.Fatalf("错误里应当带上文件名，实际是：%v", err)
	}
}

func TestRemoveStore(t *testing.T) {
	useTempConfigDir(t)

	if err := writeStore(registerFileName, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := removeStore(registerFileName); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readStore(registerFileName); !errors.Is(err, errNoOverride) {
		t.Fatalf("删掉之后应当退回 errNoOverride，实际 %v", err)
	}
	// 「恢复默认」可能被连点两次，第二次不该报错。
	if err := removeStore(registerFileName); err != nil {
		t.Fatalf("重复删除应当算成功，实际 %v", err)
	}
}

func TestConfigDirIsOverride(t *testing.T) {
	want := useTempConfigDir(t)

	dir, err := configDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != want {
		t.Fatalf("配置目录应当是临时目录 %q，实际 %q", want, dir)
	}
}
