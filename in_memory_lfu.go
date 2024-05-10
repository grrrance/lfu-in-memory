package cache

import "time"

type InMemoryLFU interface {
	Set(key string, value interface{}, duration time.Duration)
	Get(key string) (interface{}, bool)
	Delete(key string) error
	Update(isUpdated func(v interface{}) bool, update func(v interface{}), duration time.Duration)
}
