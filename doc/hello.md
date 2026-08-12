# Hello World（hello）

当前版本 **V1.0.1**，声明在 `frontend/src/modules/hello/module.ts`。

## 做什么

模块框架的最小示例。它不解决任何真实业务问题，存在的意义是给新模块提供一份
可以照抄的骨架，并验证「前端调用本模块的 Go 方法」这条链路是通的。

不想要它出现在交付产物里时，在 `modules.json` 里换一个不含 `hello` 的 profile
重新构建即可，不用删代码。

## 界面操作

输入一个名字（留空按「世界」处理），点「调用 Go」，下方绿色横幅显示后端返回的字符串。

## 后端接口

`internal/modules/hello` 的 `Service`，前端从 `wailsjs/go/hello/Service` 导入。

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `Greet` | `(name: string) => Promise<string>` | 返回问候语，`name` 为空时用「世界」 |

## 实现要点

这个模块刚好覆盖了一个模块的最小构成，三个文件缺一不可：

- **后端** `hello.go` 实现 `module.Module` 的 `ID()` 和 `Bindings()`。
  `Bindings()` 返回的对象上所有导出方法会被 Wails 自动生成 TypeScript 绑定。
- **前端** `module.ts` 默认导出一份 `ModuleManifest`，声明 id、名称、描述和视图组件。
- **前端** `HelloView.vue` 是界面本身，直接用 `style.css` 里的通用类
  （`card`、`field-row`、`banner` 等），没有自己的样式。

构造函数必须叫 `New()`、目录名必须与 Go 包名一致，否则 `tools/genmodules` 接不上。

## 已知限制

只是示例，没有错误处理、没有加载态、没有单元测试。真实模块请参考 netcfg。

## 相关文件

```
internal/modules/hello/hello.go             模块入口与 Greet 方法
frontend/src/modules/hello/module.ts        模块清单
frontend/src/modules/hello/HelloView.vue    界面
```
