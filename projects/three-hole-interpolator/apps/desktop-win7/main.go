package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"three-hole-interpolator/apps/desktop-win7/backend"
)

//go:embed all:frontend/dist
var frontendAssets embed.FS

func main() {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)

	mux := http.NewServeMux()
	backend.RegisterRoutes(mux)

	subFS, err := fs.Sub(frontendAssets, "frontend/dist")
	if err != nil {
		log.Fatalf("加载前端资源失败: %v", err)
	}
	spa := newSPAHandler(http.FS(subFS))
	mux.Handle("/", spa)

	log.Printf("========================================")
	log.Printf("  三孔探针插值计算 (Win7版)")
	log.Printf("  服务已启动: %s", addr)
	log.Printf("  请在浏览器中使用本系统")
	log.Printf("  关闭此窗口将停止服务")
	log.Printf("========================================")

	go openBrowser(addr)

	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("服务运行失败: %v", err)
	}
}

type spaHandler struct {
	fileServer http.Handler
	fileSys    http.FileSystem
}

func newSPAHandler(fsys http.FileSystem) *spaHandler {
	return &spaHandler{
		fileServer: http.FileServer(fsys),
		fileSys:    fsys,
	}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "" {
		path = "/"
	}

	f, err := h.fileSys.Open(path)
	if err == nil {
		f.Close()
		h.fileServer.ServeHTTP(w, r)
		return
	}

	r.URL.Path = "/"
	h.fileServer.ServeHTTP(w, r)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("打开浏览器失败: %v", err)
	}
}
