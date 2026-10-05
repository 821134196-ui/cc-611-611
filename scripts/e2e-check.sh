#!/usr/bin/env bash
# 端到端验证脚本：撤回/过期后原图不可下载、并发审批一致、发表位置可查、CSV 可导出。
# 用法：./scripts/e2e-check.sh
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
PORT="18080"
BASE="http://localhost:${PORT}"
PASS=0; FAIL=0
RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; NC=$'\033[0m'

check() { # desc expected actual
  if [ "$2" = "$3" ]; then echo "${GREEN}✓ $1 (HTTP $3)${NC}"; PASS=$((PASS+1));
  else echo "${RED}✗ $1 — 期望 $2 实际 $3${NC}"; FAIL=$((FAIL+1)); fi
}
body_has() { # desc needle body
  if echo "$3" | grep -qF "$2"; then echo "${GREEN}✓ $1${NC}"; PASS=$((PASS+1));
  else echo "${RED}✗ $1 — 响应中未找到「$2」${NC}"; FAIL=$((FAIL+1)); fi
}

# --- 启动服务（临时数据目录）---
if [ -x "$HOME/go-sdk/go/bin/go" ]; then export PATH="$HOME/go-sdk/go/bin:$PATH"; fi
export GOPROXY="${GOPROXY:-https://goproxy.cn,direct}" GOSUMDB="${GOSUMDB:-sum.golang.google.cn}"
TMPD="$(mktemp -d)"
trap 'kill $SRV_PID 2>/dev/null; rm -rf "$TMPD"' EXIT

# 若端口被占用（如旧实例残留），先清理。
fuser -k "${PORT}/tcp" 2>/dev/null || true
sleep 1

[ -f web/dist/index.html ] || (cd web && npm ci --no-audit --no-fund && npm run build)
go build -o "$TMPD/server" ./cmd/server
PORT=$PORT DB_PATH="$TMPD/data/test.db" STORAGE_DIR="$TMPD/storage" "$TMPD/server" >"$TMPD/server.log" 2>&1 &
SRV_PID=$!

echo -n "等待服务启动"
for i in $(seq 1 40); do
  curl -sf "$BASE/api/health" >/dev/null 2>&1 && break
  echo -n .; sleep 0.5
done
echo

jq_get() { # 用法: echo "$json" | jq_get id —— 从 stdin 取 JSON，$1 为数字字段名
  sed -n "s/.*\"$1\": *\([0-9][0-9]*\).*/\1/p" | head -1
}

login() { # user pass -> token
  curl -s -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
    -d "{\"username\":\"$1\",\"password\":\"$2\"}" | sed -n 's/.*"token": *"\([^"]*\)".*/\1/p'
}

TOK_A="$(login applicant 123456)"
TOK_C="$(login curator 123456)"
[ -n "$TOK_A" ] && { echo "${GREEN}✓ 申请人登录获得令牌${NC}"; PASS=$((PASS+1)); } || { echo "${RED}✗ 申请人登录失败${NC}"; FAIL=$((FAIL+1)); }
[ -n "$TOK_C" ] && { echo "${GREEN}✓ 馆方登录获得令牌${NC}"; PASS=$((PASS+1)); } || { echo "${RED}✗ 馆方登录失败${NC}"; FAIL=$((FAIL+1)); }

ART_ID="$(curl -s "$BASE/api/artifacts" -H "Authorization: Bearer $TOK_A" | jq_get id | head -1)"
echo "${YELLOW}使用藏品 #$ART_ID 进行验证${NC}"

create_app() { # tier -> app id
  local end; end="$(date -u -d '+3 days' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+3d +%Y-%m-%dT%H:%M:%SZ)"
  curl -s -X POST "$BASE/api/applications" -H "Authorization: Bearer $TOK_A" -H 'Content-Type: application/json' \
    -d "{\"artifact_id\":$ART_ID,\"purpose\":\"e2e 刊物配图\",\"channel\":\"纸质刊物\",\"requested_tier\":\"$1\",\"requested_edit\":true,\"end_at\":\"$end\"}" \
    | jq_get id
}

decide() { # appid token approve -> http code
  local appr="$3"
  local body
  if [ "$appr" = "1" ]; then
    local exp; exp="$(date -u -d '+5 days' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+5d +%Y-%m-%dT%H:%M:%SZ)"
    body="{\"approve\":true,\"resolution_tier\":\"$4\",\"allow_edit\":true,\"expires_at\":\"$exp\",\"note\":\"e2e\"}"
  else
    body='{"approve":false,"note":"e2e reject"}'
  fi
  curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/applications/$1/decide" \
    -H "Authorization: Bearer $2" -H 'Content-Type: application/json' -d "$body"
}

# ===== 1. 撤回场景 =====
APP="$(create_app original)"
echo "${YELLOW}— 场景1：授权 #$APP 批准 → 下载 → 登记发表 → 撤回 → 再下载被拒 —${NC}"
check "批准授权" 200 "$(decide "$APP" "$TOK_C" 1 original)"
check "授权期内下载原图" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP/download?resolution=original" -H "Authorization: Bearer $TOK_A")"
check "授权期内下载预览" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP/download?resolution=preview" -H "Authorization: Bearer $TOK_A")"

PUB='{"title":"e2e 测试期刊 2026 第4期 P8","url":"https://e2e.example.com/journal/2026/4/p8","note":"已见刊"}'
check "登记发表位置" 201 "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/applications/$APP/publications" -H "Authorization: Bearer $TOK_A" -H 'Content-Type: application/json' -d "$PUB")"

