#!/usr/bin/env bash

set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL を設定してください}"

migration_dir="${1:-db/migrations}"

if ! command -v psql >/dev/null 2>&1; then
  echo "psql が見つかりません" >&2
  exit 1
fi

if [[ ! -d "$migration_dir" ]]; then
  echo "migrationディレクトリが見つかりません: $migration_dir" >&2
  exit 1
fi

mapfile -t up_files < <(find "$migration_dir" -maxdepth 1 -type f -name '*.up.sql' | sort)
mapfile -t down_files < <(find "$migration_dir" -maxdepth 1 -type f -name '*.down.sql' | sort -r)

if (( ${#up_files[@]} == 0 )); then
  echo "up migrationが見つかりません" >&2
  exit 1
fi

if (( ${#up_files[@]} != ${#down_files[@]} )); then
  echo "up/down migrationの件数が一致しません: up=${#up_files[@]} down=${#down_files[@]}" >&2
  exit 1
fi

for up_file in "${up_files[@]}"; do
  prefix="${up_file%.up.sql}"
  down_file="${prefix}.down.sql"
  if [[ ! -f "$down_file" ]]; then
    echo "対応するdown migrationがありません: $up_file" >&2
    exit 1
  fi
done

apply_files() {
  local direction="$1"
  shift

  for migration_file in "$@"; do
    echo "[$direction] $migration_file"
    psql "$DATABASE_URL" \
      --no-psqlrc \
      --set=ON_ERROR_STOP=1 \
      --file="$migration_file"
  done
}

assert_schema_exists() {
  local required_tables=(
    shipments
    files
    upload_sessions
    users
    organizations
    organization_members
    subscriptions
  )

  for table_name in "${required_tables[@]}"; do
    exists="$(psql "$DATABASE_URL" --no-psqlrc --tuples-only --no-align --set=ON_ERROR_STOP=1 \
      --command="SELECT to_regclass('public.${table_name}') IS NOT NULL")"
    if [[ "$exists" != "t" ]]; then
      echo "up migration後に必須テーブルがありません: $table_name" >&2
      exit 1
    fi
  done
}

assert_schema_removed() {
  local table_count
  local enum_count

  table_count="$(psql "$DATABASE_URL" --no-psqlrc --tuples-only --no-align --set=ON_ERROR_STOP=1 \
    --command="SELECT count(*) FROM pg_tables WHERE schemaname = 'public'")"
  if [[ "$table_count" != "0" ]]; then
    echo "down migration後もpublic schemaにテーブルが残っています: $table_count" >&2
    psql "$DATABASE_URL" --no-psqlrc --set=ON_ERROR_STOP=1 \
      --command="SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename"
    exit 1
  fi

  enum_count="$(psql "$DATABASE_URL" --no-psqlrc --tuples-only --no-align --set=ON_ERROR_STOP=1 \
    --command="SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype = 'e'")"
  if [[ "$enum_count" != "0" ]]; then
    echo "down migration後もpublic schemaにenumが残っています: $enum_count" >&2
    psql "$DATABASE_URL" --no-psqlrc --set=ON_ERROR_STOP=1 \
      --command="SELECT t.typname FROM pg_type t JOIN pg_namespace n ON n.oid = t.typnamespace WHERE n.nspname = 'public' AND t.typtype = 'e' ORDER BY t.typname"
    exit 1
  fi
}

echo "migrationペア数: ${#up_files[@]}"

apply_files "up" "${up_files[@]}"
assert_schema_exists

apply_files "down" "${down_files[@]}"
assert_schema_removed

# 後続のrepository integration testが同じDBを利用できるよう、最終状態は最新schemaへ戻す。
apply_files "up" "${up_files[@]}"
assert_schema_exists

echo "migration up/down/up検証が完了しました"
