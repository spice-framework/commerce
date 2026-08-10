package storage

const (
	moduleID        = "github.com/spice-framework/commerce/storage"
	memoryURL       = "memory://commerce"
	migrationNumber = 202607280001
)

const createOrdersTableSQL = `CREATE TABLE IF NOT EXISTS commerce_orders (
	id text PRIMARY KEY,
	sku text NOT NULL,
	quantity bigint NOT NULL CHECK (quantity > 0),
	total_cents bigint NOT NULL CHECK (total_cents > 0),
	authorization_id text NOT NULL
);`

const insertOrderSQL = `INSERT INTO commerce_orders (
	id,
	sku,
	quantity,
	total_cents,
	authorization_id
) VALUES ($1, $2, $3, $4, $5)`

const findOrderSQL = `SELECT
	id,
	sku,
	quantity,
	total_cents,
	authorization_id
FROM commerce_orders
WHERE id = $1`
