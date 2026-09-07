#!/bin/sh
# Command handler script for /echo-cmd
# Receives JSON array of arguments on stdin
args=$(cat)
echo "echo-cmd received arguments: $args"
