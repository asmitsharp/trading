# Database Migrations

This project uses [golang-migrate](https://github.com/golang-migrate/migrate) for managing database migrations for both PostgreSQL and ClickHouse.

## Directory Structure

```
migrations/
├── postgres/           # PostgreSQL migrations
│   ├── 000001_create_tokens_table.up.sql
│   ├── 000001_create_tokens_table.down.sql
│   ├── 000002_create_token_exchange_symbols.up.sql
│   └── 000002_create_token_exchange_symbols.down.sql
└── clickhouse/        # ClickHouse migrations
    ├── 000001_initial_schema.up.sql
    ├── 000001_initial_schema.down.sql
    ├── 000002_create_views.up.sql
    └── 000002_create_views.down.sql
```

## Migration File Naming Convention

Migration files must follow this naming pattern:
- `{version}_{description}.up.sql` - For forward migrations
- `{version}_{description}.down.sql` - For rollback migrations

Where:
- `version` is a 6-digit zero-padded number (e.g., `000001`)
- `description` is a brief description using underscores (e.g., `create_users_table`)

## Usage

### Using Make Commands (Recommended)

```bash
# Run all migrations (both PostgreSQL and ClickHouse)
make migrate-up

# Run PostgreSQL migrations only
make migrate-postgres-up

# Run ClickHouse migrations only
make migrate-clickhouse-up

# Rollback last migration for both databases
make migrate-down

# Rollback specific database
make migrate-postgres-down
make migrate-clickhouse-down

# Check current migration version
make migrate-postgres-version
make migrate-clickhouse-version

# Reset all migrations (careful!)
make migrate-reset

# Complete database setup (migrations + seed data)
make db-setup
```

### Using Migration Tool Directly

```bash
# Run PostgreSQL migrations up
go run cmd/migrate/migrate.go -db=postgres -dir=up

# Run ClickHouse migrations up
go run cmd/migrate/migrate.go -db=clickhouse -dir=up

# Rollback last N migrations
go run cmd/migrate/migrate.go -db=postgres -dir=down -steps=1

# Check current version
go run cmd/migrate/migrate.go -db=postgres -version

# Force a specific version (use with caution)
go run cmd/migrate/migrate.go -db=postgres -force=3
```

## Environment Variables

The migration tool uses these environment variables:

### PostgreSQL
- `POSTGRES_HOST` (default: localhost)
- `POSTGRES_PORT` (default: 5432)
- `POSTGRES_USER` (default: asmitsingh)
- `POSTGRES_DB` (default: trading)
- `POSTGRES_PASSWORD` (default: empty)

### ClickHouse
- `CLICKHOUSE_HOST` (default: localhost)
- `CLICKHOUSE_PORT` (default: 9000)
- `CLICKHOUSE_USER` (default: default)
- `CLICKHOUSE_DATABASE` (default: trading)
- `CLICKHOUSE_PASSWORD` (default: empty)

## Creating New Migrations

### PostgreSQL Migration

1. Create new migration files:
```bash
# Get the next version number
ls migrations/postgres/ | tail -2

# Create new migration files
touch migrations/postgres/000003_your_migration_name.up.sql
touch migrations/postgres/000003_your_migration_name.down.sql
```

2. Write your migration:
```sql
-- 000003_your_migration_name.up.sql
CREATE TABLE new_table (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);

-- 000003_your_migration_name.down.sql
DROP TABLE IF EXISTS new_table;
```

### ClickHouse Migration

Follow the same pattern in the `migrations/clickhouse/` directory.

## Best Practices

1. **Always write both up and down migrations** - This ensures you can rollback if needed.

2. **Test migrations locally first** - Run migrations in your local environment before deploying.

3. **Keep migrations idempotent** - Use `IF NOT EXISTS` and `IF EXISTS` clauses where possible.

4. **Don't modify existing migrations** - Once a migration has been run in production, create a new migration to make changes.

5. **Use transactions in PostgreSQL** - Wrap complex migrations in transactions when possible.

6. **Be careful with ClickHouse** - ClickHouse doesn't support transactions, so be extra careful with destructive operations.

## Troubleshooting

### Migration is "dirty"

If a migration fails partway through, it may leave the database in a "dirty" state:

```bash
# Check the current state
go run cmd/migrate/migrate.go -db=postgres -version

# If dirty, force to a specific version
go run cmd/migrate/migrate.go -db=postgres -force=2

# Then continue with migrations
make migrate-postgres-up
```

### Connection Issues

1. Ensure databases are running:
```bash
docker-compose ps
```

2. Check environment variables:
```bash
env | grep POSTGRES
env | grep CLICKHOUSE
```

3. Test connections:
```bash
psql -h localhost -U asmitsingh -d trading -c "SELECT 1"
clickhouse-client --host localhost --query "SELECT 1"
```

## Migration History

To see applied migrations in PostgreSQL:
```sql
SELECT * FROM schema_migrations ORDER BY version;
```

To see applied migrations in ClickHouse:
```sql
SELECT * FROM schema_migrations ORDER BY version;
```