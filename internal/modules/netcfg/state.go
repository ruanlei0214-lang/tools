package netcfg

import (
	"embedtools/internal/module"
	"log"
)

// 记住上次连通的设备地址，下次打开直接用它，而不是回到 config.json 里的出厂地址。
//
// 存在共享配置 toolbox-config.json 里，remote 和 board 也读这份——三个模块连的是
// 同一台控制器，地址只该在一个地方改。整夹拷走时这份记录跟着走。
// 读取走 Defaults()：它把共享配置的地址和凭据一起叠到出厂默认上。

// rememberHost 记下一个已经确认连得通的地址。写失败不影响本次操作，
// 只是下次打开会退回默认地址，所以记日志而不是往上抛。
//
// 地址没变时也照写：省掉那次写要先把旧值读回来解析一遍，而读加解析比写 30 字节更贵。
// 只动 Host 这一个字段，凭据是顶栏凭据弹层管的，这里不能顺手抹掉。
func rememberHost(host string) {
	if host == "" {
		return
	}
	s := module.LoadShared()
	s.Host = host
	if err := module.SaveShared(s); err != nil {
		log.Printf("netcfg: 写入共享配置失败：%v", err)
	}
}
