package cache

// Table is the contract used by CacheService for storage operations.
type Table interface {
	Create(key, value string) error
	Read(key string) (string, bool)
	Update(key, value string) error
	Delete(key string) bool
}

// CacheService validates inputs and delegates storage operations to a Table.
type CacheService struct {
	table         Table
	maxKeyBytes   int
	maxValueBytes int
}

// NewCacheService constructs a service with the supplied byte limits.
func NewCacheService(table Table, maxKeyBytes, maxValueBytes int) *CacheService {
	return &CacheService{table: table, maxKeyBytes: maxKeyBytes, maxValueBytes: maxValueBytes}
}

func (s *CacheService) validateKey(key string) error {
	if len(key) == 0 || len(key) > s.maxKeyBytes {
		return ErrInvalidKey
	}
	return nil
}

func (s *CacheService) validateValue(value string) error {
	if len(value) > s.maxValueBytes {
		return ErrValueTooLarge
	}
	return nil
}

// Create stores a new key and rejects an existing one.
func (s *CacheService) Create(key, value string) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.validateValue(value); err != nil {
		return err
	}
	return s.table.Create(key, value)
}

// Read returns the value associated with key.
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

// Update replaces the value of an existing key.
func (s *CacheService) Update(key, value string) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if err := s.validateValue(value); err != nil {
		return err
	}
	return s.table.Update(key, value)
}

// Delete removes an existing key.
func (s *CacheService) Delete(key string) error {
	if err := s.validateKey(key); err != nil {
		return err
	}
	if !s.table.Delete(key) {
		return ErrNotFound
	}
	return nil
}
