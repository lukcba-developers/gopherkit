import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// Custom metrics
let traceableOperations = new Counter('traceable_operations');
let distributedTraceRate = new Rate('distributed_trace_rate');
let spanDurationTrend = new Trend('span_duration_ms');

export let options = {
  scenarios: {
    // Scenario 1: Basic tracing load
    basic_tracing: {
      executor: 'constant-vus',
      vus: 3,
      duration: '3m',
    },
    // Scenario 2: Complex operations with nested spans
    complex_tracing: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '1m', target: 5 },
        { duration: '2m', target: 10 },
        { duration: '1m', target: 1 },
      ],
      startTime: '3m',
    },
    // Scenario 3: High-frequency operations
    high_frequency: {
      executor: 'constant-arrival-rate',
      rate: 50,
      timeUnit: '1m',
      duration: '2m',
      preAllocatedVUs: 5,
      maxVUs: 15,
      startTime: '7m',
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<800'], // 95% of requests should complete below 800ms
    http_req_failed: ['rate<0.02'],   // Error rate should be below 2%
    traceable_operations: ['count>100'], // Should perform at least 100 traceable operations
  },
};

const BASE_URL = __ENV.TARGET_URL || 'http://localhost:8080';

// Mock JWT token for testing
const mockJWT = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6Ik9UZWwgVGVzdCIsImlhdCI6MTUxNjIzOTAyMiwidGVuYW50X2lkIjoidGVuYW50LW90ZWwiLCJ1c2VyX2lkIjoidXNlci1vdGVsIn0.fakesignature';

const tenants = ['tenant-otel-1', 'tenant-otel-2', 'tenant-otel-3'];

