package storage

import (
	"sync"

	"github.com/spice-framework/spice/migration"
)

type memoryDatabase struct {
	mu          sync.Mutex
	migrationMu sync.Mutex
	records     map[string]Record
	applied     []migration.Applied
	schema      bool
}
