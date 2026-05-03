# WillumpLabs Gomoku

一个 React + Go 的在线五子棋平台，支持登录、创建房间、加入对战、观战、实时落子和胜负判断。

## 目录结构

```text
.
├── cmd/server/          # Go 服务启动入口
├── internal/gomoku/     # 后端业务、HTTP API、WebSocket、规则测试
├── web/                 # React/Vite 前端应用
├── go.mod
├── package.json         # 根级联动脚本
└── README.md
```

## 本地运行

```bash
npm install --prefix web
go mod tidy
npm run dev
npm run server
```

前端开发服务默认运行在 `http://localhost:5173`，后端 API 默认运行在 `http://localhost:8080`。

生产模式：

```bash
npm run start
```

验证：

```bash
npm run test
```
