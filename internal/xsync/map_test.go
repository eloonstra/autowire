package xsync

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap_StoreAndLoad(t *testing.T) {
	var m Map[string, int]

	m.Store("key", 42)
	val, ok := m.Load("key")

	assert.True(t, ok)
	assert.Equal(t, 42, val)
}

func TestMap_Load_NotFound(t *testing.T) {
	var m Map[string, int]

	val, ok := m.Load("missing")

	assert.False(t, ok)
	assert.Equal(t, 0, val)
}

func TestMap_LoadOrStore_New(t *testing.T) {
	var m Map[string, string]

	val, loaded := m.LoadOrStore("key", "value")

	assert.False(t, loaded)
	assert.Equal(t, "value", val)
}

func TestMap_LoadOrStore_Existing(t *testing.T) {
	var m Map[string, string]
	m.Store("key", "first")

	val, loaded := m.LoadOrStore("key", "second")

	assert.True(t, loaded)
	assert.Equal(t, "first", val)
}

func TestMap_LoadAndDelete(t *testing.T) {
	var m Map[string, int]
	m.Store("key", 100)

	val, ok := m.LoadAndDelete("key")

	assert.True(t, ok)
	assert.Equal(t, 100, val)

	_, exists := m.Load("key")
	assert.False(t, exists)
}

func TestMap_LoadAndDelete_NotFound(t *testing.T) {
	var m Map[string, int]

	val, ok := m.LoadAndDelete("missing")

	assert.False(t, ok)
	assert.Equal(t, 0, val)
}

func TestMap_Delete(t *testing.T) {
	var m Map[string, int]
	m.Store("key", 1)

	m.Delete("key")

	_, ok := m.Load("key")
	assert.False(t, ok)
}

func TestMap_Range(t *testing.T) {
	var m Map[string, int]
	m.Store("a", 1)
	m.Store("b", 2)
	m.Store("c", 3)

	sum := 0
	m.Range(func(key string, value int) bool {
		sum += value
		return true
	})

	assert.Equal(t, 6, sum)
}

func TestMap_Range_EarlyExit(t *testing.T) {
	var m Map[int, int]
	for i := range 10 {
		m.Store(i, i)
	}

	count := 0
	m.Range(func(key int, value int) bool {
		count++
		return count < 3
	})

	assert.Equal(t, 3, count)
}

func TestMap_ConcurrentAccess(t *testing.T) {
	var m Map[int, int]
	var wg sync.WaitGroup

	iterations := 1000
	goroutines := 10

	for i := range goroutines {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := range iterations {
				key := base*iterations + j
				m.Store(key, key*2)
				m.Load(key)
			}
		}(i)
	}

	wg.Wait()

	count := 0
	m.Range(func(key int, value int) bool {
		count++
		return true
	})
	assert.Equal(t, goroutines*iterations, count)
}
