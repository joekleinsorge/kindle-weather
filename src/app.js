const CONFIG = {
    WEATHER_API_KEY: 'your_weather_api_key',
    WEATHER_API_ENDPOINT: 'https://api.openweathermap.org/data/3.0/onecall?lat=29.65&lon=-81.20&exclude=minutely&appid=%s&units=imperial',
    TIDE_API_ENDPOINT: 'https://api.tidesandcurrents.noaa.gov/api/prod/datagetter?product=predictions&application=NOS.COOPS.TAC.WL&datum=MLLW&station=8720218&time_zone=lst_ldt&units=english&interval=hilo&format=json&date=today',
    LAUNCHES_API_ENDPOINT: 'https://ll.thespacedevs.com/2.2.0/launch/upcoming'
};

// Utility function for fetching data
async function fetchData(url, options = {}) {
    try {
        const response = await fetch(url, options);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        return await response.json();
    } catch (error) {
        console.error('Fetch error:', error);
        return null;
    }
}

// Fetch current weather
async function fetchCurrentWeather(lat, lon) {
    const url = `${CONFIG.WEATHER_API_ENDPOINT}?lat=${lat}&lon=${lon}&appid=${CONFIG.WEATHER_API_KEY}&units=metric`;
    const data = await fetchData(url);
    
    if (data) {
        document.getElementById('current-weather').innerHTML = `
            <p>Temperature: ${data.main.temp}°C</p>
            <p>Conditions: ${data.weather[0].description}</p>
        `;
    }
}

// Fetch weather forecast
async function fetchForecast(lat, lon) {
    const url = `${CONFIG.FORECAST_API_ENDPOINT}?lat=${lat}&lon=${lon}&appid=${CONFIG.WEATHER_API_KEY}&units=metric`;
    const data = await fetchData(url);
    
    if (data) {
        const forecastHtml = data.list
            .filter((_, index) => index % 8 === 0) // Daily forecast
            .slice(0, 3)
            .map(forecast => `
                <div>
                    <p>Date: ${new Date(forecast.dt * 1000).toLocaleDateString()}</p>
                    <p>Temp: ${forecast.main.temp}°C</p>
                    <p>Conditions: ${forecast.weather[0].description}</p>
                </div>
            `).join('');
        
        document.getElementById('forecast').innerHTML = forecastHtml;
    }
}

// Fetch tide information
async function fetchTideData(locationId) {
    const url = `${CONFIG.TIDE_API_ENDPOINT}/${locationId}`;
    const data = await fetchData(url);
    
    if (data) {
        document.getElementById('tide-data').innerHTML = `
            <p>High Tide: ${data.highTide}</p>
            <p>Low Tide: ${data.lowTide}</p>
        `;
    }
}

// Fetch upcoming rocket launches
async function fetchRocketLaunches() {
    const url = CONFIG.LAUNCHES_API_ENDPOINT;
    const data = await fetchData(url);
    
    if (data && data.results) {
        const launchesHtml = data.results.slice(0, 3).map(launch => `
            <div>
                <p>Mission: ${launch.mission.name}</p>
                <p>Launch Date: ${new Date(launch.net).toLocaleString()}</p>
                <p>Location: ${launch.pad.location.name}</p>
            </div>
        `).join('');
        
        document.getElementById('launch-list').innerHTML = launchesHtml;
    }
}

// Main function to update all data
async function updateDashboard() {
    // Example coordinates (replace with your location)
    const lat = 40.7128;
    const lon = -74.0060;
    const tideLocationId = 'your_location_id';

    await Promise.all([
        fetchCurrentWeather(lat, lon),
        fetchForecast(lat, lon)
    ]);
}

// Full daily update function
async function fullDailyUpdate() {
    // Example coordinates (replace with your location)
    const lat = 40.7128;
    const lon = -74.0060;
    const tideLocationId = 'your_location_id';

    await Promise.all([
        fetchCurrentWeather(lat, lon),
        fetchForecast(lat, lon),
        fetchTideData(tideLocationId),
        fetchRocketLaunches()
    ]);
}

// Initial load
updateDashboard();

// Refresh current weather every hour
setInterval(async () => {
    const lat = 40.7128;
    const lon = -74.0060;
    await fetchCurrentWeather(lat, lon);
}, 3600000); // 1 hour in milliseconds

// Function to calculate milliseconds until next daily update
function getMillisecondsUntilNextUpdate() {
    const now = new Date();
    const nextUpdate = new Date(
        now.getFullYear(), 
        now.getMonth(), 
        now.getDate(), 
        6, 0, 0, 0 // Set to 6 AM
    );
    
    // If it's already past 6 AM, set to next day
    if (now > nextUpdate) {
        nextUpdate.setDate(nextUpdate.getDate() + 1);
    }
    
    return nextUpdate.getTime() - now.getTime();
}

// Schedule daily full update
function scheduleDailyUpdate() {
    const millisecondsUntilNextUpdate = getMillisecondsUntilNextUpdate();
    
    // First timeout to the next 6 AM
    setTimeout(() => {
        // Perform full update (including tide and launches)
        fullDailyUpdate();
        
        // Then set an interval to repeat daily
        setInterval(fullDailyUpdate, 24 * 3600000);
    }, millisecondsUntilNextUpdate);
}

// Start the daily update scheduler
scheduleDailyUpdate();
