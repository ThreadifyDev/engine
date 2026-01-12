#!/bin/bash

# Upload product_delivery contract for company-abc
# This script uploads the contract YAML file to the Threadify Engine

API_KEY="api-key-123"
OWNER_ID="user-123"
COMPANY_ID="company-abc"
CONTRACT_FILE="threadify-go/examples/contract_example.yaml"
BASE_URL="http://localhost:8081"

echo "🔐 Logging in to get JWT token..."
login_response=$(curl -s -X POST "$BASE_URL/v1/contracts/login" \
  -H "Content-Type: application/json" \
  -d "{\"api_key\": \"$API_KEY\", \"owner_id\": \"$OWNER_ID\"}")

TOKEN=$(echo "$login_response" | jq -r '.token // empty')

if [ -z "$TOKEN" ]; then
  echo "❌ Failed to login"
  echo "$login_response" | jq .
  exit 1
fi

echo "✅ Login successful"
echo ""
echo "📤 Uploading contract: product_delivery"
echo "   Company ID: $COMPANY_ID"
echo "   Owner ID: $OWNER_ID"
echo ""

# Read the YAML file
CONTRACT_YAML=$(cat "$CONTRACT_FILE")

# Upload contract using curl (sending as YAML directly)
response=$(curl -s -w "\nHTTP_STATUS:%{http_code}" -X POST "$BASE_URL/v1/contracts" \
  -H "Content-Type: application/x-yaml" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Owner-ID: $OWNER_ID" \
  -H "X-Company-ID: $COMPANY_ID" \
  --data-binary "@$CONTRACT_FILE")

# Extract HTTP status and body
http_status=$(echo "$response" | grep "HTTP_STATUS" | cut -d':' -f2)
body=$(echo "$response" | sed '/HTTP_STATUS/d')

echo "Response (HTTP $http_status):"
echo "$body" | jq . 2>/dev/null || echo "$body"

# Extract contract ID if successful
contract_id=$(echo "$body" | jq -r '.id // .contract_id // empty' 2>/dev/null)

if [ "$http_status" = "200" ] || [ "$http_status" = "201" ]; then
  echo ""
  echo "✅ Contract uploaded successfully!"
  if [ -n "$contract_id" ]; then
    echo "   Contract ID: $contract_id"
  fi
  echo "   Contract Name: product_delivery"
  echo "   Company ID: $COMPANY_ID"
else
  echo ""
  echo "❌ Failed to upload contract (HTTP $http_status)"
  echo "   Check the error message above"
fi
