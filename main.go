package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ROOT_DIR = "./files" // 可浏览和上传的根目录
	ADDR     = ":8080"   // 监听端口
)

/* =========================
   一次性验证码（.code 文件）
   ========================= */

// 从 CODES_DIR（或当前目录）加载 *.code，匹配则删除该文件
func consumeCodeFile(input string) (bool, string) {
	input = strings.TrimSpace(input)
	if input == "" {
		return false, "缺少验证码"
	}

	dir := os.Getenv("CODES_DIR")
	if dir == "" {
		dir = "." // 运行目录（main.go 同目录）
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.code"))
	if err != nil {
		return false, "服务器读取验证码失败"
	}
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		code := strings.TrimSpace(string(b))
		if subtle.ConstantTimeCompare([]byte(code), []byte(input)) == 1 {
			_ = os.Remove(p) // 使用即销毁
			return true, ""
		}
	}
	return false, "验证码无效"
}

/* =============
   临时 token 存储
   ============= */

type tokenEntry struct {
	ExpireAt time.Time
}

var (
	tokens = make(map[string]tokenEntry)
	tMu    sync.Mutex
)

func newToken(ttl time.Duration) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	t := hex.EncodeToString(b)
	tMu.Lock()
	tokens[t] = tokenEntry{ExpireAt: time.Now().Add(ttl)}
	tMu.Unlock()
	return t
}

func consumeToken(t string) bool {
	if t == "" {
		return false
	}
	now := time.Now()
	tMu.Lock()
	defer tMu.Unlock()
	ent, ok := tokens[t]
	if !ok || now.After(ent.ExpireAt) {
		delete(tokens, t)
		return false
	}
	delete(tokens, t) // 一次性
	return true
}

func startTokenJanitor() {
	go func() {
		tk := time.NewTicker(1 * time.Minute)
		for range tk.C {
			now := time.Now()
			tMu.Lock()
			for k, v := range tokens {
				if now.After(v.ExpireAt) {
					delete(tokens, k)
				}
			}
			tMu.Unlock()
		}
	}()
}

/* =========
   工具函数
   ========= */

func safePath(base, rel string) (string, error) {
	clean := filepath.Clean("/" + rel) // 确保是绝对路径（防 ../）
	full := filepath.Join(base, clean)
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rootAbs, _ := filepath.Abs(base)
	if !strings.HasPrefix(fullAbs, rootAbs) {
		return "", errors.New("越权访问")
	}
	return fullAbs, nil
}

func sanitizeName(name string) string {
	return filepath.Base(name)
}

/* =========
   接口实现
   ========= */

// 列目录
func listHandler(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	if dir == "" {
		dir = "/"
	}
	abs, err := safePath(ROOT_DIR, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type Item struct {
		Name  string `json:"name"`
		Type  string `json:"type"`            // "dir" or "file"
		Size  int64  `json:"size,omitempty"`  // bytes
		Mtime string `json:"mtime,omitempty"` // "YYYY-MM-DD HH:MM"
	}
	items := []Item{}
	for _, e := range entries {
		info, _ := e.Info()
		it := Item{
			Name:  e.Name(),
			Type:  "file",
			Mtime: info.ModTime().Format("2006-01-02 15:04"),
		}
		if e.IsDir() {
			it.Type = "dir"
			it.Size = 0
		} else {
			it.Size = info.Size()
		}
		items = append(items, it)
	}

	// 保证 cwd 以 / 开头、以 / 结尾
	cleanDir := "/" + strings.Trim(dir, "/") + "/"
	if cleanDir == "//" {
		cleanDir = "/"
	}

	resp := map[string]any{
		"ok":    true,
		"cwd":   cleanDir,
		"items": items,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

// 下载文件
func downloadHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "缺少 path", http.StatusBadRequest)
		return
	}
	abs, err := safePath(ROOT_DIR, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	filename := filepath.Base(abs)
	mimeType := mime.TypeByExtension(filepath.Ext(filename))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", mimeType)
	http.ServeFile(w, r, abs)
}

// 预检验证码（不接收文件）→ 返回短效 token
// GET /__fs_checkcode?code=...&dir=...
func checkCodeHandler(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	dir := r.URL.Query().Get("dir")

	// 验目录合法性（避免把 token 发给非法 dir）
	if _, err := safePath(ROOT_DIR, dir); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if ok, why := consumeCodeFile(code); !ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": why})
		return
	}

	token := newToken(2 * time.Minute) // 2 分钟有效的一次性 token
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "token": token})
}

