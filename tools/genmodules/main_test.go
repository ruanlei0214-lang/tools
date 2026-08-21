package main

import "testing"

func TestProfileEntryParsesBothForms(t *testing.T) {
	var e profileEntry
	if err := e.UnmarshalJSON([]byte(`"remote"`)); err != nil {
		t.Fatal(err)
	}
	if e.Module != "remote" || e.Tabs != nil {
		t.Fatalf("字符串形式：%+v", e)
	}

	if err := e.UnmarshalJSON([]byte(`{"module":"remote","tabs":["command"]}`)); err != nil {
		t.Fatal(err)
	}
	if e.Module != "remote" || len(e.Tabs) != 1 || e.Tabs[0] != "command" {
		t.Fatalf("对象形式：%+v", e)
	}

	if err := e.UnmarshalJSON([]byte(`123`)); err == nil {
		t.Fatal("数字不该解析成条目")
	}
}

func TestConstructorRendersTabs(t *testing.T) {
	if got := constructor(profileEntry{Module: "netcfg"}); got != "netcfg.New()" {
		t.Fatalf("无选项：%s", got)
	}
	if got := constructor(profileEntry{Module: "remote", Tabs: []string{"command"}}); got != `remote.New("command")` {
		t.Fatalf("带 tabs：%s", got)
	}
}

// tabs 拼错或给错模块要在生成期就报出来，不能等到编译或运行成空白页。
func TestCheckEntryValidatesTabs(t *testing.T) {
	if err := checkEntry(profileEntry{Module: "remote", Tabs: []string{"io", "command"}}); err != nil {
		t.Fatal(err)
	}
	if err := checkEntry(profileEntry{Module: "netcfg", Tabs: []string{"command"}}); err == nil {
		t.Fatal("remote 之外的模块不该支持 tabs")
	}
	if err := checkEntry(profileEntry{Module: "remote", Tabs: []string{"commnad"}}); err == nil {
		t.Fatal("拼错的 kind 应当报错")
	}
}
