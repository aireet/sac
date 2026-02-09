#!/bin/bash
# migrate-db.sh — Migrate data from old RDS to new RDS using COPY via psql.
#
# Uses explicit column names to handle column-order differences between old/new schemas.
#
# Prerequisites:
#   1. New DB already has tables created via `make migrate-up`
#   2. Both RDS instances are reachable from this machine (use Telepresence if needed)
#
# Usage:
#   ./scripts/migrate-db.sh

set -euo pipefail

# Old RDS (PG 18.1)
OLD_HOST="pgm-uf68x0dfyoth4u5g.pg.rds.aliyuncs.com"
OLD_PORT="5432"
OLD_DB="sandbox"
OLD_USER="sandbox"
OLD_PASS="4SOZfo6t6Oyj9A=="

# New RDS (PG 17.7 + TimescaleDB)
NEW_HOST="pgm-uf60863vpy60vl6s.pg.rds.aliyuncs.com"
NEW_PORT="5432"
NEW_DB="sandbox"
NEW_USER="sandbox"
NEW_PASS="4SOZfo6t6Oyj9A=="

OLD_CONN="postgresql://${OLD_USER}:${OLD_PASS}@${OLD_HOST}:${OLD_PORT}/${OLD_DB}?sslmode=disable"
NEW_CONN="postgresql://${NEW_USER}:${NEW_PASS}@${NEW_HOST}:${NEW_PORT}/${NEW_DB}?sslmode=disable"

echo "=== SAC Data Migration: Old RDS → New RDS ==="
echo ""

# Verify connections
echo "Verifying connection to old RDS..."
psql "$OLD_CONN" -c "SELECT 1" > /dev/null 2>&1 || { echo "ERROR: Cannot connect to old RDS"; exit 1; }

echo "Verifying connection to new RDS..."
psql "$NEW_CONN" -c "SELECT 1" > /dev/null 2>&1 || { echo "ERROR: Cannot connect to new RDS"; exit 1; }

echo "Both connections verified."
echo ""

# migrate_table TABLE COLUMNS
# Copies data using explicit column names to handle column-order mismatches.
migrate_table() {
  local TABLE="$1"
  local COLS="$2"

  echo -n "Migrating ${TABLE}... "

  # Check if table exists in old DB
  EXISTS=$(psql "$OLD_CONN" -tAc "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = '${TABLE}')")
  if [ "$EXISTS" != "t" ]; then
    echo "SKIP (not in old DB)"
    return
  fi

  # Count rows in old DB
  OLD_COUNT=$(psql "$OLD_CONN" -tAc "SELECT COUNT(*) FROM ${TABLE}")

  if [ "$OLD_COUNT" -eq 0 ]; then
    echo "SKIP (0 rows)"
    return
  fi

  # Truncate target table first (CASCADE to handle FK constraints)
  psql "$NEW_CONN" -c "TRUNCATE TABLE ${TABLE} CASCADE" > /dev/null 2>&1 || true

  # COPY with explicit columns: old DB → stdout → new DB stdin
  psql "$OLD_CONN" -c "\\COPY ${TABLE} (${COLS}) TO STDOUT" | \
    psql "$NEW_CONN" -c "\\COPY ${TABLE} (${COLS}) FROM STDIN"

  # Reset sequences (auto-increment IDs)
  SEQ_NAME=$(psql "$NEW_CONN" -tAc "
    SELECT pg_get_serial_sequence('${TABLE}', 'id')
  " 2>/dev/null | tr -d '[:space:]')

  if [ -n "$SEQ_NAME" ]; then
    psql "$NEW_CONN" -c "SELECT setval('${SEQ_NAME}', COALESCE((SELECT MAX(id) FROM ${TABLE}), 1))" > /dev/null 2>&1
  fi

  # Verify row count
  NEW_COUNT=$(psql "$NEW_CONN" -tAc "SELECT COUNT(*) FROM ${TABLE}")
  echo "OK (${OLD_COUNT} → ${NEW_COUNT} rows)"
}

# Migrate in dependency order with explicit column lists
migrate_table "users"             "id, username, email, display_name, password_hash, role, created_at, updated_at"
migrate_table "skills"            "id, name, description, icon, category, prompt, command_name, parameters, is_official, created_by, is_public, forked_from, created_at, updated_at"
migrate_table "agents"            "id, name, description, icon, config, created_by, cpu_request, cpu_limit, memory_request, memory_limit, created_at, updated_at"
migrate_table "agent_skills"      'id, agent_id, skill_id, "order", created_at'
migrate_table "sessions"          "id, user_id, agent_id, session_id, pod_name, pod_ip, status, last_active, created_at, updated_at"
migrate_table "conversation_logs" "id, user_id, session_id, type, content, timestamp"
migrate_table "system_settings"   "id, key, value, description, created_at, updated_at"
migrate_table "user_settings"     "id, user_id, key, value, created_at, updated_at"
migrate_table "bun_migrations"    "id, name, group_id, migrated_at"
migrate_table "bun_migration_locks" "id, table_name"

echo ""
echo "=== Migration complete ==="
