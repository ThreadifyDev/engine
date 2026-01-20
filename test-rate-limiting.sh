#!/bin/bash

# Use legitimate browser user-agent to bypass bot scanner
USER_AGENT="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36"

echo "╔════════════════════════════════════════════════════════╗"
echo "║  Rate Limiting Test Suite (with Latency)              ║"
echo "╚════════════════════════════════════════════════════════╝"
echo

# Test 0: Baseline Latency (no rate limiting - health endpoint)
echo "📊 Test 0: Baseline Latency (health endpoint, no rate limit)"
echo "   Measuring 10 requests..."
LATENCIES=""
for i in {1..10}; do 
  LATENCY=$(curl -s -A "$USER_AGENT" -o /dev/null -w "%{time_total}" http://localhost:8081/health)
  LATENCIES="$LATENCIES $LATENCY"
done
echo "   Latencies (seconds): $LATENCIES"
# Calculate average in milliseconds
AVG=$(echo "$LATENCIES" | awk '{sum=0; for(i=1;i<=NF;i++) sum+=$i; print (sum/NF)*1000}')
echo "   📈 Average latency: ${AVG}ms"
echo

# Test 1: IP Rate Limiting (60 req/min = 1 req/sec, burst 10)
echo "📊 Test 1: IP Rate Limiting with Latency (burst=10)"
echo "   Sending 15 rapid requests to /health..."
echo -n "   Results: "
TOTAL_TIME=0
for i in {1..15}; do 
  RESPONSE=$(curl -s -A "$USER_AGENT" -o /dev/null -w "%{http_code}:%{time_total}" http://localhost:8081/health)
  CODE=$(echo $RESPONSE | cut -d: -f1)
  TIME=$(echo $RESPONSE | cut -d: -f2)
  TOTAL_TIME=$(echo "$TOTAL_TIME + $TIME" | bc)
  echo -n "$CODE "
done
echo
AVG_TIME=$(echo "scale=4; ($TOTAL_TIME / 15) * 1000" | bc)
echo "   📈 Average latency: ${AVG_TIME}ms"
echo "   ✅ Expected: First 10 = 200, Next 5 = 429"
echo

# Wait for rate limit to reset
echo "⏳ Waiting 2 seconds for rate limit window..."
sleep 2
echo

# Test 2: User Rate Limiting with Latency (1000 req/min, burst 50)
echo "📊 Test 2: User Rate Limiting with Latency (burst=50)"
echo "   Sending 60 rapid authenticated requests to /graphql..."
echo "   Measuring latency for first 10 requests (cache miss)..."
FIRST_10_TIME=0
for i in {1..10}; do 
  TIME=$(curl -s -A "$USER_AGENT" -X POST http://localhost:8081/graphql \
    -H "X-API-Key: api-key-123" \
    -H "Content-Type: application/json" \
    -d '{"query": "{ __typename }"}' \
    -o /dev/null -w "%{time_total}")
  FIRST_10_TIME=$(echo "$FIRST_10_TIME + $TIME" | bc)
done
AVG_FIRST=$(echo "scale=4; ($FIRST_10_TIME / 10) * 1000" | bc)
echo "   📈 First 10 avg latency (cache miss): ${AVG_FIRST}ms"

echo "   Measuring latency for next 10 requests (cache hit)..."
NEXT_10_TIME=0
for i in {1..10}; do 
  TIME=$(curl -s -A "$USER_AGENT" -X POST http://localhost:8081/graphql \
    -H "X-API-Key: api-key-123" \
    -H "Content-Type: application/json" \
    -d '{"query": "{ __typename }"}' \
    -o /dev/null -w "%{time_total}")
  NEXT_10_TIME=$(echo "$NEXT_10_TIME + $TIME" | bc)
done
AVG_NEXT=$(echo "scale=4; ($NEXT_10_TIME / 10) * 1000" | bc)
echo "   📈 Next 10 avg latency (cache hit): ${AVG_NEXT}ms"

# Calculate cache benefit
BENEFIT=$(echo "scale=4; $AVG_FIRST - $AVG_NEXT" | bc)
echo "   🚀 Cache benefit: ${BENEFIT}ms faster"

echo "   Sending remaining 40 requests..."
echo -n "   Results (last 15): "
for i in {1..40}; do 
  curl -s -A "$USER_AGENT" -X POST http://localhost:8081/graphql \
    -H "X-API-Key: api-key-123" \
    -H "Content-Type: application/json" \
    -d '{"query": "{ __typename }"}' \
    -o /dev/null -w "%{http_code} "
done | tail -c 75
echo
echo "   ✅ Expected: First 50 = 200, Next 10 = 429"
echo

# Test 3: Different users should have separate limits
echo "📊 Test 3: Per-User Isolation"
echo "   Testing with different API key..."
echo -n "   User 2 Results: "
for i in {1..5}; do 
  curl -s -A "$USER_AGENT" -X POST http://localhost:8081/graphql \
    -H "X-API-Key: api-key-456" \
    -H "Content-Type: application/json" \
    -d '{"query": "{ __typename }"}' \
    -o /dev/null -w "%{http_code} "
done
echo
echo "   ✅ Expected: All 200 (separate user limit)"
echo

echo "╔════════════════════════════════════════════════════════╗"
echo "║  Test Complete                                        ║"
echo "╚════════════════════════════════════════════════════════╝"
