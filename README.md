# Kindle Weather

A web application that displays weather, tide, moon phase, and space launch information in a Kindle-friendly format.

## Features

- Large-type overview readable from across the room: current temperature and
  feels-like, condition and daily high/low; when rain starts (or stops) with
  time-labeled hourly rain-chance bars; the
  next high or low tide with today's tide curve, and a good-surf or super-low-tide
  band when one applies
- Tide predictions
- Moon phase display
- Sunrise and sunset times
- Upcoming space launches
- Conditional beach notices for good surf and upcoming daytime super-low tides
- Best remaining surf window with wave height and period
- Actionable rain, UV, wind, temperature, and weather-alert notices (small
  craft advisories are omitted)
- Internal source-freshness metrics for stale-data monitoring
- Adaptive refresh timing for overnight hours and approaching events
- Static SVG dashboard endpoint for screensaver integrations
- Simple design optimized for Kindle displays
- Scrollable surf, tide/beach, and hourly rain details with large section links
- Caching for API responses to reduce calls
- OpenTelemetry tracing (OTLP exporter)

## OpenTelemetry

Tracing is enabled when either `OTEL_EXPORTER_OTLP_ENDPOINT` or
`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set.

Optional environment variables:
- `OTEL_SERVICE_NAME` (default: `kindle-weather`)

Example:
```bash
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 \
OTEL_SERVICE_NAME=kindle-weather \
./kindle-weather
```

The Docker Compose stack includes an OpenTelemetry Collector and Jaeger for
local tracing. Start it with `docker compose up --build`, then open Jaeger at
`http://localhost:16686`.

## Runtime Configuration

Optional environment variables:
- `AUTO_REFRESH_SECONDS` (default: `1800`)
- `CACHE_EXPIRATION` (weather cache, default: `3600`)
- `TIDE_CACHE_EXPIRATION` (default: `1800`)
- `LAUNCH_CACHE_EXPIRATION` (default: `900`)
- `LAUNCH_API_TIMEOUT_SECONDS` (default: `2`)
- `SURF_API_URL` (defaults to the Open-Meteo Marine API for Crescent Beach)
- `SURF_CACHE_EXPIRATION` (default: `1800`)
- `ENABLE_ROCKET_PREVIEW` (default: disabled)
- `WEATHER_API_URL` (complete OpenWeather-compatible URL; primarily for testing)
- `NOAA_API_URL` (defaults to Crescent Beach station 8720218)
- `SPACEDEVS_API_URL` (defaults to Kennedy Space Center launches)