// 上传文件（带 token，流式写盘，无大小限制）
// POST /__fs_upload?dir=...&token=...
// multipart/form-data: 只需要字段 file
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	token := r.URL.Query().Get("token")
	if dir == "" {
		http.Error(w, "缺少 dir", http.StatusBadRequest)
		return
	}
	if !consumeToken(token) {
		http.Error(w, "token 无效", http.StatusForbidden)
		return
	}

	absDir, err := safePath(ROOT_DIR, dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// 只解析 multipart 边界；不要用 ParseMultipartForm（避免临时文件阈值）
	ct := r.Header.Get("Content-Type")
	mediatype, params, err := mime.ParseMediaType(ct)
	if err != nil || !strings.HasPrefix(mediatype, "multipart/") {
		http.Error(w, "Content-Type 必须是 multipart/form-data", http.StatusBadRequest)
		return
	}
	mr := multipart.NewReader(r.Body, params["boundary"])

	var savedAs string

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "读取上传数据失败: "+err.Error(), http.StatusBadRequest)
			return
		}

		if part.FormName() != "file" {
			// 丢弃非 file 字段
			_, _ = io.Copy(io.Discard, part)
			_ = part.Close()
			continue
		}

		filename := sanitizeName(part.FileName())
		if filename == "" {
			_ = part.Close()
			http.Error(w, "缺少文件名", http.StatusBadRequest)
			return
		}

		dstPath := filepath.Join(absDir, filename)
		dst, err := os.Create(dstPath)
		if err != nil {
			_ = part.Close()
			http.Error(w, "无法创建目标文件: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 核心：**流式拷贝，无大小限制**
		if _, err := io.Copy(dst, part); err != nil {
			_ = dst.Close()
			_ = part.Close()
			_ = os.Remove(dstPath) // 失败清理半成品
			http.Error(w, "写入失败: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = dst.Close()
		_ = part.Close()

		savedAs = filename
		break // 仅允许 1 个文件
	}

	if savedAs == "" {
		http.Error(w, "未接收到文件字段 file", http.StatusBadRequest)
		return
	}

	resp := map[string]any{
		"ok":       true,
		"saved_as": savedAs,
		"url":      fmt.Sprintf("/__fs_download?path=%s", filepath.ToSlash(filepath.Join(dir, savedAs))),
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(resp)
}

/* ===== main ===== */

func main() {
	// 创建根目录（不存在时）
	if err := os.MkdirAll(ROOT_DIR, 0o755); err != nil {
		log.Fatal(err)
	}

	startTokenJanitor()

	mux := http.NewServeMux()
	// API
	mux.HandleFunc("/__fs_list", listHandler)
	mux.HandleFunc("/__fs_download", downloadHandler)
	mux.HandleFunc("/__fs_checkcode", checkCodeHandler)
	mux.HandleFunc("/__fs_upload", uploadHandler)

	// 静态文件（前端页面放 ./static 目录：index.html/styles.css/app.js）
	mux.Handle("/", http.FileServer(http.Dir("./static")))

	srv := &http.Server{
		Addr:         ADDR,
		Handler:      mux,
		ReadTimeout:  0, // 大文件时可适当放宽；生产建议设置为 1-4 小时
		WriteTimeout: 0,
	}
	fmt.Println("服务启动于", ADDR)
	fmt.Println("根目录:", ROOT_DIR)
	if cd := os.Getenv("CODES_DIR"); cd != "" {
		fmt.Println("验证码目录(CODES_DIR):", cd)
	} else {
		fmt.Println("验证码目录: 当前运行目录（*.code）")
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
