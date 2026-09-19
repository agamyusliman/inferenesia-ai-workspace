#!/usr/bin/env bash
# Inferenesia — one-command full stack (hub :4110 + Vite :4150)
#
# Usage:
#   ./scripts/dev.sh              # start hub + Vite (foreground Vite; hub in bg)
#   ./scripts/dev.sh start        # same
#   ./scripts/dev.sh hub          # hub only (SPA at :4110, no Vite)
#   ./scripts/dev.sh stop         # stop hub + Vite
#   ./scripts/dev.sh status       # ports + health
#   ./scripts/dev.sh build        # rebuild Go binaries + web SPA
#   ./scripts/dev.sh restart      # stop → start
#
# Env (optional):
#   INFERENESIA_HOME / YURA_AI_HOME   config home (default: ~/.inferenesia)
#   HUB_PORT       default 4110 (must be 4100–4199)
#   VITE_PORT      default 4150
#   SKIP_BUILD=1   do not auto-build missing binary/dist
#   OPEN_BROWSER=1 open URL after healthy (macOS open / xdg-open)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

HUB_PORT="${HUB_PORT:-4110}"
VITE_PORT="${VITE_PORT:-4150}"
HUB_BIN="${HUB_BIN:-$ROOT/bin/inferenesia-desktop}"
CLI_BIN="${CLI_BIN:-$ROOT/bin/inferenesia}"
SPA_INDEX="$ROOT/cmd/desktop/frontend/dist/index.html"
RUN_DIR="${YURA_AI_RUN_DIR:-$ROOT/.run}"
HUB_LOG="$RUN_DIR/hub.log"
VITE_LOG="$RUN_DIR/vite.log"
HUB_PID_FILE="$RUN_DIR/hub.pid"
VITE_PID_FILE="$RUN_DIR/vite.pid"

RED=$'\033[0;31m'
GRN=$'\033[0;32m'
YLW=$'\033[0;33m'
CYN=$'\033[0;36m'
DIM=$'\033[2m'
RST=$'\033[0m'

log()  { printf '%s%s%s %s\n' "$CYN" '[inferenesia-dev]' "$RST" "$*"; }
ok()   { printf '%s%s%s %s\n' "$GRN" '[inferenesia-dev]' "$RST" "$*"; }
warn() { printf '%s%s%s %s\n' "$YLW" '[inferenesia-dev]' "$RST" "$*"; }
err()  { printf '%s%s%s %s\n' "$RED" '[inferenesia-dev]' "$RST" "$*" >&2; }

need_port_range() {
  local p="$1"
  if [[ "$p" -lt 4100 || "$p" -gt 4199 ]]; then
    err "port $p outside 4100–4199 (app constraint)"
    exit 1
  fi
}

port_in_use() {
  lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1
}

pid_on_port() {
  lsof -t -nP -iTCP:"$1" -sTCP:LISTEN 2>/dev/null | head -1 || true
}

is_our_hub() {
  local pid="${1:-}"
  [[ -n "$pid" ]] || return 1
  local cmd
  cmd="$(ps -p "$pid" -o args= 2>/dev/null || true)"
  [[ "$cmd" == *inferenesia-desktop* ]] || [[ "$cmd" == *inferenesia* ]] || [[ "$cmd" == *yura-ai-agent-desktop* ]] || [[ "$cmd" == *yura-ai-agent* ]]
}

is_our_vite() {
  local pid="${1:-}"
  [[ -n "$pid" ]] || return 1
  local cmd
  cmd="$(ps -p "$pid" -o args= 2>/dev/null || true)"
  [[ "$cmd" == *vite* ]] && [[ "$cmd" == *4150* || "$cmd" == *"$VITE_PORT"* || "$cmd" == *inferenesia* || "$cmd" == *yura-ai-agent* || "$cmd" == *web* ]]
}

ensure_run_dir() {
  mkdir -p "$RUN_DIR"
}

ensure_env() {
  if [[ ! -f "$ROOT/.env" && -f "$ROOT/.env.example" ]]; then
    warn ".env missing — copying .env.example (fill TEMP_AI_API_KEY for chat)"
    cp "$ROOT/.env.example" "$ROOT/.env"
  fi
}

ensure_web_deps() {
  if [[ ! -d "$ROOT/web/node_modules" ]]; then
    log "npm install (web/)"
    (cd "$ROOT/web" && npm install)
  fi
}

