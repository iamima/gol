/*
	in-memory key-value store
	(c) 2013 Cergoo
	under terms of ISC license
*/
package cache

import (
	"encoding/gob"
	"fmt"
	"gol/counter"
	"io"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	t_item struct {
		r   uint8
		Val interface{}
		Key string
	}

	t_bucket struct {
		items []*t_item
		sync.RWMutex
	}

	t_hash struct {
		ht   []*t_bucket         // hash table
		hash func([]byte) uint32 // hash function
	}

	t_cache struct {
		t              *t_hash           // hash table
		duration       time.Duration     // duration janitor
		ifreadthenlive bool              // ifreadthenlive enable
		count          counter.T_counter // limit items count in cache, 0 - unlimit
		growcountgo    int               // count items in bucket to go grow hase table
		growlock       uint32
		callback       func(key *string, val *interface{})
	}
)

// Key to ID function
func (t *t_hash) keyToID(key []byte) uint32 {
	return t.hash(key) % uint32(len(t.ht))
}

/*
	Constructor new cache
	ifreadthenlive - if item read then item live
	duration - time to clear items, if 0 then never
*/
func New(
	hash func([]byte) uint32,
	init_bucketscount int,
	ifreadthenlive bool,
	duration time.Duration,
	callback func(key *string, val *interface{})) Cache {

	const growcountgo = 14
	if init_bucketscount < 1 {
		init_bucketscount = 1000
	}

	ht := new(t_hash)
	ht.ht = make([]*t_bucket, init_bucketscount)
	ht.hash = hash
	for i := range ht.ht {
		ht.ht[i] = &t_bucket{
			items: make([]*t_item, 0, growcountgo+4),
		}
	}

	t := &t_cache{
		duration:       duration,
		ifreadthenlive: ifreadthenlive,
		t:              ht,
		growcountgo:    growcountgo,
		callback:       callback,
	}

	if duration > 0 {
		chan_stop := make(chan bool)
		stopJanitor := func(t *t_cache) {
			close(chan_stop)
		}
		go t.janitor(chan_stop)
		runtime.SetFinalizer(t, stopJanitor)
	}

	return t
}

// Point to countern
func (t *t_cache) Len() I_counter {
	return &t.count
}

// Get statistics records Bucket
func (t *t_cache) GetBucketsStat() (countitem uint64, countbucket uint32, stat [][2]int) {
	var i int
	ht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
	tmp1 := make(map[int]int)
	for _, bucket := range ht.ht {
		bucket.RLock()
		tmp1[len(bucket.items)]++
		bucket.RUnlock()
		countbucket++
	}

	// sort
	tmp2 := make([]int, 0, len(tmp1))
	for i = range tmp1 {
		tmp2 = append(tmp2, i)
	}
	sort.Ints(tmp2)
	stat = make([][2]int, 0, len(tmp1))
	for i = range tmp2 {
		stat = append(stat, [2]int{tmp2[i], tmp1[tmp2[i]]})
	}
	countitem = t.count.Get()
	return
}

// Get item value or nil
func (t *t_cache) Get(key string) (val interface{}) {
	ht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
	bucket := ht.ht[ht.keyToID([]byte(key))]
	bucket.RLock()
	for _, v := range bucket.items {
		if v.Key == key {
			if t.ifreadthenlive {
				v.r = 1
			}
			val = v.Val
			break
		}
	}
	bucket.RUnlock()
	return
}

// Add or Update item
func (t *t_cache) Set(key string, val interface{}) (r bool) {
	var (
		v *t_item
	)
	ht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
	bucket := ht.ht[ht.keyToID([]byte(key))]
	bucket.Lock()
	// Update
	for _, v = range bucket.items {
		if v.Key == key {
			v.Val = val
			r = true
			bucket.Unlock()
			return
		}
	}
	// Add
	if t.count.Check() {
		lenbucet := len(bucket.items)
		bucket.items = append(bucket.items, &t_item{Key: key, Val: val, r: 1})
		bucket.Unlock()
		if lenbucet > t.growcountgo && atomic.CompareAndSwapUint32(&t.growlock, 0, 2) {
			go t.grow()
		}
		r = true
		t.count.Inc()
	} else {
		bucket.Unlock()
	}

	return
}

// Get and Delete item key
func (t *t_cache) Del(key string) (val interface{}) {
	var endi int
	ht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
	bucket := ht.ht[ht.keyToID([]byte(key))]
	bucket.Lock()
	for i, v := range bucket.items {
		if v.Key == key {
			val = v.Val
			bucket.items[i] = nil
			endi = len(bucket.items) - 1
			if i != endi {
				bucket.items[i], bucket.items[endi] = bucket.items[endi], bucket.items[i]
			}
			bucket.items = bucket.items[:endi]
			t.count.Dec()
			break
		}
	}
	bucket.Unlock()
	return
}

