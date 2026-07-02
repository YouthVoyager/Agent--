package observability

import (
	"net/http"
	"net/http/pprof"
)

// registerPprof 将 pprof 调试路由注册到 HTTP 多路复用器。
func registerPprof(mux *http.ServeMux) {
	// /debug/pprof/ 提供 pprof 调试首页和可用 profile 列表。
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	// /debug/pprof/cmdline 返回当前进程的启动命令行参数。
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	// /debug/pprof/profile 采集 CPU profile，默认采样 30 秒。
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	// /debug/pprof/symbol 将程序计数器地址解析为函数符号。
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	// /debug/pprof/trace 采集 Go 执行追踪数据。
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	// /debug/pprof/goroutine 返回 goroutine 栈信息 profile。
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	// /debug/pprof/heap 返回堆内存分配 profile。
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	// /debug/pprof/threadcreate 返回系统线程创建 profile。
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	// /debug/pprof/block 返回 goroutine 阻塞事件 profile。
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	// /debug/pprof/mutex 返回互斥锁竞争 profile。
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
}
