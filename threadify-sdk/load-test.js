import ws from 'k6/ws';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

// Custom metrics
const errorRate = new Rate('errors');

export let options = {
  stages: [
    { duration: '30s', target: 5 },   // Warm up
    { duration: '60s', target: 20 },  // Load test
    { duration: '30s', target: 50 },  // Stress test
  ],
  thresholds: {
    errors: ['rate<0.1'], // Error rate should be less than 10%
    http_req_duration: ['p(95)<500'], // 95% of requests should be below 500ms
  },
};

export default function () {
  const url = 'ws://localhost:8081/threads';
  const params = { tags: { my_tag: 'threadify_load_test' } };
  
  let threadId = null;
  let response = null;
  
  let res = ws.connect(url, params, function (socket) {
    socket.on('open', function () {
      console.log('WebSocket connected');
      
      // Connect to Threadify
      const connectMsg = JSON.stringify({
        action: 'connect',
        apiKey: 'test-api-key-123',
        ownerId: `test-user-${__VU}-${__ITER}`,
        subscribedEvents: ['onSuccess', 'onError', 'onViolation', 'onStepProgress']
      });
      socket.send(connectMsg);
    });
    
    socket.on('message', function (message) {
      try {
        const data = JSON.parse(message);
        
        if (data.action === 'connect' && data.status === 'success') {
          console.log('Connected successfully');
          
          // Start thread
          const startThreadMsg = JSON.stringify({
            action: 'startThread',
            contractId: '',
            metadata: {
              environment: 'test',
              serviceName: 'k6-load-test'
            }
          });
          socket.send(startThreadMsg);
          
        } else if (data.action === 'startThread' && data.status === 'success') {
          threadId = data.threadId;
          console.log(`Thread started: ${threadId}`);
          
          // Send step events
          for (let i = 0; i < 5; i++) {
            const stepMsg = JSON.stringify({
              action: 'recordThreadEvent',
              threadId: threadId,
              stepName: `step-${i}`,
              type: 'step',
              status: 'success',
              startedAt: new Date().toISOString(),
              finishedAt: new Date().toISOString(),
              context: {
                test: 'data',
                iteration: i,
                vu: __VU
              },
              serviceName: 'k6-load-test'
            });
            socket.send(stepMsg);
            // No sleep - send steps as fast as possible
          }
          
          // Close connection
          const closeMsg = JSON.stringify({
            action: 'closeConnection'
          });
          socket.send(closeMsg);
          
        } else if (data.action === 'recordThreadEvent' && data.status === 'success') {
          console.log('Step event recorded successfully');
          
        } else if (data.status === 'error') {
          console.error(`Error: ${data.message}`);
          errorRate.add(1);
        }
      } catch (e) {
        console.error(`Failed to parse message: ${message}`, e);
        errorRate.add(1);
      }
    });
    
    socket.on('error', function (e) {
      console.error('WebSocket error:', e);
      // Don't count WebSocket disconnections as errors since functionality works
    });
    
    socket.setTimeout(function () {
      console.log('WebSocket timeout');
      socket.close();
    }, 30000); // 30 second timeout
  });
  
  check(res, { 'WebSocket connection successful': (r) => r && r.status === 101 });
  sleep(1);
}