// Incremet and Decrement any type, return modified value or nil
func (t *t_cache) Inc(key string, n float64) interface{} {
	ht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
	bucket := ht.ht[ht.keyToID([]byte(key))]

	bucket.Lock()
	for _, v := range bucket.items {
		if v.Key == key {
			switch value := v.Val.(type) {
			case int:
				v.Val = value + int(n)
			case int8:
				v.Val = value + int8(n)
			case int16:
				v.Val = value + int16(n)
			case int32:
				v.Val = value + int32(n)
			case int64:
				v.Val = value + int64(n)
			case uint:
				v.Val = value + uint(n)
			case uint8:
				v.Val = value + uint8(n)
			case uint16:
				v.Val = value + uint16(n)
			case uint32:
				v.Val = value + uint32(n)
			case uint64:
				v.Val = value + uint64(n)
			case float32:
				v.Val = value + float32(n)
			case float64:
				v.Val = value + n
			}

			v.r = 1
			bucket.Unlock()
			return v.Val
		}
	}
	bucket.Unlock()
	return nil
}

// Write the cache's items (using Gob) to an io.Writer.
func (t *t_cache) Save(w io.Writer) (err error) {
	var (
		item   *t_item
		bucket *t_bucket
	)
	enc := gob.NewEncoder(w)
	ht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))

	defer func() {
		if x := recover(); x != nil {
			for _, bucket = range ht.ht {
				bucket.RUnlock()
			}
			err = fmt.Errorf("Error registering item types with Gob library")
		}
	}()
	for _, bucket = range ht.ht {
		bucket.RLock()
		for _, item = range bucket.items {
			gob.Register(item.Val)
			err = enc.Encode(*item)
			if err != nil {
				return
			}
		}
		bucket.RUnlock()
	}
	return
}

// Save the cache's items to the given filename, creating the file if it
// doesn't exist, and overwriting it if it does.
func (t *t_cache) SaveFile(fname string) error {
	fp, err := os.Create(fname)
	if err != nil {
		return err
	}
	err = t.Save(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Add (Gob-serialized) cache items from an io.Reader, excluding any items with
// keys that already exist (and haven't expired) in the current cache.
func (t *t_cache) Load(r io.Reader) error {
	var (
		err  error
		item t_item
	)
	dec := gob.NewDecoder(r)
	for err = dec.Decode(&item); err == nil; err = dec.Decode(&item) {
		t.Set(item.Key, item.Val)
	}
	return err
}

// Load and add cache items from the given filename, excluding any items with
// keys that already exist in the current cache.
func (t *t_cache) LoadFile(fname string) error {
	fp, err := os.Open(fname)
	if err != nil {
		return err
	}
	err = t.Load(fp)
	if err != nil && err.Error() != "EOF" {
		fp.Close()
		return err
	}
	return fp.Close()
}

/*
	Grow hash table procedure
	old hash table (n buckets)	->	new hash table	(n*2 buckets)
	example:
	0    0       0Lock   0       0       0       0
	1    1       1       1Lock   1       1       1
	2    2       2       2       2Lock   2       2
	3    3       3       3       3       3Lock   3
	     4Lock   4Lock   4       4       4       4
	     5Lock   5Lock   5Lock   5       5       5
	     6Lock   6Lock   6Lock   6Lock   6       6
	     7Lock   7Lock   7Lock   7Lock   7Lock   7
*/
func (t *t_cache) grow() {
	var (
		i, j int
		val  *t_item
	)

	oldht := (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
	oldlen := len(oldht.ht)
	newlen := oldlen << 1
	newht := new(t_hash)
	newht.ht = make([]*t_bucket, newlen)
	copy(newht.ht, oldht.ht) // possible since it links
	for i = oldlen; i < newlen; i++ {
		newht.ht[i] = &t_bucket{}
		newht.ht[i].Lock()
		j++
	}
	newht.hash = oldht.hash
	atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&t.t)), unsafe.Pointer(newht))
	oldht = nil
	// rehash

	j = oldlen
	for i = 0; i < oldlen; i++ {
		itemsold := make([]*t_item, 0, t.growcountgo+4)
		itemsnew := make([]*t_item, 0, t.growcountgo+4)
		newht.ht[i].Lock()
		for _, val = range newht.ht[i].items {
			if newht.keyToID([]byte(val.Key)) == uint32(i) {
				itemsold = append(itemsold, val)
			} else {
				itemsnew = append(itemsnew, val)
			}
		}

		newht.ht[j].items = itemsnew
		newht.ht[j].Unlock()
		newht.ht[i].items = itemsold
		newht.ht[i].Unlock()
		j++
	}
	// grow only singleton runed
	atomic.StoreUint32(&t.growlock, 0)
}

func (t *t_cache) janitor(stop <-chan bool) {
	var (
		i, lenbucket int
		ht           *t_hash
		bucket       *t_bucket
		count_del    uint32
	)

	for {
		select {
		case <-stop:
			return
		default:
			time.Sleep(t.duration)
			count_del = 0
			ht = (*t_hash)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&t.t))))
			for _, bucket = range ht.ht {
				bucket.Lock()
				lenbucket = len(bucket.items)
				for i = 0; i < lenbucket; {
					if bucket.items[i].r == 0 {
						if t.callback != nil {
							t.callback(&bucket.items[i].Key, &bucket.items[i].Val)
						}
						bucket.items[i].Val = nil
						lenbucket--
						bucket.items[i], bucket.items[lenbucket] = bucket.items[lenbucket], bucket.items[i]
						count_del++
					} else {
						bucket.items[i].r--
						i++
					}

				}
				bucket.items = bucket.items[:lenbucket]
				bucket.Unlock()
			}
			t.count.Add(uint64(count_del), true)
		}
	}
}
