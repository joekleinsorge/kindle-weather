# Kindle Weather

A web application that displays weather, tide, moon phase, and space launch information in a Kindle-friendly format.

## Features

- Current weather conditions with temperature and description
- 4-hour weather forecast
- Tide predictions
- Moon phase display
- Sunrise and sunset times
- Upcoming space launches
- Simple design optimized for Kindle displays
- Caching for API responses to reduce calls

## Build

1. Install Go 1.22.3 or later
2. Clone the repository
3. Create a `.secrets` file with your API keys
4. Build the application:
   ```bash
   go build -o kindle-weather
   ```
5. Run the binary:
   ```bash
   ./kindle-weather
   ```

## Deployment

### Docker

Build and run the application using Docker:

```bash
# Build the container
docker build -t kindle-weather .

# Run the container
docker run -p 8080:8080 \
  -v $(pwd)/.secrets:/app/.secrets \
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
- `service.yaml` - Creates a ClusterIP service on port 80
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

# Apply secrets
kubectl create secret generic kindle-secrets --from-file=.secrets
kubectl create secret tls weather-tls --cert=tls.crt --key=tls.key

# Apply the manifests
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml
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
- `/css/*` - Static CSS files

## License

MIT License