-- Add fiat currency tokens to the tokens table
-- These are commonly used as quote currencies on crypto exchanges

-- USD (US Dollar)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90001, 'USD', 'US Dollar', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- EUR (Euro)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90002, 'EUR', 'Euro', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- GBP (British Pound)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90003, 'GBP', 'British Pound', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- JPY (Japanese Yen)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90004, 'JPY', 'Japanese Yen', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- KRW (Korean Won)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90005, 'KRW', 'South Korean Won', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- INR (Indian Rupee)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90006, 'INR', 'Indian Rupee', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- TRY (Turkish Lira)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90007, 'TRY', 'Turkish Lira', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- BRL (Brazilian Real)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90008, 'BRL', 'Brazilian Real', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- MXN (Mexican Peso)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90009, 'MXN', 'Mexican Peso', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- ARS (Argentine Peso)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90010, 'ARS', 'Argentine Peso', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- ZAR (South African Rand)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90011, 'ZAR', 'South African Rand', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- UAH (Ukrainian Hryvnia)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90012, 'UAH', 'Ukrainian Hryvnia', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- COP (Colombian Peso)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90013, 'COP', 'Colombian Peso', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- SGD (Singapore Dollar)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90014, 'SGD', 'Singapore Dollar', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- AUD (Australian Dollar)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90015, 'AUD', 'Australian Dollar', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- CAD (Canadian Dollar)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90016, 'CAD', 'Canadian Dollar', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- CHF (Swiss Franc)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90017, 'CHF', 'Swiss Franc', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- PLN (Polish Zloty)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90018, 'PLN', 'Polish Zloty', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- RUB (Russian Ruble)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90019, 'RUB', 'Russian Ruble', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- CNY (Chinese Yuan)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90020, 'CNY', 'Chinese Yuan', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- HKD (Hong Kong Dollar)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90021, 'HKD', 'Hong Kong Dollar', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- NZD (New Zealand Dollar)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90022, 'NZD', 'New Zealand Dollar', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- THB (Thai Baht)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90023, 'THB', 'Thai Baht', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- IDR (Indonesian Rupiah)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90024, 'IDR', 'Indonesian Rupiah', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();

-- PHP (Philippine Peso)
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (90025, 'PHP', 'Philippine Peso', true, ARRAY['fiat', 'currency'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    categories = ARRAY['fiat', 'currency'],
    updated_at = NOW();