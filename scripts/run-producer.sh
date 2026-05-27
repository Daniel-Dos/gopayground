#!/usr/bin/env bash
#
# run-producer.sh
#
# Runs the Kafka payment-event producer locally (outside Docker) for
# development / testing.  By default it connects to Kafka on localhost:9092
# and publishes 10 events at 2 events/second.
#
# Usage:
#   ./scripts/run-producer.sh                          # defaults
#   ./scripts/run-producer.sh --count 5                # override count
#   ./scripts/run-producer.sh --rate 10 --brokers kafka:9092
#   COUNT=100 RATE=5 ./scripts/run-producer.sh         # env vars
#
# All flags are forwarded as-is to "go run ./cmd/producer publish".
# See "go run ./cmd/producer publish --help" for the full flag list.
#
# Environment variable overrides (applied before CLI flags):
#   KAFKA_BROKERS   default: localhost:9092
#   PUBLISH_COUNT   default: 10
#   PUBLISH_RATE    default: 2
#

set -euo pipefail

# -------- Project root (parent of the scripts/ directory) --------
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

# -------- Defaults (overridable via environment) --------
BROKERS="${KAFKA_BROKERS:-localhost:9092}"
COUNT="${PUBLISH_COUNT:-10}"
RATE="${PUBLISH_RATE:-2}"

# -------- Colour helpers (disabled when not in a terminal) --------
if [[ -t 1 ]]; then
  BOLD='\033[1m'
  DIM='\033[2m'
  GREEN='\033[0;32m'
  NC='\033[0m'  # No Colour
else
  BOLD=''
  DIM=''
  GREEN=''
  NC=''
fi

# -------- Helper: check whether a flag was already passed by the user --------
# Returns 0 (true) if the given flag name appears in the remaining arguments.
# Supports both --flag VALUE and --flag=VALUE forms.
flag_already_set() {
  local needle="$1"
  for arg in "${user_args[@]}"; do
    if [[ "$arg" == "$needle" ]] || [[ "$arg" == "$needle="* ]]; then
      return 0
    fi
  done
  return 1
}

# -------- Build the flag list --------
# Capture all user-supplied arguments first.
user_args=("$@")

# Start with user-supplied flags (they take precedence when placed first).
FLAGS=("$@")

# Append defaults ONLY for flags the user did NOT explicitly provide.
# This prevents Go's flag package (which honours the last occurrence)
# from silently overriding the user's intent.
if ! flag_already_set "--brokers"; then
  FLAGS+=(--brokers "$BROKERS")
fi
if ! flag_already_set "--count"; then
  FLAGS+=(--count "$COUNT")
fi
if ! flag_already_set "--rate"; then
  FLAGS+=(--rate "$RATE")
fi

# -------- Summary --------
echo -e "${BOLD}Kafka Payment Producer${NC}"
echo -e "${DIM}Command:${NC} go run ./cmd/producer publish ${FLAGS[*]}"
echo

# -------- Run --------
cd "$ROOT_DIR"

if [[ "${DRY_RUN:-}" == "1" ]]; then
  echo -e "${GREEN}[dry-run]${NC} Would have executed:"
  echo "  go run ./cmd/producer publish ${FLAGS[*]}"
  exit 0
fi

exec go run ./cmd/producer publish "${FLAGS[@]}"
