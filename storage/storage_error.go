package storage

// StorageError is an immutable private memory-backend sentinel.
type StorageError string

// Error returns the stable sentinel text.
func (failure StorageError) Error() string {
	return string(failure)
}

const (
	errMemoryDuplicate StorageError = "memory order already exists"
	errMemorySchema    StorageError = "memory order schema is not ready"
)
