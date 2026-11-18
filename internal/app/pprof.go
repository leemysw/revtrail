// INPUT: 环境变量 REVTRAIL_PPROF（本机地址，如 127.0.0.1:6060）。
// OUTPUT: startProfiling：在该地址开启 net/http/pprof，供开发时分析内存与 CPU。
// POS: 开发诊断入口；不设环境变量时什么也不做。
// PROTOCOL: 变更时更新此头部，然后检查本包 doc.go

package app

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
)

// startProfiling serves Go's profiles on the loopback address in
// REVTRAIL_PPROF, for `go tool pprof http://<addr>/debug/pprof/heap`.
// Other addresses are refused: the profiles show the app's memory.
func startProfiling() {
	addr := os.Getenv("REVTRAIL_PPROF")
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); err != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		log.Printf("REVTRAIL_PPROF: %q is not a loopback address", addr)
		return
	}
	go func() { log.Println("REVTRAIL_PPROF:", http.ListenAndServe(addr, nil)) }()
}
