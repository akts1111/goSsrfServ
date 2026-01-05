package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sync"
	"time"
)

type LogEntry struct {
	ID          int64  `json:"id"`
	Timestamp   string `json:"timestamp"`
	IP          string `json:"ip"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	RawRequest  string `json:"rawRequest"`
	RawResponse string `json:"rawResponse"`
}

var (
	accessLogs []LogEntry
	mutex      sync.Mutex
	maxLogs    = 50
)

func buildRawRequest(r *http.Request, body []byte) string {
	raw := fmt.Sprintf("%s %s %s\n", r.Method, r.URL.RequestURI(), r.Proto)
	for name, values := range r.Header {
		for _, value := range values {
			raw += fmt.Sprintf("%s: %s\n", name, value)
		}
	}
	raw += "\n"
	if len(body) > 0 {
		raw += string(body)
	}
	return raw
}

func main() {
	http.HandleFunc("/admin", handleAdmin)
	http.HandleFunc("/admin/clear", handleClear)
	
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin" || r.URL.Path == "/admin/clear" {
			return
		}
		body, _ := io.ReadAll(r.Body)
		entry := LogEntry{
			ID:         time.Now().UnixNano(),
			Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
			IP:         r.RemoteAddr,
			Method:     r.Method,
			URL:        r.URL.String(),
			RawRequest: buildRawRequest(r, body),
			RawResponse: "HTTP/1.1 200 OK\nDate: " + time.Now().UTC().Format(http.TimeFormat) + "\nContent-Type: text/plain\n\nLogged",
		}
		mutex.Lock()
		accessLogs = append([]LogEntry{entry}, accessLogs...)
		if len(accessLogs) > maxLogs {
			accessLogs = accessLogs[:maxLogs]
		}
		mutex.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Logged"))
	})

	fmt.Println("URL: http://localhost:3001/admin")
	http.ListenAndServe(":3001", nil)
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	mutex.Lock()
	// 全ログをJSON化してBase64にする（一括保存ボタン用）
	allLogsJson, _ := json.Marshal(accessLogs)
	allLogsBase64 := base64.StdEncoding.EncodeToString(allLogsJson)
	
	// テンプレートに渡すデータ
	data := struct {
		Logs          []LogEntry
		AllLogsBase64 string
	}{
		Logs:          accessLogs,
		AllLogsBase64: allLogsBase64,
	}
	mutex.Unlock()

	t := template.Must(template.New("admin").Funcs(template.FuncMap{
		"base64": func(s string) string {
			return base64.StdEncoding.EncodeToString([]byte(s))
		},
	}).Parse(htmlTemplate))
	t.Execute(w, data)
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	mutex.Lock()
	accessLogs = []LogEntry{}
	mutex.Unlock()
	w.Write([]byte("ok"))
}

// 元のHTMLレイアウトをGoのテンプレート構文に移植
const htmlTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>SSRF Monitor (Go)</title>
    <meta charset="utf-8">
</head>
<body style="font-family:sans-serif; background:#f4f7f6; padding:20px;">
    <div style="max-width:1100px; margin:0 auto;">
        <div style="background:white; padding:20px; border-radius:8px; display:flex; justify-content:space-between; align-items:center; margin-bottom:20px;">
            <h1 style="margin:0;">SSRF Monitor (Go)</h1>
            <div>
                <button style="padding:10px 20px; background:#28a745; color:white; border:none; border-radius:4px; cursor:pointer;" onclick="location.reload()">更新</button>
                <button style="padding:10px 20px; background:#007bff; color:white; border:none; border-radius:4px; cursor:pointer;" onclick="downloadAll()">全ログ一括保存 (.json)</button>
                <button style="padding:10px 20px; background:#6c757d; color:white; border:none; border-radius:4px; cursor:pointer;" onclick="fetch('/admin/clear').then(()=>location.reload())">クリア</button>
            </div>
        </div>
        <div>
            {{range .Logs}}
            <div style="background:white; border-radius:8px; margin-bottom:20px; padding:15px; box-shadow:0 2px 5px rgba(0,0,0,0.1); border-left:5px solid #007bff;">
                <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:10px;">
                    <span><strong>[{{.Timestamp}}]</strong> {{.Method}} {{.URL}}</span>
                    <button 
                        style="padding:5px 10px; background:#f8f9fa; border:1px solid #ccc; cursor:pointer;"
                        onclick="downloadSingle('{{base64 (printf "=== REQUEST ===\n%s\n\n=== RESPONSE ===\n%s" .RawRequest .RawResponse)}}', 'log_{{.Timestamp}}.txt')">
                        このログを保存 (.txt)
                    </button>
                </div>
                <div style="display:grid; grid-template-columns: 1fr 1fr; gap:10px;">
                    <pre style="background:#2d2d2d; color:#ccc; padding:10px; font-size:11px; overflow-x:auto; white-space:pre-wrap; margin:0;">{{.RawRequest}}</pre>
                    <pre style="background:#2d2d2d; color:#85adad; padding:10px; font-size:11px; overflow-x:auto; white-space:pre-wrap; margin:0;">{{.RawResponse}}</pre>
                </div>
            </div>
            {{else}}
            <p style="text-align:center; color:#999;">ログ待機中...</p>
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
        function downloadAll() { downloadFile("{{$.AllLogsBase64}}", "ssrf_all_logs.json", "application/json"); }
    </script>
</body>
</html>
`