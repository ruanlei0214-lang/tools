package ping

import (
	"encoding/binary"
	"net"
	"syscall"
	"time"
	"unsafe"
)

// ICMP 回显走 iphlpapi 的 IcmpSendEcho——和系统 ping.exe 同一条路径，
// 不需要管理员权限。直接开原始套接字的方案（go-ping、pro-bing）在 Windows 上
// 必须管理员才能跑，现场不会接受「右键以管理员身份运行」。
var (
	iphlpapi            = syscall.NewLazyDLL("iphlpapi.dll")
	procIcmpCreateFile  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho    = iphlpapi.NewProc("IcmpSendEcho")
	procIcmpCloseHandle = iphlpapi.NewProc("IcmpCloseHandle")
)

const ipSuccess = 0

type ipOptionInformation struct {
	Ttl         uint8
	Tos         uint8
	Flags       uint8
	OptionsSize uint8
	OptionsData uintptr
}

type icmpEchoReply struct {
	Address       uint32
	Status        uint32
	RoundTripTime uint32
	DataSize      uint16
	Reserved      uint16
	Data          uintptr
	Options       ipOptionInformation
}

// echo 向 ip 发一个 ICMP 回显，ok 为假表示超时或对端不可达。
// 每次调用独立开句柄：扫描时上百个 goroutine 并发调它，不共享状态。
func echo(ip net.IP, timeout time.Duration) (rtt time.Duration, ok bool) {
	ip4 := ip.To4()
	if ip4 == nil {
		return 0, false
	}
	h, _, _ := procIcmpCreateFile.Call()
	if h == 0 {
		return 0, false
	}
	defer procIcmpCloseHandle.Call(h)

	payload := []byte("c2tools-ping-c2tools-ping!!")
	replyBuf := make([]byte, unsafe.Sizeof(icmpEchoReply{})+uintptr(len(payload))+8)

	// Destination 与 inet_addr 的返回一致：网络字节序的字节按小端装进 ULONG，
	// 所以这里用 LittleEndian——用 BigEndian 会把 192.168.1.1 发去 1.1.168.192。
	n, _, _ := procIcmpSendEcho.Call(
		h,
		uintptr(binary.LittleEndian.Uint32(ip4)),
		uintptr(unsafe.Pointer(&payload[0])),
		uintptr(len(payload)),
		0,
		uintptr(unsafe.Pointer(&replyBuf[0])),
		uintptr(len(replyBuf)),
		uintptr(timeout.Milliseconds()),
	)
	if n == 0 {
		return 0, false
	}
	reply := (*icmpEchoReply)(unsafe.Pointer(&replyBuf[0]))
	if reply.Status != ipSuccess {
		return 0, false
	}
	return time.Duration(reply.RoundTripTime) * time.Millisecond, true
}
