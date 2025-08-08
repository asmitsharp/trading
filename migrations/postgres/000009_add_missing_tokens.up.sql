-- Add commonly traded tokens that are missing from the database

-- Wrapped Bitcoin
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (10001, 'WBTC', 'Wrapped Bitcoin', true, ARRAY['defi', 'wrapped'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    name = EXCLUDED.name,
    categories = EXCLUDED.categories,
    updated_at = NOW();

-- Velodrome Finance
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES (10002, 'VELODROME', 'Velodrome Finance', true, ARRAY['defi', 'dex'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    name = EXCLUDED.name,
    categories = EXCLUDED.categories,
    updated_at = NOW();

-- Other common tokens
INSERT INTO tokens (id, symbol, name, is_active, categories, created_at, updated_at)
VALUES 
    (10003, 'BEAMX', 'Beam', true, ARRAY['gaming'], NOW(), NOW()),
    (10004, 'RONIN', 'Ronin', true, ARRAY['gaming', 'sidechain'], NOW(), NOW()),
    (10005, 'STETH', 'Lido Staked ETH', true, ARRAY['defi', 'liquid-staking'], NOW(), NOW()),
    (10006, 'MSOL', 'Marinade Staked SOL', true, ARRAY['defi', 'liquid-staking'], NOW(), NOW()),
    (10007, 'BNSOL', 'Binance Staked SOL', true, ARRAY['defi', 'liquid-staking'], NOW(), NOW()),
    (10008, 'WBETH', 'Wrapped Beacon ETH', true, ARRAY['defi', 'wrapped'], NOW(), NOW()),
    (10009, '1000SATS', '1000SATS', true, ARRAY['meme'], NOW(), NOW()),
    (10010, '1000CAT', '1000CAT', true, ARRAY['meme'], NOW(), NOW()),
    (10011, '1MBABYDOGE', '1M Baby Doge', true, ARRAY['meme'], NOW(), NOW()),
    (10012, '1000CHEEMS', '1000 Cheems', true, ARRAY['meme'], NOW(), NOW()),
    (10013, 'SAHARA', 'Sahara', true, ARRAY['ai'], NOW(), NOW()),
    (10014, 'RESOLV', 'Resolv', true, ARRAY['defi'], NOW(), NOW()),
    (10015, 'SPK', 'SparkPoint', true, ARRAY['defi'], NOW(), NOW()),
    (10016, 'TREE', 'Tree', true, ARRAY['defi'], NOW(), NOW()),
    (10017, 'HOME', 'Home', true, ARRAY['defi'], NOW(), NOW()),
    (10018, 'NEWT', 'Newt', true, ARRAY['defi'], NOW(), NOW()),
    (10019, 'ERA', 'Era', true, ARRAY['defi'], NOW(), NOW()),
    (10020, 'PROVE', 'Prove', true, ARRAY['defi'], NOW(), NOW())
ON CONFLICT (symbol) DO UPDATE SET
    name = EXCLUDED.name,
    categories = EXCLUDED.categories,
    updated_at = NOW();