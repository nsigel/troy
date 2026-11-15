---
name: food
description: Browse USC pickup restaurants, menu prices and modifiers, including illy cafe, and prepare explicitly confirmed orders.
---

Use the CLI directly. Locations and menus are read-only; neither creates a cart or order.

```python
locations = usc.mobileorder.locations()
print(json.dumps([x for x in locations if 'illy' in x['name'].lower()]))
```

Returns `[{"id":123,"name":"Illy Cafe","pickup_status":"Closed"}]`. Select the location matching the request, then use its ID:
```python
menu = usc.mobileorder.menu(location=123, fulfillment="pickup", payment_method=0, flat=True)
print(menu['status'])
print(json.dumps([item for item in menu['items'] if 'latte' in item['name'].lower()]))
```

Replace the ID and search term. The menu is `{"location_id":123,"status":"Closed","items":[{"id":456,"name":"Latte","price":450,"available":true,"section_id":7,"section":"Coffee","section_available":true}]}`. Prices are integer cents. Item/section availability does not mean the venue is open: retain the menu status and location pickup status. An empty filter result only means no matching returned item. For multiple relevant venues, fetch their menus with `parallel`; don't fetch every campus menu for a narrow request. Reuse `locations` and `menu` within the task.

For modifiers, `usc.mobileorder.item(456, location=123, fulfillment="pickup", payment_method=0)` returns item details and `options[]` with selection minimum/maximum and `values[]` with IDs, names, prices and availability. Use returned choices.

Payment method 0 is campus card; browsing with it is not purchase authorization. For ordering, read `usc mobileorder cart --help`, `quote --help` and `checkout --help` to use actual modifiers, pickup slots and payment/tender choices. Present exact items, location, pickup time and quoted total for confirmation before purchase. After an ambiguous purchase, inspect existing orders before retrying. Mobile ordering is distinct from residential dining hall menus.
