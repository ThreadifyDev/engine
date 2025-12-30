#!/bin/bash

# Threadify Quick Start Script

set -e

echo "🚀 Threadify Quick Start"
echo "========================"
echo ""

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Error: Docker is not running"
    echo "Please start Docker and try again"
    exit 1
fi

# Check if docker-compose is available
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Error: docker-compose is not installed"
    echo "Please install docker-compose and try again"
    exit 1
fi

# Create .env if it doesn't exist
if [ ! -f .env ]; then
    echo "📝 Creating .env file from template..."
    cp .env.example .env
    echo "✅ Created .env file"
    echo "⚠️  Please review .env and update passwords for production!"
    echo ""
fi

# Create network if it doesn't exist
if ! docker network inspect threadify-network > /dev/null 2>&1; then
    echo "🌐 Creating Docker network..."
    docker network create threadify-network
    echo "✅ Network created"
    echo ""
fi

# Build images
echo "🔨 Building Docker images..."
docker-compose build
echo "✅ Images built"
echo ""

# Start services
echo "🚀 Starting services..."
docker-compose up -d
echo "✅ Services started"
echo ""

# Wait for services to be healthy
echo "⏳ Waiting for services to be healthy..."
sleep 5

# Check health
echo ""
echo "🏥 Health Check:"
echo "----------------"

# Check Threadify Engine
if curl -s http://localhost:8081/health > /dev/null 2>&1; then
    echo "✅ Threadify Engine: http://localhost:8081"
else
    echo "⏳ Threadify Engine: Starting..."
fi

# Check PostgreSQL
if docker-compose exec -T postgres pg_isready -U td_engine > /dev/null 2>&1; then
    echo "✅ PostgreSQL: localhost:5434"
else
    echo "⏳ PostgreSQL: Starting..."
fi

# Check Valkey
if docker-compose exec -T valkey valkey-cli -a threadify_secure_password ping > /dev/null 2>&1; then
    echo "✅ Valkey: localhost:6379"
else
    echo "⏳ Valkey: Starting..."
fi

echo ""
echo "🎉 Threadify is starting!"
echo ""
echo "📊 View logs:"
echo "   docker-compose logs -f"
echo ""
echo "📊 View status:"
echo "   docker-compose ps"
echo ""
echo "🛑 Stop services:"
echo "   docker-compose down"
echo ""
echo "📚 Full documentation: See DOCKER.md"
echo ""
