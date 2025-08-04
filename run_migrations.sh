#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Running Database Migrations${NC}"
echo "================================"

# Method 1: Using Go migration tool (Recommended for production)
run_with_go_tool() {
    echo -e "${YELLOW}Running migrations with Go tool...${NC}"
    go run cmd/migrate/main.go -v
}

# Method 2: Direct SQL execution (Quick for development)
run_direct_sql() {
    echo -e "${YELLOW}Running PostgreSQL migrations directly...${NC}"
    
    # Combine all PostgreSQL migrations
    cat migrations/00*.sql 2>/dev/null | docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform
    
    echo -e "${YELLOW}Running ClickHouse migrations...${NC}"
    
    # Run ClickHouse migrations
    for file in migrations/clickhouse_*.sql; do
        if [[ -f "$file" ]]; then
            echo "  Running: $(basename $file)"
            docker exec -i crypto_clickhouse clickhouse-client --database crypto_platform --multiquery < "$file" 2>/dev/null || true
        fi
    done
    
    echo -e "${YELLOW}Seeding data...${NC}"
    
    # Run seed data
    cat migrations/seed_*.sql 2>/dev/null | docker exec -i crypto_postgres psql -U crypto_user -d crypto_platform
}

# Method 3: Using golang-migrate tool (Industry standard)
run_with_golang_migrate() {
    echo -e "${YELLOW}Checking for golang-migrate...${NC}"
    
    if ! command -v migrate &> /dev/null; then
        echo "Installing golang-migrate..."
        go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
    fi
    
    DATABASE_URL="postgres://crypto_user:crypto_password@localhost:5432/crypto_platform?sslmode=disable"
    
    echo -e "${YELLOW}Running migrations with golang-migrate...${NC}"
    migrate -path migrations -database "$DATABASE_URL" up
}

# Check which method to use
if [ "$1" == "go" ]; then
    run_with_go_tool
elif [ "$1" == "migrate" ]; then
    run_with_golang_migrate
elif [ "$1" == "direct" ]; then
    run_direct_sql
else
    echo "Usage: $0 [go|migrate|direct]"
    echo ""
    echo "Options:"
    echo "  go      - Use custom Go migration tool"
    echo "  migrate - Use golang-migrate (recommended for production)"
    echo "  direct  - Run SQL files directly (quick for development)"
    echo ""
    echo "Default: Running direct SQL method..."
    echo ""
    run_direct_sql
fi

echo -e "${GREEN}✅ Migrations complete!${NC}"

# Show current database status
echo -e "\n${BLUE}Database Status:${NC}"
echo "PostgreSQL tables:"
docker exec crypto_postgres psql -U crypto_user -d crypto_platform -c "\dt" 2>/dev/null | head -20

echo -e "\nClickHouse tables:"
docker exec crypto_clickhouse clickhouse-client --database crypto_platform --query "SHOW TABLES" 2>/dev/null