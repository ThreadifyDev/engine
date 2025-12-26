# Authentication Flow Test

## Mock API Keys Available

The mock authentication service provides these test API keys:

| API Key        | Owner ID   | Company ID   | Role       |
|----------------|------------|--------------|------------|
| `api-key-123`  | `user-123` | `company-abc` | `admin`    |
| `api-key-456`  | `user-456` | `company-xyz` | `user`     |
| `test-api-key` | `test-user`| `test-company`| `developer`|
| `demo-key`     | `demo-user`| `demo-company`| `user`     |

## Testing the Flow

1. **Connect with valid API key:**

```json
{
  "action": "connect",
  "apiKey": "api-key-123",
  "serviceName": "payment-service"
}
```

Expected response:

```json
{
  "action": "connect",
  "status": "success",
  "message": "Connected successfully",
  "ownerId": "user-123",
  "companyId": "company-abc",
  "subscribedEvents": []
}
```

1. **Connect with invalid API key:**

```json
{
  "action": "connect",
  "apiKey": "invalid-key",
  "serviceName": "payment-service"
}
```

Expected response:

```json
{
  "action": "connect",
  "status": "error",
  "message": "Invalid API key: invalid API key: invalid-key"
}
```

1. **Create workflow (after successful connect):**

```json
{
  "action": "startThread",
  "contractName": "payment-contract",
  "role": "processor"
}
```

This will create a workflow associated with `user-123` and `company-abc`.

1. **Cross-company permission test:**

- User from `company-abc` cannot update workflows created by users from `company-xyz`
- Each user can only create/update workflows within their own company
