package board

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
)

// pipeConn 把两个半双工管道拼成 sftp.NewServer 要的 ReadWriteCloser。
// Close 两端都关：只关写端的话，对面那半会一直等在读上。
type pipeConn struct {
	*io.PipeReader
	*io.PipeWriter
}

func (p pipeConn) Close() error {
	p.PipeReader.Close()
	return p.PipeWriter.Close()
}

// testSFTP 起一个跑在本地文件系统上的真 SFTP 服务端，用管道直连客户端。
//
// 用真服务端而不是打桩：要验的是「上传写临时文件再顶替」「下载核对字节数」这类
// 与协议行为绑在一起的事，桩件只会证明桩件自己的行为。
func testSFTP(t *testing.T) *sftp.Client {
	t.Helper()

	clientRead, serverWrite := io.Pipe()
	serverRead, clientWrite := io.Pipe()

	server, err := sftp.NewServer(pipeConn{PipeReader: serverRead, PipeWriter: serverWrite})
	if err != nil {
		t.Fatalf("起 SFTP 服务端失败：%v", err)
	}
	go server.Serve()

	client, err := sftp.NewClientPipe(clientRead, clientWrite)
	if err != nil {
		t.Fatalf("连 SFTP 服务端失败：%v", err)
	}
	// 先关服务端。反过来的话客户端会卡在等服务端那边的 EOF 上，
	// 而服务端还握着管道，两边就一起停在那儿了。
	t.Cleanup(func() {
		server.Close()
		client.Close()
	})
	return client
}

// remotePath 把本地临时目录里的路径转成 SFTP 用的正斜杠形式。
// 服务端跑在本地文件系统上，但协议里的路径一律是正斜杠。
func remotePath(parts ...string) string {
	return filepath.ToSlash(filepath.Join(parts...))
}

// 二进制内容必须逐字节一致：这是文件传输唯一不能打折的要求。
func TestUploadDownloadRoundTrip(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()

	// 覆盖 0x00 到 0xff 全部字节值，任何按文本处理的实现都会在这儿露出来。
	payload := make([]byte, 4096)
	for i := range payload {
		payload[i] = byte(i)
	}
	local := filepath.Join(dir, "src.bin")
	if err := os.WriteFile(local, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	target := remotePath(dir, "uploaded.bin")
	if err := upload(c, local, target); err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "uploaded.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("上传后内容不一致：%d 字节 vs %d 字节", len(got), len(payload))
	}

	back := filepath.Join(dir, "back.bin")
	if err := download(c, target, back); err != nil {
		t.Fatalf("下载失败：%v", err)
	}
	got, err = os.ReadFile(back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("下载后内容不一致：%d 字节 vs %d 字节", len(got), len(payload))
	}
}

// 上传成功之后临时文件不能留下，否则设备上会堆满 .tmp。
func TestUploadLeavesNoTempFile(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()

	local := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := upload(c, local, remotePath(dir, "dst.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dst.txt.tmp")); !os.IsNotExist(err) {
		t.Errorf("临时文件还在：%v", err)
	}
}

// 覆盖同名文件要真的换成新内容，而不是留下旧的或者拼在一起。
func TestUploadReplacesExistingFile(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()

	target := filepath.Join(dir, "app")
	if err := os.WriteFile(target, []byte("这是旧版本，比新的长很多很多"), 0o644); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(dir, "new")
	if err := os.WriteFile(local, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := upload(c, local, remotePath(target)); err != nil {
		t.Fatalf("上传失败：%v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("内容是 %q，期望 %q", got, "new")
	}
}

// 本地文件打不开时，远端一个字节都不该被碰——尤其是那个同名的旧文件。
func TestUploadFailureLeavesRemoteUntouched(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()

	target := filepath.Join(dir, "app")
	const original = "原来的内容"
	if err := os.WriteFile(target, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	err := upload(c, filepath.Join(dir, "不存在的文件"), remotePath(target))
	if err == nil {
		t.Fatal("上传一个不存在的本地文件居然成功了")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("远端文件被动过了：%q", got)
	}
	if _, err := os.Stat(target + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("失败后临时文件还在：%v", err)
	}
}

// 目录不能下载，而且拒绝之后本地不许留下半个文件。
func TestDownloadRejectsDirectory(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()

	sub := filepath.Join(dir, "somedir")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(dir, "out")
	err := download(c, remotePath(sub), local)
	if err == nil {
		t.Fatal("下载目录居然成功了")
	}
	if !strings.Contains(err.Error(), "目录") {
		t.Errorf("错误里该说清是目录：%v", err)
	}
	if _, err := os.Stat(local + ".part"); !os.IsNotExist(err) {
		t.Errorf("本地留下了 .part：%v", err)
	}
}

// 目录排在文件前面，各自按名字排。设备给的顺序是它自己的，
// 同一个目录两次列出来顺序不一样的话，眼睛就得每次重新找。
func TestListDirSortsDirsFirst(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()

	for _, name := range []string{"b.txt", "a.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zdir", "adir"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := listDir(c, remotePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name)
	}
	want := []string{"adir", "zdir", "a.txt", "b.txt"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("顺序是 %v，期望 %v", got, want)
	}
	if !entries[0].IsDir || entries[3].IsDir {
		t.Errorf("目录标记不对：%+v", entries)
	}
}

// 空目录要返回空切片而不是 nil：前端拿到 null 得多写一次判断。
func TestListDirEmptyReturnsEmptySlice(t *testing.T) {
	c := testSFTP(t)

	entries, err := listDir(c, remotePath(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	if entries == nil {
		t.Fatal("空目录应当返回空切片，不是 nil")
	}
	if len(entries) != 0 {
		t.Fatalf("空目录居然有 %d 个条目", len(entries))
	}
}

func TestListDirRejectsEmptyPath(t *testing.T) {
	c := testSFTP(t)

	if _, err := listDir(c, "   "); err == nil {
		t.Fatal("空路径应当报错")
	}
}

// 远端是 Linux，拼出来必须是正斜杠——filepath.Join 在 Windows 上会拼出反斜杠，
// 设备不认那种路径。
func TestReadRemoteTextAcceptsSmallText(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(p, []byte("echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readRemoteText(c, remotePath(dir, "a.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "echo hi\n" {
		t.Fatalf("读到 %q", got)
	}
}

func TestReadRemoteTextRejectsBinary(t *testing.T) {
	c := testSFTP(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(p, []byte{1, 0, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readRemoteText(c, remotePath(dir, "a.bin")); err == nil {
		t.Fatal("二进制应当被拒")
	}
}

func TestRemoteJoinUsesForwardSlash(t *testing.T) {
	cases := []struct {
		dir, name, want string
	}{
		{"/opt", "app.sh", "/opt/app.sh"},
		{"/opt/", "app.sh", "/opt/app.sh"},
		{" /opt/runtime ", "a b.txt", "/opt/runtime/a b.txt"},
		{"/", "x", "/x"},
	}
	for _, c := range cases {
		if got := remoteJoin(c.dir, c.name); got != c.want {
			t.Errorf("remoteJoin(%q, %q)=%q，期望 %q", c.dir, c.name, got, c.want)
		}
	}
}
