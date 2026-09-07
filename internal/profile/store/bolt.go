package store

import (
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	bolt "go.etcd.io/bbolt"
)

// bbolt allows a single writer handle per file; Store serialises access via
// its own mutex, so a small per-path handle registry lets the helper
// functions share one handle per store path.
var (
	handlesMu sync.Mutex
	handles   = map[string]*bolt.DB{}
)

func initBolt(path string) (*bolt.DB, error) {
	handlesMu.Lock()
	defer handlesMu.Unlock()
	if db, ok := handles[path]; ok {
		return db, nil
	}
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 0})
	if err != nil {
		return nil, err
	}
	handles[path] = db
	return db, nil
}

func closeBolt(path string) error {
	handlesMu.Lock()
	db, ok := handles[path]
	if ok {
		delete(handles, path)
	}
	handlesMu.Unlock()
	if !ok {
		return nil
	}
	return db.Close()
}

func domainBucket(tx *bolt.Tx, domain string, create bool) (*bolt.Bucket, error) {
	raw := tx.Bucket(bucketRaw)
	if raw == nil {
		if !create {
			return nil, nil
		}
		var err error
		raw, err = tx.CreateBucketIfNotExists(bucketRaw)
		if err != nil {
			return nil, err
		}
	}
	if create {
		return raw.CreateBucketIfNotExists([]byte(domain))
	}
	return raw.Bucket([]byte(domain)), nil
}

func initMeta(path string) (uint64, error) {
	db, err := initBolt(path)
	if err != nil {
		return 0, err
	}
	var revision uint64
	err = db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists(bucketMeta)
		if err != nil {
			return err
		}
		if meta.Get(keySchema) == nil {
			if err := meta.Put(keySchema, []byte(fmt.Sprintf("%d", SchemaVersion))); err != nil {
				return err
			}
		}
		if meta.Get(keyRevision) == nil {
			if err := meta.Put(keyRevision, putU64(0)); err != nil {
				return err
			}
		}
		revision = binary.BigEndian.Uint64(meta.Get(keyRevision))
		return nil
	})
	return revision, err
}

func getRaw(path, domain, key string) ([]byte, error) {
	db, err := initBolt(path)
	if err != nil {
		return nil, err
	}
	var value []byte
	err = db.View(func(tx *bolt.Tx) error {
		bucket, err := domainBucket(tx, domain, false)
		if err != nil || bucket == nil {
			return err
		}
		if raw := bucket.Get([]byte(key)); raw != nil {
			value = append([]byte(nil), raw...)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, ErrNoSuchKey
	}
	return value, nil
}

func applyWrite(path string, request WriteRequest, currentRevision uint64) (uint64, error) {
	db, err := initBolt(path)
	if err != nil {
		return 0, err
	}
	for _, mutation := range request.Mutations {
		if !validDomain(mutation.Domain) {
			return 0, fmt.Errorf("%w: %q", ErrUnknownDomain, mutation.Domain)
		}
		if !validKey(mutation.Key) {
			return 0, fmt.Errorf("%w: %q", ErrInvalidKey, mutation.Key)
		}
		if !mutation.Delete && mutation.Value == nil {
			return 0, fmt.Errorf("%w: %q/%q", ErrInvalidKey, mutation.Domain, mutation.Key)
		}
		if !mutation.Delete && len(mutation.Value) > MaxRawValueBytes {
			return 0, fmt.Errorf("%w: %d > %d bytes", ErrValueTooLarge, len(mutation.Value), MaxRawValueBytes)
		}
	}
	newRevision := currentRevision
	err = db.Update(func(tx *bolt.Tx) error {
		meta := tx.Bucket(bucketMeta)
		if meta == nil {
			return fmt.Errorf("profile meta bucket missing")
		}
		stored := binary.BigEndian.Uint64(meta.Get(keyRevision))
		if request.ExpectedRevision != 0 && stored != request.ExpectedRevision {
			return ErrRevisionConflict
		}
		newRevision = stored + 1
		for _, mutation := range request.Mutations {
			bucket, err := domainBucket(tx, mutation.Domain, true)
			if err != nil {
				return err
			}
			if mutation.Delete {
				if err := bucket.Delete([]byte(mutation.Key)); err != nil {
					return err
				}
				continue
			}
			if err := bucket.Put([]byte(mutation.Key), mutation.Value); err != nil {
				return err
			}
		}
		return meta.Put(keyRevision, putU64(newRevision))
	})
	if err != nil {
		return 0, err
	}
	return newRevision, nil
}

// DomainKeys lists the keys of one domain in sorted order (used by export).
func (s *Store) DomainKeys(domain string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, ErrClosed
	}
	if !validDomain(domain) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownDomain, domain)
	}
	db, err := initBolt(s.path)
	if err != nil {
		return nil, err
	}
	var keys []string
	err = db.View(func(tx *bolt.Tx) error {
		bucket, err := domainBucket(tx, domain, false)
		if err != nil || bucket == nil {
			return err
		}
		return bucket.ForEach(func(k, _ []byte) error {
			keys = append(keys, string(k))
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(keys)
	return keys, nil
}
