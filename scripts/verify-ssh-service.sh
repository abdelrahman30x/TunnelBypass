#!/bin/bash
# SSH Service Verification Script
# This script verifies that the SSH service is properly installed and running

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

SSH_SERVICE="TunnelBypass-SSH"
FORWARDER_SERVICE="TunnelBypass-SSH-Forwarder"
WSS_SERVICE="TunnelBypass-WSS"

echo "========================================"
echo "SSH Service Verification Script"
echo "========================================"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}Error: Please run as root (use sudo)${NC}"
    exit 1
fi

# Function to check service status
check_service() {
    local service=$1
    echo -n "Checking $service... "
    
    if systemctl is-active --quiet "$service"; then
        echo -e "${GREEN}ACTIVE${NC}"
        return 0
    else
        echo -e "${RED}INACTIVE${NC}"
        return 1
    fi
}

# Function to check if port is listening
check_port() {
    local port=$1
    local name=$2
    echo -n "Checking port $port ($name)... "
    
    if ss -tln | grep -q ":$port "; then
        echo -e "${GREEN}LISTENING${NC}"
        return 0
    else
        echo -e "${RED}NOT LISTENING${NC}"
        return 1
    fi
}

# 1. Check systemd unit file exists
echo "1. Checking systemd unit files..."
echo -n "   SSH service unit... "
if [ -f "/etc/systemd/system/${SSH_SERVICE}.service" ]; then
    echo -e "${GREEN}EXISTS${NC}"
else
    echo -e "${RED}MISSING${NC}"
    echo "   Run: sudo tunnelbypass wizard"
fi

# 2. Check service status
echo ""
echo "2. Checking service status..."
check_service "$SSH_SERVICE"
check_service "$FORWARDER_SERVICE"
check_service "$WSS_SERVICE"

# 3. Get port configuration
echo ""
echo "3. Reading port configuration..."
CONFIG_FILE="/opt/tunnelbypass/configs/ssh/ssh_ports.json"
if [ -f "$CONFIG_FILE" ]; then
    INTERNAL_PORT=$(grep -o '"internal_port":[0-9]*' "$CONFIG_FILE" | grep -o '[0-9]*')
    EXTERNAL_PORT=$(grep -o '"external_port":[0-9]*' "$CONFIG_FILE" | grep -o '[0-9]*')
    USERNAME=$(grep -o '"username":"[^"]*"' "$CONFIG_FILE" | cut -d'"' -f4)
    
    echo "   Internal Port: $INTERNAL_PORT"
    echo "   External Port: $EXTERNAL_PORT"
    echo "   Username: $USERNAME"
else
    echo -e "${RED}   Config file not found: $CONFIG_FILE${NC}"
    exit 1
fi

# 4. Check ports
echo ""
echo "4. Checking network ports..."
check_port "$INTERNAL_PORT" "SSH Internal"
check_port "$EXTERNAL_PORT" "SSH External"

# 5. Check logs
echo ""
echo "5. Recent logs from SSH service..."
echo "   (Last 5 lines)"
journalctl -u "$SSH_SERVICE" -n 5 --no-pager 2>/dev/null || echo "   No logs available"

# 6. Check auto-start status
echo ""
echo "6. Checking auto-start configuration..."
echo -n "   SSH service enabled for boot... "
if systemctl is-enabled --quiet "$SSH_SERVICE" 2>/dev/null; then
    echo -e "${GREEN}YES${NC}"
else
    echo -e "${YELLOW}NO${NC} (run: sudo systemctl enable $SSH_SERVICE)"
fi

# 7. Test SSH connection (optional)
echo ""
echo "7. Testing SSH connection..."
echo "   Attempting to connect to port $EXTERNAL_PORT..."

# Wait a moment for connection
if timeout 5 bash -c "echo >/dev/tcp/127.0.0.1/$EXTERNAL_PORT" 2>/dev/null; then
    echo -e "   ${GREEN}SUCCESS${NC} - SSH port is accepting connections"
else
    echo -e "   ${YELLOW}WARNING${NC} - Could not connect (may need password or key)"
fi

# Summary
echo ""
echo "========================================"
echo "Summary"
echo "========================================"

ALL_OK=true

if systemctl is-active --quiet "$SSH_SERVICE"; then
    echo -e "${GREEN}✓${NC} SSH service is active"
else
    echo -e "${RED}✗${NC} SSH service is NOT active"
    ALL_OK=false
fi

if ss -tln | grep -q ":$INTERNAL_PORT "; then
    echo -e "${GREEN}✓${NC} SSH is listening on internal port $INTERNAL_PORT"
else
    echo -e "${RED}✗${NC} SSH is NOT listening on internal port"
    ALL_OK=false
fi

if systemctl is-enabled --quiet "$SSH_SERVICE" 2>/dev/null; then
    echo -e "${GREEN}✓${NC} SSH service will auto-start on boot"
else
    echo -e "${YELLOW}⚠${NC} SSH service will NOT auto-start on boot"
fi

echo ""
if [ "$ALL_OK" = true ]; then
    echo -e "${GREEN}All checks passed! SSH service is properly configured.${NC}"
    echo ""
    echo "Management commands:"
    echo "  systemctl status $SSH_SERVICE"
    echo "  journalctl -u $SSH_SERVICE -f"
    echo "  systemctl restart $SSH_SERVICE"
    exit 0
else
    echo -e "${RED}Some checks failed. Please review the output above.${NC}"
    echo ""
    echo "Troubleshooting:"
    echo "  1. Check logs: journalctl -u $SSH_SERVICE -n 50"
    echo "  2. Restart service: systemctl restart $SSH_SERVICE"
    echo "  3. Re-run wizard: sudo tunnelbypass wizard"
    exit 1
fi
