import requests
import pytz
from datetime import datetime, timedelta
import requests
import pytz
from datetime import datetime, timedelta
import math

class WeatherCondition:
    def __init__(self, data):
        self.id = data.get('id')
        self.main = data.get('main')
        self.description = data.get('description')
        self.icon = data.get('icon')

class Rain:
    def __init__(self, data):
        self.one_hour = data.get('1h', 0)

class CurrentWeather:
    def __init__(self, data):
        self.dt = datetime.fromtimestamp(data.get('dt', 0), tz=pytz.UTC)
        self.sunrise = datetime.fromtimestamp(data.get('sunrise', 0), tz=pytz.UTC)
        self.sunset = datetime.fromtimestamp(data.get('sunset', 0), tz=pytz.UTC)
        
        self.temp = data.get('temp', 0)
        self.feels_like = data.get('feels_like', 0)
        self.pressure = data.get('pressure', 0)
        self.humidity = data.get('humidity', 0)
        self.dew_point = data.get('dew_point', 0)
        self.uvi = data.get('uvi', 0)
        self.clouds = data.get('clouds', 0)
        self.visibility = data.get('visibility', 0)
        self.wind_speed = data.get('wind_speed', 0)
        self.wind_deg = data.get('wind_deg', 0)
        self.wind_gust = data.get('wind_gust', 0)
        
        self.weather = [WeatherCondition(w) for w in data.get('weather', [])]
        
        # Formatted times
        self.sunrise_formatted = self.sunrise.strftime('%I:%M %p')
        self.sunset_formatted = self.sunset.strftime('%I:%M %p')

class HourlyWeather:
    def __init__(self, data):
        self.dt = datetime.fromtimestamp(data.get('dt', 0), tz=pytz.UTC)
        self.dt_formatted = self.dt.strftime('%Y-%m-%d %H:%M')
        self.temp = data.get('temp', 0)
        self.feels_like = data.get('feels_like', 0)
        self.pressure = data.get('pressure', 0)
        self.humidity = data.get('humidity', 0)
        self.dew_point = data.get('dew_point', 0)
        self.uvi = data.get('uvi', 0)
        self.clouds = data.get('clouds', 0)
        self.visibility = data.get('visibility', 0)
        self.wind_speed = data.get('wind_speed', 0)
        self.wind_deg = data.get('wind_deg', 0)
        self.wind_gust = data.get('wind_gust', 0)
        self.weather = [WeatherCondition(w) for w in data.get('weather', [])]
        self.pop = data.get('pop', 0)
        self.rain = Rain(data.get('rain', {}))

class DailyWeather:
    def __init__(self, data):
        self.dt = datetime.fromtimestamp(data.get('dt', 0), tz=pytz.UTC)
        self.sunrise = datetime.fromtimestamp(data.get('sunrise', 0), tz=pytz.UTC)
        self.sunset = datetime.fromtimestamp(data.get('sunset', 0), tz=pytz.UTC)
        
        # Temperature details
        self.temp = data.get('temp', {})
        self.feels_like = data.get('feels_like', {})
        
        # Moon details
        self.moonrise = datetime.fromtimestamp(data.get('moonrise', 0), tz=pytz.UTC) if data.get('moonrise') else None
        self.moonset = datetime.fromtimestamp(data.get('moonset', 0), tz=pytz.UTC) if data.get('moonset') else None
        self.moon_phase = data.get('moon_phase', 0)
        
        # Weather conditions
        self.weather = [WeatherCondition(w) for w in data.get('weather', [])]
        
        # Additional details
        self.humidity = data.get('humidity', 0)
        self.wind_speed = data.get('wind_speed', 0)
        self.summary = data.get('summary', '')
        
        # Precipitation probability
        self.pop = data.get('pop', 0)

class WeatherData:
    def __init__(self, data):
        self.current = CurrentWeather(data.get('current', {}))
        self.hourly = [HourlyWeather(h) for h in data.get('hourly', [])]
        self.daily = [DailyWeather(d) for d in data.get('daily', [])]

class APIService:
    def __init__(self):
        self.open_weather_api_key = 'YOUR_OPENWEATHER_API_KEY'
        self.rocket_api_key = 'YOUR_ROCKET_API_KEY'

    def get_weather_data(self, lat, lon, units='metric', exclude='minutely,alerts'):
        """
        Fetch comprehensive weather data using OpenWeatherMap OneCall API
        
        :param lat: Latitude
        :param lon: Longitude
        :param units: 'metric' or 'imperial'
        :param exclude: Comma-separated string of data to exclude
        :return: WeatherData object
        """
        try:
            # OneCall API v2.5 URL
            url = f"https://api.openweathermap.org/data/2.5/onecall?lat={lat}&lon={lon}&exclude={exclude}&appid={self.open_weather_api_key}&units={units}"
            
            response = requests.get(url)
            response.raise_for_status()  # Raise an exception for bad responses
            
            data = response.json()
            
            # Create and return WeatherData object
            return WeatherData(data)
        
        except requests.RequestException as e:
            print(f"Weather API error: {e}")
            return None

    def get_tide_data(self, location):
        """Fetch tide information"""
        try:
            # Replace with actual tide API endpoint
            tide_url = f"https://api.tideapi.com/tides?location={location}&key={self.tide_api_key}"
            response = requests.get(tide_url).json()

            return {
                'high_tide': response.get('high_tide'),
                'low_tide': response.get('low_tide')
            }
        except Exception as e:
            print(f"Tide API error: {e}")
            return None

    def get_rocket_launches(self):
        """Fetch upcoming rocket launches"""
        try:
            # Replace with actual rocket launch API
            launches_url = f"https://ll.thespacedevs.com/2.2.0/launch/upcoming/?
            response = requests.get(launches_url).json()

            return [
                {
                    'name': launch['name'],
                    'date': datetime.fromisoformat(launch['net']),
                    'location': launch['pad']['location']['name']
                } for launch in response.get('results', [])[:3]
            ]
        except Exception as e:
            print(f"Rocket Launches API error: {e}")
            return None

