// Package board 通过 SSH 控制控制器主板：跑自定义指令、传文件。
//
// 指令走 SSH 的 exec 通道，文件走同一条连接上的 SFTP。实测这台设备是 OpenSSH 8.6、
// /usr/libexec/sftp-server 在位，所以不为「万一没有 SFTP」准备第二套传输实现。
package board

import (
	"context"
	"embedtools/internal/module"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

// Module 是 board 模块的入口。
type Module struct {
	svc *Service
}

func New() *Module { return &Module{svc: &Service{settings: loadSettings()}} }

func (m *Module) ID() string { return "board" }

func (m *Module) Bindings() []any { return []any{m.svc} }

// Startup 收下 Wails 的上下文。选本地文件要弹系统对话框，那需要它。
func (m *Module) Startup(ctx context.Context) { m.svc.ctx = ctx }

// Status 是当前连接状态，界面靠它决定按钮能不能点。
type Status struct {
	Connected bool `json:"connected"`
	// Addr 是连上的 user@host:port，显示出来好确认连的是哪台。
	Addr string `json:"addr"`
	// Error 是连接被动断开的原因。主动断开时为空。
	Error string `json:"error"`
}

// UploadResult 是一次上传的结果。
//
// NeedsConfirm 为真时什么都没传：远端已有同名文件，等界面确认后带 overwrite 再调一次。
// 用一个返回值而不是特殊错误字符串，是因为「要确认」不是失败，前端也不该去比对错误文本。
type UploadResult struct {
	RemotePath   string `json:"remotePath"`
	NeedsConfirm bool   `json:"needsConfirm"`
}

// Service 暴露给前端。它持有一条 SSH 长连接和挂在上面的 SFTP 客户端，
// 终端会话按 ID 索引，最多 maxTerminalSessions 个（界面上的分屏）。
//
// 连接显式建立、显式断开，不做自动重连：这里的命令是重启进程、删文件这种做过就回不去
// 的事，藏在每次调用背后重连会让「刚才那一下到底发出去没有」说不清楚。
type Service struct {
	settings Settings
	ctx      context.Context

	mu        sync.Mutex
	conn      *ssh.Client
	sftp      *sftp.Client
	terminals map[string]*terminalSession
	addr      string
	lastErr   string
}

// maxTerminalSessions 是终端分屏的上限。每个会话在设备上是一个 shell 进程，
// 界面上四个格子也已经摆满了。
const maxTerminalSessions = 4

// Config 返回页面默认值。地址和 SSH 凭据优先用共享配置 toolbox-config.json，
// 没有或坏掉才退回编译进产物的 config.json——三个模块连的是同一台控制器。
func (s *Service) Config() Settings {
	shared := module.LoadShared()
	out := s.settings
	if shared.Host != "" {
		out.Device.Host = shared.Host
	}
	if shared.User != "" {
		out.Device.User = shared.User
	}
	if shared.Password != "" {
		out.Device.Password = shared.Password
	}
	if shared.KeyPath != "" {
		out.Device.KeyPath = shared.KeyPath
	}
	return out
}

// Status 报告当前连接状态。
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return Status{Error: s.lastErr}
	}
	return Status{Connected: true, Addr: s.addr}
}

// Connect 建连并在同一条连接上开 SFTP，重复调用会先断开旧的。
func (s *Service) Connect(d Device) (Status, error) {
	timeout := time.Duration(s.settings.ConnectTimeoutSeconds) * time.Second

	// 建连不持锁：它最长要等 connectTimeoutSeconds，持着锁的话这段时间里
	// Status 也会被卡住，界面连状态都读不出来。
	conn, err := dial(d, timeout)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.mu.Unlock()
		return Status{Error: err.Error()}, err
	}

	// SFTP 单独报错。认证过了但子系统起不来是另一类问题——设备上没有 sftp-server，
	// 现场得能一眼分清是网络不通还是这台设备不支持，而不是笼统一句「连接失败」。
	sc, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		wrapped := fmt.Errorf("SSH 已连上，但 SFTP 子系统起不来（设备上可能没有 sftp-server）: %w", err)
		s.mu.Lock()
		s.lastErr = wrapped.Error()
		s.mu.Unlock()
		return Status{Error: wrapped.Error()}, wrapped
	}

	addr := fmt.Sprintf("%s@%s:%d", d.User, d.Host, d.Port)

	s.mu.Lock()
	s.closeLocked()
	s.conn, s.sftp, s.addr, s.lastErr = conn, sc, addr, ""
	s.mu.Unlock()

	// 连上了就记下来：netcfg 和 remote 下次打开用的就是这个地址。
	// 只记地址，不记端口——WS 端口和 SSH 端口是两回事，共享配置里不存端口。
	// 写失败不影响连接本身，只是下次打开退回默认地址，所以只记日志。
	if err := module.SaveShared(module.Shared{Host: d.Host, User: d.User, Password: d.Password, KeyPath: d.KeyPath}); err != nil {
		log.Printf("board: 写入共享配置失败：%v", err)
	}

	go s.watch(conn)
	return Status{Connected: true, Addr: addr}, nil
}

