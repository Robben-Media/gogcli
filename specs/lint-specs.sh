#!/bin/bash
# lint-specs.sh — Validates spec files conform to CLI Design System
# Usage: ./specs/lint-specs.sh [spec-file]  (or run without args to lint all)
#
# Checks:
#   1. Required sections present
#   2. Command naming follows conventions
#   3. Global flags not redeclared
#   4. Pagination fields declared for list commands
#   5. Output schema present (JSON + text)
#   6. Delete commands mention confirmDestructive
#   7. Test requirements section exists

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ERRORS=0
WARNINGS=0
FILES_CHECKED=0

warn() {
  echo -e "${YELLOW}  WARN${NC} [$1]: $2"
  ((WARNINGS++))
}

fail() {
  echo -e "${RED}  FAIL${NC} [$1]: $2"
  ((ERRORS++))
}

pass() {
  echo -e "${GREEN}  PASS${NC} $1"
}

lint_spec() {
  local file="$1"
  local basename
  basename=$(basename "$file")
  echo ""
  echo "Linting: $basename"
  echo "─────────────────────────────────────"
  ((FILES_CHECKED++))

  local content
  content=$(cat "$file")

  # 1. Required sections
  for section in "Overview" "Missing Methods" "Test Requirements"; do
    if echo "$content" | grep -qi "## .*${section}"; then
      : # found
    elif echo "$content" | grep -qi "# .*${section}"; then
      : # found as H1
    else
      # Try partial match
      case "$section" in
        "Missing Methods")
          if echo "$content" | grep -qi "methods\|commands\|gaps"; then
            : # acceptable variant
          else
            fail "$basename" "Missing section: $section"
          fi
          ;;
        "Test Requirements")
          if echo "$content" | grep -qi "test\|testing\|verification"; then
            : # acceptable variant
          else
            fail "$basename" "Missing section: $section"
          fi
          ;;
        *)
          fail "$basename" "Missing section: $section"
          ;;
      esac
    fi
  done

  # 2. Command naming — check for uppercase commands or underscores in CLI names
  local bad_cmd
  bad_cmd=$(echo "$content" | grep -E 'gog[[:space:]]+[A-Z]' | grep -v '^#' | head -1 || true)
  if [ -n "$bad_cmd" ]; then
    warn "$basename" "Possible uppercase in CLI command: $(echo "$bad_cmd" | head -c 80)"
  fi

  # 3. Global flags not redeclared
  for flag in "--account" "--json" "--plain" "--force" "--no-input" "--verbose" "--color" "--client"; do
    local redeclared
    redeclared=$(echo "$content" | grep -c "declare\|add.*${flag}\|new.*flag.*${flag}" || true)
    if [ "$redeclared" -gt 0 ]; then
      warn "$basename" "Global flag $flag may be redeclared (inherited from RootFlags)"
    fi
  done

  # 4. Pagination for list commands
  if echo "$content" | grep -qi "\.list\|list command\|list "; then
    if ! echo "$content" | grep -qi "\-\-max\|max.*results\|pagination\|page.*token\|--page"; then
      warn "$basename" "List commands found but no pagination flags (--max, --page) mentioned"
    fi
  fi

  # 5. Output schema
  if ! echo "$content" | grep -qi "json\|output.*format\|outfmt"; then
    fail "$basename" "No output format/JSON schema mentioned"
  fi
  if ! echo "$content" | grep -qi "tsv\|table\|text.*output\|plain\|tableWriter"; then
    warn "$basename" "No text/TSV output format mentioned"
  fi

  # 6. Delete commands mention confirmation
  if echo "$content" | grep -qi "delete\|remove\|destroy\|purge"; then
    if ! echo "$content" | grep -qi "confirm\|destructive\|--force\|confirmation"; then
      warn "$basename" "Delete operations found but no confirmDestructive/--force mention"
    fi
  fi

  # 7. Test section
  if echo "$content" | grep -qi "test\|testing\|verification\|httptest"; then
    : # found
  else
    fail "$basename" "No testing/verification section"
  fi

  # 8. Check for service factory mention
  if ! echo "$content" | grep -qi "service\|factory\|newService\|new.*Service"; then
    warn "$basename" "No service factory pattern mentioned"
  fi

  # 9. Check for error handling mention
  if ! echo "$content" | grep -qi "error\|usage()\|validation"; then
    warn "$basename" "No error handling/validation mentioned"
  fi
}

# Main
SPEC_DIR="$(cd "$(dirname "$0")" && pwd)"

if [ $# -gt 0 ]; then
  # Lint specific file
  for f in "$@"; do
    if [ -f "$f" ]; then
      lint_spec "$f"
    else
      echo "File not found: $f"
      ((ERRORS++))
    fi
  done
else
  # Lint all specs
  for f in "$SPEC_DIR"/features/*.md; do
    if [ -f "$f" ]; then
      lint_spec "$f"
    fi
  done
fi

echo ""
echo "═══════════════════════════════════════════════"
echo "  Spec Lint Results"
echo "═══════════════════════════════════════════════"
echo "  Files checked: $FILES_CHECKED"
echo -e "  ${GREEN}Errors:${NC}   $ERRORS"
echo -e "  ${YELLOW}Warnings:${NC} $WARNINGS"

if [ "$ERRORS" -gt 0 ]; then
  echo -e "  ${RED}FAILED${NC} — fix errors before running ./loop.sh plan"
  exit 1
else
  if [ "$WARNINGS" -gt 0 ]; then
    echo -e "  ${YELLOW}PASSED with warnings${NC}"
  else
    echo -e "  ${GREEN}PASSED${NC}"
  fi
  exit 0
fi
