# Trading Application - Development Notes

## Current Status

### Fixed Issues
1. ✅ Created proper database migrations for ClickHouse and PostgreSQL
2. ✅ Added fiat currency tokens to the database via migrations
3. ✅ Updated exchanges.json with fiat quote currencies
4. ✅ Many fiat pairs now resolving correctly (LINKTRY, ARPATRY, etc.)

### Remaining Issues
Some major pairs still fail to resolve:
- BTCTRY, ETHTRY, USDTTRY (TRY pairs with major cryptos)
- BTCZAR, ETHZAR, USDTZAR (ZAR pairs)
- BTCUAH, USDTUAH (UAH pairs)
- USDTBRL, BTCMXN, etc.

### Root Cause
The symbol parser in `internal/exchanges/generic_client.go` has a logic issue. When both the base and quote are in the quote currencies list (e.g., BTC and TRY are both quote currencies), the parser fails because of this check:

```go
if base != "" && !b.isQuoteCurrency(base) {
    return base, q
}
```

For BTCTRY, it finds TRY as a quote currency, extracts BTC as base, but then rejects it because BTC is also in the quote currencies list.

### Proposed Fix
The parser needs to be updated to:
1. Try longest match first (already doing)
2. For crypto-fiat pairs, prioritize fiat as quote
3. Have a separate list of "primary" cryptos that should be treated as base when paired with fiat

## Database Migrations Created
- `/migrations/clickhouse/000009_add_symbol_columns_to_price_tickers.up.sql` - Adds symbol columns to price_tickers table
- `/migrations/postgres/000008_add_fiat_currency_tokens.up.sql` - Adds 25 fiat currency tokens

## Commands to Run
- `make migrate-postgres-up` - Run PostgreSQL migrations
- `make migrate-clickhouse-up` - Run ClickHouse migrations (may have issues with schema_migrations table)
- `make seed-symbols` - Populate symbol mappings

## Testing
Run the application and check logs:
```bash
go run cmd/main_rest.go 2>&1 | grep -E "(Failed to resolve|Symbol-based)"
```

## Notes for Production Deployment
All changes are in migration files, so deployment is straightforward:
1. Run migrations on production databases
2. Update exchanges.json config
3. Deploy new application code