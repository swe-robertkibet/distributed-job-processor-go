#!/bin/bash

echo "🔍 Testing MongoDB Connectivity"
echo "==============================="

# Start containers
echo "Starting containers..."
docker compose -f docker-compose.bully.yml up -d

echo "Waiting for containers to start..."
sleep 10

# Test MongoDB connectivity from each container
for i in 1 2 3; do
    echo
    echo "Testing MongoDB connectivity from job-processor-$i:"
    
    # Get the MongoDB URI from environment
    MONGODB_URI=$(grep MONGODB_URI .env | cut -d'=' -f2)
    
    # Test basic connectivity
    docker compose -f docker-compose.bully.yml exec job-processor-$i sh -c "
        echo 'Testing MongoDB connection...'
        go run -c 'import os; import pymongo; print(pymongo.MongoClient(os.environ[\"MONGODB_URI\"]).admin.command(\"ismaster\"))' 2>/dev/null || echo 'Connection test failed'
    " 2>/dev/null || echo "❌ Container not responding"
done

echo
echo "Checking recent logs for MongoDB errors:"
for i in 1 2 3; do
    echo "--- job-processor-$i logs ---"
    docker compose -f docker-compose.bully.yml logs job-processor-$i --tail=5 | grep -i "mongo\|error\|failed" || echo "No MongoDB errors found"
done

echo
echo "Test complete. Containers are still running for manual inspection."
echo "To stop: docker compose -f docker-compose.bully.yml down"