import os
from flask import Flask
import logging

def create_app():
    """
    Application factory function to create and configure the Flask application
    with Kubernetes secret integration.
    """
    app = Flask(__name__)

    # Kubernetes Secret Environment Variable Mapping
    app.config['OPEN_WEATHER_API_KEY'] = os.environ.get('OPEN_WEATHER_API_KEY', '')

    # Location Configuration from Secrets or Defaults
    app.config['LATITUDE'] = os.environ.get('LOCATION_LATITUDE', '40.7128')
    app.config['LONGITUDE'] = os.environ.get('LOCATION_LONGITUDE', '-74.0060')
    app.config['LAUNCH_PAD_ID'] = os.environ.get('LAUNCH_PAD_ID', '27')

    # Logging
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )

    # Validate Critical Secrets
    critical_secrets = [
        'OPEN_WEATHER_API_KEY', 
    ]
    
    for secret in critical_secrets:
        if not app.config[secret]:
            logging.error(f"CRITICAL: {secret} is not configured!")
            raise ValueError(f"{secret} must be provided")

    app.logger.info('Application configured with Kubernetes secrets')

    # Import routes after app creation
    from . import main

    return app
