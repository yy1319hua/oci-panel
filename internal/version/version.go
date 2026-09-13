package version

// AppVersion 是面板统一版本号，后端接口、Telegram 机器人、前端均引用此处，
// 作为唯一数据源，避免「系统概览 / 设置」与二进制实际版本对不上的问题。
//
// 正式构建时会由 CI 通过 -ldflags 覆盖为 git tag 值：
//
//	go build -ldflags "-X github.com/adiecho/oci-panel/internal/version.AppVersion=$(git describe --tags)" ...
//
// 此处的常量是「未注入时的兜底值」，请与最新 tag 保持一致。
const AppVersion = "1.0.12"
