-- Seed major cryptocurrency tokens
INSERT INTO tokens (symbol, name, decimals, categories, market_cap_rank, is_active) VALUES
-- Major cryptocurrencies
('BTC', 'Bitcoin', 8, ARRAY['cryptocurrency', 'store-of-value'], 1, true),
('ETH', 'Ethereum', 18, ARRAY['cryptocurrency', 'smart-contracts', 'defi'], 2, true),
('USDT', 'Tether', 6, ARRAY['stablecoin', 'usd-pegged'], 3, true),
('BNB', 'BNB', 18, ARRAY['cryptocurrency', 'exchange-token', 'smart-contracts'], 4, true),
('SOL', 'Solana', 9, ARRAY['cryptocurrency', 'smart-contracts', 'defi'], 5, true),
('USDC', 'USD Coin', 6, ARRAY['stablecoin', 'usd-pegged'], 6, true),
('XRP', 'XRP', 6, ARRAY['cryptocurrency', 'payments'], 7, true),
('DOGE', 'Dogecoin', 8, ARRAY['cryptocurrency', 'meme'], 8, true),
('ADA', 'Cardano', 6, ARRAY['cryptocurrency', 'smart-contracts'], 9, true),
('TRX', 'TRON', 6, ARRAY['cryptocurrency', 'smart-contracts'], 10, true),

-- Layer 2 and scaling solutions
('MATIC', 'Polygon', 18, ARRAY['cryptocurrency', 'layer-2', 'scaling'], 11, true),
('AVAX', 'Avalanche', 18, ARRAY['cryptocurrency', 'smart-contracts', 'defi'], 12, true),
('TON', 'Toncoin', 9, ARRAY['cryptocurrency', 'smart-contracts'], 13, true),
('SHIB', 'Shiba Inu', 18, ARRAY['cryptocurrency', 'meme'], 14, true),
('DOT', 'Polkadot', 10, ARRAY['cryptocurrency', 'interoperability'], 15, true),
('LINK', 'Chainlink', 18, ARRAY['cryptocurrency', 'oracle', 'defi'], 16, true),
('BCH', 'Bitcoin Cash', 8, ARRAY['cryptocurrency', 'payments'], 17, true),
('DAI', 'Dai', 18, ARRAY['stablecoin', 'defi', 'usd-pegged'], 18, true),
('LTC', 'Litecoin', 8, ARRAY['cryptocurrency', 'payments'], 19, true),
('UNI', 'Uniswap', 18, ARRAY['cryptocurrency', 'defi', 'dex'], 20, true),

-- DeFi tokens
('AAVE', 'Aave', 18, ARRAY['defi', 'lending'], 21, true),
('MKR', 'Maker', 18, ARRAY['defi', 'governance', 'dao'], 22, true),
('CRV', 'Curve DAO Token', 18, ARRAY['defi', 'dex', 'dao'], 23, true),
('COMP', 'Compound', 18, ARRAY['defi', 'lending'], 24, true),
('SNX', 'Synthetix', 18, ARRAY['defi', 'derivatives'], 25, true),

-- Exchange tokens
('OKB', 'OKB', 18, ARRAY['exchange-token'], 26, true),
('CRO', 'Cronos', 8, ARRAY['exchange-token', 'smart-contracts'], 27, true),
('KCS', 'KuCoin Token', 6, ARRAY['exchange-token'], 28, true),
('FTT', 'FTX Token', 18, ARRAY['exchange-token'], 29, true),
('HT', 'Huobi Token', 18, ARRAY['exchange-token'], 30, true),

