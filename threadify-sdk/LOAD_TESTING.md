# Load Testing with k6

## Setup

### Installation
```bash
brew install k6
```

### Server Setup
Start the server with pprof profiling:
```bash
cd threadify-go
make run
```

## Load Testing

### Basic Test
```bash
k6 run load-test.js
```

### Advanced Options
```bash
# Run with specific VUs
k6 run --vus 5000 --duration 60s load-test.js

# Run with HTML report
k6 run --out html=report.html load-test.js

# Run with JSON output
k6 run --out json=results.json load-test.js
```

## Test Scenarios

The k6 test simulates realistic Threadify usage:
1. **Connect**: WebSocket connection with authentication
2. **Start Thread**: Create a new thread instance
3. **Step Events**: Send 5 step events with 100ms intervals
4. **Close Connection**: Clean WebSocket shutdown

### Load Stages
- **Warm up**: 5 users for 30 seconds
- **Load test**: 20 users for 60 seconds  
- **Stress test**: 50 users for 30 seconds

## Metrics

### Built-in Metrics
- WebSocket connection success rate
- Request duration percentiles
- Error rate tracking

### Custom Metrics
- `errors`: Tracks failed operations
- Response times for each action type

## Profiling During Load Tests

### Capture Heap Profile
```bash
curl "http://localhost:6060/debug/pprof/heap" -o load-test-heap.pprof
```

### Capture CPU Profile
```bash
curl "http://localhost:6060/debug/pprof/profile?seconds=30" -o load-test-cpu.pprof
```

### Analyze Profiles
```bash
go tool pprof load-test-heap.pprof
go tool pprof load-test-cpu.pprof

# Web interface
go tool pprof -http=:8080 load-test-heap.pprof
```

## Expected Results

### Performance Targets
- **Error rate**: < 10%
- **Response time**: 95% < 500ms
- **Concurrent users**: 50+ without degradation

### Key Metrics to Monitor
1. **WebSocket connections**: Concurrent connection limits
2. **Memory usage**: Thread state caching efficiency
3. **CPU usage**: Message processing overhead
4. **Database performance**: PostgreSQL/Valkey response times

## Troubleshooting

### Common Issues
- **Thread not found errors**: Ensure threads are created before sending events
- **WebSocket timeouts**: Check server capacity and network latency
- **High memory usage**: Monitor thread state caching and cleanup

### Debug Mode
Enable verbose logging:
```bash
k6 run --verbose load-test.js
```
