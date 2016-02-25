package db

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/boltdb/bolt"
)

type DB interface {
	Load(string, interface{}) (bool, error)
	Save(string, interface{}) error
}

type BoltDB struct {
	db *bolt.DB
}

var (
	versionIdent       = []byte("version")
	persistenceVersion = []byte{1, 0} // major.minor
	topBucket          = []byte("top")
)

func NewBoltDB(dbPathname string) (*BoltDB, error) {
	db, err := bolt.Open(dbPathname, 0660, nil)
	if err != nil {
		return nil, fmt.Errorf("[boltDB] Unable to open %s: %s", dbPathname, err)
	}
	// check the persistence version
	err = db.Update(func(tx *bolt.Tx) error {
		if top := tx.Bucket(topBucket); top == nil {
			top, err := tx.CreateBucket(topBucket)
			if err != nil {
				return err
			}
			if err := top.Put(versionIdent, persistenceVersion); err != nil {
				return err
			}
		} else {
			if checkVersion := top.Get(versionIdent); checkVersion != nil {
				if checkVersion[0] != persistenceVersion[0] {
					return fmt.Errorf("[boltDB] Cannot use persistence file %s - version %x", dbPathname, checkVersion)
				}
			}
		}
		return nil
	})
	return &BoltDB{db: db}, err
}

func (d *BoltDB) Load(ident string, data interface{}) (bool, error) {
	found := true
	return found, d.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(topBucket)
		v := b.Get([]byte(ident))
		if v == nil {
			found = false
			return nil
		}
		reader := bytes.NewReader(v)
		decoder := gob.NewDecoder(reader)
		err := decoder.Decode(data)
		return err
	})
}

func (d *BoltDB) Save(ident string, data interface{}) error {
	return d.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(topBucket)
		buf := new(bytes.Buffer)
		enc := gob.NewEncoder(buf)
		if err := enc.Encode(data); err != nil {
			return err
		}
		return b.Put([]byte(ident), buf.Bytes())
	})
}

func (d *BoltDB) Close() error {
	return d.db.Close()
}
