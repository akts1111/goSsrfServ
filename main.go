package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"
)

type LogEntry struct {
	ID          int64  `json:"id"`
	Timestamp   string `json:"timestamp"`
	FilenameTS  string `json:"filename_ts"`
	IP          string `json:"ip"`
	RawRequest  string `json:"raw_request"`
	RawResponse string `json:"raw_response"`
}

var (
	accessLogs   []LogEntry
	mutex        sync.RWMutex
	maxLogs      int
	serverDomain string // 追加：サーバーのドメイン保持用
	tmpl         = template.Must(template.New("admin").Funcs(template.FuncMap{
		"base64": func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		},
	}).Parse(htmlTemplate))
)

func main() {
	port := flag.String("p", "3001", "Port to listen on")
	limit := flag.Int("limit", 50, "Maximum number of logs to keep")
	domain := flag.String("d", "", "Domain name (e.g., example.com)") // 追加
	flag.Parse()

	maxLogs = *limit

	// ドメインの設定（未指定なら localhost:port）
	if *domain == "" {
		serverDomain = fmt.Sprintf("localhost:%s", *port)
	} else {
		serverDomain = *domain
	}

	http.HandleFunc("/admin", handleAdmin)
	http.HandleFunc("/admin/clear", handleClear)
	http.HandleFunc("/", handleAll)

	// コンソール表示も動的に変更
	fmt.Printf("==========================================\n")
	fmt.Printf(" SSRF Monitor (Go) Running\n")
	fmt.Printf(" Domain: %s\n", serverDomain)
	fmt.Printf(" Admin URL: http://%s/admin\n", serverDomain)
	fmt.Printf("==========================================\n")

	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func handleAll(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/favicon.ico" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	clientIP := r.Header.Get("X-Forwarded-For")
	if clientIP != "" {
		clientIP = strings.TrimSpace(strings.Split(clientIP, ",")[0])
	} else {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		clientIP = ip
	}

	responseBody := "Active"
	if r.URL.Path == "/log" {
		responseBody = "Logged"
	} else if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "404 Not Found")
		return
	}

	requestDump, _ := httputil.DumpRequest(r, true)
	dateStr := time.Now().UTC().Format(http.TimeFormat)
	rawResponse := fmt.Sprintf("HTTP/1.1 200 OK\nDate: %s\nContent-Type: text/plain; charset=utf-8\nContent-Length: %d\n\n%s",
		dateStr, len(responseBody), responseBody)

	now := time.Now()
	entry := LogEntry{
		ID:          now.UnixNano(),
		Timestamp:   now.Format("2006-01-02 15:04:05"),
		FilenameTS:  now.Format("20060102_150405"),
		IP:          clientIP,
		RawRequest:  string(requestDump),
		RawResponse: rawResponse,
	}

	mutex.Lock()
	accessLogs = append([]LogEntry{entry}, accessLogs...)
	if len(accessLogs) > maxLogs {
		accessLogs = accessLogs[:maxLogs]
	}
	mutex.Unlock()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(responseBody))
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	logsCopy := make([]LogEntry, len(accessLogs))
	copy(logsCopy, accessLogs)
	mutex.RUnlock()

	allLogsJson, _ := json.Marshal(logsCopy)
	allLogsBase64 := base64.StdEncoding.EncodeToString(allLogsJson)

	data := struct {
		Logs          []LogEntry
		AllLogsBase64 string
		Domain        string // テンプレートにドメインを渡す
	}{
		Logs:          logsCopy,
		AllLogsBase64: allLogsBase64,
		Domain:        serverDomain,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	mutex.Lock()
	accessLogs = []LogEntry{}
	mutex.Unlock()
	w.Write([]byte("ok"))
}

const htmlTemplate = `
<!DOCTYPE html>
<html lang="ja">
<head>
    <title>SSRF Monitor - {{.Domain}}</title>
    <meta charset="utf-8">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f0f2f5; padding: 20px; color: #1c1e21; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: #fff; padding: 20px; border-radius: 12px; display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; box-shadow: 0 4px 12px rgba(0,0,0,0.05); }
        .card { background: #fff; border-radius: 12px; margin-bottom: 20px; padding: 20px; box-shadow: 0 2px 8px rgba(0,0,0,0.08); border-left: 6px solid #007bff; }
        .card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px; border-bottom: 1px solid #eee; padding-bottom: 10px; }
        .log-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
        pre { background: #1e1e1e; color: #d4d4d4; padding: 15px; font-size: 13px; overflow-x: auto; white-space: pre-wrap; word-break: break-all; margin: 0; border-radius: 8px; line-height: 1.5; }
        .res-pre { color: #9cdcfe; }
        .label { font-size: 12px; font-weight: bold; color: #65676b; margin-bottom: 8px; text-transform: uppercase; }
        button { padding: 10px 18px; border: none; border-radius: 6px; cursor: pointer; font-weight: 600; transition: opacity 0.2s; }
        button:hover { opacity: 0.8; }
        .btn-green { background: #42b72a; color: white; }
        .btn-blue { background: #1877f2; color: white; }
        .btn-grey { background: #ebedf0; color: #4b4f56; }
        .sub-title { font-size: 14px; color: #65676b; font-weight: normal; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div>
                <h1 style="margin:0; font-size: 24px;">SSRF Monitor</h1>
                <div class="sub-title">Running on: <strong>{{.Domain}}</strong></div>
            </div>
            <div style="display: flex; gap: 10px;">
                <button class="btn-green" onclick="location.reload()">更新</button>
                <button class="btn-blue" onclick="downloadAll()">全ログDL (.json)</button>
                <button class="btn-grey" onclick="confirmClear()">クリア</button>
            </div>
        </div>
        <div>
            {{range .Logs}}
            <div class="card">
                <div class="card-header">
                    <span><strong style="color:#007bff;">[{{.Timestamp}}]</strong> From: {{.IP}}</span>
                    <button style="background:#f0f2f5; border:1px solid #ddd; font-size:12px; padding: 5px 10px;"
                        onclick="downloadSingle('{{base64 (printf "=== REQUEST ===\n%s\n\n=== RESPONSE ===\n%s" .RawRequest .RawResponse)}}', '{{$.Domain}}_{{.FilenameTS}}.txt')">
                        保存
                    </button>
                </div>
                <div class="log-grid">
                    <div><div class="label">Request</div><pre>{{.RawRequest}}</pre></div>
                    <div><div class="label">Response</div><pre class="res-pre">{{.RawResponse}}</pre></div>
                </div>
            </div>
            {{else}}
            <div style="text-align:center; padding: 100px; background: white; border-radius: 12px; color: #999;">
                <h3>リクエスト待機中... ({{.Domain}})</h3>
            </div>
            {{end}}
        </div>
    </div>
    <script>
        function confirmClear() {
            if(confirm("全てのログを削除しますか？")) {
                fetch('/admin/clear').then(() => location.reload());
            }
        }
        function downloadFile(b64, name, type) {
            const bin = atob(b64);
            const buf = new Uint8Array(bin.length);
            for(let i=0; i<bin.length; i++) buf[i] = bin.charCodeAt(i);
            const a = document.createElement("a");
            a.href = URL.createObjectURL(new Blob([buf], {type}));
            a.download = name; a.click();
        }
        function downloadSingle(data, name) { downloadFile(data, name, "text/plain"); }
        function downloadAll() { downloadFile("{{.AllLogsBase64}}", "ssrf_logs_{{.Domain}}.json", "application/json"); }
    </script>
</body>
</html>
`
