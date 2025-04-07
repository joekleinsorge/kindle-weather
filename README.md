# Kindle Weather Dashboard

A minimalist weather dashboard designed for E-ink displays like Kindle devices. Displays current weather conditions, tide information, and upcoming rocket launches.

## Features

- Current weather conditions with temperature and forecast
- Tide information for coastal locations
- Upcoming rocket launch schedule
- E-ink friendly design
- Containerized deployment with Kubernetes support

## Prerequisites

- Python 3.9+
- Docker (for containerized deployment)
- Kubernetes cluster (optional)
- API Keys:
  - OpenWeather API
  - Tide API (for coastal data)
  - Space Launch API (for rocket launches)

## Setup

1. Clone the repository:
```bash
git clone https://github.com/yourusername/kindle-weather.git
cd kindle-weather
```

2. Create a `.env` file based on example.env:
```bash
cp example.env .env
```

3. Edit `.env` with your API keys and location settings.

4. Install dependencies:
```bash
pip install -r requirements.txt
```

5. Run the application:
```bash
flask run
```

## Docker Deployment

Build and run using Docker:

```bash
docker build -t kindle-weather .
docker run -p 5000:5000 --env-file .env kindle-weather
```

## Kubernetes Deployment

1. Update `deploy.yaml` with your API keys and settings
2. Apply the configuration:
```bash
kubectl apply -f deploy.yaml
```

## Development

- Pre-commit hooks are configured for code quality
- GitHub Actions handle CI/CD pipeline
- Container images are automatically built and published to GitHub Container Registry

## License

MIT License

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a new Pull Request
