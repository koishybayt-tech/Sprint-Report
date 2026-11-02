#!/usr/bin/env bash
# Ежедневный бэкап Postgres. cron: 30 2 * * * /opt/adata-reports/deploy/backup.sh
set -euo pipefail
cd "$(dirname "$0")"
set -a; . ./.env; set +a
mkdir -p "$BACKUP_DIR"
f="$BACKUP_DIR/reports_$(date +%F_%H%M).sql.gz"
docker compose exec -T postgres pg_dump -U reports -d reports --no-owner | gzip > "$f"
if [ -n "${BACKUP_AGE_RECIPIENT:-}" ]; then
  age -r "$BACKUP_AGE_RECIPIENT" -o "$f.age" "$f" && rm "$f"
fi
find "$BACKUP_DIR" -name 'reports_*' -mtime +14 -delete
echo "backup ok: $(ls -1t "$BACKUP_DIR" | head -1)"
