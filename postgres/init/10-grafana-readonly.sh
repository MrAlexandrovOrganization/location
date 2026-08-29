#!/bin/bash
# Creates a read-only PostgreSQL role for Grafana (PostgreSQL datasource).
# Runs automatically only on first DB init (empty volume), so a fresh
# deployment is fully self-contained.
# On an already-initialized DB, create the role manually, e.g.:
#   docker exec -e PW=... location-postgres-1 psql -U location -d location \
#     -c "CREATE ROLE grafana_ro LOGIN PASSWORD '$PW';" \
#     -c "GRANT CONNECT ON DATABASE location TO grafana_ro;" \
#     -c "GRANT USAGE ON SCHEMA public TO grafana_ro;" \
#     -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana_ro;" \
#     -c "GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO grafana_ro;" \
#     -c "ALTER DEFAULT PRIVILEGES FOR ROLE location IN SCHEMA public GRANT SELECT ON TABLES TO grafana_ro;" \
#     -c "ALTER DEFAULT PRIVILEGES FOR ROLE location IN SCHEMA public GRANT SELECT ON SEQUENCES TO grafana_ro;"

set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
  DO \$\$ BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'grafana_ro') THEN
      CREATE ROLE grafana_ro LOGIN PASSWORD '$GRAFANA_DB_PASSWORD';
    END IF;
  END \$\$;

  GRANT CONNECT ON DATABASE "$POSTGRES_DB" TO grafana_ro;
  GRANT USAGE ON SCHEMA public TO grafana_ro;
  GRANT SELECT ON ALL TABLES IN SCHEMA public TO grafana_ro;
  GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO grafana_ro;
  ALTER DEFAULT PRIVILEGES FOR ROLE "$POSTGRES_USER" IN SCHEMA public
    GRANT SELECT ON TABLES TO grafana_ro;
  ALTER DEFAULT PRIVILEGES FOR ROLE "$POSTGRES_USER" IN SCHEMA public
    GRANT SELECT ON SEQUENCES TO grafana_ro;
EOSQL
