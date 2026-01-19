#!/bin/bash

# HPA Scenario Test Script
# 
# This script simulates HPA (Horizontal Pod Autoscaling) scenarios
# by running multiple consumers and a publisher to verify that
# notifications are delivered to only ONE consumer (no duplicates).

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
NUM_CONSUMERS=${NUM_CONSUMERS:-3}
NOTIFICATION_COUNT=${NOTIFICATION_COUNT:-5}
STEP_NAME=${STEP_NAME:-order_placed}

echo -e "${CYAN}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║  HPA Scenario Test                                    ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo -e "${BLUE}Configuration:${NC}"
echo "  • Number of consumers (pods): $NUM_CONSUMERS"
echo "  • Notifications to publish: $NOTIFICATION_COUNT"
echo "  • Step name: $STEP_NAME"
echo ""

# Cleanup function
cleanup() {
    echo -e "\n${YELLOW}Cleaning up...${NC}"
    
    # Kill all background jobs
    jobs -p | xargs -r kill 2>/dev/null || true
    
    # Wait a bit for processes to terminate
    sleep 1
    
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set trap to cleanup on exit
trap cleanup EXIT INT TERM

# Start consumers
echo -e "${CYAN}Starting $NUM_CONSUMERS consumers...${NC}"
for i in $(seq 1 $NUM_CONSUMERS); do
    CONSUMER_ID="consumer-$i"
    LOG_FILE="/tmp/consumer-$i.log"
    
    echo -e "${BLUE}  • Starting consumer $i (ID: $CONSUMER_ID)${NC}"
    
    CONSUMER_ID=$CONSUMER_ID node test-consumer.js > "$LOG_FILE" 2>&1 &
    CONSUMER_PID=$!
    
    echo "    PID: $CONSUMER_PID, Log: $LOG_FILE"
    
    # Give it a moment to connect
    sleep 0.5
done

echo -e "${GREEN}✅ All consumers started${NC}\n"

# Wait for consumers to be ready
echo -e "${YELLOW}Waiting 3 seconds for consumers to be ready...${NC}"
sleep 3

# Start publisher
echo -e "${CYAN}Starting publisher...${NC}"
NOTIFICATION_COUNT=$NOTIFICATION_COUNT STEP_NAME=$STEP_NAME node test-publisher.js

echo -e "${GREEN}✅ Publisher completed${NC}\n"

# Wait for notifications to be processed
echo -e "${YELLOW}Waiting 3 seconds for notifications to be processed...${NC}"
sleep 3

# Analyze results
echo -e "${CYAN}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║  Test Results                                         ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"

echo -e "${BLUE}Consumer logs:${NC}\n"

# Collect all notification IDs and thread IDs
ALL_NOTIFICATIONS_FILE="/tmp/all_notifications.txt"
> "$ALL_NOTIFICATIONS_FILE"

total_notifications=0
for i in $(seq 1 $NUM_CONSUMERS); do
    LOG_FILE="/tmp/consumer-$i.log"
    
    if [ -f "$LOG_FILE" ]; then
        count=$(grep -c "Notification received:" "$LOG_FILE" || echo "0")
        total_notifications=$((total_notifications + count))
        
        echo -e "${BLUE}Consumer $i:${NC}"
        echo "  • Notifications received: $count"
        
        # Extract and show all notifications with IDs
        if [ "$count" -gt 0 ]; then
            echo "  • Notifications:"
            grep "Notification received:" "$LOG_FILE" | while read -r line; do
                # Extract thread ID from the line
                thread_id=$(echo "$line" | grep -oE 'thread: [a-f0-9-]+' | cut -d' ' -f2)
                echo "    - Thread: $thread_id"
                
                # Add to global list for duplicate detection
                echo "$thread_id" >> "$ALL_NOTIFICATIONS_FILE"
            done
        fi
        echo ""
    fi
done

# Detect duplicates
echo -e "${CYAN}Duplicate Detection:${NC}"
unique_threads=$(sort "$ALL_NOTIFICATIONS_FILE" | uniq | wc -l | tr -d ' ')
duplicate_count=$((total_notifications - unique_threads))

echo -e "${BLUE}  • Unique thread IDs: ${NC}$unique_threads"
echo -e "${BLUE}  • Total notifications: ${NC}$total_notifications"

if [ "$duplicate_count" -gt 0 ]; then
    echo -e "${RED}  • Duplicates detected: ${NC}$duplicate_count"
    echo ""
    echo -e "${RED}Duplicate thread IDs:${NC}"
    sort "$ALL_NOTIFICATIONS_FILE" | uniq -d | while read -r dup_thread; do
        dup_count=$(grep -c "$dup_thread" "$ALL_NOTIFICATIONS_FILE")
        echo -e "${RED}  • $dup_thread (received $dup_count times)${NC}"
    done
else
    echo -e "${GREEN}  • Duplicates detected: ${NC}0"
fi
echo ""

# Verify results
echo -e "${CYAN}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║  Verification                                         ║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════════════════╝${NC}\n"

echo -e "${BLUE}Expected:${NC} $NOTIFICATION_COUNT notifications total"
echo -e "${BLUE}Actual:${NC} $total_notifications notifications received"
echo -e "${BLUE}Unique:${NC} $unique_threads unique thread IDs"
echo ""

if [ "$total_notifications" -eq "$NOTIFICATION_COUNT" ] && [ "$duplicate_count" -eq 0 ]; then
    echo -e "${GREEN}✅ SUCCESS: Each notification delivered to exactly ONE consumer!${NC}"
    echo -e "${GREEN}   No duplicate processing detected.${NC}"
    exit 0
elif [ "$duplicate_count" -gt 0 ]; then
    echo -e "${RED}❌ FAILURE: Duplicate processing detected!${NC}"
    echo -e "${RED}   Expected $NOTIFICATION_COUNT unique but got $unique_threads unique${NC}"
    echo -e "${RED}   $duplicate_count notification(s) were processed by multiple consumers.${NC}"
    exit 1
elif [ "$total_notifications" -lt "$NOTIFICATION_COUNT" ]; then
    echo -e "${YELLOW}⚠️  WARNING: Some notifications were not received!${NC}"
    echo -e "${YELLOW}   Expected $NOTIFICATION_COUNT but got $total_notifications${NC}"
    echo -e "${YELLOW}   Check consumer logs for errors.${NC}"
    exit 1
else
    echo -e "${RED}❌ FAILURE: Unexpected result${NC}"
    exit 1
fi
