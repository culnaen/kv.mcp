package kv

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	bolt "go.etcd.io/bbolt"
	bbolterrors "go.etcd.io/bbolt/errors"
)

// errStop is returned by ForEach callbacks to halt iteration early.
// It is suppressed internally; callers never see it.
var errStop = errors.New("stop iteration")

// locFilePath extracts the file path from a loc string ("relpath/file.go:10-20").
func locFilePath(loc, root string) string {
	for i := len(loc) - 1; i >= 0; i-- {
		if loc[i] == ':' {
			p := loc[:i]
			if root != "" {
				return root + "/" + p
			}
			return p
		}
	}
	return loc
}

// Store manages extracted and curated function records.
type Store interface {
	ClearExtracted() error
	PutExtracted(f ExtractedFunction) error
	ScanExtracted(fn func(ExtractedFunction) bool) error

	PutCurated(f CuratedFunction) error
	GetCurated(name string) (CuratedFunction, bool, error)

	GetMerged(name, root string) (Function, bool, error)
	ScanMerged(root string, fn func(Function) bool) error

	Close() error
}

type boltStore struct {
	db *bolt.DB
	mu sync.Mutex // serializes write transactions
}

func Open(path string) (Store, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{BucketExtracted, BucketCurated} {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init buckets: %w", err)
	}
	return &boltStore{db: db}, nil
}

func (s *boltStore) Close() error {
	return s.db.Close()
}

func (s *boltStore) ClearExtracted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte(BucketExtracted)); err != nil && err != bbolterrors.ErrBucketNotFound {
			return err
		}
		_, err := tx.CreateBucket([]byte(BucketExtracted))
		return err
	})
}

func (s *boltStore) PutExtracted(f ExtractedFunction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(f)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(BucketExtracted)).Put([]byte(f.Name), data)
	})
}

func (s *boltStore) ScanExtracted(fn func(ExtractedFunction) bool) error {
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(BucketExtracted)).ForEach(func(k, v []byte) error {
			var f ExtractedFunction
			if err := json.Unmarshal(v, &f); err != nil {
				return err
			}
			if !fn(f) {
				return errStop
			}
			return nil
		})
	})
	if errors.Is(err, errStop) {
		err = nil
	}
	return err
}

func (s *boltStore) PutCurated(f CuratedFunction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(f)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(BucketCurated)).Put([]byte(f.Name), data)
	})
}

func (s *boltStore) GetCurated(name string) (CuratedFunction, bool, error) {
	var f CuratedFunction
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket([]byte(BucketCurated)).Get([]byte(name))
		if v == nil {
			return nil
		}
		return json.Unmarshal(v, &f)
	})
	if err != nil {
		return CuratedFunction{}, false, err
	}
	if f.Name == "" {
		return CuratedFunction{}, false, nil
	}
	return f, true, nil
}

func (s *boltStore) GetMerged(name, root string) (Function, bool, error) {
	var ef ExtractedFunction
	var cf *CuratedFunction
	err := s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket([]byte(BucketExtracted)).Get([]byte(name))
		if v == nil {
			return nil
		}
		if err := json.Unmarshal(v, &ef); err != nil {
			return err
		}
		vc := tx.Bucket([]byte(BucketCurated)).Get([]byte(name))
		if vc != nil {
			cf = &CuratedFunction{}
			return json.Unmarshal(vc, cf)
		}
		return nil
	})
	if err != nil {
		return Function{}, false, err
	}
	if ef.Name == "" {
		return Function{}, false, nil
	}
	return MergeFunction(ef, cf, root), true, nil
}

func (s *boltStore) ScanMerged(root string, fn func(Function) bool) error {
	err := s.db.View(func(tx *bolt.Tx) error {
		curatedBucket := tx.Bucket([]byte(BucketCurated))
		return tx.Bucket([]byte(BucketExtracted)).ForEach(func(k, v []byte) error {
			var ef ExtractedFunction
			if err := json.Unmarshal(v, &ef); err != nil {
				return err
			}
			var cf *CuratedFunction
			if vc := curatedBucket.Get(k); vc != nil {
				cf = &CuratedFunction{}
				if err := json.Unmarshal(vc, cf); err != nil {
					return err
				}
			}
			if !fn(MergeFunction(ef, cf, root)) {
				return errStop
			}
			return nil
		})
	})
	if errors.Is(err, errStop) {
		err = nil
	}
	return err
}