// SaveDevice 保存连接参数到共享配置。地址、用户名、密码、密钥路径都写进
// toolbox-config.json。界面不再提供编辑入口，现场改凭据直接改这份文件。
// 返回更新后的页面默认值（共享配置优先）。
func (s *Service) SaveDevice(d Device) (Settings, error) {
	if err := module.SaveShared(module.Shared{
		Host:     d.Host,
		User:     d.User,
		Password: d.Password,
		KeyPath:  d.KeyPath,
	}); err != nil {
		return Settings{}, err
	}
	return s.Config(), nil
}

// Disconnect 主动断开。切走页面或换设备时调用，别把连接挂在那儿。
func (s *Service) Disconnect() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closeLocked()
	s.lastErr = ""
	return Status{}
}

// watch 等这条连接死掉，把状态改回未连接。
//
// 没有它的话，设备重启或网线被拔之后界面还显示着「已连接」，直到下一次点按钮才发现。
// 只在 s.conn 还是自己那条时才动状态：主动断开或重连之后这条已经不是当前连接了。
func (s *Service) watch(conn *ssh.Client) {
	err := conn.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != conn {
		return
	}
	s.closeLocked()
	if err != nil {
		s.lastErr = fmt.Sprintf("与主板的连接已断开：%v", err)
	} else {
		s.lastErr = "与主板的连接已断开"
	}
}

func (s *Service) closeLocked() {
	for id, t := range s.terminals {
		t.close()
		delete(s.terminals, id)
	}
	if s.sftp != nil {
		s.sftp.Close()
		s.sftp = nil
	}
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	s.addr = ""
}

// ListCommands 读按钮清单。现场那份优先，没有就用出厂默认。不需要连接。
func (s *Service) ListCommands() CommandList { return loadCommands() }

// SaveCommands 校验并整份写回按钮清单，返回写进去的那份（补齐了编号）。
func (s *Service) SaveCommands(cmds []Command) (CommandList, error) { return saveCommands(cmds) }

// ResetCommands 删掉现场清单，退回出厂默认。
func (s *Service) ResetCommands() (CommandList, error) {
	if err := removeCommandsStore(); err != nil {
		return CommandList{}, err
	}
	return loadCommands(), nil
}

// ExportCommands 把当前清单存成用户选的 JSON 文件。取消时返回空路径。
func (s *Service) ExportCommands() (string, error) {
	if s.ctx == nil {
		return "", errors.New("界面还没准备好，稍后再试")
	}
	raw, err := commandsBytes()
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{
		Title:           "导出指令清单",
		DefaultFilename: commandsFileName,
		Filters:         []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", fmt.Errorf("写入 %s 失败：%w", path, err)
	}
	return path, nil
}

