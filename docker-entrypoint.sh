#!/bin/bash
set -e

# Set FFmpeg path for yt-dlp
export FFMPEG_PATH="/usr/bin/ffmpeg"

# Start Redis in background
echo "Starting Redis..."
redis-server --daemonize yes --logfile logs/redis.log

# Wait for Redis to be ready
sleep 2

# Start Worker in background
echo "Starting Worker..."
./bin/worker &
WORKER_PID=$!

# Start API in foreground
echo "Starting API..."
exec ./bin/api
