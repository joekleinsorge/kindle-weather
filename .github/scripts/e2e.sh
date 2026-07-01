#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMPDIR="$(mktemp -d)"
MOCK_PORT="${MOCK_PORT:-18080}"
APP_URL="http://127.0.0.1:8080"
MOCK_URL="http://127.0.0.1:${MOCK_PORT}"

MOCK_PID=""
APP_PID=""

cleanup() {
  if [[ -n "${APP_PID}" ]]; then
    kill "${APP_PID}" >/dev/null 2>&1 || true
    wait "${APP_PID}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${MOCK_PID}" ]]; then
    kill "${MOCK_PID}" >/dev/null 2>&1 || true
    wait "${MOCK_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${TMPDIR}"
}
trap cleanup EXIT

wait_for_http() {
  local url="$1"
  local name="$2"
  for _ in {1..60}; do
    if curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done

  echo "Timed out waiting for ${name} at ${url}" >&2
  if [[ -f "${TMPDIR}/app.log" ]]; then
    echo "--- app.log ---" >&2
    cat "${TMPDIR}/app.log" >&2
  fi
  if [[ -f "${TMPDIR}/mock.log" ]]; then
    echo "--- mock.log ---" >&2
    cat "${TMPDIR}/mock.log" >&2
  fi
  return 1
}

wait_for_http_down() {
  local url="$1"
  local name="$2"
  for _ in {1..60}; do
    if ! curl -fsS "${url}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.25
  done

  echo "Timed out waiting for ${name} to stop at ${url}" >&2
  return 1
}

assert_contains() {
  local file="$1"
  local expected="$2"
  if ! grep -Fq "${expected}" "${file}"; then
    echo "Expected ${file} to contain: ${expected}" >&2
    echo "--- ${file} ---" >&2
    cat "${file}" >&2
    return 1
  fi
}

stop_app() {
  if [[ -n "${APP_PID}" ]]; then
    kill -INT "${APP_PID}" >/dev/null 2>&1 || true
    wait "${APP_PID}" >/dev/null 2>&1 || true
    APP_PID=""
    wait_for_http_down "${APP_URL}/health" "kindle-weather"
  fi
}

start_app() {
  local tide_path="$1"
  : > "${TMPDIR}/app.log"
  (
    cd "${ROOT}"
    export OPENWEATHER_API_KEY=e2e
    export WEATHER_API_URL="${MOCK_URL}/weather"
    export NOAA_API_URL="${MOCK_URL}${tide_path}"
    export SPACEDEVS_API_URL="${MOCK_URL}/launches/upcoming/"
    export AUTO_REFRESH_SECONDS=60
    export LAUNCH_API_TIMEOUT_SECONDS=5
    exec "${TMPDIR}/kindle-weather"
  ) > "${TMPDIR}/app.log" 2>&1 &
  APP_PID="$!"
  wait_for_http "${APP_URL}/health" "kindle-weather"
}

(
  cd "${ROOT}"
  go build -o "${TMPDIR}/kindle-weather" .
)

python3 "${ROOT}/.github/scripts/mock_api.py" --host 127.0.0.1 --port "${MOCK_PORT}" > "${TMPDIR}/mock.log" 2>&1 &
MOCK_PID="$!"
wait_for_http "${MOCK_URL}/health" "mock api"

start_app "/tide"
curl -fsS "${APP_URL}/" > "${TMPDIR}/page.html"
curl -fsS "${APP_URL}/css/kindle.css" > "${TMPDIR}/kindle.css"
curl -fsS "${APP_URL}/metrics" > "${TMPDIR}/metrics.txt"
assert_contains "${TMPDIR}/page.html" "Weather & Tide"
assert_contains "${TMPDIR}/page.html" "E2E clear skies"
assert_contains "${TMPDIR}/page.html" "3:17 AM"
assert_contains "${TMPDIR}/page.html" "9:24 AM"
assert_contains "${TMPDIR}/page.html" "id=\"launches\""
assert_contains "${TMPDIR}/kindle.css" ".tide-section"
assert_contains "${TMPDIR}/metrics.txt" "http_requests_total"
stop_app

start_app "/tide-empty"
curl -fsS "${APP_URL}/" > "${TMPDIR}/page-no-tide.html"
assert_contains "${TMPDIR}/page-no-tide.html" "Weather & Tide"
assert_contains "${TMPDIR}/page-no-tide.html" "E2E clear skies"
assert_contains "${TMPDIR}/page-no-tide.html" "Tide data unavailable"
stop_app

echo "E2E checks passed"