export default function() {
  const tenant = tenants[Math.floor(Math.random() * tenants.length)];
  
  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${mockJWT}`,
      'X-Tenant-ID': tenant,
      'X-User-ID': 'user-otel-test',
      // Custom headers for trace correlation
      'X-Test-ID': `k6-test-${__VU}-${__ITER}`,
      'X-Scenario': 'load-testing',
    },
  };

  let response;
  let success;
  
  // Test 1: Basic ping with automatic HTTP tracing
  response = http.get(`${BASE_URL}/api/v1/ping`, params);
  success = check(response, {
    'ping status is 200': (r) => r.status === 200,
    'ping has trace_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.trace_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'ping response time < 100ms': (r) => r.timings.duration < 100,
  });
  
  if (success) {
    traceableOperations.add(1);
    distributedTraceRate.add(1);
    
    // Extract trace duration if available
    try {
      const json = JSON.parse(response.body);
      if (json.trace_id) {
        spanDurationTrend.add(response.timings.duration);
      }
    } catch (e) {}
  }

  sleep(0.3);

  // Test 2: Complex operation with nested spans
  response = http.get(`${BASE_URL}/api/v1/complex-operation`, params);
  success = check(response, {
    'complex operation status is 200': (r) => r.status === 200,
    'complex operation has trace_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.trace_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'complex operation has span_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.span_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'complex operation includes operations': (r) => {
      try {
        const json = JSON.parse(r.body);
        return Array.isArray(json.operations) && json.operations.length > 0;
      } catch (e) {
        return false;
      }
    }
  });

  if (success) {
    traceableOperations.add(1);
    distributedTraceRate.add(1);
    spanDurationTrend.add(response.timings.duration);
  }

  sleep(0.5);

  // Test 3: Authenticated operations with database and cache spans
  const createPayload = JSON.stringify({
    name: `TracedExample-${__VU}-${__ITER}`,
    status: Math.random() > 0.7 ? 'inactive' : 'active'
  });

  response = http.post(`${BASE_URL}/api/v1/traced-examples`, createPayload, params);
  success = check(response, {
    'create traced example status is 201': (r) => r.status === 201,
    'create has trace_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.trace_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'create has tracing message': (r) => r.body.includes('full distributed tracing'),
  });

  if (success) {
    traceableOperations.add(1);
    distributedTraceRate.add(1);
    spanDurationTrend.add(response.timings.duration);
  }

  sleep(0.2);

  // Test 4: List operation with cache tracing
  response = http.get(`${BASE_URL}/api/v1/traced-examples`, params);
  success = check(response, {
    'list traced examples status is 200': (r) => r.status === 200,
    'list has trace_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.trace_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'list has span_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.span_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'list indicates tracing': (r) => r.body.includes('full_distributed_tracing_enabled'),
  });

  if (success) {
    traceableOperations.add(1);
    distributedTraceRate.add(1);
    spanDurationTrend.add(response.timings.duration);
  }

  sleep(0.3);

  // Test 5: Trace propagation test
  response = http.post(`${BASE_URL}/api/v1/trace-propagation`, '{}', params);
  success = check(response, {
    'trace propagation status is 200': (r) => r.status === 200,
    'propagation has trace_id': (r) => {
      try {
        const json = JSON.parse(r.body);
        return json.trace_id !== undefined;
      } catch (e) {
        return false;
      }
    },
    'propagation mentions distributed trace': (r) => r.body.includes('distributed_trace'),
    'propagation simulates external call': (r) => r.body.includes('external-api'),
  });

  if (success) {
    traceableOperations.add(1);
    distributedTraceRate.add(1);
    spanDurationTrend.add(response.timings.duration);
  }

  sleep(0.2);

  // Test 6: OpenTelemetry metrics endpoint (occasionally)
  if (Math.random() > 0.8) {
    response = http.get(`${BASE_URL}/otel-metrics`, params);
    success = check(response, {
      'otel metrics status is 200': (r) => r.status === 200,
      'otel metrics has features': (r) => {
        try {
          const json = JSON.parse(r.body);
          return Array.isArray(json.features) && json.features.length > 0;
        } catch (e) {
          return false;
        }
      },
    });

    if (success) {
      traceableOperations.add(1);
    }
  }

  // Test 7: Health check with trace header propagation
  if (Math.random() > 0.9) {
    response = http.get(`${BASE_URL}/health`, params);
    check(response, {
      'health check is accessible': (r) => r.status === 200,
    });
  }

  // Vary sleep time to simulate realistic user behavior
  sleep(Math.random() * 1.5 + 0.5);
}

// Setup function to log test configuration
export function setup() {
  console.log('🔍 Starting OpenTelemetry Load Test');
  console.log('📊 Target URL:', BASE_URL);
  console.log('🎯 Testing distributed tracing capabilities');
  console.log('📈 Tracking span creation and propagation');
  
  // Test connectivity
  let response = http.get(`${BASE_URL}/health`);
  if (response.status !== 200) {
    throw new Error(`Service not available at ${BASE_URL}`);
  }
  
  console.log('✅ Service connectivity confirmed');
  return { target_url: BASE_URL };
}

// Teardown function to log results
export function teardown(data) {
  console.log('🏁 OpenTelemetry Load Test completed');
  console.log('📋 Check Jaeger UI at http://localhost:16686 for traces');
  console.log('📊 Check Grafana at http://localhost:3000 for metrics');
  console.log('🔍 Check Prometheus at http://localhost:9090 for raw metrics');
}

export function handleSummary(data) {
  return {
    'otel-load-test-results.json': JSON.stringify(data, null, 2),
    'stdout': `
📊 OpenTelemetry Load Test Summary:
=================================
• Total requests: ${data.metrics.http_reqs.values.count}
• Average response time: ${data.metrics.http_req_duration.values.avg.toFixed(2)}ms
• 95th percentile: ${data.metrics.http_req_duration.values['p(95)'].toFixed(2)}ms
• Error rate: ${(data.metrics.http_req_failed.values.rate * 100).toFixed(2)}%
• Traceable operations: ${data.metrics.traceable_operations ? data.metrics.traceable_operations.values.count : 'N/A'}
• Distributed trace rate: ${data.metrics.distributed_trace_rate ? (data.metrics.distributed_trace_rate.values.rate * 100).toFixed(2) + '%' : 'N/A'}

🔍 Tracing Infrastructure:
• Jaeger UI: http://localhost:16686
• Grafana: http://localhost:3000  
• Prometheus: http://localhost:9090
• OTEL Collector: http://localhost:8888

✨ Features Tested:
• HTTP request/response tracing
• Database operation spans
• Cache operation spans
• Business event tracking
• Trace context propagation
• Nested span creation
    `
  };
}