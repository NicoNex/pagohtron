package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/akrylysov/pogreb"
)

type cachable struct {
	PPNick        string
	PPAmount      float64
	IsYearly      bool
	ReminderDay   int
	ReminderMonth time.Month
	Payers        []int64
	ReminderID    int
}

var (
	cc *pogreb.DB

	errIterationDone = pogreb.ErrIterationDone

	cacheDir  string
	dbDir     string
	pagahPath string
)

func (c cachable) String() string {
	return fmt.Sprintf(
		"Nickname PayPal ricevente: %s\nSomma richiesta: %f\nGiorno del reminder: %d",
		c.PPNick,
		c.PPAmount,
		c.ReminderDay,
	)
}

func Cachable(id int64) (c cachable, err error) {
	var (
		buf bytes.Buffer
		dec = gob.NewDecoder(&buf)
	)

	b, err := cc.Get(itob(id))
	if err != nil {
		return cachable{}, fmt.Errorf("Get cc.Get %w", err)
	}
	if _, err := buf.Write(b); err != nil {
		return cachable{}, fmt.Errorf("Get buf.Write %w", err)
	}

	if err = dec.Decode(&c); err != nil {
		err = fmt.Errorf("Get, dec.Decode, %w", err)
	}
	return
}

func (c cachable) Put(id int64) error {
	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(c); err != nil {
		return fmt.Errorf("Put gob.NewEncoder(&buf).Encode(c) %w", err)
	}
	if err := cc.Put(itob(id), buf.Bytes()); err != nil {
		return fmt.Errorf("Put cc.Put %w", err)
	}
	return nil
}

func fold(fn func(key int64, val cachable) error) (err error) {
	iter := cc.Items()
	for err == nil {
		var k, v []byte

		if k, v, err = iter.Next(); err == nil {
			err = fn(btoi(k), bytesToCachable(v))
		}
	}

	if errors.Is(err, pogreb.ErrIterationDone) {
		err = nil
	}
	return
}

func keys() []int64 {
	var keys []int64

	fold(func(k int64, _ cachable) error {
		keys = append(keys, k)
		return nil
	})
	return keys
}

func itob(i int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(i))
	return b
}

func btoi(b []byte) int64 {
	return int64(binary.BigEndian.Uint64(b))
}

func bytesToCachable(b []byte) (c cachable) {
	var (
		buf = bytes.NewBuffer(b)
		dec = gob.NewDecoder(buf)
	)
	dec.Decode(&c)
	return
}

func init() {
	const week = time.Hour * 24 * 7

	cd, err := os.UserCacheDir()
	if err != nil {
		log.Fatal("init", "os.UserCacheDir", err)
	}
	cacheDir = filepath.Join(cd, "pagohtron")
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			log.Fatal("init", "os.MkdirAll", err)
		}
	}
	dbDir = filepath.Join(cacheDir, "cache")
	pagahPath = filepath.Join(cacheDir, "pagah_id")

	cc, err = pogreb.Open(dbDir, &pogreb.Options{
		BackgroundSyncInterval:       -1,
		BackgroundCompactionInterval: week,
	})
	if err != nil {
		log.Fatal("init", "pogreb.Open", err)
	}
}
