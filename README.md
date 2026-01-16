## 起動方法
- ローカルで実行する場合（デフォルト）
```
go run main.go -p 3001
# コンソール出力: Admin URL: http://localhost:3001/admin
```

- 公開サーバーでドメインを指定して実行する場合
```
go run main.go -p 80 -d monitor.example.com
# コンソール出力: Admin URL: http://monitor.example.com/admin
```
