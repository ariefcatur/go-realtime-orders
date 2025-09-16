INSERT INTO products (sku, name, stock, price_cents)
VALUES
    ('SKU-APPLE','Apple',100,1500),
    ('SKU-BREAD','Bread',50,12000),
    ('SKU-MILK','Milk',80,18000),
    ('SKU-RICE','Rice',200,55000),
    ('SKU-TEA','Tea',150,7000)
    ON CONFLICT (sku) DO UPDATE
                             SET name = EXCLUDED.name,
                             stock = EXCLUDED.stock,
                             price_cents = EXCLUDED.price_cents,
                             updated_at = now();