// ImportCommands 用用户选的 JSON 替换当前清单。取消选文件时清单不动。
func (s *Service) ImportCommands() (CommandFileResult, error) {
	if s.ctx == nil {
		return CommandFileResult{}, errors.New("界面还没准备好，稍后再试")
	}
	path, err := runtime.OpenFileDialog(s.ctx, runtime.OpenDialogOptions{
		Title:   "导入指令清单",
		Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return CommandFileResult{}, err
	}
	if path == "" {
		return CommandFileResult{List: loadCommands(), Canceled: true}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return CommandFileResult{}, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	list, err := applyImportedCommands(raw, filepath.Base(path))
	if err != nil {
		return CommandFileResult{}, err
	}
	return CommandFileResult{List: list, Path: path}, nil
}

// RunCommand 按编号取出命令并原样下发。
//
// 从清单文件里取而不是让前端把命令送过来：执行的必须是那个被保存下来的命令，
// 界面上显示的和实际跑的不能是两回事。
func (s *Service) RunCommand(id string) (CommandResult, error) {
	list := loadCommands()
	for _, c := range list.Commands {
		if c.ID == id {
			conn, err := s.client()
			if err != nil {
				return CommandResult{}, err
			}
			timeout := time.Duration(s.settings.CommandTimeoutSeconds) * time.Second
			return run(conn, c.Command, timeout)
		}
	}
	return CommandResult{}, fmt.Errorf("按钮 %q 已经不在清单里了，刷新一下页面", id)
}

// StartTerminal 在当前 SSH 连接上为 id 打开一个持久 PTY。id 已存在且存活时复用；
// 新 id 在会话数到顶时被拒——前端分屏最多四个格子，这里兜底。
func (s *Service) StartTerminal(id string) error {
	if id == "" {
		return errors.New("终端会话 ID 不能为空")
	}
	s.mu.Lock()
	if s.conn == nil {
		err := s.notConnectedLocked()
		s.mu.Unlock()
		return err
	}
	if t, ok := s.terminals[id]; ok && t.alive() {
		s.mu.Unlock()
		return nil
	}
	conn := s.conn
	s.mu.Unlock()

	terminal, err := newTerminalSession(conn)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != conn {
		terminal.close()
		return errors.New("打开终端期间主板连接已变化，请重试")
	}
	if old := s.terminals[id]; old != nil {
		old.close()
	}
	if _, ok := s.terminals[id]; !ok && len(s.terminals) >= maxTerminalSessions {
		terminal.close()
		return fmt.Errorf("最多同时开 %d 个终端", maxTerminalSessions)
	}
	if s.terminals == nil {
		s.terminals = map[string]*terminalSession{}
	}
	s.terminals[id] = terminal
	return nil
}

// WriteTerminal 把文本原样写入 id 对应的终端。换行由调用方明确传入，因而也能发送 Ctrl+C 等控制字符。
func (s *Service) WriteTerminal(id, text string) error {
	if len(text) > 64*1024 {
		return errors.New("单次终端输入不能超过 64KB")
	}
	s.mu.Lock()
	terminal := s.terminals[id]
	s.mu.Unlock()
	if terminal == nil || !terminal.alive() {
		return errors.New("终端尚未打开")
	}
	return terminal.write(text)
}

// ReadTerminal 取走 id 对应终端当前累积的输出。前端短轮询调用；读过的内容不会再次返回。
// id 不存在时返回空——会话可能刚被关掉，轮询晚到一步不算错误。
func (s *Service) ReadTerminal(id string) (string, error) {
	s.mu.Lock()
	terminal := s.terminals[id]
	s.mu.Unlock()
	if terminal == nil {
		return "", nil
	}
	return terminal.drain(), nil
}

// CloseTerminal 关闭 id 对应的 PTY，不影响 SSH 连接、SFTP 和其他终端。
func (s *Service) CloseTerminal(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.terminals[id]; ok {
		t.close()
		delete(s.terminals, id)
	}
}

// RunCommandInTerminal 从持久化清单取出命令，写进 id 对应的终端。
func (s *Service) RunCommandInTerminal(id, cmdID string) error {
	list := loadCommands()
	for _, c := range list.Commands {
		if c.ID == cmdID {
			return s.WriteTerminal(id, c.Command+"\n")
		}
	}
	return fmt.Errorf("按钮 %q 已经不在清单里了，刷新一下页面", cmdID)
}

// ListDir 列出远端目录。
func (s *Service) ListDir(dir string) ([]Entry, error) {
	c, err := s.sftpClient()
	if err != nil {
		return nil, err
	}
	return listDir(c, dir)
}

// ReadRemoteText 读一份够小的文本文件，给文件页的编辑框用。
func (s *Service) ReadRemoteText(remotePath string) (string, error) {
	c, err := s.sftpClient()
	if err != nil {
		return "", err
	}
	return readRemoteText(c, remotePath)
}

// ReadRemoteBytes 读一份够小的文件，给终端下方的图片预览用。
//
// 返回 base64 字符串而不是 []byte：Wails 的绑定生成器把 []byte 标成 number[]，
// 而运行时 JSON 实际给的是 base64 字符串，类型和值对不上。在这里显式编码，
// 前端拿到的类型和值就一致了。
func (s *Service) ReadRemoteBytes(remotePath string) (string, error) {
	c, err := s.sftpClient()
	if err != nil {
		return "", err
	}
	data, err := readRemoteBytes(c, remotePath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// PickKeyFile 弹系统对话框选一把本机私钥，取消时返回空字符串。
func (s *Service) PickKeyFile() (string, error) {
	if s.ctx == nil {
		return "", errors.New("界面还没准备好，稍后再试")
	}
	return runtime.OpenFileDialog(s.ctx, runtime.OpenDialogOptions{
		Title: "选择 SSH 私钥",
		Filters: []runtime.FileFilter{
			{DisplayName: "私钥", Pattern: "*.pem;*.key;id_rsa;id_ed25519;id_ecdsa;id_dsa"},
			{DisplayName: "所有文件", Pattern: "*.*"},
		},
	})
}

// PickLocalFile 弹系统对话框选一个要上传的本地文件，取消时返回空字符串。
func (s *Service) PickLocalFile() (string, error) {
	if s.ctx == nil {
		return "", errors.New("界面还没准备好，稍后再试")
	}
	return runtime.OpenFileDialog(s.ctx, runtime.OpenDialogOptions{Title: "选择要上传的文件"})
}

// PickSaveTarget 弹系统对话框选下载落点，取消时返回空字符串。
// 目标已存在时由系统对话框自己问要不要覆盖，这里不再问一遍。
func (s *Service) PickSaveTarget(name string) (string, error) {
	if s.ctx == nil {
		return "", errors.New("界面还没准备好，稍后再试")
	}
	return runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{
		Title:           "保存到",
		DefaultFilename: name,
	})
}

// Upload 把本地文件传到远端目录。
//
// overwrite 为 false 且远端已有同名文件时什么都不做，返回 NeedsConfirm 让界面去问。
func (s *Service) Upload(localPath, remoteDir string, overwrite bool) (UploadResult, error) {
	if localPath == "" {
		return UploadResult{}, errors.New("没有选择要上传的文件")
	}
	// 目标目录为空的话拼出来是个相对路径，文件会落到登录用户的家目录里——
	// 那不是任何人想要的结果，而且事后很难找。
	if strings.TrimSpace(remoteDir) == "" {
		return UploadResult{}, errors.New("请先列出一个目录，再往里上传")
	}
	c, err := s.sftpClient()
	if err != nil {
		return UploadResult{}, err
	}

	remotePath := remoteJoin(remoteDir, filepath.Base(localPath))
	if !overwrite {
		if _, err := c.Stat(remotePath); err == nil {
			return UploadResult{RemotePath: remotePath, NeedsConfirm: true}, nil
		}
	}
	if err := upload(c, localPath, remotePath); err != nil {
		return UploadResult{}, err
	}
	return UploadResult{RemotePath: remotePath}, nil
}

// Download 把远端文件取到本地。
func (s *Service) Download(remotePath, localPath string) error {
	if localPath == "" {
		return errors.New("没有选择保存位置")
	}
	c, err := s.sftpClient()
	if err != nil {
		return err
	}
	return download(c, remotePath, localPath)
}

// Delete 删一个远端文件。
//
// 不在这儿判断它是不是目录：界面上那个「是目录」来自上一次列目录的结果，可能已经过期。
// 交给设备判断，非空目录它自己会拒绝——SFTP 的 Remove 不递归，删不掉一整棵树。
func (s *Service) Delete(remotePath string) error {
	if remotePath == "" {
		return errors.New("没有选择要删除的文件")
	}
	c, err := s.sftpClient()
	if err != nil {
		return err
	}
	if err := c.Remove(remotePath); err != nil {
		return fmt.Errorf("删除 %s 失败: %w", remotePath, err)
	}
	return nil
}

func (s *Service) client() (*ssh.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		return nil, s.notConnectedLocked()
	}
	return s.conn, nil
}

func (s *Service) sftpClient() (*sftp.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sftp == nil {
		return nil, s.notConnectedLocked()
	}
	return s.sftp, nil
}

// notConnectedLocked 带上断开的原因。只说「尚未连接」的话，被设备单方面踢掉的那种
// 情况看起来就像用户自己忘了点连接。
func (s *Service) notConnectedLocked() error {
	if s.lastErr != "" {
		return fmt.Errorf("尚未连接主板（%s）", s.lastErr)
	}
	return errors.New("尚未连接主板")
}
