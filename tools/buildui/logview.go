package main

import (
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
)

func init() {
	_, _ = windows.LoadDLL("msftedit.dll")
}

type LogView struct {
	walk.WidgetBase
	fg   win.COLORREF
	bold bool
}

type logSpan struct {
	text string
	fg   win.COLORREF
	bold bool
}

var (
	colorDefault = win.RGB(230, 232, 236)
	colorRed     = win.RGB(248, 113, 113)
	colorGreen   = win.RGB(74, 222, 128)
	colorYellow  = win.RGB(250, 204, 21)
	colorBlue    = win.RGB(147, 197, 253)
	colorMagenta = win.RGB(216, 180, 254)
	colorCyan    = win.RGB(103, 232, 249)
	colorGray    = win.RGB(156, 163, 175)
	colorBg      = win.RGB(31, 36, 48)
)

var ansi16 = [16]win.COLORREF{
	win.RGB(80, 80, 80),
	colorRed,
	colorGreen,
	colorYellow,
	colorBlue,
	colorMagenta,
	colorCyan,
	colorDefault,
	colorGray,
	win.RGB(252, 165, 165),
	win.RGB(134, 239, 172),
	win.RGB(253, 224, 71),
	win.RGB(191, 219, 254),
	win.RGB(233, 213, 255),
	win.RGB(165, 243, 252),
	win.RGB(255, 255, 255),
}

func NewLogView(parent walk.Container) (*LogView, error) {
	lv := &LogView{fg: colorDefault}
	if err := walk.InitWidget(
		lv,
		parent,
		win.MSFTEDIT_CLASS,
		win.WS_VISIBLE|win.WS_VSCROLL|win.WS_HSCROLL|win.WS_TABSTOP|
			win.ES_MULTILINE|win.ES_READONLY|win.ES_WANTRETURN|win.ES_AUTOVSCROLL,
		win.WS_EX_CLIENTEDGE,
	); err != nil {
		return nil, err
	}
	lv.SendMessage(win.EM_SETBKGNDCOLOR, 0, uintptr(colorBg))
	lv.SendMessage(win.EM_EXLIMITTEXT, 0, 8<<20)
	lv.setFormat(colorDefault, false)
	return lv, nil
}

func (*LogView) CreateLayoutItem(ctx *walk.LayoutContext) walk.LayoutItem {
	return walk.NewGreedyLayoutItem()
}

func (lv *LogView) AppendLine(s string) {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimRight(s, "\n") + "\n"
	if strings.Contains(s, "\x1b") {
		for _, sp := range lv.consumeANSI(s) {
			if sp.text != "" {
				lv.write(sp.text, sp.fg, sp.bold)
			}
		}
		return
	}
	lv.write(s, guessColor(s), strings.HasPrefix(strings.TrimSpace(s), ">"))
}

func (lv *LogView) consumeANSI(s string) []logSpan {
	var out []logSpan
	i := 0
	for i < len(s) {
		esc := strings.IndexByte(s[i:], '\x1b')
		if esc < 0 {
			out = append(out, logSpan{s[i:], lv.fg, lv.bold})
			break
		}
		esc += i
		if esc > i {
			out = append(out, logSpan{s[i:esc], lv.fg, lv.bold})
		}
		if esc+1 >= len(s) {
			break
		}
		if s[esc+1] != '[' {
			i = esc + 2
			continue
		}
		j := esc + 2
		for j < len(s) && (s[j] == ';' || s[j] >= '0' && s[j] <= '9') {
			j++
		}
		if j < len(s) && s[j] == 'm' {
			lv.applySGR(s[esc+2 : j])
			i = j + 1
			continue
		}
		if j < len(s) {
			i = j + 1
			continue
		}
		break
	}
	return out
}

func (lv *LogView) applySGR(params string) {
	if params == "" {
		lv.fg, lv.bold = colorDefault, false
		return
	}
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		switch {
		case n == 0:
			lv.fg, lv.bold = colorDefault, false
		case n == 1:
			lv.bold = true
		case n == 22:
			lv.bold = false
		case n == 39:
			lv.fg = colorDefault
		case n >= 30 && n <= 37:
			lv.fg = ansi16[n-30]
		case n >= 90 && n <= 97:
			lv.fg = ansi16[n-90+8]
		case n == 38 && i+1 < len(parts):
			mode, _ := strconv.Atoi(parts[i+1])
			if mode == 5 && i+2 < len(parts) {
				idx, _ := strconv.Atoi(parts[i+2])
				lv.fg = color256(idx)
				i += 2
			} else if mode == 2 && i+4 < len(parts) {
				r, _ := strconv.Atoi(parts[i+2])
				g, _ := strconv.Atoi(parts[i+3])
				b, _ := strconv.Atoi(parts[i+4])
				lv.fg = win.RGB(byte(r), byte(g), byte(b))
				i += 4
			}
		}
	}
}

func color256(n int) win.COLORREF {
	if n < 0 {
		n = 0
	}
	if n < 16 {
		return ansi16[n]
	}
	if n < 232 {
		n -= 16
		r := byte((n / 36) * 51)
		g := byte(((n / 6) % 6) * 51)
		b := byte((n % 6) * 51)
		return win.RGB(r, g, b)
	}
	if n > 255 {
		n = 255
	}
	v := byte(8 + (n-232)*10)
	return win.RGB(v, v, v)
}

func guessColor(s string) win.COLORREF {
	low := strings.ToLower(s)
	switch {
	case strings.Contains(s, "失败") || strings.Contains(low, "error") || strings.Contains(low, "fail") ||
		strings.Contains(s, "已停止") || strings.Contains(s, "中止失败"):
		return colorRed
	case strings.Contains(s, "退出码 0") || strings.Contains(s, "全部完成") || strings.Contains(low, "success") ||
		strings.Contains(s, "构建完成"):
		return colorGreen
	case strings.HasPrefix(strings.TrimSpace(s), "退出码"):
		return colorRed
	case strings.Contains(s, "警告") || strings.Contains(low, "warning") || strings.Contains(low, "warn"):
		return colorYellow
	case strings.HasPrefix(strings.TrimSpace(s), ">"):
		return colorCyan
	default:
		return colorDefault
	}
}

func (lv *LogView) write(text string, fg win.COLORREF, bold bool) {
	text = strings.ReplaceAll(text, "\n", "\r\n")
	end := lv.SendMessage(win.WM_GETTEXTLENGTH, 0, 0)
	lv.SendMessage(win.EM_SETSEL, end, end)
	lv.setFormat(fg, bold)
	u, err := syscall.UTF16FromString(text)
	if err != nil {
		return
	}
	lv.SendMessage(win.EM_REPLACESEL, 0, uintptr(unsafe.Pointer(&u[0])))
	lv.SendMessage(win.WM_VSCROLL, win.SB_BOTTOM, 0)
}

func (lv *LogView) setFormat(fg win.COLORREF, bold bool) {
	var cf win.CHARFORMAT
	cf.CbSize = uint32(unsafe.Sizeof(cf))
	cf.DwMask = win.CFM_COLOR | win.CFM_BOLD | win.CFM_FACE | win.CFM_SIZE
	cf.CrTextColor = fg
	if bold {
		cf.DwEffects = win.CFE_BOLD
	}
	cf.YHeight = 180
	copy(cf.SzFaceName[:], syscall.StringToUTF16("Consolas"))
	lv.SendMessage(win.EM_SETCHARFORMAT, win.SCF_SELECTION, uintptr(unsafe.Pointer(&cf)))
}
