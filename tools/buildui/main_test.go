package main

import (
	"os"
	"path/filepath"
	"testing"
)

// modules.json 的条目可以是模块名，也可以是 {"module":...,"tabs":[...]} 带选项的对象。
// 两种写法都不能把 profile 列表搞空——下拉框只关心名字，不该去解析条目内容。
func TestLoadProfilesWithMixedEntries(t *testing.T) {
	dir := t.TempDir()
	raw := `{"profiles":{
		"all": ["*"],
		"plain": ["netcfg"],
		"with-tabs": ["netcfg", {"module": "remote", "tabs": ["command"]}]
	}}`
	if err := os.WriteFile(filepath.Join(dir, "modules.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	names, err := loadProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 || names[0] != "all" {
		t.Fatalf("names=%v", names)
	}
}
