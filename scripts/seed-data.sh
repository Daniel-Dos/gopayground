#!/usr/bin/env bash
#
# seed-data.sh
#
# Popula o Kafka com dados REALISTAS de pagamento (não aleatórios).
# Útil para demonstrar o sistema com dados variados:
#   - diferentes status (confirmed, failed, pending, refunded)
#   - valores variados (pequenos, médios, grandes)
#   - múltiplas moedas (BRL, USD, EUR)
#   - descrições em português
#
# Usage:
#   ./scripts/seed-data.sh                              # usa localhost:9092
#   ./scripts/seed-data.sh --brokers kafka:9092         # broker customizado
#   KAFKA_BROKERS=kafka:9092 ./scripts/seed-data.sh     # via env var
#   ./scripts/seed-data.sh --dry-run                    # preview sem publicar
#
# O script é idempotente: cada evento tem um UUID único,
# portanto pode ser executado múltiplas vezes sem duplicação visível
# (cada evento é único no Kafka).
#
# Dependências:
#   - Go (para compilar/executar o producer)
#   - scripts/seed-data.json (arquivo com os eventos realistas)
#

set -euo pipefail

# -------- Paths --------
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DATA_FILE="$SCRIPT_DIR/seed-data.json"

# -------- Defaults --------
BROKERS="${KAFKA_BROKERS:-localhost:9092}"
DRY_RUN=0

# -------- Colour helpers --------
if [[ -t 1 ]]; then
  BOLD='\033[1m'
  DIM='\033[2m'
  GREEN='\033[0;32m'
  YELLOW='\033[0;33m'
  CYAN='\033[0;36m'
  RED='\033[0;31m'
  NC='\033[0m'
else
  BOLD=''
  DIM=''
  GREEN=''
  YELLOW=''
  CYAN=''
  RED=''
  NC=''
fi

# -------- Parse CLI --------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --brokers)
      BROKERS="$2"
      shift 2
      ;;
    --brokers=*)
      BROKERS="${1#*=}"
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --help)
      sed -n '3,20p' "$0" | sed 's/^# //; s/^#$//'
      exit 0
      ;;
    *)
      echo -e "${RED}error:${NC} Unknown flag: $1"
      echo "Usage: $0 [--brokers HOST:PORT] [--dry-run]"
      exit 1
      ;;
  esac
done

# -------- Validate dependencies --------
if [[ ! -f "$DATA_FILE" ]]; then
  echo -e "${RED}error:${NC} Data file not found: $DATA_FILE"
  echo "      Ensure scripts/seed-data.json exists."
  exit 1
fi

# Validate JSON syntax
if ! jq empty "$DATA_FILE" 2>/dev/null; then
  # Fallback: try python3
  if ! python3 -c "import json; json.load(open('$DATA_FILE'))" 2>/dev/null; then
    echo -e "${YELLOW}warning:${NC} Could not validate JSON syntax (jq not found).
      Continuing anyway..."
  fi
fi

# Count events
EVENT_COUNT=$(jq 'length' "$DATA_FILE" 2>/dev/null || python3 -c "import json; print(len(json.load(open('$DATA_FILE'))))" 2>/dev/null || echo "?")
if [[ "$EVENT_COUNT" == "?" ]]; then
  EVENT_COUNT=$(grep -c '"payment_id"' "$DATA_FILE")
fi

# -------- Summary --------
echo -e "${BOLD}══════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}  Seed Data - Payment Events${NC}"
echo -e "${BOLD}══════════════════════════════════════════════════════${NC}"
echo -e "  ${DIM}Data file:${NC}   $DATA_FILE"
echo -e "  ${DIM}Events:${NC}      ${BOLD}$EVENT_COUNT${NC}"
echo -e "  ${DIM}Brokers:${NC}     $BROKERS"
echo -e "  ${DIM}Mode:${NC}        $([[ "$DRY_RUN" == 1 ]] && echo "${YELLOW}dry-run${NC}" || echo "${GREEN}publish${NC}")"
echo

# -------- Preview events --------
echo -e "${BOLD}Preview of events to be published:${NC}"
echo

if command -v jq &>/dev/null; then
  jq -r '.[] | "  \(.payment_id | .[0:8])... │ \(.status | .[0:8]) │ \(.amount) \(.currency) │ \(.description)"' "$DATA_FILE"
else
  # Fallback: simple Python formatting
  python3 -c "
import json
events = json.load(open('$DATA_FILE'))
for e in events:
    pid = e['payment_id'][:8]
    print(f\"  {pid}... │ {e['status']:<8} │ {e['amount']:>8.2f} {e['currency']} │ {e['description']}\")
" 2>/dev/null || {
    # Last resort: grep-based preview
    echo "  (install jq for a better preview)"
  }
fi

echo
echo -e "${DIM}Total:${NC} $EVENT_COUNT events"
echo

# -------- Dry-run: exit early --------
if [[ "$DRY_RUN" == 1 ]]; then
  echo -e "${YELLOW}[dry-run]${NC} Preview complete. Pass --dry-run to see events.
  Run without --dry-run to publish."
  exit 0
fi

# -------- Confirmation prompt --------
echo -e -n "${BOLD}Publish $EVENT_COUNT events to Kafka at ${CYAN}$BROKERS${NC}${BOLD}?${NC} [Y/n] "
read -r CONFIRM
if [[ ! "$CONFIRM" =~ ^[Yy]?$ ]]; then
  echo -e "${YELLOW}Aborted.${NC}"
  exit 0
fi
echo

# -------- Publish --------
echo -e "${BOLD}Publishing events...${NC}"

cd "$ROOT_DIR"

go run ./cmd/producer publish \
  --file "$DATA_FILE" \
  --brokers "$BROKERS" \
  --rate 0

EXIT_CODE=$?

echo

# -------- Result --------
if [[ "$EXIT_CODE" == 0 ]]; then
  echo -e "${GREEN}✓${NC} ${BOLD}Successfully published $EVENT_COUNT events.${NC}"
  echo
  echo -e "${BOLD}Next steps:${NC}"
  echo -e "  • ${DIM}View the UI:${NC}       ${CYAN}http://localhost:8081${NC}"
  echo -e "  • ${DIM}Check the consumer:${NC} go run ./cmd/consumer"
  echo -e "  • ${DIM}Publish more:${NC}       ./scripts/run-producer.sh --count 5"
  echo
  echo -e "${DIM}All events carry unique UUIDs — running this script again${NC}"
  echo -e "${DIM}will create additional unique events without conflict.${NC}"
else
  echo -e "${RED}✗${NC} ${BOLD}Publishing failed with exit code $EXIT_CODE.${NC}"
  echo "  Check Kafka connectivity and broker address."
fi

exit "$EXIT_CODE"