The surf notice uses wave height, period, and direction from Open-Meteo,
combined with the existing OpenWeather wind forecast. It appears only when a
remaining daylight hour is expected to have 1.5–6 ft waves at 7 seconds or
longer, an easterly swell direction, and light or offshore wind. Surf forecast
data is provided by [Open-Meteo](https://open-meteo.com/en/docs/marine-weather-api).

The tide notice appears for the next low tide at or below 0 ft when it is still
upcoming and falls between 7:00 AM and 7:00 PM. It replaces the surf notice when
both conditions apply.

## Browsing forecast details

Scroll down from the overview to see surf conditions, tides/beach, and rain
details, tap a rain/tide/beach panel, or use the large Overview / Surf / Tides /
Rain links. These are ordinary
HTML links and tables, designed for the original Paperwhite browser without
JavaScript or custom swipe gestures. When a section link is used, automatic
refresh returns to that section. Scrolling alone does not change the selected
section; refresh returns to the last section selected by a link (the overview by
default).

Surf details show up to 12 upcoming forecast hours of offshore wave height,
period, direction, and matching wind forecasts. Tide details show today's NOAA
high/low predictions and heights relative to MLLW. Rain details show up to 12
upcoming hours of precipitation probability and hourly amounts in inches.
The current forecast hour is included. Unavailable optional sources show an
explicit message, and cached fallback data is labeled with its last update time.
The overview includes the weather update time, marks extrapolated tide times,
and prioritizes weather warnings and wind over routine UV advice.
The rain band is solid black only when rain is likely; dry and possible-rain
forecasts use an outlined band. When weather data comes from the cache after a
failed refresh, a black status bar names its age and the current temperature is
greyed with an "as of" time. Type scales up on taller screens to fill the
overview. In the surf table, the best surf window's hours are shaded.

## Build

1. Install Go 1.26.6 or later
2. Clone the repository
3. Export `OPENWEATHER_API_KEY`, or create a `secrets/openweather-api-key`
   file containing only the OpenWeather API key
4. Build the application:
   ```bash
   go build -o kindle-weather
   ```
5. Run the binary:
   ```bash
   ./kindle-weather
   ```

## Testing

The 2012 Kindle Paperwhite (5th generation, firmware 5.6.1.1) is a supported
layout target. Browser CI checks portrait and landscape viewports, reduced
heights for the browser toolbar, the photographed 7:32 PM tide row, and wider
12:59 PM times. Tests check that tide text stays clear of the chart and caption,
assets load, and forecast navigation works without JavaScript. Text autosizing
is disabled to keep older WebKit from expanding text beyond its table cells.
Modern WebKit is an approximation; changes to layout should also be checked on
the physical Kindle using Freshlab's LAN-only HTTP endpoint.

Run unit tests:

```bash
go test ./...
```

Run the local end-to-end check:

```bash
make e2e
```

The e2e check builds the binary, starts a local mock upstream API, runs the real
HTTP server, and verifies the main page, static CSS, metrics, normal tide
rendering, and the no-tide fallback. Pull requests targeting `main` run this
check in CI before the Docker image build.

## Deployment

### Docker

Build and run the application using Docker:

```bash
# Build the container
docker build -t kindle-weather .

# Run the container
docker run -p 8080:8080 \
  -v $(pwd)/secrets:/etc/secrets:ro \
  kindle-weather
```

### Kubernetes

Prerequisites:
1. Install nginx-ingress controller:
   ```bash
   helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
   helm install nginx-ingress ingress-nginx/ingress-nginx
   ```

2. Install cert-manager:
   ```bash
   helm repo add jetstack https://charts.jetstack.io
   helm install cert-manager jetstack/cert-manager \
     --namespace cert-manager \
     --create-namespace \
     --set installCRDs=true
   ```

3. Create ClusterIssuer:
   ```bash
   kubectl apply -f - <<EOF
   apiVersion: cert-manager.io/v1
   kind: ClusterIssuer
   metadata:
     name: letsencrypt-prod
   spec:
     acme:
       email: your-email@example.com
       server: https://acme-v02.api.letsencrypt.org/directory
       privateKeySecretRef:
         name: letsencrypt-prod
       solvers:
       - http01:
           ingress:
             class: nginx
   EOF
   ```

The application includes Kubernetes manifests in the `k8s/` directory:
- `deployment.yaml` - Defines the application deployment with:
  - Resource limits and requests
  - Liveness probe
  - Secret volume mount
  - Single replica
- `svc.yaml` - Creates a ClusterIP service on port 80
- `ingress.yaml` - Configures external access with:
  - TLS termination
  - Host-based routing
  - NGINX ingress configuration
- `certificate.yaml` - Configures TLS certificate using cert-manager

Deploy to a Kubernetes cluster:

```bash
# Create namespace (optional)
kubectl create namespace kindle-weather
kubectl config set-context --current --namespace=kindle-weather

# Apply the API key expected by the Deployment
kubectl create secret generic api-keys \
  --from-literal=openweather-api-key="$OPENWEATHER_API_KEY"

# Apply the manifests
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/svc.yaml
kubectl apply -f k8s/certificate.yaml
kubectl apply -f k8s/ingress.yaml
```

Monitor the deployment:
```bash
kubectl get pods -l app=kindle-weather
kubectl get service kindle-weather
kubectl logs -l app=kindle-weather
kubectl describe deployment kindle-weather
```

## Dependencies

- github.com/patrickmn/go-cache - For caching API responses
- Weather Icons font - For weather symbols

## API Endpoints

- `/` - Main weather display page
- `/dashboard.svg` - Static 758×1024 version of the overview: rain, tide, and
  beach bands, alerts, and cached-data status (`?h` scales it into 1024×758)
- `/css/*` - Static CSS files
- `/font/*` - Weather icon fonts
- `/health` - Process liveness
- `/ready` - Runtime readiness
- `/metrics` - Prometheus metrics

## License

MIT License

![DA231259-8420-4A90-B54E-2B0E3DB98755_1_102_a](https://github.com/user-attachments/assets/004f6d79-ebd4-4341-8600-9a03d612ca20)

### Kindle layout regression tests

Run `npm ci`, then `npx playwright install chromium webkit`, then `npm run test:layout`.
The suite renders the real Go HTML template, tide SVG, CSS, and icon fonts with deterministic data; no API keys or live weather services are needed.

Chromium and WebKit cover 600×800 and 758×1024 portrait screens plus 1024×758 landscape, with reduced viewport heights of 660, 880, and 600 pixels to reserve space for browser controls. These are CSS viewport assumptions, not identification of a particular Kindle model. Tests include the photographed summary, longer text, wrapped forecast labels, alerts, a launch, and missing optional sections. They assert no row overlap, clipped labels, horizontal overflow, or footer below the screen. CI saves screenshots and traces on failure.

Modern WebKit does not reproduce the Kindle's exact legacy engine or e-ink rendering; this suite catches layout regressions at the target dimensions but does not emulate the physical device.

For browser inspection when a local listening port is unavailable, export the
same production-template fixtures to a temporary directory:

```bash
KINDLE_LAYOUT_OUTPUT=/tmp/kindle-layout go test -run '^TestLayoutFixtureFiles$'
```

The exported HTML uses the normal `/css/` and `/font/` asset paths. A browser
harness can route those requests to the repository assets. Fixtures cover rain
uncertainty, cached data, missing sources, wide tide times, and both orientations.
The overview includes four upcoming forecasts, notices above current conditions,
full-width rain/tide/beach rows, and the selected section in navigation. It uses
ordinary links and requires no JavaScript.
