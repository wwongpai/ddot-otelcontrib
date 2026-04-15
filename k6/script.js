import http from 'k6/http';
import { sleep, check } from 'k6';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const options = {
  vus: 5,
  duration: '999h',
};

const TARGET_URL = __ENV.TARGET_URL || 'http://localhost:8080';

const accounts = ['ACC001', 'ACC002', 'ACC003', 'ACC004', 'ACC005'];
const invalidAccounts = ['ACC999', 'INVALID', 'NOTEXIST'];

export default function () {
  const scenario = Math.random();

  if (scenario < 0.6) {
    // POST /transfer — triggers full service chain
    const fromAccount = accounts[randomIntBetween(0, accounts.length - 1)];
    let toAccount = accounts[randomIntBetween(0, accounts.length - 1)];
    while (toAccount === fromAccount) {
      toAccount = accounts[randomIntBetween(0, accounts.length - 1)];
    }
    const useInvalid = Math.random() < 0.1;
    const finalFrom = useInvalid ? invalidAccounts[randomIntBetween(0, invalidAccounts.length - 1)] : fromAccount;
    const amount = randomIntBetween(100, 50000);

    const payload = JSON.stringify({
      fromAccount: finalFrom,
      toAccount: toAccount,
      amount: amount,
      currency: 'THB',
    });

    const res = http.post(`${TARGET_URL}/transfer`, payload, {
      headers: { 'Content-Type': 'application/json' },
    });
    check(res, {
      'transfer status 2xx or 4xx': (r) => r.status >= 200 && r.status < 500,
    });
  } else {
    // GET /accounts/{id}/balance — read path
    const accountId = Math.random() < 0.1
      ? invalidAccounts[randomIntBetween(0, invalidAccounts.length - 1)]
      : accounts[randomIntBetween(0, accounts.length - 1)];

    const res = http.get(`${TARGET_URL}/accounts/${accountId}/balance`);
    check(res, {
      'balance status 2xx or 4xx': (r) => r.status >= 200 && r.status < 500,
    });
  }

  sleep(randomIntBetween(1, 3));
}
