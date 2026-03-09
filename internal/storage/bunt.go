package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/tidwall/buntdb"
)

// Storable items must provide a function to retrieve the database key
type Storable interface {
	Key() string
}

type DB struct {
	*buntdb.DB
}

func NewBunt(filePath string) *DB {
	db, err := buntdb.Open(filePath)
	if err != nil {
		log.Fatal(err)
	}

	return &DB{db}
}

// Exists checks is storable item exists
func (db *DB) Exists(storable Storable) (ok bool, err error) {
	ok = false
	err = db.View(func(tx *buntdb.Tx) error {
		_, err := tx.Get(storable.Key())
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		if err == buntdb.ErrNotFound {
			return
		}
		return
	}
	ok = true
	return

}

// Get a storable item
func (db *DB) Get(object Storable) error {
	err := db.View(func(tx *buntdb.Tx) error {
		val, err := tx.Get(object.Key())
		if err != nil {
			return err
		}
		err = json.Unmarshal([]byte(val), object)
		if err != nil {
			fmt.Println(err)
			return err
		}
		return nil
	})
	return err
}

// Set a storable item.
func (db *DB) Set(object Storable) error {
	err := db.Update(func(tx *buntdb.Tx) error {
		b, err := json.Marshal(object)
		if err != nil {
			return err
		}
		_, _, err = tx.Set(object.Key(), string(b), nil)

		return err
	})
	return err
}

// Delete a storable item.
func (db *DB) Delete(index string, object Storable) error {
	return db.Update(func(tx *buntdb.Tx) error {
		_, err := tx.Get(object.Key())
		if err != nil {
			return err
		}
		if _, err := tx.Delete(object.Key()); err != nil {
			return err
		}
		return nil
	})
}

// Cleanup cancella dal BuntDB tutti gli oggetti la cui data "updated" è
// più vecchia di maxAge. Restituisce il numero di oggetti eliminati.
// È sicuro chiamarlo con il bot attivo: usa transazioni atomiche in batch.
func (db *DB) Cleanup(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge)
	var toDelete []string

	// Fase 1: scansione in sola lettura — raccoglie le chiavi da eliminare
	err := db.View(func(tx *buntdb.Tx) error {
		return tx.AscendKeys("*", func(key, value string) bool {
			updatedStr := extractJSONStringField(value, "updated")
			if updatedStr == "" {
				return true // nessun campo updated, salta
			}
			t, err := time.Parse(time.RFC3339Nano, updatedStr)
			if err != nil {
				return true // data non parsabile, salta
			}
			if t.Before(cutoff) {
				toDelete = append(toDelete, key)
			}
			return true
		})
	})
	if err != nil {
		return 0, fmt.Errorf("[Cleanup] scan error: %w", err)
	}

	if len(toDelete) == 0 {
		return 0, nil
	}

	// Fase 2: eliminazione in batch da 1000 per non tenere il lock troppo a lungo
	deleted := 0
	batchSize := 1000
	for i := 0; i < len(toDelete); i += batchSize {
		end := i + batchSize
		if end > len(toDelete) {
			end = len(toDelete)
		}
		batch := toDelete[i:end]

		err = db.Update(func(tx *buntdb.Tx) error {
			for _, key := range batch {
				if _, err := tx.Delete(key); err != nil && err != buntdb.ErrNotFound {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return deleted, fmt.Errorf("[Cleanup] delete error: %w", err)
		}
		deleted += len(batch)
	}

	// Fase 3: Shrink compatta il file su disco e libera la RAM
	// senza Shrink il file rimane della dimensione originale
	if err := db.Shrink(); err != nil {
		return deleted, fmt.Errorf("[Cleanup] shrink error: %w", err)
	}

	return deleted, nil
}

// extractJSONStringField estrae il valore di un campo stringa da un JSON grezzo
// senza fare Unmarshal completo — molto più veloce su 100k+ oggetti.
func extractJSONStringField(jsonStr, field string) string {
	needle := `"` + field + `":"`
	idx := strings.Index(jsonStr, needle)
	if idx == -1 {
		return ""
	}
	start := idx + len(needle)
	end := strings.Index(jsonStr[start:], `"`)
	if end == -1 {
		return ""
	}
	return jsonStr[start : start+end]
}
