package main

import (
	"encoding/binary"
	"hash/fnv"
	"sync"
)

type HashableKey interface {
	~string | ~int | ~int64 | ~uint64 | ~uintptr
}

func hashKey[K HashableKey](key K) uint64 {
	h := fnv.New64a()
	switch k := any(key).(type) {
	case string:
		h.Write([]byte(k))
	case int:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(k))
		h.Write(buf[:])
	case int64:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(k))
		h.Write(buf[:])
	case uint64:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], k)
		h.Write(buf[:])
	case uintptr:
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], uint64(k))
		h.Write(buf[:])
	}
	return h.Sum64()
}

type Shard[K HashableKey, V any] struct {
	m map[K]V
	sync.RWMutex
}

type ShardedMap[K HashableKey, V any] struct {
	shards []*Shard[K, V]
}

func NewShardedMap[K HashableKey, V any](count uint64) *ShardedMap[K, V] {
	if count <= 0 {
		count = 32
	}

	sm := &ShardedMap[K, V]{
		shards: make([]*Shard[K, V], count),
	}

	for i := range sm.shards {
		sm.shards[i] = &Shard[K, V]{
			m: make(map[K]V),
		}
	}
	return sm
}

func (sm *ShardedMap[K, V]) getShardIndex(key K) int {
	checksum := hashKey(key)
	return int(checksum % uint64(len(sm.shards)))
}

func (sm *ShardedMap[K, V]) getShard(key K) *Shard[K, V] {
	idx := sm.getShardIndex(key)
	return sm.shards[idx]
}

func (sm *ShardedMap[K, V]) Get(key K) (V, bool) {
	shard := sm.getShard(key)
	shard.RLock()
	defer shard.RUnlock()

	value, ok := shard.m[key]

	return value, ok
}

func (sm *ShardedMap[K, V]) Set(key K, val V) {
	shard := sm.getShard(key)
	shard.Lock()
	defer shard.Unlock()

	shard.m[key] = val
}

func (sm *ShardedMap[K, V]) Delete(key K) {
	shard := sm.getShard(key)
	shard.Lock()
	defer shard.Unlock()

	delete(shard.m, key)
}

func (sm *ShardedMap[K, V]) Keys() []K {
	keys := make([]K, 0)
	for _, s := range sm.shards {
		s.RLock()
		for key := range s.m {
			keys = append(keys, key)
		}
		s.RUnlock()
	}
	return keys
}
