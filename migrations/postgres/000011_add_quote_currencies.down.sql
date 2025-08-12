-- Remove quote currencies added in this migration
DELETE FROM tokens 
WHERE metadata->>'is_quote' = 'true' 
AND symbol IN (
    'USD', 'EUR', 'GBP', 'JPY', 'AUD', 'CAD', 'CHF', 'SGD', 'TRY', 'BRL', 
    'MXN', 'ARS', 'UAH', 'PLN', 'ZAR', 'AED', 'KZT', 'RON', 'CZK', 'BGN',
    'FDUSD', 'BUSD', 'TUSD', 'GUSD', 'PYUSD', 'USDE', 'USDe', 'USDD', 'RLUSD',
    'USD1', 'USDR', 'EURC', 'EURI', 'BRZ', 'UAHG', 'XBT', 'BTR', 'KCS', 'CRO',
    'TRX', 'DOGE', 'ADA'
);