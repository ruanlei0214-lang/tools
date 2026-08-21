// wsprobe 是一次性诊断工具：走 remote 模块的真实代码路径连上控制器，
// 验证机器人状态订阅（publish/RobotStatus）能不能拿到数据。
package main

import (
	"fmt"
	"os"
	"time"

	"embedtools/internal/modules/remote"
)

func main() {
	host := "192.168.1.136"
	if len(os.Args) > 1 {
		host = os.Args[1]
	}

	m := remote.New()
	svc := m.Bindings()[0].(*remote.Service)

	st, err := svc.Connect(remote.Device{Host: host, Port: 9000, Path: "/"})
	if err != nil {
		fmt.Println("连接失败:", err)
		os.Exit(1)
	}
	fmt.Println("已连接:", st.Addr)

	rs, err := svc.GetRobotStatus()
	if err != nil {
		fmt.Println("获取机器人状态失败:", err)
		os.Exit(1)
	}
	fmt.Printf("首帧：state=%d stateName=%q mode=%d modeName=%q isSimulation=%v\n",
		rs.State, rs.StateName, rs.Mode, rs.ModeName, rs.IsSimulation)

	// 缓存路径：第二次应当立即返回，不等来回。
	start := time.Now()
	rs, err = svc.GetRobotStatus()
	if err != nil {
		fmt.Println("第二次获取失败:", err)
		os.Exit(1)
	}
	fmt.Printf("缓存：state=%d stateName=%q（耗时 %s）\n", rs.State, rs.StateName, time.Since(start))

	svc.Disconnect()
	fmt.Println("通过")
}
