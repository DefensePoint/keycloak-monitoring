#!/bin/bash

# Script simples para ver logs de debug

echo "======================================"
echo "📋 Logs Filtrados (DEBUG, ERROR, WARN)"
echo "======================================"
echo ""

docker logs -f kmt-server 2>&1 | grep --line-buffered --color=always -E "\[DEBUG\]|ERROR|WARN|Starting|Collected|Monitor"
