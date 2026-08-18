# 模块文档

每个模块一份文档，文件名与模块目录名一致。

| 模块 | 版本 | 文档 | 说明 |
| --- | --- | --- | --- |
| remote | V1.3.13 | [remote.md](remote.md) | 控制上位机 IO 与寄存器，点位和连接参数在界面上改 |
| netcfg | V1.0.21 | [netcfg.md](netcfg.md) | 远程修改设备的 IP、掩码与网关，一键恢复网络，设置 WiFi 名称、密码、频段与信道 |
| board | V1.2.4 | [board.md](board.md) | 侧栏「终端」：SSH 终端与自定义指令、SFTP 上传下载 |
| ping | V1.1.2 | [ping.md](ping.md) | 侧栏「网络检测」：长 ping 与网段扫描（IP、设备名、MAC） |

新增模块时在此追加一行（含版本号），并按下面的结构写一份同名文档：

```
# <模块名>（<目录名>）

## 做什么          一两句话说清楚这个模块解决什么问题
## 界面操作        用户视角的操作步骤
## 后端接口        暴露给前端的方法签名与语义
## 实现要点        非显而易见的设计取舍，看代码看不出来的那些
## 已知限制        做不到什么、什么情况下会失败
## 相关文件        前后端文件清单
```

## 版本号

**每个模块自己一个版本号，互不牵连。** 改了 netcfg 只动 netcfg 的号，别的模块不跟着跳——
模块本来就是独立发布、按 profile 选装的，统一编号只会让"这台机器上装的到底是哪一版"
更难说清。

版本声明在模块目录下的 `frontend/src/modules/<模块>/module.ts` 里，和 `name`、
`description` 放在一起。后端不再存一份：两处各写一遍迟早对不上，而对不上的时候没有
任何机制会发现。

工具箱整体另有一个版本，在 `frontend/src/shell/version.ts`。它回答的是"这个 exe 是
哪一版"——按 profile 裁剪之后，两台机器上装的模块可能都不一样，光有模块版本对不上号。

改工具箱版本要同步改 `wails.json` 里的两处：

- `info.productVersion` —— 决定 exe 属性里显示的版本
- `outputfilename` —— 产物文件名，形如 `C2toolsV1.0.1`

加上 `version.ts`，一共三处。没有共同的读取点：一个是打进前端包的 TS 常量，两个是
构建工具读的 JSON，只能手工保持一致。好在后两处挨在同一个文件里。

产物文件名不要在别处再写一遍。`build.ps1` 和 `tools/pickbuild` 的完成提示都是从
`wails.json` 读的 `outputfilename`，改版本时不用管它们。

`build/windows/info.json` 里的 `FileVersion` 是手工补的——Wails 的默认模板漏了它，
而 Windows 读版本信息时字符串表缺这个键会把整张表一起丢掉，表现成「配了版本号但
exe 属性里什么都没有」。别把它删了。

现场看版本：顶部右侧「关于」。弹窗分两组——**工具箱**给名称和版本，**当前模块**给
名称、标识、版本和一句说明。切到别的模块再打开，第二组整组跟着变。

只讲当前模块，不把所有模块铺出来：打开「关于」通常是因为手上这个模块出了问题，
列一堆版本号只会让人去里面找自己那行。

## 参考

项目整体架构、模块接入方式、profile 选装机制见根目录 [README.md](../README.md)，
架构约束见 [openspec/specs/module-independence/spec.md](../openspec/specs/module-independence/spec.md)。
