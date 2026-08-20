/**
 * 工具箱的名字。侧栏标题和「关于」都用它，别再各写一份。
 *
 * 和 wails.json 的 info.productName（决定 exe 属性里的产品名）是两处，
 * 改一个记得改另一个。
 */
export const APP_NAME = 'Estun Codroid 机器人工具箱'

/**
 * 整个工具箱的版本，和各模块的版本相互独立。
 *
 * 模块版本回答"这个功能是哪一版"，这个值回答"这个 exe 是哪一版"——按 profile
 * 裁剪之后，两台机器上装的模块可能都不一样，光有模块版本对不上号。
 *
 * 改这里要同步改 wails.json 里的两处：info.productVersion（exe 属性里显示的版本）
 * 和 outputfilename（产物文件名，形如 C2toolsV1.0.4）。三处只能手工保持一致：
 * 这里是打进前端包的 TS 常量，那两处是构建工具读的 JSON，没有共同的读取点。
 */
export const APP_VERSION = 'V1.0.10'
