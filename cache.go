package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/akrylysov/pogreb"
)

type cachable struct {
	PPNick      string
	PPAmount    float64
	ReminderDay int
}

var (
	cacheDir string
	dbDir    string
)

func (c cachable) String() string {
	return fmt.Sprintf(
		"Nickname PayPal ricevente: %s\nSomma richiesta: %f\nGiorno del reminder: %d",
		c.PPNick,
		c.PPAmount,
		c.ReminderDay,
	)
}

func Get(id int64) (c cachable, err error) {
	var (
		buf bytes.Buffer
		dec = gob.NewDecoder(&buf)
	)

	cc, err := pogreb.Open(dbDir, nil)
	if err != nil {
		return cachable{}, fmt.Errorf("Get, pogreb.Open, %w", err)
	}
	defer cc.Close()

	b, err := cc.Get(itob(id))
	if err != nil {
		return cachable{}, fmt.Errorf("Get, cc.Get, %w", err)
	}
	if _, err := buf.Write(b); err != nil {
		return cachable{}, fmt.Errorf("Get, buf.Write, %w", err)
	}

	if err = dec.Decode(&c); err != nil {
		err = fmt.Errorf("Get, dec.Decode, %w", err)
	}
	return
}

func (c cachable) Put(id int64) error {
	cc, err := pogreb.Open(dbDir, nil)
	if err != nil {
		return fmt.Errorf("Put, pogreb.Open, %w", err)
	}
	defer cc.Close()

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(c); err != nil {
		return fmt.Errorf("Put, gob.NewEncoder(&buf).Encode(c), %w", err)
	}

	if err := cc.Put(itob(id), buf.Bytes()); err != nil {
		return fmt.Errorf("Put, cc.Put, %w", err)
	}
	return nil
}

func itob(i int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(i))
	return b
}

func init() {
	cd, err := os.UserCacheDir()
	if err != nil {
		log.Fatal("init", "os.UserCacheDir", err)
	}
	cacheDir = filepath.Join(cd, "pagohtron")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, os.ModeDir); err != nil {
			log.Fatal("init", "os.MkdirAll", err)
		}
	}
	dbDir = filepath.Join(cacheDir, "cache")
}
