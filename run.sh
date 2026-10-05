#!/usr/bin/env bash
# 一条命令启动：构建前端 -> 启动 Go 服务（同时托管前端与 API）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT="${PORT:-8080}"
export DATA_DIR="${DATA_DIR:-$ROOT/backend/data}"

# 1. 依赖检查（兼容常见的用户态 Go 安装位置）
for d in "$HOME/sdk/go/bin" "$HOME/.local/go/bin" /usr/local/go/bin; do
  [ -x "$d/go" ] && PATH="$d:$PATH"
done
export PATH
need() { command -v "$1" >/dev/null 2>&1 || { echo "缺少依赖：$1，请先安装。"; exit 1; }; }
need go
need node
need npm

# 2. 前端（首次或产物缺失时安装依赖并构建）
if [ ! -f "$ROOT/frontend/dist/index.html" ]; then
  echo "==> 首次启动，构建前端…"
  (cd "$ROOT/frontend" && npm install --no-audit --no-fund && npm run build)
else
  echo "==> 前端已构建（如修改了前端代码，请 cd frontend && npm run build 或删除 dist 后重新运行本脚本）"
fi

# 3. 后端（go.mod 声明的工具链版本缺失时 go 命令会自动下载）
echo "==> 启动服务： http://localhost:$PORT"
echo "    演示账号： curator(馆方) / alice、bob(申请人)"
cd "$ROOT/backend"
exec go run ./cmd/server
