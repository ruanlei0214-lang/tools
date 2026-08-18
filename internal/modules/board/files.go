package board

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/sftp"
)

// 文本编辑只读这一档大小：再大就该用本机编辑器，不该塞进对话框。
const maxEditBytes = 48 * 1024

// 图片预览比文本框宽松：设备上的截图、相机图经常几百 KB，但整颗固件仍不该塞进来。
const maxPreviewBytes = 4 * 1024 * 1024

// Entry 是远端目录里的一个条目。
type Entry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"isDir"`
}

// listDir 列出一个远端目录。
//
// 走 SFTP 的 ReadDir 而不是解析 ls 的输出：名称、大小、是不是目录都是结构化的，
// 不用跟 `ls -lA` 的格式差异较劲——那份输出里符号链接、带空格的名字、
// 各家实现的日期列都得单独处置，而这台设备的 / 目录里就摆着两个符号链接。
func listDir(c *sftp.Client, dir string) ([]Entry, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, errors.New("请填写远端路径")
	}

	infos, err := c.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("列出 %s 失败: %w", dir, err)
	}

	entries := make([]Entry, 0, len(infos))
	for _, fi := range infos {
		entries = append(entries, Entry{Name: fi.Name(), Size: fi.Size(), IsDir: fi.IsDir()})
	}
	// 目录排在前面，各自按名字排。设备给的顺序是它自己的，同一个目录两次列出来
	// 顺序不一样的话，眼睛就得每次重新找。
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

// readRemoteText 读一份够小的文本文件，给界面上的编辑框用。
// 二进制或超大的直接拒绝，避免把整颗固件塞进文本框。
func readRemoteText(c *sftp.Client, remotePath string) (string, error) {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" {
		return "", errors.New("没有选择要编辑的文件")
	}
	st, err := c.Stat(remotePath)
	if err != nil {
		return "", fmt.Errorf("读取 %s 的信息失败: %w", remotePath, err)
	}
	if st.IsDir() {
		return "", fmt.Errorf("%s 是目录", remotePath)
	}
	if st.Size() > maxEditBytes {
		return "", fmt.Errorf("%s 有 %d 字节，超过 %d，不能当文本编辑", remotePath, st.Size(), maxEditBytes)
	}

	f, err := c.Open(remotePath)
	if err != nil {
		return "", fmt.Errorf("打开 %s 失败: %w", remotePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxEditBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", remotePath, err)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", fmt.Errorf("%s 是二进制文件，不能当文本编辑", remotePath)
	}
	return string(data), nil
}

// readRemoteBytes 读一份够小的文件，给界面预览图片用。
func readRemoteBytes(c *sftp.Client, remotePath string) ([]byte, error) {
	remotePath = strings.TrimSpace(remotePath)
	if remotePath == "" {
		return nil, errors.New("没有选择要预览的文件")
	}
	st, err := c.Stat(remotePath)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 的信息失败: %w", remotePath, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("%s 是目录", remotePath)
	}
	if st.Size() > maxPreviewBytes {
		return nil, fmt.Errorf("%s 有 %d 字节，超过 %d，不能预览", remotePath, st.Size(), maxPreviewBytes)
	}

	f, err := c.Open(remotePath)
	if err != nil {
		return nil, fmt.Errorf("打开 %s 失败: %w", remotePath, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxPreviewBytes+1))
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", remotePath, err)
	}
	if int64(len(data)) > maxPreviewBytes {
		return nil, fmt.Errorf("%s 超过 %d 字节，不能预览", remotePath, maxPreviewBytes)
	}
	return data, nil
}

// upload 把本地文件传到远端。
//
// 先写 <目标>.tmp、完整落盘之后再顶替目标。不直接往目标上写：SFTP 的 Create 会当场
// 把它截断，传到一半断掉就留下半个文件——而那可能是设备正在跑的东西（/opt 下就摆着
// zlgmaster、autorun.sh 这类）。任何一步失败都把临时文件清掉，原文件一个字节没动。
func upload(c *sftp.Client, localPath, remotePath string) error {
	src, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("打开本地文件失败: %w", err)
	}
	defer src.Close()

	tmp := remotePath + ".tmp"
	dst, err := c.Create(tmp)
	if err != nil {
		return fmt.Errorf("在设备上创建 %s 失败: %w", tmp, err)
	}

	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		c.Remove(tmp)
		return fmt.Errorf("上传中断: %w", err)
	}
	// Close 的错误必须看：写缓冲是在这一步才真正落到设备上的，
	// 磁盘满这类问题只有它会报出来。
	if err := dst.Close(); err != nil {
		c.Remove(tmp)
		return fmt.Errorf("上传收尾失败: %w", err)
	}

	if err := replace(c, tmp, remotePath); err != nil {
		c.Remove(tmp)
		return err
	}
	return nil
}

// replace 用 tmp 顶替 target。
//
// 优先用 posix-rename 扩展：它在目标已存在时直接原子替换。退回到普通 Rename 时得先
// 删掉目标（SFTP 的 rename 在目标存在时会失败），那中间就有一小段目标不存在的窗口——
// 所以只在设备不支持扩展时才走这条。实测这台设备是 OpenSSH 8.6，走的是前者。
func replace(c *sftp.Client, tmp, target string) error {
	if err := c.PosixRename(tmp, target); err == nil {
		return nil
	}
	if _, err := c.Stat(target); err == nil {
		if err := c.Remove(target); err != nil {
			return fmt.Errorf("替换 %s 失败（旧文件删不掉）: %w", target, err)
		}
	}
	if err := c.Rename(tmp, target); err != nil {
		return fmt.Errorf("替换 %s 失败: %w", target, err)
	}
	return nil
}

// download 把远端文件取到本地。
//
// 先写 <目标>.part，核对字节数与远端报告的大小一致之后才改名。核对这一步是必要的：
// 流式读到 EOF 分不清「文件读完了」和「连接断了」，不比一次大小就可能把半个文件
// 当成完整的收下来。
func download(c *sftp.Client, remotePath, localPath string) error {
	st, err := c.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("读取 %s 的信息失败: %w", remotePath, err)
	}
	if st.IsDir() {
		return fmt.Errorf("%s 是目录，不支持下载目录", remotePath)
	}

	src, err := c.Open(remotePath)
	if err != nil {
		return fmt.Errorf("打开 %s 失败: %w", remotePath, err)
	}
	defer src.Close()

	part := localPath + ".part"
	dst, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("创建本地文件失败: %w", err)
	}

	n, err := io.Copy(dst, src)
	if err != nil {
		dst.Close()
		os.Remove(part)
		return fmt.Errorf("下载中断: %w", err)
	}
	if err := dst.Close(); err != nil {
		os.Remove(part)
		return fmt.Errorf("下载收尾失败: %w", err)
	}
	if n != st.Size() {
		os.Remove(part)
		return fmt.Errorf("下载不完整：设备上是 %d 字节，只收到 %d 字节", st.Size(), n)
	}

	if err := os.Rename(part, localPath); err != nil {
		os.Remove(part)
		return fmt.Errorf("保存到 %s 失败: %w", localPath, err)
	}
	return nil
}

// remoteJoin 拼远端路径。远端是 Linux，一律用正斜杠——
// 用 filepath.Join 会在 Windows 上拼出反斜杠，设备不认。
func remoteJoin(dir, name string) string {
	return path.Join(strings.TrimSpace(dir), name)
}

// 一次拖入或点上传可能带进整棵目录。5000 个文件已经远超现场固件包的体量，
// 再大多半是拖错了盘符，及时停比传一半再报错干净。
const maxTransferItems = 5000

// uploadTree 把本地文件或整棵目录传到远端。目录会在设备上按同名建出来再往里填。
func uploadTree(c *sftp.Client, local, remote string) error {
	lst, err := os.Stat(local)
	if err != nil {
		return fmt.Errorf("打开本地路径失败: %w", err)
	}
	rst, rerr := c.Stat(remote)
	exists := rerr == nil

	if lst.IsDir() {
		if exists && !rst.IsDir() {
			return fmt.Errorf("%s 在设备上已是文件，不能用文件夹覆盖", remote)
		}
		if err := c.MkdirAll(remote); err != nil {
			return fmt.Errorf("在设备上创建目录 %s 失败: %w", remote, err)
		}
		ents, err := os.ReadDir(local)
		if err != nil {
			return fmt.Errorf("读取本地目录 %s 失败: %w", local, err)
		}
		for _, e := range ents {
			if err := uploadTree(c, filepath.Join(local, e.Name()), remoteJoin(remote, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	if exists && rst.IsDir() {
		return fmt.Errorf("%s 在设备上已是目录，不能用文件覆盖", remote)
	}
	return upload(c, local, remote)
}

// downloadTree 把远端文件或整棵目录取到本地。目录会按同名建出来再往里填。
func downloadTree(c *sftp.Client, remote, local string) error {
	st, err := c.Stat(remote)
	if err != nil {
		return fmt.Errorf("读取 %s 的信息失败: %w", remote, err)
	}
	if st.IsDir() {
		if err := os.MkdirAll(local, 0o755); err != nil {
			return fmt.Errorf("创建本地目录失败: %w", err)
		}
		ents, err := c.ReadDir(remote)
		if err != nil {
			return fmt.Errorf("列出 %s 失败: %w", remote, err)
		}
		for _, e := range ents {
			if err := downloadTree(c, remoteJoin(remote, e.Name()), filepath.Join(local, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		return fmt.Errorf("创建本地目录失败: %w", err)
	}
	return download(c, remote, local)
}

// countLocalItems 统计这次上传会碰到多少个文件/目录，用来挡误拖整盘。
func countLocalItems(root string) (int, error) {
	n := 0
	err := filepath.WalkDir(root, func(_ string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n++
		if n > maxTransferItems {
			return fmt.Errorf("超过 %d 项，像是拖错了位置", maxTransferItems)
		}
		return nil
	})
	return n, err
}
