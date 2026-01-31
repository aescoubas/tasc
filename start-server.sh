#!/bin/bash
set -e

# Default port
PORT=${PORT:-8081}

# Build the application
echo "Building tasc..."
go build -tags fts5 -o tasc

# Information for the user
echo "---------------------------------------------------"
echo "Starting tasc server on port $PORT..."
echo ""
echo "To configure your client to use this server, run:"
echo "  export TASC_REMOTE=http://localhost:$PORT"
echo "---------------------------------------------------"

# Start the server
./tasc serve --port $PORT
