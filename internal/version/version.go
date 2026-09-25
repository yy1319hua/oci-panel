package version

// AppVersion 是面板统一版本号，后端接口、Telegram 机器人、前端均引用此处，
// 作为唯一数据源，避免「系统概览 / 设置」与二进制实际版本对不上的问题。
//
// 正式构建时会由 CI 通过 -ldflags 覆盖为 git tag 值：
//
//	go build -ldflags "-X github.com/adiecho/oci-panel/internal/version.AppVersion=1.0.29" ...
//
// ⚠️ 必须是 var，不能写成 const —— `-X` 只能改写「字符串变量」的初始值，
// 对 const 完全无效（Go 会把它内联到每个使用点，不生成可被 linker 覆盖的符号）。
// 曾经这里就是 const，结果 CI 的注入静默失效、面板版本号常年卡在旧值上查不出原因。
//
// 存储的值是**不带 v 前缀**的纯数字版本号（如 1.0.28）：
// tag 是 v1.0.28，CI 注入时用 ${GITHUB_REF_NAME#v} 去掉前缀，
// 前端展示时自己拼 v（`OCI Panel · v{{ version }}`）。
//
// 此处的字面量是「未注入时的兜底值」，请与最新 tag 保持一致。
var AppVersion = "1.0.30"
