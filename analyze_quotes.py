import json
import os

quotes = {}
base_path = "cmd/mapper/coinmarketcap exchange"

for folder in os.listdir(base_path):
    folder_path = os.path.join(base_path, folder)
    if os.path.isdir(folder_path):
        for file in os.listdir(folder_path):
            if file.endswith('.json'):
                file_path = os.path.join(folder_path, file)
                try:
                    with open(file_path, 'r') as f:
                        data = json.load(f)
                        for pair in data.get('data', {}).get('marketPairs', []):
                            quote = pair.get('quoteSymbol', '')
                            quote_slug = pair.get('quoteCurrencySlug', '')
                            quote_id = pair.get('quoteCurrencyId', 0)
                            if quote:
                                if quote not in quotes:
                                    quotes[quote] = {
                                        'slug': quote_slug, 
                                        'id': quote_id,
                                        'count': 0, 
                                        'exchanges': set()
                                    }
                                quotes[quote]['count'] += 1
                                quotes[quote]['exchanges'].add(data['data']['name'])
                                if quote_slug and not quotes[quote]['slug']:
                                    quotes[quote]['slug'] = quote_slug
                                if quote_id and not quotes[quote]['id']:
                                    quotes[quote]['id'] = quote_id
                except Exception as e:
                    print(f"Error reading {file_path}: {e}")

print("All unique quote currencies found:")
print("="*80)
for symbol, info in sorted(quotes.items(), key=lambda x: x[1]['count'], reverse=True)[:50]:
    print(f"{symbol:10} | Used {info['count']:5} times | {len(info['exchanges']):2} exchanges | Slug: {info['slug'] or 'N/A':30} | CMC ID: {info['id'] or 'N/A'}")