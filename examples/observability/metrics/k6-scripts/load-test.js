import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
let pingCounter = new Counter('ping_requests');
let errorRate = new Rate('error_rate');
let responseTime = new Trend('custom_response_time');

export let options = {
  scenarios: {
    // Scenario 1: Constant load
    constant_load: {
      executor: 'constant-vus',
      vus: 5,
      duration: '5m',
    },
    // Scenario 2: Spike testing
    spike_test: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '1m', target: 1 },
        { duration: '30s', target: 20 }, // Spike
        { duration: '1m', target: 1 },
      ],
      startTime: '5m', // Start after constant load
    },
    // Scenario 3: Stress testing
    stress_test: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '2m', target: 10 },
        { duration: '2m', target: 15 },
        { duration: '2m', target: 20 },
        { duration: '2m', target: 0 },
      ],
      startTime: '7m', // Start after spike test
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<500'], // 95% of requests should complete below 500ms
    http_req_failed: ['rate<0.05'],   // Error rate should be below 5%
    error_rate: ['rate<0.1'],         // Custom error rate threshold
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';

// Sample tenants and users for multi-tenant testing
const tenants = ['tenant-1', 'tenant-2', 'tenant-3'];
const users = ['user-1', 'user-2', 'user-3', 'user-4', 'user-5'];

// Mock JWT token for testing (in real scenario, this would be obtained from auth)
const mockJWT = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyLCJ0ZW5hbnRfaWQiOiJ0ZW5hbnQtMSIsInVzZXJfaWQiOiJ1c2VyLTEifQ.fakesignature';

export default function() {
  // Select random tenant and user
  const tenant = tenants[Math.floor(Math.random() * tenants.length)];
  const user = users[Math.floor(Math.random() * users.length)];
  
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${mockJWT}`,
      'X-Tenant-ID': tenant,
      'X-User-ID': user,
    },
  };

  let response;
  let success;

  // Test 1: Health check (public endpoint)
  response = http.get(`${BASE_URL}/health`, params);
  success = check(response, {
    'health check status is 200': (r) => r.status === 200,
    'health check response time < 100ms': (r) => r.timings.duration < 100,
  });
  
  if (!success) {
    errorRate.add(1);
  }

  sleep(0.5);

  // Test 2: Ping endpoint
  response = http.get(`${BASE_URL}/api/v1/ping`, params);
  success = check(response, {
    'ping status is 200': (r) => r.status === 200,
    'ping contains pong': (r) => r.body.includes('pong'),
  });
  
  pingCounter.add(1);
  responseTime.add(response.timings.duration);
  
  if (!success) {
    errorRate.add(1);
  }

  sleep(0.3);

  // Test 3: Create example (protected endpoint)
  const createPayload = JSON.stringify({
    name: `Example-${__VU}-${__ITER}`,
    status: Math.random() > 0.8 ? 'inactive' : 'active'
  });

  response = http.post(`${BASE_URL}/api/v1/examples`, createPayload, params);
  success = check(response, {
    'create example status is 201': (r) => r.status === 201,
    'create example has ID': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.id !== undefined;
      } catch (e) {
        return false;
      }
    }
  });

  if (!success) {
    errorRate.add(1);
  }

  let exampleId = null;
  try {
    exampleId = JSON.parse(response.body).id;
  } catch (e) {
    console.log('Failed to parse create response');
  }

  sleep(0.2);

  // Test 4: List examples (protected endpoint)
  response = http.get(`${BASE_URL}/api/v1/examples?page=1&limit=10`, params);
  success = check(response, {
    'list examples status is 200': (r) => r.status === 200,
    'list examples has data': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.data !== undefined;
      } catch (e) {
        return false;
      }
    }
  });

  if (!success) {
    errorRate.add(1);
  }

  sleep(0.2);

  // Test 5: Get specific example (if we have an ID)
  if (exampleId) {
    response = http.get(`${BASE_URL}/api/v1/examples/${exampleId}`, params);
    success = check(response, {
      'get example status is 200': (r) => r.status === 200,
      'get example has correct ID': (r) => {
        try {
          const json = JSON.parse(r.body);
          return json.id == exampleId;
        } catch (e) {
          return false;
        }
      }
    });

    if (!success) {
      errorRate.add(1);
    }

    sleep(0.2);
  }

  // Test 6: Simulate business events (occasionally)
  if (Math.random() > 0.7) {
    response = http.post(`${BASE_URL}/api/v1/simulate-events`, '{}', params);
    success = check(response, {
      'simulate events status is 200': (r) => r.status === 200,
    });

    if (!success) {
      errorRate.add(1);
    }
  }

  // Test 7: Check metrics endpoint (occasionally)
  if (Math.random() > 0.9) {
    response = http.get(`${BASE_URL}/metrics`);
    check(response, {
      'metrics endpoint is accessible': (r) => r.status === 200,
      'metrics contains prometheus format': (r) => r.body.includes('# HELP'),
    });
  }

  // Vary sleep time to simulate realistic user behavior
  sleep(Math.random() * 2 + 0.5);
}

export function handleSummary(data) {
  return {
    'load-test-results.json': JSON.stringify(data, null, 2),
  };
}