-- Layer 1 blockchains
('ATOM', 'Cosmos', 6, ARRAY['cryptocurrency', 'interoperability'], 31, true),
('NEAR', 'NEAR Protocol', 24, ARRAY['cryptocurrency', 'smart-contracts'], 32, true),
('ICP', 'Internet Computer', 8, ARRAY['cryptocurrency', 'smart-contracts'], 33, true),
('APT', 'Aptos', 8, ARRAY['cryptocurrency', 'smart-contracts'], 34, true),
('FIL', 'Filecoin', 18, ARRAY['cryptocurrency', 'storage'], 35, true),
('VET', 'VeChain', 18, ARRAY['cryptocurrency', 'supply-chain'], 36, true),
('ALGO', 'Algorand', 6, ARRAY['cryptocurrency', 'smart-contracts'], 37, true),
('XLM', 'Stellar', 7, ARRAY['cryptocurrency', 'payments'], 38, true),
('HBAR', 'Hedera', 8, ARRAY['cryptocurrency', 'smart-contracts'], 39, true),
('EOS', 'EOS', 4, ARRAY['cryptocurrency', 'smart-contracts'], 40, true),

-- Meme coins
('PEPE', 'Pepe', 18, ARRAY['meme'], 41, true),
('WIF', 'dogwifhat', 6, ARRAY['meme', 'solana'], 42, true),
('BONK', 'Bonk', 5, ARRAY['meme', 'solana'], 43, true),
('FLOKI', 'FLOKI', 9, ARRAY['meme'], 44, true),

-- Gaming and Metaverse
('IMX', 'Immutable', 18, ARRAY['gaming', 'nft', 'layer-2'], 45, true),
('SAND', 'The Sandbox', 18, ARRAY['gaming', 'metaverse'], 46, true),
('MANA', 'Decentraland', 18, ARRAY['gaming', 'metaverse'], 47, true),
('AXS', 'Axie Infinity', 18, ARRAY['gaming', 'nft'], 48, true),
('GALA', 'Gala', 8, ARRAY['gaming'], 49, true),
('ENJ', 'Enjin Coin', 18, ARRAY['gaming', 'nft'], 50, true),

-- AI tokens
('FET', 'Fetch.ai', 18, ARRAY['ai', 'cryptocurrency'], 51, true),
('RNDR', 'Render Token', 18, ARRAY['ai', 'computing'], 52, true),
('OCEAN', 'Ocean Protocol', 18, ARRAY['ai', 'data'], 53, true),
('AGIX', 'SingularityNET', 8, ARRAY['ai'], 54, true),

-- Privacy coins
('XMR', 'Monero', 12, ARRAY['privacy', 'cryptocurrency'], 55, true),
('ZEC', 'Zcash', 8, ARRAY['privacy', 'cryptocurrency'], 56, true),

-- Stablecoins and fiat
('BUSD', 'Binance USD', 18, ARRAY['stablecoin', 'usd-pegged'], 57, true),
('TUSD', 'TrueUSD', 18, ARRAY['stablecoin', 'usd-pegged'], 58, true),
('USDD', 'USDD', 18, ARRAY['stablecoin', 'usd-pegged'], 59, true),
('FRAX', 'Frax', 18, ARRAY['stablecoin', 'algorithmic'], 60, true),

-- Fiat currencies (for trading pairs)
('USD', 'US Dollar', 2, ARRAY['fiat'], null, true),
('EUR', 'Euro', 2, ARRAY['fiat'], null, true),
('GBP', 'British Pound', 2, ARRAY['fiat'], null, true),
('JPY', 'Japanese Yen', 0, ARRAY['fiat'], null, true),
('KRW', 'Korean Won', 0, ARRAY['fiat'], null, true),
('INR', 'Indian Rupee', 2, ARRAY['fiat'], null, true),
('CAD', 'Canadian Dollar', 2, ARRAY['fiat'], null, true),
('AUD', 'Australian Dollar', 2, ARRAY['fiat'], null, true),
('HKD', 'Hong Kong Dollar', 2, ARRAY['fiat'], null, true),
('SGD', 'Singapore Dollar', 2, ARRAY['fiat'], null, true)

ON CONFLICT (symbol) DO UPDATE SET
    name = EXCLUDED.name,
    decimals = EXCLUDED.decimals,
    categories = EXCLUDED.categories,
    market_cap_rank = EXCLUDED.market_cap_rank,
    updated_at = NOW();