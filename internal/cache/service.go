package cache

// Table is the storage contract used by CacheService.
// Create returns ErrKeyExists for an existing key; Update returns ErrNotFound
// for a missing key. Read and Delete report presence through their boolean result.
type Table interface {
	Create(key, value string) error
	Read(key string) (string, bool)
	Update(key, value string) error
	Delete(key string) bool
}

// CacheService validates inputs before delegating storage operations to a Table.
// Limits count bytes, not characters. Keys must be nonempty; values may be empty.
type CacheService struct {
	table         Table
	maxKeyBytes   int
	maxValueBytes int
}

// NewCacheService uses the supplied table and byte limits without changing them.
// The caller is responsible for validating configuration and supplying a table
// that supports concurrent access when the service is shared by HTTP handlers.
func NewCacheService(table Table, maxKeyBytes, maxValueBytes int) *CacheService {
	return &CacheService{table: table, maxKeyBytes: maxKeyBytes, maxValueBytes: maxValueBytes}
}

// validateKey rejects empty keys and keys larger than the configured byte limit.
func (s *CacheService) validateKey(key string) error {
	if len(key) == 0 || len(key) > s.maxKeyBytes {
		return ErrInvalidKey
	}
	return nil
}

// validateValue permits empty values and enforces the configured byte limit.
func (s *CacheService) validateValue(value string) error {
	if len(value) > s.maxValueBytes {
		return ErrValueTooLarge
	}
	return nil
}

// Create validates key and value, then stores a new entry.
// An existing key returns the table's ErrKeyExists without replacing its value.
func (s *CacheService) Create(key, value string) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.validateValue(value); err != nil {
		return err
	}
	return s.table.Create(key, value)
}

// Read validates key and returns its value, or ErrNotFound when it is absent.
func (s *CacheService) Read(key string) (string, error) {
	if err := s.validateKey(key); err != nil {
		return "", err
	}
	value, found := s.table.Read(key)
	if !found {
		return "", ErrNotFound
	}
	return value, nil
}

// Update validates key and value, then replaces an existing entry's value.
// The table returns ErrNotFound when the key is absent.
func (s *CacheService) Update(key, value string) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.validateValue(value); err != nil {
		return err
	}
	return s.table.Update(key, value)
}

// Delete validates key and removes its entry, returning ErrNotFound if absent.
func (s *CacheService) Delete(key string) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if !s.table.Delete(key) {
		return ErrNotFound
	}
	return nil
}
