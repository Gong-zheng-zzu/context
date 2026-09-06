package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// LRUCache 线程安全的LRU缓存
type LRUCache struct {
	capacity  int
	cache     map[string]*list.Element
	lruList   *list.List
	mu        sync.RWMutex
	ttl       time.Duration // 缓存过期时间
	hitCount  int64         // 缓存命中次数
	missCount int64         // 缓存未命中次数
}

// CacheEntry 缓存条目
type CacheEntry struct {
	key       string
	value     interface{}
	timestamp time.Time
}

// NewLRUCache 创建LRU缓存
// capacity: 最大缓存条目数
// ttl: 缓存过期时间（0表示永不过期）
func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lruList:  list.New(),
		ttl:      ttl,
	}
}

// Get 获取缓存值
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, exists := c.cache[key]
	if !exists {
		c.missCount++
		return nil, false
	}

	entry := element.Value.(*CacheEntry)

	// 检查是否过期
	if c.ttl > 0 && time.Since(entry.timestamp) > c.ttl {
		c.lruList.Remove(element)
		delete(c.cache, key)
		c.missCount++
		return nil, false
	}

	// 移动到链表头部（最近使用）
	c.lruList.MoveToFront(element)
	c.hitCount++
	return entry.value, true
}

// Set 设置缓存值
func (c *LRUCache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 如果已存在，更新值并移到头部
	if element, exists := c.cache[key]; exists {
		c.lruList.MoveToFront(element)
		entry := element.Value.(*CacheEntry)
		entry.value = value
		entry.timestamp = time.Now()
		return
	}

	// 如果缓存已满，删除最久未使用的条目
	if c.lruList.Len() >= c.capacity {
		oldest := c.lruList.Back()
		if oldest != nil {
			c.lruList.Remove(oldest)
			oldEntry := oldest.Value.(*CacheEntry)
			delete(c.cache, oldEntry.key)
		}
	}

	// 添加新条目到头部
	entry := &CacheEntry{
		key:       key,
		value:     value,
		timestamp: time.Now(),
	}
	element := c.lruList.PushFront(entry)
	c.cache[key] = element
}

// Delete 删除缓存条目
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if element, exists := c.cache[key]; exists {
		c.lruList.Remove(element)
		delete(c.cache, key)
	}
}

// Clear 清空缓存
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lruList = list.New()
	c.hitCount = 0
	c.missCount = 0
}

// Size 获取当前缓存大小
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lruList.Len()
}

// GetStats 获取缓存统计信息
func (c *LRUCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hitCount + c.missCount
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hitCount) / float64(total) * 100
	}

	return CacheStats{
		Size:      c.lruList.Len(),
		Capacity:  c.capacity,
		HitCount:  c.hitCount,
		MissCount: c.missCount,
		HitRate:   hitRate,
	}
}

// CacheStats 缓存统计信息
type CacheStats struct {
	Size      int     // 当前缓存大小
	Capacity  int     // 最大容量
	HitCount  int64   // 命中次数
	MissCount int64   // 未命中次数
	HitRate   float64 // 命中率（百分比）
}

// HashKey 生成缓存键的哈希值
func HashKey(data string) string {
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// CleanupExpired 清理过期缓存（定期调用）
func (c *LRUCache) CleanupExpired() int {
	if c.ttl == 0 {
		return 0 // 永不过期
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	now := time.Now()

	// 遍历链表，删除过期条目
	for element := c.lruList.Back(); element != nil; {
		entry := element.Value.(*CacheEntry)
		if now.Sub(entry.timestamp) > c.ttl {
			next := element.Prev()
			c.lruList.Remove(element)
			delete(c.cache, entry.key)
			removed++
			element = next
		} else {
			break // 链表按时间排序，后面的都没过期
		}
	}

	return removed
}
