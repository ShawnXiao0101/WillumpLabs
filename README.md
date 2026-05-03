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

## GitHub Actions + GHCR + EC2 部署

部署链路：

```text
push main
→ GitHub Actions 测试
→ 构建 Docker 镜像
→ 推送到 GitHub Container Registry
→ SSH 到 EC2
→ docker compose pull + 重启容器
```

### 1. 创建 EC2

推荐从简单稳定的配置开始：

- AMI：Ubuntu Server LTS
- 实例：t3.micro / t4g.micro 均可；如果选 ARM 实例，GitHub Actions 当前镜像构建目标需要再扩展为 multi-platform
- 安全组入站：`22` 只允许你的 IP，`80` 和 `443` 允许 `0.0.0.0/0` 和 `::/0`
- Key pair：保存好 `.pem` 私钥，后面要放进 GitHub Secrets

首次登录：

```bash
ssh -i /path/to/key.pem ubuntu@YOUR_EC2_PUBLIC_IP
```

安装 Docker：

```bash
sudo bash deploy/install-ec2.sh
```

如果你是在本机运行脚本，需要先把脚本传到 EC2；也可以复制脚本内容到服务器执行。

### 2. 配置 GitHub Secrets

在仓库 `Settings → Secrets and variables → Actions → New repository secret` 添加：

```text
EC2_HOST=你的 EC2 公网 IP 或域名
EC2_USER=ubuntu
EC2_SSH_KEY=你的 EC2 私钥完整内容
GHCR_READ_TOKEN=GitHub Personal Access Token，至少需要 read:packages
DOMAIN=willumplabs.com
```

`GHCR_READ_TOKEN` 用于让 EC2 从 GHCR 拉取镜像。创建 token 后，可以在 GitHub 用户设置里的 Developer settings → Personal access tokens 创建。

### 3. 配置域名

在你的域名 DNS 控制台添加：

```text
A    @      YOUR_EC2_PUBLIC_IP
A    www    YOUR_EC2_PUBLIC_IP
```

如果之后给 EC2 绑定 Elastic IP，请把 DNS 指向 Elastic IP，避免实例重启后公网 IP 变化。

### 4. 首次发布

确认默认分支是 `main`，然后 push：

```bash
git add .
git commit -m "Add Docker and EC2 deployment pipeline"
git push origin main
```

GitHub Actions 会自动执行 `.github/workflows/deploy.yml`。部署成功后访问：

```text
https://你的域名/
https://你的域名/healthz
```

### 5. 后续发布

之后每次 push 到 `main` 都会自动：

1. 运行测试和前端构建
2. 构建并推送 `ghcr.io/<owner>/willumplabs-gomoku:<commit-sha>`
3. SSH 到 EC2 拉取新镜像
4. 用 docker compose 重启服务
