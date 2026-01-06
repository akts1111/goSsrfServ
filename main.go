package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"
)

// LogEntry は一つのリクエスト/レスポンスログを保持
type LogEntry struct {
	ID          int64  `json:"id"`
	Timestamp   string `json:"timestamp"`
	FilenameTS  string `json:"filename_ts"`
	IP          string `json:"ip"`
	RawRequest  string `json:"rawRequest"`
	RawResponse string `json:"rawResponse"`
}

var (
	accessLogs []LogEntry
	mutex      sync.RWMutex
	maxLogs    = 50
	// テンプレートの事前コンパイル（効率化）
	tmpl = template.Must(template.New("admin").Funcs(template.FuncMap{
		"base64": func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		},
	}).Parse(htmlTemplate))
)

func main() {
	// 1. ポート番号の外部指定 (-p 3001)
	port := flag.String("p", "3001", "Port to listen on")
	flag.Parse()

	http.HandleFunc("/admin", handleAdmin)
	http.HandleFunc("/admin/clear", handleClear)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// --- リクエストの生データをキャプチャ ---
		// trueを渡すとボディもキャプチャする
		requestDump, _ := httputil.DumpRequest(r, true)

		// レスポンスの決定
		responseBody := "Active"
		if r.URL.Path == "/log" {
			responseBody = "Logged"
		}

		// --- レスポンスの生データを構築 ---
		// 実際に送信されるヘッダーに近い形式で保存
		respHeader := fmt.Sprintf("HTTP/1.1 200 OK\nDate: %s\nContent-Type: text/plain; charset=utf-8\nContent-Length: %d\n\n",
			time.Now().UTC().Format(http.TimeFormat), len(responseBody))
		rawResponse := respHeader + responseBody

		// ログエントリー作成
		now := time.Now()
		entry := LogEntry{
			ID:          now.UnixNano(),
			Timestamp:   now.Format("2006-01-02 15:04:05"),
			FilenameTS:  now.Format("20060102_150405"),
			IP:          r.RemoteAddr,
			RawRequest:  string(requestDump),
			RawResponse: rawResponse,
		}

		// ログ保存
		mutex.Lock()
		accessLogs = append([]LogEntry{entry}, accessLogs...)
		if len(accessLogs) > maxLogs {
			accessLogs = accessLogs[:maxLogs]
		}
		mutex.Unlock()

		// 実際のレスポンス送信
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(responseBody))
	})

	fmt.Printf("Server started at http://localhost:%s/admin\n", *port)
	http.ListenAndServe(":"+*port, nil)
}

// --- ハンドラー関数 ---

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	mutex.RLock()
	// JSONダウンロード用にスライスをコピー
	logsCopy := make([]LogEntry, len(accessLogs))
	copy(logsCopy, accessLogs)
	mutex.RUnlock()

	allLogsJson, _ := json.Marshal(logsCopy)
	allLogsBase64 := base64.StdEncoding.EncodeToString(allLogsJson)

	data := struct {
		Logs          []LogEntry
		AllLogsBase64 string
	}{
		Logs:          logsCopy,
		AllLogsBase64: allLogsBase64,
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

// --- HTMLテンプレート (スタイル微調整) ---
const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>SSRF Monitor (Go)</title>
    <meta charset="utf-8">
    <style>
        body { font-family: sans-serif; background: #f4f7f6; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: white; padding: 20px; border-radius: 8px; display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.05); }
        .card { background: white; border-radius: 8px; margin-bottom: 20px; padding: 15px; box-shadow: 0 2px 5px rgba(0,0,0,0.1); border-left: 5px solid #007bff; }
        .card-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; font-size: 0.9em; color: #555; }
        .log-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 15px; }
        pre { background: #2d2d2d; color: #f8f8f2; padding: 12px; font-size: 12px; overflow-x: auto; white-space: pre-wrap; word-break: break-all; margin: 0; border-radius: 4px; border: 1px solid #111; line-height: 1.4; }
        .res-pre { color: #a6e22e; }
        button { padding: 8px 16px; border: none; border-radius: 4px; cursor: pointer; font-weight: bold; }
        .btn-green { background: #28a745; color: white; }
        .btn-blue { background: #007bff; color: white; }
        .btn-grey { background: #6c757d; color: white; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1 style="margin:0;">SSRF Monitor (Go)</h1>
            <div>
                <button class="btn-green" onclick="location.reload()">更新</button>
                <button class="btn-blue" onclick="downloadAll()">全ログ一括保存 (.json)</button>
                <button class="btn-grey" onclick="fetch('/admin/clear').then(()=>location.reload())">クリア</button>
            </div>
        </div>
        <div>
            {{range .Logs}}
            <div class="card">
                <div class="card-header">
                    <span><strong>[{{.Timestamp}}]</strong> From: {{.IP}}</span>
					<button style="background:#f8f9fa; border:1px solid #ccc; font-size:11px;"
						onclick="downloadSingle('{{base64 (printf "=== REQUEST ===\n%s\n\n=== RESPONSE ===\n%s" .RawRequest .RawResponse)}}', 'log_{{.FilenameTS}}.txt')">
						保存 (.txt)
					</button>
                </div>
                <div class="log-grid">
                    <div>
                        <div style="font-size:11px; color:#666; margin-bottom:4px;">REQUEST</div>
                        <pre>{{.RawRequest}}</pre>
                    </div>
                    <div>
                        <div style="font-size:11px; color:#666; margin-bottom:4px;">RESPONSE</div>
                        <pre class="res-pre">{{.RawResponse}}</pre>
                    </div>
                </div>
            </div>
            {{else}}
            <p style="text-align:center; color:#999; margin-top:50px;">ログ待機中... ({{.Logs | len}} 件)</p>
            {{end}}
        </div>
    </div>

    <script>
        function downloadFile(contentBase64, fileName, contentType) {
            const binary = atob(contentBase64);
            const array = new Uint8Array(binary.length);
            for(let i=0; i<binary.length; i++) array[i] = binary.charCodeAt(i);
            const file = new Blob([array], {type: contentType});
            const a = document.createElement("a");
            a.href = URL.createObjectURL(file);
            a.download = fileName;
            a.click();
        }
        function downloadSingle(base64Data, name) { downloadFile(base64Data, name, "text/plain"); }
        function downloadAll() { downloadFile("{{.AllLogsBase64}}", "ssrf_all_logs.json", "application/json"); }
    </script>
</body>
</html>
`
