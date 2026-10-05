#!/usr/bin/env bash
# 一条命令启动：构建前端 → 编译 Go（嵌入前端）→ 启动服务。
# 用法：./run.sh            （默认 8080 端口）
#       PORT=9000 ./run.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT"

PORT="${PORT:-8080}"

# 兼容 Go 安装在用户目录（如本项目的沙箱环境）的情况。
if [ -x "$HOME/go-sdk/go/bin/go" ]; then
  export PATH="$HOME/go-sdk/go/bin:$PATH"
fi
if [ -x "$HOME/go/bin/go" ] && ! command -v go >/dev/null 2>&1; then
  export PATH="$HOME/go/bin:$PATH"
fi

command -v go >/dev/null 2>&1 || { echo "✗ 未找到 Go（需要 Go 1.23+）"; exit 1; }

# 国内网络可使用国内模块代理（已配置则不覆盖）。
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}"
export GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"

# 1) 前端构建（dist 已存在且不旧则跳过；设置 FORCE_WEB=1 可强制重建）。
if [ ! -f web/dist/index.html ] || [ "${FORCE_WEB:-0}" = "1" ]; then
  echo "▶ 构建前端 (Vue + Vite)…"
  if [ ! -d web/node_modules ]; then
    (cd web && npm install --no-audit --no-fund)
  fi
  (cd web && npm run build)
else
  echo "✓ 前端构建产物已存在 (web/dist)，跳过构建；FORCE_WEB=1 可强制重建"
fi

# 2) 编译（前端通过 go:embed 打进单一二进制）。
echo "▶ 编译后端 (Go + Gin)…"
mkdir -p bin
go build -o bin/museum-server ./cmd/server

# 3) 启动。
echo "▶ 启动服务: http://localhost:${PORT}"
echo "  演示账号: curator/123456 (馆方)  applicant/123456 (申请人)"
PORT="$PORT" exec ./bin/museum-server
