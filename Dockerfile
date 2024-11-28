# Use Node.js 20 Alpine as the base image
FROM node:20-alpine

# Set the working directory in the container
WORKDIR /usr/src/app

# Copy package.json and package-lock.json first
COPY package*.json ./

# Install dependencies
RUN npm install

# Copy the entire project
COPY . .

# Expose the port the app runs on
EXPOSE 80

# Use a lightweight web server to serve static files
RUN npm install -g http-server

# Command to run the application
CMD ["http-server", "src", "-p", "80", "-c-1"]