ensure_binaries() {
  if [[ "${SKIP_BUILD:-0}" == "1" ]]; then
    return 0
  fi
  local need=0
  [[ -x "$HUB_BIN" ]] || need=1
  [[ -x "$CLI_BIN" ]] || need=1
  if [[ "$need" -eq 1 ]]; then
    log "building Go binaries…"
    # Package path: ./cmd/inferenesia (CLI) + ./cmd/desktop (Wails+hub); binary names Inferenesia.
    go build -o "$CLI_BIN" ./cmd/inferenesia
    go build -o "$HUB_BIN" ./cmd/desktop
    ok "binaries → bin/"
  fi
}

ensure_spa() {
  if [[ "${SKIP_BUILD:-0}" == "1" ]]; then
    return 0
  fi
  if [[ ! -f "$SPA_INDEX" ]]; then
    log "building web SPA (missing cmd/desktop/frontend/dist)…"
    ensure_web_deps
    (cd "$ROOT/web" && npm run build)
    ok "SPA dist ready"
  fi
}

hub_healthy() {
  curl -sf --max-time 2 "http://127.0.0.1:${HUB_PORT}/api/health" >/dev/null 2>&1
}

wait_hub() {
  local i
  for i in $(seq 1 40); do
    if hub_healthy; then
      return 0
    fi
    sleep 0.25
  done
  return 1
}

cmd_build() {
  ensure_env
  ensure_web_deps
  log "go build CLI + desktop…"
  go build -o "$CLI_BIN" ./cmd/inferenesia
  go build -o "$HUB_BIN" ./cmd/desktop
  log "web build (typecheck + vite + copy-dist)…"
  (cd "$ROOT/web" && npm run build)
  ok "build complete"
}

cmd_stop() {
  ensure_run_dir
  local p

  # Prefer recorded PIDs
  if [[ -f "$HUB_PID_FILE" ]]; then
    p="$(cat "$HUB_PID_FILE" 2>/dev/null || true)"
    if [[ -n "${p:-}" ]] && kill -0 "$p" 2>/dev/null; then
      log "stopping hub pid=$p"
      kill "$p" 2>/dev/null || true
      sleep 0.3
      kill -9 "$p" 2>/dev/null || true
    fi
    rm -f "$HUB_PID_FILE"
  fi
  if [[ -f "$VITE_PID_FILE" ]]; then
    p="$(cat "$VITE_PID_FILE" 2>/dev/null || true)"
    if [[ -n "${p:-}" ]] && kill -0 "$p" 2>/dev/null; then
      log "stopping vite pid=$p"
      kill "$p" 2>/dev/null || true
      sleep 0.3
      kill -9 "$p" 2>/dev/null || true
    fi
    rm -f "$VITE_PID_FILE"
  fi

  # Fallback: free ports if still held by our processes
  p="$(pid_on_port "$HUB_PORT")"
  if [[ -n "$p" ]]; then
    if is_our_hub "$p" || [[ "$(ps -p "$p" -o args= 2>/dev/null || true)" == *inferenesia-desktop* ]] || [[ "$(ps -p "$p" -o args= 2>/dev/null || true)" == *yura-ai-agent-desktop* ]]; then
      log "freeing :$HUB_PORT (pid=$p)"
      kill "$p" 2>/dev/null || true
      sleep 0.2
      kill -9 "$p" 2>/dev/null || true
    else
      warn "port :$HUB_PORT held by pid=$p (not auto-killed — not our hub?)"
    fi
  fi

  p="$(pid_on_port "$VITE_PORT")"
  if [[ -n "$p" ]]; then
    if is_our_vite "$p" || [[ "$(ps -p "$p" -o args= 2>/dev/null || true)" == *vite* ]]; then
      log "freeing :$VITE_PORT (pid=$p)"
      kill "$p" 2>/dev/null || true
      sleep 0.2
      kill -9 "$p" 2>/dev/null || true
    else
      warn "port :$VITE_PORT held by pid=$p (not auto-killed)"
    fi
  fi

  ok "stopped"
}

