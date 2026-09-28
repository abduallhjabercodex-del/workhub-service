// ==============================================================================
// WorkHub Declarative k6 Load Test Script
// Usage: k6 run scripts/k6_load_test.js
// Or: docker run --net=host -i grafana/k6 run - < scripts/k6_load_test.js
// Note: scripts/load_test.sh is the self-contained runner for local execution.
// ==============================================================================

import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
    stages: [
        { duration: '5s', target: 20 },  // Ramp-up to 20 VUs
        { duration: '15s', target: 50 }, // Sustained load at 50 VUs
        { duration: '5s', target: 0 },   // Ramp-down
    ],
    thresholds: {
        http_req_duration: ['p(95)<500'], // 95% of requests must complete below 500ms
        http_req_failed: ['rate<0.01'],    // Error rate must be less than 1%
    },
};

export default function () {
    const res = http.get(__ENV.TARGET_URL || 'http://localhost:8090/health');
    check(res, {
        'status is 200': (r) => r.status === 200,
        'healthy status': (r) => r.json().status === 'ok',
    });
    sleep(0.05);
}
