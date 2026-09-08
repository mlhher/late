#!/bin/sh
# onToolCall hook script
# Receives JSON payload: {"tool": "...", "arguments": {...}, "timestamp": "..."}
# Return "blocked" to veto the tool call.
# Return valid JSON to mutate arguments.
# Return empty to pass through unchanged.
payload=$(cat)
case "$payload" in
  *"\"block_me\":true"*|*"\"block_me\": true"*)
    echo "blocked"
    ;;
  *)
    # Pass through unchanged
    ;;
esac
