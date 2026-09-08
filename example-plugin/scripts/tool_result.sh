#!/bin/sh
# onToolResult hook script
# Receives JSON payload: {"tool": "...", "result": "..."}
# Return "blocked" to veto the result.
# Return valid JSON to replace the result.
# Return empty to pass through unchanged.
payload=$(cat)
case "$payload" in
  *block_result*)
    echo "blocked"
    ;;
  *)
    # Pass through unchanged
    ;;
esac
