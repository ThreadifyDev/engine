# 🧪 Notification System Test Guide

## ✅ Services Running

All services are now running and ready for testing:

- ✅ **NATS Server** - Running on port 4222
- ✅ **PostgreSQL** - Running on port 5432
- ✅ **Valkey/Redis** - Running on port 6379
- ✅ **Threadify Server** - Running on port 8081
- ✅ **Archiver Service** - Running and consuming streams

---

## 🌐 Test Interface

**File:** `test-notifications.html` (should be open in your browser)

**URL:** `file:///Users/martins2/Downloads/ThreadifyEngine/test-notifications.html`

If not open, run:
```bash
open test-notifications.html
```

---

## 📋 Testing Steps

### 1. Connect to WebSocket
- Click the **"Connect"** button
- Wait for status to show "Connected" (green)
- Console log should show: `✅ Authenticated as: {owner_id}`

### 2. Start a Thread
- Click **"Start Thread"** button
- Wait for thread creation
- Console log should show: `✅ Thread started: {thread_id}`
- Console log should show: `🔔 Subscribed to notifications for thread`

### 3. Trigger Notifications
- Click **"Record Step (Trigger Notification)"** button
- This records a step event which triggers validation
- Watch for notifications to appear in real-time!

---

## 🎯 Expected Notifications

You should see notifications for:

1. **Step Recorded** - When step is successfully recorded
2. **Validation Warnings** - If any validation rules are violated
3. **Step Timeout** - If step takes too long (if configured)
4. **Invalid Transitions** - If step order is wrong
5. **Missing Optional Fields** - If optional fields are missing

---

## 📊 What to Watch

### In the Browser:
- **Status Bar** - Shows connection status and thread ID
- **Notifications Section** - Real-time notifications appear here
- **Console Log** - Shows all WebSocket messages

### In Server Logs (Terminal):
Look for these messages:
```
NATS publisher and consumer initialized successfully
[WS-NOTIFICATION] Subscribed user {id} to thread {id} notifications (scope: owner)
[NATS-PUBLISHER] Publishing notification to thread.{id}.owner
[CONSUMER] Delivered notification {id} to {threadId}:{userId}
[WS-NOTIFICATION] Sent notification {id} to user {id}
```

---

## 🔍 Debugging

### No Notifications Appearing?

**Check 1: WebSocket Connected?**
```
Status should be green "Connected"
```

**Check 2: Thread Started?**
```
Thread ID should appear in status bar
```

**Check 3: Server Logs**
```bash
# In the terminal running the server, look for:
[WS-NOTIFICATION] Subscribed user...
[NATS-PUBLISHER] Publishing notification...
```

**Check 4: NATS Running?**
```bash
docker ps | grep nats
# Should show: threadify-nats running
```

**Check 5: Browser Console**
```
Open browser DevTools (F12)
Check Console tab for errors
```

---

## 🛠️ Manual Testing Commands

### Check NATS Stream
```bash
# Install NATS CLI if needed
brew install nats-io/nats-tools/nats

# View stream
nats stream view NOTIFICATIONS --follow

# Check consumers
nats consumer ls NOTIFICATIONS
```

### Check Server Health
```bash
curl http://localhost:8081/health
```

### Restart Services
```bash
# Restart Docker services
docker-compose restart nats threadify-queue threadify_storage

# Restart server (Ctrl+C in terminal, then):
cd threadify-go
go run ./cmd/server

# Restart archiver (Ctrl+C in terminal, then):
cd threadify-go
go run ./cmd/archiver
```

---

## 📝 Sample Notification JSON

When a notification is received, it looks like:

```json
{
  "action": "notification",
  "notification": {
    "notification_id": "notif-abc123",
    "notification_type": "validation_warning",
    "severity": "warning",
    "message": "Step timeout detected",
    "thread_id": "thread-xyz789",
    "step_name": "test_step_1234567890",
    "timestamp": "2026-01-06T09:47:00Z",
    "details": "Additional context here"
  }
}
```

---

## 🎨 Notification Types

The test interface will show different colors based on severity:

- 🔴 **Critical** - Red border
- ❌ **Error** - Red border
- ⚠️ **Warning** - Orange border
- ℹ️ **Info** - Blue border
- ✅ **Success** - Green border

---

## 🔄 Test Scenarios

### Scenario 1: Basic Flow
1. Connect → Start Thread → Record Step
2. Should see at least 1 notification

### Scenario 2: Multiple Steps
1. Connect → Start Thread
2. Click "Record Step" multiple times
3. Should see multiple notifications

### Scenario 3: Reconnection
1. Connect → Start Thread → Record Step
2. Refresh browser page
3. Connect again → Start new thread
4. Should work without issues

---

## 📊 Success Criteria

✅ **Test Passes If:**
- WebSocket connects successfully
- Thread starts successfully
- Notifications appear in real-time after recording steps
- Notifications show correct data (ID, type, message, timestamp)
- Console logs show subscription and delivery messages

❌ **Test Fails If:**
- Cannot connect to WebSocket
- Thread creation fails
- No notifications appear after recording steps
- Errors in browser console or server logs

---

## 🚀 Next Steps After Testing

Once notifications are working:

1. **Test with Real Contracts** - Upload actual contract and test with real steps
2. **Test Multiple Clients** - Open test page in multiple browser tabs
3. **Test Validation Rules** - Trigger different validation scenarios
4. **Performance Testing** - Record many steps quickly
5. **Phase 6** - Implement client ACK and position tracking (optional)

---

## 📞 Quick Reference

**Test File:** `test-notifications.html`  
**Server Port:** 8081  
**WebSocket URL:** `ws://localhost:8080/threads`  
**Server Logs:** Terminal running `go run ./cmd/server`  
**Archiver Logs:** Terminal running `go run ./cmd/archiver`  

---

## 🎉 Happy Testing!

The notification system is fully operational. Enjoy testing real-time notifications! 🚀
