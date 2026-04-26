// Package signatures provides a local bbolt-backed malware hash database.
package signatures

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const bucketName = "malware_hashes"

// EICARTestFileSHA256 is the SHA-256 of the standard EICAR test string (for offline validation).
const EICARTestFileSHA256 = "275a021bbfb6489e54d471899f7db9d1663fc695ec2fe2a2c4538aabf651fd0f"

// Record is stored as JSON in the malware_hashes bucket.
type Record struct {
	Name     string `json:"name"`
	Family   string `json:"family"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
	AddedAt  string `json:"added_at"`
}

// Store wraps a bbolt database with one bucket for SHA-256 → Record JSON.
type Store struct {
	db *bolt.DB
}

// Open opens or creates the database at path (parent dirs created).
func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("signatures: mkdir: %w", err)
		}
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("signatures: open db: %w", err)
	}
	s := &Store{db: db}
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
		return err
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("signatures: init bucket: %w", err)
	}
	// Built-in NDJSON (at minimum EICAR) — idempotent merge.
	if len(builtinHashesNDJSON) > 0 {
		if _, _, err := s.MergeFromNDJSON(bytes.NewReader(builtinHashesNDJSON), false); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("signatures: builtin seed: %w", err)
		}
	}
	// Optional operator-supplied seed next to the DB file (same dir as signatures.db).
	seedPath := filepath.Join(dir, "signature_seed.ndjson")
	if raw, err := os.ReadFile(seedPath); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		if _, _, err := s.MergeFromNDJSON(bytes.NewReader(raw), false); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("signatures: %s: %w", seedPath, err)
		}
	}
	return s, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Lookup returns the record for a lowercase 64-char hex SHA-256, if present.
func (s *Store) Lookup(sha256Hex string) (*Record, bool) {
	if s == nil || s.db == nil {
		return nil, false
	}
	key := strings.ToLower(strings.TrimSpace(sha256Hex))
	if len(key) != 64 {
		return nil, false
	}
	var rec Record
	var ok bool
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		v := b.Get([]byte(key))
		if v == nil || json.Unmarshal(v, &rec) != nil {
			return nil
		}
		ok = true
		return nil
	})
	if !ok {
		return nil, false
	}
	return &rec, true
}

type importLine struct {
	SHA256   string `json:"sha256"`
	Name     string `json:"name"`
	Family   string `json:"family"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
}

// MergeFromNDJSON reads newline-delimited JSON objects with a "sha256" field.
// If force is false, existing keys are left unchanged. Returns counts of inserted and skipped rows.
func (s *Store) MergeFromNDJSON(r io.Reader, force bool) (inserted int, skipped int, err error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("signatures: nil store")
	}
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 4*1024*1024)

	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var row importLine
		if err := json.Unmarshal(line, &row); err != nil {
			return inserted, skipped, fmt.Errorf("signatures: bad json line: %w", err)
		}
		key := strings.ToLower(strings.TrimSpace(row.SHA256))
		if len(key) != 64 {
			return inserted, skipped, fmt.Errorf("signatures: invalid sha256 in line")
		}
		rec := Record{
			Name:     row.Name,
			Family:   row.Family,
			Severity: row.Severity,
			Source:   row.Source,
			AddedAt:  time.Now().UTC().Format(time.RFC3339),
		}
		raw, _ := json.Marshal(rec)
		err := s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(bucketName))
			if b == nil {
				return fmt.Errorf("signatures: missing bucket")
			}
			if !force {
				if existing := b.Get([]byte(key)); existing != nil {
					skipped++
					return nil
				}
			}
			if err := b.Put([]byte(key), raw); err != nil {
				return err
			}
			inserted++
			return nil
		})
		if err != nil {
			return inserted, skipped, err
		}
	}
	return inserted, skipped, sc.Err()
}

// mergeHashList writes many SHA-256 keys with the same Record metadata (internal helper).
func (s *Store) mergeHashList(hashes []string, rec Record, force bool) (inserted int, skipped int, err error) {
	if rec.AddedAt == "" {
		rec.AddedAt = time.Now().UTC().Format(time.RFC3339)
	}
	raw, _ := json.Marshal(rec)
	for _, h := range hashes {
		key := strings.ToLower(strings.TrimSpace(h))
		if len(key) != 64 {
			continue
		}
		upErr := s.db.Update(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(bucketName))
			if b == nil {
				return fmt.Errorf("signatures: missing bucket")
			}
			if !force {
				if existing := b.Get([]byte(key)); existing != nil {
					skipped++
					return nil
				}
			}
			if err := b.Put([]byte(key), raw); err != nil {
				return err
			}
			inserted++
			return nil
		})
		if upErr != nil {
			return inserted, skipped, upErr
		}
	}
	return inserted, skipped, nil
}

// Version returns a coarse version string (bucket key count).
func (s *Store) Version() (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("signatures: nil store")
	}
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketName))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			n++
			return nil
		})
	})
	return n, err
}
