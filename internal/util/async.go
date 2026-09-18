package util

import (
	"log"
	"runtime/debug"
)

// Go 启动一个带 panic 恢复的后台 goroutine。
//
// 项目中存在大量 fire-and-forget 的后台任务（缓存刷新、自动救援等）。
// 此前这些 goroutine 一旦 panic 会直接导致整个进程崩溃。Go 用 recover 兜底，
// 把 panic 转成日志（含堆栈），保证单个后台任务出错不影响主服务。
//
// name 仅用于日志定位，便于排查是哪个后台任务出错。
func Go(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[panic recovered] background goroutine %q: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}
