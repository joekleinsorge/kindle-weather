#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse


def utc_timestamp(offset_seconds=0):
    return int(time.time()) + offset_seconds


def rfc3339_utc(offset_seconds=0):
    value = dt.datetime.now(dt.timezone.utc) + dt.timedelta(seconds=offset_seconds)
    return value.replace(microsecond=0).isoformat().replace("+00:00", "Z")


def weather_payload():
    now = utc_timestamp()
    hourly = []
    for hour in range(1, 10):
        hourly.append({
            "dt": now + hour * 3600,
            "temp": 70 + hour,
            "feels_like": 70 + hour,
            "pressure": 1014,
            "humidity": 61,
            "dew_point": 58,
            "uvi": 4,
            "clouds": 3,
            "visibility": 10000,
            "wind_speed": 8,
            "wind_gust": 12,
            "wind_deg": 90,
            "weather": [{
                "id": 800,
                "main": "Clear",
                "description": "clear sky",
                "icon": "01d",
            }],
            "pop": 0.12,
            "rain": {"1h": 0},
        })

    return {
        "timezone": "America/New_York",
        "timezone_offset": -14400,
        "current": {
            "dt": now,
            "sunrise": now - 3600,
            "sunset": now + 8 * 3600,
            "temp": 72.6,
            "feels_like": 73.1,
            "pressure": 1014,
            "humidity": 61,
            "dew_point": 58,
            "uvi": 4,
            "clouds": 3,
            "visibility": 10000,
            "wind_speed": 8,
            "wind_gust": 12,
            "wind_deg": 90,
            "weather": [{
                "id": 800,
                "main": "Clear",
                "description": "clear sky",
                "icon": "01d",
            }],
        },
        "hourly": hourly,
        "daily": [{
            "moonrise": now + 1200,
            "moonset": now + 43200,
            "moon_phase": 0.5,
            "summary": "E2E clear skies",
        }],
    }


def tide_payload():
    today = dt.date.today().isoformat()
    return {
        "predictions": [
            {"t": f"{today} 03:17", "type": "L", "v": "0.1"},
            {"t": f"{today} 09:24", "type": "H", "v": "4.2"},
            {"t": f"{today} 15:41", "type": "L", "v": "0.3"},
            {"t": f"{today} 21:58", "type": "H", "v": "4.0"},
        ],
    }


def launch_payload():
    return {
        "results": [{
            "window_start": rfc3339_utc(3600),
            "window_end": rfc3339_utc(7200),
            "name": "E2E Mission",
            "pad": {
                "name": "LC-39A, Kennedy Space Center",
                "location": {"name": "Kennedy Space Center, FL, USA"},
            },
        }],
    }


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        path = urlparse(self.path).path
        if path == "/health":
            self.write_json({"status": "healthy"})
        elif path == "/weather":
            self.write_json(weather_payload())
        elif path == "/tide":
            self.write_json(tide_payload())
        elif path == "/tide-empty":
            self.write_json({"predictions": []})
        elif path.startswith("/launches/upcoming"):
            self.write_json(launch_payload())
        else:
            self.send_error(404)

    def write_json(self, payload):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, format, *args):
        return


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18080)
    args = parser.parse_args()

    server = ThreadingHTTPServer((args.host, args.port), Handler)
    server.serve_forever()


if __name__ == "__main__":
    main()
