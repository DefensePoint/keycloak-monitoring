#!/bin/bash

# Script para rebuild, restart e ver logs de debug de eventos

echo "======================================"
echo "🔨 Rebuilding Docker container..."
echo "======================================"
docker-compose -f deployments/local/docker-compose.yml build --no-cache api-server

echo ""
echo "======================================"
echo "🔄 Restarting container..."
echo "======================================"
docker-compose -f deployments/local/docker-compose.yml up -d api-server

echo ""
echo "======================================"
echo "⏳ Waiting 5 seconds for startup..."
echo "======================================"
sleep 5

echo ""
echo "======================================"
echo "📋 Showing recent logs..."
echo "======================================"
docker logs --tail 100 kmt-server

echo ""
echo "======================================"
echo "👁️  Following logs in real-time..."
echo "======================================"
echo "Press Ctrl+C to stop"
echo ""

docker logs -f kmt-server 2>&1 | grep --line-buffered -E "\[DEBUG\]|ERROR|WARN|Starting|Collected"
