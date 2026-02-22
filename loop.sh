#!/bin/bash
# loop.sh - Ralph Wiggum execution engine for gogcli API gap coverage
# Usage:
#   ./loop.sh           # Build mode, unlimited
#   ./loop.sh plan      # Plan mode (create IMPLEMENTATION_PLAN.md)
#   ./loop.sh 20        # Build mode, max 20 iterations
#   ./loop.sh plan 5    # Plan mode, max 5 iterations

set -e

MODE="${1:-build}"
MAX_ITERATIONS="${2:-999999}"
COOLDOWN_SECONDS="${COOLDOWN:-30}"  # Delay between iterations
RATE_LIMIT_WAIT="${RATE_LIMIT_WAIT:-60}"  # Initial wait on rate limit
MAX_RETRIES="${MAX_RETRIES:-3}"  # Max retries on rate limit

# Handle ./loop.sh 20 (number as first arg = build mode with limit)
if [[ "$MODE" =~ ^[0-9]+$ ]]; then
  MAX_ITERATIONS="$MODE"
  MODE="build"
fi

# Select prompt file based on mode
if [ "$MODE" = "plan" ]; then
  PROMPT_FILE="PROMPT_plan.md"
  AUTO_LEVEL="medium"
else
  PROMPT_FILE="PROMPT_build.md"
  AUTO_LEVEL="high"
fi

# Verify prompt file exists
if [ ! -f "$PROMPT_FILE" ]; then
  echo "❌ Error: $PROMPT_FILE not found"
  exit 1
fi

echo "🚀 Ralph Wiggum Starting — gogcli API Gap Coverage"
echo "   Mode: $MODE"
echo "   Max iterations: $MAX_ITERATIONS"
echo "   Prompt: $PROMPT_FILE"
echo "   Autonomy: $AUTO_LEVEL"
echo "   Cooldown: ${COOLDOWN_SECONDS}s between iterations"
echo "   Rate limit wait: ${RATE_LIMIT_WAIT}s initial"
echo "   Target: 588 missing API methods across 19 Google APIs"
echo "================================================"

for ((i=1; i<=MAX_ITERATIONS; i++)); do
  echo ""
  echo "═══════════════════════════════════════════════"
  echo "  Iteration $i of $MAX_ITERATIONS"
  echo "═══════════════════════════════════════════════"

  # Run droid exec with retry on rate limit
  retry_count=0
  wait_time=$RATE_LIMIT_WAIT
  
  while true; do
    result=$(droid exec --auto "$AUTO_LEVEL" -f "$PROMPT_FILE" 2>&1) || true
    
    # Check for rate limit errors
    if echo "$result" | grep -qiE "(429|rate limit|too many requests)"; then
      retry_count=$((retry_count + 1))
      if [ $retry_count -ge $MAX_RETRIES ]; then
        echo "❌ Max retries ($MAX_RETRIES) exceeded on rate limit"
        echo "   Consider increasing RATE_LIMIT_WAIT or switching models"
        exit 1
      fi
      echo "⏳ Rate limited (attempt $retry_count/$MAX_RETRIES). Waiting ${wait_time}s..."
      sleep $wait_time
      wait_time=$((wait_time * 2))  # Exponential backoff
      continue
    fi
    
    break
  done

  echo "$result"

  # Push after each iteration
  git push 2>/dev/null || true

  # Check for completion signal
  if [[ "$result" == *"<promise>COMPLETE</promise>"* ]]; then
    echo ""
    echo "✅ All tasks complete at iteration $i"
    exit 0
  fi

  echo "Iteration $i complete. Cooling down for ${COOLDOWN_SECONDS}s..."
  sleep $COOLDOWN_SECONDS
done

echo ""
echo "⚠️ Max iterations ($MAX_ITERATIONS) reached"
exit 1
