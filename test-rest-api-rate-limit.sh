#!/bin/bash

echo "╔════════════════════════════════════════════════════════╗"
echo "║  REST API Rate Limiting Test                          ║"
echo "╚════════════════════════════════════════════════════════╝"
echo

# First, login to get JWT token
echo "📝 Step 1: Login to get JWT token..."
LOGIN_RESPONSE=$(curl -s -X POST http://localhost:8081/v1/contracts/login \
  -H "X-API-Key: api-key-123" \
  -H "Content-Type: application/json")

TOKEN=$(echo $LOGIN_RESPONSE | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
  echo "❌ Failed to get token"
  echo "Response: $LOGIN_RESPONSE"
  exit 1
fi

echo "✅ Got JWT token"
echo

# Test REST API with user rate limiting
echo "📊 Test: REST API User Rate Limiting (burst=50)"
echo "   Sending 60 rapid requests to /v1/contracts..."
echo -n "   Results (last 15): "
for i in {1..60}; do 
  curl -s -X GET http://localhost:8081/v1/contracts \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -o /dev/null -w "%{http_code} "
done | tail -c 75
echo
echo "   ✅ Expected: First 50 = 200, Next 10 = 429"
echo

echo "╔════════════════════════════════════════════════════════╗"
echo "║  Test Complete                                        ║"
echo "╚════════════════════════════════════════════════════════╝"