check "馆方撤回授权" 200 "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/applications/$APP/revoke" -H "Authorization: Bearer $TOK_C" -H 'Content-Type: application/json' -d '{"reason":"e2e 撤回"}')"
check "撤回后原图立即停止" 410 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP/download?resolution=original" -H "Authorization: Bearer $TOK_A")"
check "撤回后预览也停止" 410 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP/download?resolution=preview" -H "Authorization: Bearer $TOK_A")"
check "无凭证旧链接拒绝" 401 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP/download?resolution=original")"
check "重复撤回冲突" 409 "$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/api/applications/$APP/revoke" -H "Authorization: Bearer $TOK_C" -H 'Content-Type: application/json' -d '{}')"
check "历史申请仍可查" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP" -H "Authorization: Bearer $TOK_C")"
PUBBODY="$(curl -s "$BASE/api/publications?keyword=e2e.example.com" -H "Authorization: Bearer $TOK_C")"
body_has "撤回后发表位置仍可查找" "e2e.example.com" "$PUBBODY"
body_has "检索结果标注授权已撤回" "revoked" "$PUBBODY"
CSVBODY="$(curl -s "$BASE/api/publications/export" -H "Authorization: Bearer $TOK_C")"
body_has "CSV 导出含已撤回引用链接" "e2e.example.com" "$CSVBODY"
body_has "CSV 导出含撤回状态" "revoked" "$CSVBODY"
MAILBODY="$(curl -s "$BASE/api/emails" -H "Authorization: Bearer $TOK_A")"
body_has "撤回模拟邮件已发送" "撤回" "$MAILBODY"

# ===== 2. 分级权限：预览档位不得下载原图 =====
APP2="$(create_app preview)"
decide "$APP2" "$TOK_C" 1 preview >/dev/null
echo "${YELLOW}— 场景2：仅预览档位 —${NC}"
check "预览档位下载预览" 200 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP2/download?resolution=preview" -H "Authorization: Bearer $TOK_A")"
check "预览档位下载原图被拒(403)" 403 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP2/download?resolution=original" -H "Authorization: Bearer $TOK_A")"

# ===== 3. 到期场景 =====
APP3="$(create_app original)"
decide "$APP3" "$TOK_C" 1 original >/dev/null
echo "${YELLOW}— 场景3：授权到期（直接改库模拟时间流逝）—${NC}"
DB="$TMPD/data/test.db"
python3 - "$DB" "$APP3" <<'PY'
import sqlite3, sys, datetime
db, appid = sys.argv[1], sys.argv[2]
con = sqlite3.connect(db)
past = (datetime.datetime.utcnow()-datetime.timedelta(minutes=1)).strftime("%Y-%m-%dT%H:%M:%SZ")
con.execute("UPDATE applications SET expires_at=? WHERE id=?", (past, int(appid)))
con.commit()
PY
check "过期后原图旧链接立即失效" 410 "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/api/applications/$APP3/download?resolution=original" -H "Authorization: Bearer $TOK_A")"

# ===== 4. 并发审批一致性（20 轮：批准 vs 驳回）=====
echo "${YELLOW}— 场景4：并发审批（20 轮）—${NC}"
CONFLICT_BAD=0
for i in $(seq 1 20); do
  P="$(create_app preview)"
  exp="$(date -u -d '+5 days' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v+5d +%Y-%m-%dT%H:%M:%SZ)"
  curl -s --max-time 20 -o "$TMPD/r1" -w '%{http_code}' -X POST "$BASE/api/applications/$P/decide" -H "Authorization: Bearer $TOK_C" -H 'Content-Type: application/json' -d "{\"approve\":true,\"resolution_tier\":\"preview\",\"expires_at\":\"$exp\"}" >"$TMPD/c1" &
  local_p1=$!
  curl -s --max-time 20 -o "$TMPD/r2" -w '%{http_code}' -X POST "$BASE/api/applications/$P/decide" -H "Authorization: Bearer $TOK_C" -H 'Content-Type: application/json' -d '{"approve":false,"note":"concurrent reject"}' >"$TMPD/c2" &
  local_p2=$!
  wait "$local_p1" "$local_p2"
  a="$(cat "$TMPD/c1")"; b="$(cat "$TMPD/c2")"
  final="$(curl -s "$BASE/api/applications/$P" -H "Authorization: Bearer $TOK_C")"
  consistent=0
  if [ "$a" = 200 ] && [ "$b" = 409 ] && echo "$final" | grep -q '"effective_status":"approved"'; then consistent=1; fi
  if [ "$b" = 200 ] && [ "$a" = 409 ] && echo "$final" | grep -q '"effective_status":"rejected"'; then consistent=1; fi
  if [ "$consistent" != 1 ]; then
    echo "${RED}✗ 第 $i 轮并发结果矛盾: approve=$a reject=$b${NC}"; CONFLICT_BAD=$((CONFLICT_BAD+1))
  fi
done
[ "$CONFLICT_BAD" = 0 ] && { echo "${GREEN}✓ 20 轮并发审批均恰有一个生效、另一个 409，终态与获胜决定一致${NC}"; PASS=$((PASS+1)); } || FAIL=$((FAIL+1))

# ===== 5. 前端托管 =====
echo "${YELLOW}— 场景5：前端页面托管 —${NC}"
IDX="$(curl -s "$BASE/")"
body_has "根路径返回前端页面" "藏品图片授权" "$IDX"

echo
echo "================================"
echo "${GREEN}通过: $PASS${NC}  ${RED}失败: $FAIL${NC}"
[ "$FAIL" = 0 ]
