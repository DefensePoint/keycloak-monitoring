#!/bin/bash

# Script para buscar logs específicos

echo "======================================"
echo "🔍 EVENT DEBUG LOGS"
echo "======================================"
echo ""

echo "--- 1. MonitorPoolManager startup ---"
docker logs kmt-server 2>&1 | grep -i "monitor pool"
echo ""

echo "--- 2. Tenant prod-keycloak startup ---"
docker logs kmt-server 2>&1 | grep -i "prod-keycloak"
echo ""

echo "--- 3. Event collection cycles ---"
docker logs kmt-server 2>&1 | grep "\[DEBUG\].*event"
echo ""

echo "--- 4. Realm monitoring ---"
docker logs kmt-server 2>&1 | grep "Realms to monitor"
echo ""

echo "--- 5. Fetched events from API ---"
docker logs kmt-server 2>&1 | grep "Fetched events"
echo ""

echo "--- 6. Saving to database ---"
docker logs kmt-server 2>&1 | grep "Saving.*events"
echo ""

echo "--- 7. Any errors ---"
docker logs kmt-server 2>&1 | grep -i "error\|failed"
echo ""