cmd_status() {
  need_port_range "$HUB_PORT"
  need_port_range "$VITE_PORT"

  printf '%s\n' "── Inferenesia dev status ──"
  printf '  hub  :%s  ' "$HUB_PORT"
  if port_in_use "$HUB_PORT"; then
    if hub_healthy; then
      printf '%sUP + healthy%s  pid=%s\n' "$GRN" "$RST" "$(pid_on_port "$HUB_PORT")"
      curl -sf --max-time 2 "http://127.0.0.1:${HUB_PORT}/api/health" || true
      echo
    else
      printf '%sLISTEN but health FAIL%s  pid=%s\n' "$YLW" "$RST" "$(pid_on_port "$HUB_PORT")"
    fi
  else
    printf '%sDOWN%s\n' "$RED" "$RST"
  fi

  printf '  vite :%s  ' "$VITE_PORT"
  if port_in_use "$VITE_PORT"; then
    printf '%sUP%s  pid=%s\n' "$GRN" "$RST" "$(pid_on_port "$VITE_PORT")"
    if curl -sf --max-time 2 "http://127.0.0.1:${VITE_PORT}/api/health" >/dev/null 2>&1; then
      ok "  Vite → hub proxy OK"
    else
      warn "  Vite up but /api/health via proxy failed (hub down?)"
    fi
  else
    printf '%sDOWN%s\n' "$DIM" "$RST"
  fi

  printf '  binary  %s\n' "$( [[ -x "$HUB_BIN" ]] && echo OK || echo MISSING )"
  printf '  SPA     %s\n' "$( [[ -f "$SPA_INDEX" ]] && echo OK || echo MISSING )"
  printf '  .env    %s\n' "$( [[ -f "$ROOT/.env" ]] && echo present || echo missing )"
  printf '  home    %s\n' "${INFERENESIA_HOME:-${YURA_AI_HOME:-$HOME/.inferenesia}}"
  if [[ -f "$HUB_LOG" ]]; then
    printf '  hub log %s\n' "$HUB_LOG"
  fi
}

start_hub() {
  need_port_range "$HUB_PORT"
  ensure_run_dir
  ensure_env
  ensure_binaries
  ensure_spa

  if hub_healthy; then
    ok "hub already healthy on :$HUB_PORT"
    return 0
  fi

  if port_in_use "$HUB_PORT"; then
    err "port :$HUB_PORT in use but /api/health failed"
    err "  lsof: $(lsof -nP -iTCP:"$HUB_PORT" -sTCP:LISTEN 2>/dev/null || true)"
    err "  fix: ./scripts/dev.sh stop   # or free the port"
    exit 1
  fi

  if [[ ! -x "$HUB_BIN" ]]; then
    err "missing $HUB_BIN — run: ./scripts/dev.sh build"
    exit 1
  fi

  log "starting hub :$HUB_PORT (config: ${INFERENESIA_HOME:-${YURA_AI_HOME:-$HOME/.inferenesia}})"
  # Load repo .env into this process so child inherits keys (desktop also LoadDotEnvFromCWD)
  set -a
  # shellcheck disable=SC1091
  [[ -f "$ROOT/.env" ]] && . "$ROOT/.env" || true
  set +a

  nohup "$HUB_BIN" --serve --port "$HUB_PORT" >"$HUB_LOG" 2>&1 &
  echo $! >"$HUB_PID_FILE"
  log "hub pid=$(cat "$HUB_PID_FILE")  log=$HUB_LOG"

  if ! wait_hub; then
    err "hub did not become healthy — last log lines:"
    tail -n 40 "$HUB_LOG" 2>/dev/null || true
    exit 1
  fi
  ok "hub ready  http://127.0.0.1:${HUB_PORT}"
  ok "health     http://127.0.0.1:${HUB_PORT}/api/health"
}

start_vite_fg() {
  need_port_range "$VITE_PORT"
  ensure_web_deps

  if port_in_use "$VITE_PORT"; then
    warn "Vite already on :$VITE_PORT — reusing"
    ok "UI  http://127.0.0.1:${VITE_PORT}  (proxies /api → :$HUB_PORT)"
    if [[ "${OPEN_BROWSER:-0}" == "1" ]]; then
      open_url "http://127.0.0.1:${VITE_PORT}"
    fi
    log "hub stays running in background. Stop all: ./scripts/dev.sh stop"
    # Block until Ctrl+C so script "stays" like a normal dev server
    trap 'log "leaving hub running (use ./scripts/dev.sh stop to kill hub+vite)"; exit 0' INT TERM
    while port_in_use "$VITE_PORT"; do sleep 2; done
    return 0
  fi

  log "starting Vite :$VITE_PORT (Ctrl+C stops Vite; hub stays up unless you ./scripts/dev.sh stop)"
  ok "UI  http://127.0.0.1:${VITE_PORT}"
  ok "hub http://127.0.0.1:${HUB_PORT}"

  if [[ "${OPEN_BROWSER:-0}" == "1" ]]; then
    (sleep 1.2 && open_url "http://127.0.0.1:${VITE_PORT}") &
  fi

  # Clean Vite on exit of this shell; leave hub up (common dev expectation).
  # Use stop to kill both.
  trap 'log "Vite stopped. Hub still on :$HUB_PORT — ./scripts/dev.sh stop to kill hub."' EXIT

  cd "$ROOT/web"
  # package.json locks --port 4150 --strictPort; honor VITE_PORT via npm if default
  if [[ "$VITE_PORT" == "4150" ]]; then
    exec npm run dev
  else
    exec npx vite --port "$VITE_PORT" --strictPort --host 127.0.0.1
  fi
}

