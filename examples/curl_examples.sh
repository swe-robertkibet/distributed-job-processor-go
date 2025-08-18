#!/bin/bash

BASE_URL="http://localhost:8080/api/v1"

echo "=== Distributed Job Processor API Examples ==="

echo -e "\n1. Health Check"
curl -X GET http://localhost:8080/health

echo -e "\n\n2. Create Email Job"
curl -X POST $BASE_URL/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "email",
    "priority": 5,
    "payload": {
      "recipient": "user@example.com",
      "subject": "Welcome!",
      "body": "Welcome to our service!"
    },
    "max_retries": 3
  }'

echo -e "\n\n3. Create High Priority Image Processing Job"
curl -X POST $BASE_URL/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "type": "image_processing",
    "priority": 10,
    "payload": {
      "image_url": "https://example.com/image.jpg",
      "operation": "resize",
      "width": 800,
      "height": 600
    },
    "max_retries": 2
  }'

echo -e "\n\n4. Create Delayed Job (scheduled for 30 seconds from now)"
FUTURE_TIME=$(date -u -d '+30 seconds' '+%Y-%m-%dT%H:%M:%S.000Z')
curl -X POST $BASE_URL/jobs \
  -H "Content-Type: application/json" \
  -d "{
    \"type\": \"email\",
    \"priority\": 1,
    \"scheduled_at\": \"$FUTURE_TIME\",
    \"payload\": {
      \"recipient\": \"delayed@example.com\",
      \"subject\": \"Delayed Email\",
      \"body\": \"This email was scheduled!\"
    }
  }"

echo -e "\n\n5. List All Jobs"
curl -X GET $BASE_URL/jobs

echo -e "\n\n6. List Jobs by Status (pending)"
curl -X GET "$BASE_URL/jobs?status=pending"

echo -e "\n\n7. List Jobs by Type (email)"
curl -X GET "$BASE_URL/jobs?type=email"

echo -e "\n\n8. Get System Stats"
curl -X GET $BASE_URL/stats

echo -e "\n\n9. Get Worker Stats"
curl -X GET $BASE_URL/workers

echo -e "\n\n10. Get Cluster Nodes"
curl -X GET $BASE_URL/nodes

echo -e "\n\n11. Get Leader Information"
curl -X GET $BASE_URL/leader

echo -e "\n\n12. Prometheus Metrics"
curl -X GET http://localhost:9090/metrics

echo -e "\n\n=== Examples Complete ==="