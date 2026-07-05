package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/agent-gateway/telemetry-gateway/internal/config"
)

//go:embed static
var staticFiles embed.FS

// Register 注册管理页面静态资源路由和管理 API。
func Register(mux *http.ServeMux, stacks ...config.ObservabilityStack) {
	var stack config.ObservabilityStack
	if len(stacks) > 0 {
		stack = stacks[0]
	}

	mux.HandleFunc("/admin/api/observability", ObservabilityHandler(stack))
	mux.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("/admin/", http.StripPrefix("/admin/", Handler()))
}

// Handler 返回管理页面静态资源处理器。
func Handler() http.Handler {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("加载管理页面静态资源失败: " + err.Error())
	}

	fileServer := http.FileServer(http.FS(staticRoot))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "仅支持 GET 和 HEAD 方法", http.StatusMethodNotAllowed)
			return
		}

		requestPath := cleanRequestPath(r.URL.Path)
		if shouldServeIndex(staticRoot, requestPath) {
			serveIndex(w, r, staticRoot)
			return
		}

		setCacheHeader(w, requestPath)
		fileServer.ServeHTTP(w, r)
	})
}

func cleanRequestPath(rawPath string) string {
	return strings.TrimPrefix(path.Clean("/"+rawPath), "/")
}

func shouldServeIndex(staticRoot fs.FS, requestPath string) bool {
	if requestPath == "" || requestPath == "." {
		return true
	}

	info, err := fs.Stat(staticRoot, requestPath)
	if err != nil {
		return true
	}

	return info.IsDir()
}

func serveIndex(w http.ResponseWriter, r *http.Request, staticRoot fs.FS) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}

	indexHTML, err := fs.ReadFile(staticRoot, "index.html")
	if err != nil {
		http.Error(w, "管理页面不可用", http.StatusInternalServerError)
		return
	}

	_, _ = w.Write(indexHTML)
}

func setCacheHeader(w http.ResponseWriter, requestPath string) {
	if strings.HasSuffix(requestPath, ".html") {
		w.Header().Set("Cache-Control", "no-store")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
}
