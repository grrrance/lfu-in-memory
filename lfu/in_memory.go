package lfu

import (
	"errors"
	"math"
	"sync"
	"time"
)

type InMemoryCache struct {
	sync.Mutex
	items             map[string]Item
	freqGroup         map[uint64]map[string]struct{}
	minFreq           uint64
	defaultExpiration time.Duration
	cleanupInterval   time.Duration
	size              int
}

type Item struct {
	Value      interface{}
	Expiration time.Time
	Frequency  uint64
}

func (i Item) isExpired() bool {
	return time.Now().After(i.Expiration)
}

func NewInMemoryCache(size int, defaultExpiration, cleanupInterval time.Duration) *InMemoryCache {
	items := make(map[string]Item, size)
	freqGroup := make(map[uint64]map[string]struct{}, size)

	cache := InMemoryCache{
		items:             items,
		freqGroup:         freqGroup,
		minFreq:           1,
		defaultExpiration: defaultExpiration,
		cleanupInterval:   cleanupInterval,
		size:              size,
	}

	if cleanupInterval > 0 {
		go cache.startGC()
	}

	return &cache
}

func (c *InMemoryCache) getExp(duration time.Duration) time.Time {
	if duration <= 0 {
		duration = c.defaultExpiration
	}

	return time.Now().Add(duration)
}

func (c *InMemoryCache) Set(key string, value interface{}, duration time.Duration) {
	if c.size <= 0 {
		return
	}

	c.Lock()
	defer c.Unlock()

	newItem := Item{
		Value:      value,
		Expiration: c.getExp(duration),
		Frequency:  0,
	}

	if _, ok := c.items[key]; ok {
		newItem.Frequency = c.items[key].Frequency
		c.upgradeItem(newItem, key)
		return
	}

	if len(c.items) >= c.size {
		for keyToDelete := range c.freqGroup[c.minFreq] {
			c.deleteItemInGroup(c.items[keyToDelete], keyToDelete)
			delete(c.items, keyToDelete)
			break
		}
	}

	c.freqGroup[newItem.Frequency] = make(map[string]struct{})
	c.freqGroup[newItem.Frequency][key] = struct{}{}
	c.minFreq = newItem.Frequency
	c.upgradeItem(newItem, key)
}

func (c *InMemoryCache) Get(key string) (interface{}, bool) {

	c.Lock()

	defer c.Unlock()

	item, found := c.items[key]

	if !found {
		return nil, false
	}

	if item.isExpired() {
		return nil, false
	}

	c.upgradeItem(item, key)

	return item.Value, true
}

func (c *InMemoryCache) upgradeItem(item Item, key string) {
	c.items[key] = item
	newFreq := item.Frequency + 1
	if c.deleteItemInGroup(item, key) {
		c.minFreq = newFreq
	}
	item.Frequency = newFreq
	if _, ok := c.freqGroup[item.Frequency]; !ok {
		c.freqGroup[item.Frequency] = make(map[string]struct{})
	}
	c.freqGroup[item.Frequency][key] = struct{}{}
	c.items[key] = item
}

func (c *InMemoryCache) Delete(key string) error {
	c.Lock()
	defer c.Unlock()

	var item Item
	var found bool
	if item, found = c.items[key]; !found {
		return errors.New("Key not found")
	}

	delete(c.items, key)

	if c.deleteItemInGroup(item, key) {
		c.minFreq = c.findNewMinFreq()
	}

	return nil
}

func (c *InMemoryCache) deleteItemInGroup(item Item, key string) (minChanged bool) {
	delete(c.freqGroup[item.Frequency], key)
	if len(c.freqGroup[item.Frequency]) == 0 {
		delete(c.freqGroup, item.Frequency)
		return c.minFreq == item.Frequency
	}
	return false
}

func (c *InMemoryCache) findNewMinFreq() uint64 {
	minFreq := uint64(math.MaxUint64)
	for cur, _ := range c.freqGroup {
		if minFreq > cur {
			minFreq = cur
		}
	}

	return minFreq
}

func (c *InMemoryCache) Update(isUpdated func(v interface{}) bool, update func(v interface{}), duration time.Duration) {
	c.Lock()
	defer c.Unlock()

	exp := c.getExp(duration)

	for key, item := range c.items {
		if isUpdated(item.Value) && !item.isExpired() {
			update(item.Value)
			item.Expiration = exp
			c.upgradeItem(item, key)
		}
	}
}

func (c *InMemoryCache) startGC() {
	for range time.Tick(c.cleanupInterval) {
		c.Lock()

		var isFindMin bool
		for key, item := range c.items {
			if item.isExpired() {
				delete(c.items, key)
				isFindMin = c.deleteItemInGroup(item, key) || isFindMin
			}
		}

		if isFindMin {
			c.minFreq = c.findNewMinFreq()
		}

		c.Unlock()
	}
}
