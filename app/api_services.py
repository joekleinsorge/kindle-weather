import requests
import pytz
from datetime import datetime, timedelta

class APIService:
    def __init__(self):
        # API keys would typically come from environment variables
        self.open_weather_api_key = 'YOUR_OPENWEATHER_API_KEY'
        self.tide_api_key = 'YOUR_TIDE_API_KEY'
        self.rocket_api_key = 'YOUR_ROCKET_API_KEY'

    def get_weather_data(self, lat, lon):
        """Fetch current weather and forecast"""
        try:
            # Current weather
            current_url = f"https://api.openweathermap.org/data/2.5/weather?lat={lat}&lon={lon}&appid={self.open_weather_api_key}&units=metric"
            current_response = requests.get(current_url).json()

            # 5-day forecast
            forecast_url = f"https://api.openweathermap.org/data/2.5/forecast?lat={lat}&lon={lon}&appid={self.open_weather_api_key}&units=metric"
            forecast_response = requests.get(forecast_url).json()

            return {
                'current': {
                    'temp': current_response['main']['temp'],
                    'description': current_response['weather'][0]['description'],
                    'icon': current_response['weather'][0]['icon']
                },
                'forecast': [
                    {
                        'date': datetime.fromtimestamp(forecast['dt'], tz=pytz.UTC),
                        'temp': forecast['main']['temp'],
                        'description': forecast['weather'][0]['description'],
                        'icon': forecast['weather'][0]['icon']
                    } for forecast in forecast_response['list'][:5]
                ]
            }
        except Exception as e:
            print(f"Weather API error: {e}")
            return None

    # ... rest of the class remains the same
