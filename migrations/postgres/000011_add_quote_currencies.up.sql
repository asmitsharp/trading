-- Add quote currencies (fiat, stablecoins, and crypto used as quotes)
-- These are essential for trading pairs

INSERT INTO tokens (symbol, name, metadata, is_active, created_at, updated_at) 
VALUES 
    -- Major Fiat Currencies
    ('USD', 'US Dollar', '{"type": "fiat", "slug": "usd", "cmc_id": 2781, "is_quote": true}', true, NOW(), NOW()),
    ('EUR', 'Euro', '{"type": "fiat", "slug": "eur", "cmc_id": 2790, "is_quote": true}', true, NOW(), NOW()),
    ('GBP', 'British Pound', '{"type": "fiat", "slug": "gbp", "cmc_id": 2791, "is_quote": true}', true, NOW(), NOW()),
    ('JPY', 'Japanese Yen', '{"type": "fiat", "slug": "jpy", "cmc_id": 2797, "is_quote": true}', true, NOW(), NOW()),
    ('AUD', 'Australian Dollar', '{"type": "fiat", "slug": "aud", "cmc_id": 2782, "is_quote": true}', true, NOW(), NOW()),
    ('CAD', 'Canadian Dollar', '{"type": "fiat", "slug": "cad", "cmc_id": 2784, "is_quote": true}', true, NOW(), NOW()),
    ('CHF', 'Swiss Franc', '{"type": "fiat", "slug": "chf", "cmc_id": 2785, "is_quote": true}', true, NOW(), NOW()),
    ('SGD', 'Singapore Dollar', '{"type": "fiat", "slug": "sgd", "cmc_id": 2808, "is_quote": true}', true, NOW(), NOW()),
    ('TRY', 'Turkish Lira', '{"type": "fiat", "slug": "try", "cmc_id": 2810, "is_quote": true}', true, NOW(), NOW()),
    ('BRL', 'Brazilian Real', '{"type": "fiat", "slug": "brl", "cmc_id": 2783, "is_quote": true}', true, NOW(), NOW()),
    ('MXN', 'Mexican Peso', '{"type": "fiat", "slug": "mxn", "cmc_id": 2799, "is_quote": true}', true, NOW(), NOW()),
    ('ARS', 'Argentine Peso', '{"type": "fiat", "slug": "ars", "cmc_id": 2821, "is_quote": true}', true, NOW(), NOW()),
    ('UAH', 'Ukrainian Hryvnia', '{"type": "fiat", "slug": "uah", "cmc_id": 2824, "is_quote": true}', true, NOW(), NOW()),
    ('PLN', 'Polish Zloty', '{"type": "fiat", "slug": "pln", "cmc_id": 2805, "is_quote": true}', true, NOW(), NOW()),
    ('ZAR', 'South African Rand', '{"type": "fiat", "slug": "zar", "cmc_id": 2812, "is_quote": true}', true, NOW(), NOW()),
    ('AED', 'UAE Dirham', '{"type": "fiat", "slug": "aed", "cmc_id": 2813, "is_quote": true}', true, NOW(), NOW()),
    ('KZT', 'Kazakhstani Tenge', '{"type": "fiat", "slug": "kzt", "cmc_id": 3551, "is_quote": true}', true, NOW(), NOW()),
    ('RON', 'Romanian Leu', '{"type": "fiat", "slug": "ron", "cmc_id": 2817, "is_quote": true}', true, NOW(), NOW()),
    ('CZK', 'Czech Koruna', '{"type": "fiat", "slug": "czk", "cmc_id": 2788, "is_quote": true}', true, NOW(), NOW()),
    ('BGN', 'Bulgarian Lev', '{"type": "fiat", "slug": "bgn", "cmc_id": 2814, "is_quote": true}', true, NOW(), NOW()),
    
    -- Stablecoins (if not already present)
    ('FDUSD', 'First Digital USD', '{"type": "stablecoin", "slug": "first-digital-usd", "cmc_id": 26081, "is_quote": true}', true, NOW(), NOW()),
    ('BUSD', 'Binance USD', '{"type": "stablecoin", "slug": "binance-usd", "cmc_id": 4687, "is_quote": true}', true, NOW(), NOW()),
    ('TUSD', 'TrueUSD', '{"type": "stablecoin", "slug": "trueusd", "cmc_id": 2563, "is_quote": true}', true, NOW(), NOW()),
    ('GUSD', 'Gemini Dollar', '{"type": "stablecoin", "slug": "gemini-dollar", "cmc_id": 3306, "is_quote": true}', true, NOW(), NOW()),
    ('PYUSD', 'PayPal USD', '{"type": "stablecoin", "slug": "paypal-usd", "cmc_id": 27772, "is_quote": true}', true, NOW(), NOW()),
    ('USDE', 'Ethena USDe', '{"type": "stablecoin", "slug": "ethena-usde", "cmc_id": 29470, "is_quote": true}', true, NOW(), NOW()),
    ('USDe', 'Ethena USDe', '{"type": "stablecoin", "slug": "ethena-usde", "cmc_id": 29470, "is_quote": true}', true, NOW(), NOW()),
    ('USDD', 'USDD', '{"type": "stablecoin", "slug": "usdd", "cmc_id": 19891, "is_quote": true}', true, NOW(), NOW()),
    ('RLUSD', 'Real USD', '{"type": "stablecoin", "slug": "real-usd", "cmc_id": 34387, "is_quote": true}', true, NOW(), NOW()),
    ('USD1', 'USD1', '{"type": "stablecoin", "slug": "usd1", "cmc_id": 36148, "is_quote": true}', true, NOW(), NOW()),
    ('USDR', 'Real USD', '{"type": "stablecoin", "slug": "real-usd", "cmc_id": 35372, "is_quote": true}', true, NOW(), NOW()),
    ('EURC', 'Euro Coin', '{"type": "stablecoin", "slug": "euro-coin", "cmc_id": 20641, "is_quote": true}', true, NOW(), NOW()),
    ('EURI', 'Monerium EUR', '{"type": "stablecoin", "slug": "monerium-eur", "cmc_id": 32644, "is_quote": true}', true, NOW(), NOW()),
    ('BRZ', 'Brazilian Digital Token', '{"type": "stablecoin", "slug": "brz", "cmc_id": 4139, "is_quote": true}', true, NOW(), NOW()),
    ('UAHG', 'Hryvnia', '{"type": "stablecoin", "slug": "uahg", "cmc_id": 31625, "is_quote": true}', true, NOW(), NOW()),
    
    -- Crypto tokens commonly used as quote currencies
    ('XBT', 'Bitcoin (XBT)', '{"type": "crypto", "slug": "bitcoin", "cmc_id": 1, "is_quote": true, "alias_of": "BTC"}', true, NOW(), NOW()),
    ('BTR', 'Bitrue Coin', '{"type": "crypto", "slug": "bitrue-coin", "cmc_id": 4167, "is_quote": true}', true, NOW(), NOW()),
    ('KCS', 'KuCoin Token', '{"type": "crypto", "slug": "kucoin-shares", "cmc_id": 2087, "is_quote": true}', true, NOW(), NOW()),
    ('CRO', 'Cronos', '{"type": "crypto", "slug": "crypto-com-coin", "cmc_id": 3635, "is_quote": true}', true, NOW(), NOW()),
    ('TRX', 'TRON', '{"type": "crypto", "slug": "tron", "cmc_id": 1958, "is_quote": true}', true, NOW(), NOW()),
    ('DOGE', 'Dogecoin', '{"type": "crypto", "slug": "dogecoin", "cmc_id": 74, "is_quote": true}', true, NOW(), NOW()),
    ('ADA', 'Cardano', '{"type": "crypto", "slug": "cardano", "cmc_id": 2010, "is_quote": true}', true, NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE 
SET 
    metadata = EXCLUDED.metadata,
    updated_at = NOW()
WHERE tokens.metadata->>'is_quote' IS NULL;