start_vite_bg() {
  need_port_range "$VITE_PORT"
  ensure_run_dir
  ensure_web_deps

  if port_in_use "$VITE_PORT"; then
    ok "Vite already on :$VITE_PORT"
    return 0
  fi

  log "starting Vite :$VITE_PORT (background)"
  (
    cd "$ROOT/web"
    if [[ "$VITE_PORT" == "4150" ]]; then
      npm run dev
    else
      npx vite --port "$VITE_PORT" --strictPort --host 127.0.0.1
    fi
  ) >"$VITE_LOG" 2>&1 &
  echo $! >"$VITE_PID_FILE"
  sleep 0.8
  if port_in_use "$VITE_PORT"; then
    ok "Vite ready  http://127.0.0.1:${VITE_PORT}  log=$VITE_LOG"
  else
    warn "Vite may still be starting — see $VITE_LOG"
  fi
}

open_url() {
  local url="$1"
  if command -v open >/dev/null 2>&1; then
    open "$url" 2>/dev/null || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" 2>/dev/null || true
  fi
}

cmd_start() {
  # Default: hub bg + vite fg
  start_hub
  start_vite_fg
}

cmd_hub_only() {
  start_hub
  ok "SPA (embedded)  http://127.0.0.1:${HUB_PORT}"
  if [[ "${OPEN_BROWSER:-0}" == "1" ]]; then
    open_url "http://127.0.0.1:${HUB_PORT}"
  fi
  log "hub running in background. Logs: $HUB_LOG"
  log "stop: ./scripts/dev.sh stop"
}

cmd_start_bg() {
  start_hub
  start_vite_bg
  ok "full stack (background)"
  ok "  UI  http://127.0.0.1:${VITE_PORT}"
  ok "  hub http://127.0.0.1:${HUB_PORT}"
  log "stop: ./scripts/dev.sh stop"
  log "logs: $HUB_LOG  $VITE_LOG"
  if [[ "${OPEN_BROWSER:-0}" == "1" ]]; then
    open_url "http://127.0.0.1:${VITE_PORT}"
  fi
}

cmd_restart() {
  cmd_stop
  sleep 0.4
  cmd_start
}

usage() {
  cat <<EOF
${CYN}Inferenesia — scripts/dev.sh${RST}

${DIM}One script to run hub (:${HUB_PORT}) + Vite (:${VITE_PORT}).${RST}

  ${GRN}./scripts/dev.sh${RST}           start hub + Vite (Vite foreground)
  ${GRN}./scripts/dev.sh start${RST}     same
  ${GRN}./scripts/dev.sh start-bg${RST}  hub + Vite both background
  ${GRN}./scripts/dev.sh hub${RST}       hub only (http://127.0.0.1:${HUB_PORT})
  ${GRN}./scripts/dev.sh stop${RST}      stop hub + Vite
  ${GRN}./scripts/dev.sh status${RST}    ports + health
  ${GRN}./scripts/dev.sh build${RST}     rebuild binaries + web SPA
  ${GRN}./scripts/dev.sh restart${RST}   stop then start

Env:
  INFERENESIA_HOME=…  config home (default ~/.inferenesia; legacy YURA_AI_HOME ok)
  HUB_PORT=4110      VITE_PORT=4150   (must be 4100–4199)
  SKIP_BUILD=1       skip auto build of missing binary/dist
  OPEN_BROWSER=1     open UI after start

Examples:
  ./scripts/dev.sh
  OPEN_BROWSER=1 ./scripts/dev.sh
  INFERENESIA_HOME=/tmp/inferenesia-smoke ./scripts/dev.sh hub
  ./scripts/dev.sh stop
EOF
}

main() {
  local cmd="${1:-start}"
  case "$cmd" in
    start|"")   cmd_start ;;
    start-bg|bg) cmd_start_bg ;;
    hub|serve)  cmd_hub_only ;;
    stop|down)  cmd_stop ;;
    status|st)  cmd_status ;;
    build)      cmd_build ;;
    restart)    cmd_restart ;;
    -h|--help|help) usage ;;
    *)
      err "unknown command: $cmd"
      usage
      exit 1
      ;;
  esac
}

main "$@"
