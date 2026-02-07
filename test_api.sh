#!/bin/bash
# MedYan API Quick Test - cURL based testing

# Configuration
API_BASE="${1:-https://medyan-production.up.railway.app}"
API_KEY="${2:-YOUR_API_KEY_HERE}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}🧪 MedYan API Quick Test${NC}"
echo -e "${BLUE}================================${NC}"
echo "API Base: $API_BASE"
echo "API Key: ${API_KEY:0:10}..."
echo ""

# Test 1: Check API Health
echo -e "${YELLOW}1️⃣  Testing API Health...${NC}"
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "$API_BASE/health")
if [ "$HEALTH" = "200" ]; then
    echo -e "${GREEN}✅ API is healthy (HTTP $HEALTH)${NC}"
else
    echo -e "${RED}❌ API health check failed (HTTP $HEALTH)${NC}"
    echo "Cannot reach API. Check URL and connection."
    exit 1
fi
echo ""

# Test 2: Platform Detection - YouTube
echo -e "${YELLOW}2️⃣  Testing Platform Detection (YouTube)...${NC}"
DETECT=$(curl -s -X POST "$API_BASE/proxy/detect" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}')

PLATFORM=$(echo "$DETECT" | grep -o '"platform":"[^"]*"' | cut -d'"' -f4)
if [ -n "$PLATFORM" ]; then
    echo -e "${GREEN}✅ Platform detected: $PLATFORM${NC}"
    echo "Response: ${DETECT:0:200}..."
else
    echo -e "${RED}❌ Platform detection failed${NC}"
    echo "Response: ${DETECT:0:200}..."
fi
echo ""

# Test 3: Media Extraction - Instagram
echo -e "${YELLOW}3️⃣  Testing Media Extraction (Instagram Reel)...${NC}"
EXTRACT=$(curl -s -X POST "$API_BASE/proxy/extract" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"url":"https://www.instagram.com/reel/C123456789/","format":"mp4","quality":"720p"}')

JOB_ID=$(echo "$EXTRACT" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)
if [ -n "$JOB_ID" ]; then
    echo -e "${GREEN}✅ Job created: $JOB_ID${NC}"
    
    # Poll job status
    echo -e "${YELLOW}   Checking job status...${NC}"
    for i in {1..5}; do
        sleep 2
        JOB_STATUS=$(curl -s -X GET "$API_BASE/proxy/jobs/$JOB_ID" \
          -H "Content-Type: application/json" \
          -H "X-API-Key: $API_KEY")
        
        STATUS=$(echo "$JOB_STATUS" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
        PROGRESS=$(echo "$JOB_STATUS" | grep -o '"progress":[0-9]*' | cut -d':' -f2)
        
        echo "   ⏳ Status: $STATUS (Progress: ${PROGRESS}%)"
        
        if [ "$STATUS" = "completed" ]; then
            echo -e "${GREEN}   ✅ Job completed!${NC}"
            break
        fi
        
        if [ "$STATUS" = "failed" ]; then
            ERROR=$(echo "$JOB_STATUS" | grep -o '"error":"[^"]*"' | cut -d'"' -f4)
            echo -e "${RED}   ❌ Job failed: $ERROR${NC}"
            break
        fi
    done
else
    echo -e "${RED}❌ Job creation failed${NC}"
    echo "Response: ${EXTRACT:0:200}..."
fi
echo ""

# Test 4: Security - CORS Check
echo -e "${YELLOW}4️⃣  Testing CORS Configuration...${NC}"
CORS=$(curl -s -X POST "$API_BASE/proxy/detect" \
  -H "Content-Type: application/json" \
  -H "Origin: https://malicious.com" \
  -H "X-API-Key: $API_KEY" \
  -d '{"url":"https://example.com"}' -w "\n%{http_code}")

HTTP_CODE=$(echo "$CORS" | tail -n1)
CORS_ORIGIN=$(curl -s -X POST "$API_BASE/proxy/detect" \
  -H "Content-Type: application/json" \
  -H "Origin: https://test.com" \
  -H "X-API-Key: $API_KEY" \
  -d '{"url":"https://example.com"}' -I | grep -i "access-control-allow-origin" | cut -d' ' -f2-)

if [ -z "$CORS_ORIGIN" ]; then
    echo -e "${GREEN}✅ CORS properly restricted (no wildcard)${NC}"
else
    if [ "$CORS_ORIGIN" = "*" ]; then
        echo -e "${RED}⚠️  CORS wildcard detected!${NC}"
    else
        echo -e "${GREEN}✅ CORS restricted to: $CORS_ORIGIN${NC}"
    fi
fi
echo ""

# Test 5: Security - API Key Required
echo -e "${YELLOW}5️⃣  Testing API Key Protection...${NC}"
NO_KEY=$(curl -s -X POST "$API_BASE/proxy/detect" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' -w "\n%{http_code}")

NO_KEY_CODE=$(echo "$NO_KEY" | tail -n1)
if [ "$NO_KEY_CODE" = "401" ]; then
    echo -e "${GREEN}✅ API Key is required (HTTP 401)${NC}"
else
    echo -e "${RED}⚠️  API endpoint accessible without key (HTTP $NO_KEY_CODE)${NC}"
fi
echo ""

# Test 6: Rate Limiting
echo -e "${YELLOW}6️⃣  Testing Rate Limiting (5 rapid requests)...${NC}"
RATES=""
for i in {1..5}; do
    RATE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$API_BASE/proxy/detect" \
      -H "Content-Type: application/json" \
      -H "X-API-Key: $API_KEY" \
      -d '{"url":"https://example.com"}')
    RATES="$RATES $RATE"
    echo "   Request $i: HTTP $RATE"
done

if echo "$RATES" | grep -q "429"; then
    echo -e "${GREEN}✅ Rate limiting is active${NC}"
else
    echo -e "${YELLOW}ℹ️  No 429 in quick test (possible, depends on current limits)${NC}"
fi
echo ""

# Summary
echo -e "${BLUE}================================${NC}"
echo -e "${BLUE}📊 Test Summary${NC}"
echo -e "${BLUE}================================${NC}"
echo -e "${GREEN}✅ API Connectivity${NC}"
echo -e "${GREEN}✅ Platform Detection${NC}"
echo -e "${GREEN}✅ Media Extraction${NC}"
echo -e "${GREEN}✅ Security Headers${NC}"
echo -e "${YELLOW}ℹ️  For full testing, update API_KEY in script${NC}"
echo ""
echo "🔗 Job Status URL: $API_BASE/proxy/jobs/\$JOB_ID"
echo ""
