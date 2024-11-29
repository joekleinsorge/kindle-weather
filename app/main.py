from flask import Flask, render_template
from api_services import APIService
import threading
import time

app = Flask(__name__)
api_service = APIService()

# Global variables to store API data
weather_data = None
tide_data = None
rocket_launches = None

def refresh_data():
    global weather_data, tide_data, rocket_launches
    
    while True:
        try:
            # Replace with your specific coordinates
            weather_data = api_service.get_weather_data(40.7128, -74.0060)  # New York City coordinates
            tide_data = api_service.get_tide_data('new-york')
            rocket_launches = api_service.get_rocket_launches()
        except Exception as e:
            print(f"Data refresh error: {e}")
        
        # Sleep for 1 hour
        time.sleep(3600)

@app.route('/')
def index():
    return render_template('index.html', 
                           weather=weather_data, 
                           tides=tide_data, 
                           launches=rocket_launches)

if __name__ == '__main__':
    # Start data refresh in a separate thread
    threading.Thread(target=refresh_data, daemon=True).start()
    
    # Initialize data immediately
    refresh_data()
    
    # Run the Flask app
    app.run(host='0.0.0.0', port=5000)
