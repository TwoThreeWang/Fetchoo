package cache

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"web_fetcher/internal/types"
)

// SQLiteCache 轻量级缓存（纯 Go 实现，无 CGO）
type SQLiteCache struct {
	DB      *sql.DB
	TTLDays int
}

func NewSQLiteCache(dbPath string, ttlDays int) (*SQLiteCache, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	c := &SQLiteCache{DB: db, TTLDays: ttlDays}
	c.initDB()
	return c, nil
}

func (c *SQLiteCache) initDB() {
	c.DB.Exec(`CREATE TABLE IF NOT EXISTS cache (
		url_hash TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		content TEXT NOT NULL,
		metadata TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
}

func (c *SQLiteCache) urlHash(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	return hex.EncodeToString(h[:])[:16]
}

// Get 获取缓存
func (c *SQLiteCache) Get(rawURL string) (string, types.WebMetadata, bool) {
	hash := c.urlHash(rawURL)

	var content, metaJSON, createdAt string
	err := c.DB.QueryRow(
		"SELECT content, metadata, created_at FROM cache WHERE url_hash = ?",
		hash,
	).Scan(&content, &metaJSON, &createdAt)
	if err != nil {
		return "", types.WebMetadata{}, false
	}

	// 检查 TTL
	var createdTime time.Time
	for _, fmtStr := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z07:00",
	} {
		if t, err := time.Parse(fmtStr, createdAt); err == nil {
			createdTime = t
			break
		}
	}
	if createdTime.IsZero() || time.Since(createdTime) > time.Duration(c.TTLDays)*24*time.Hour {
		c.Delete(rawURL)
		return "", types.WebMetadata{}, false
	}

	var meta types.WebMetadata
	if err := json.Unmarshal([]byte(metaJSON), &meta); err != nil {
		return "", types.WebMetadata{}, false
	}

	return content, meta, true
}

// Set 存储缓存
func (c *SQLiteCache) Set(rawURL string, content string, meta types.WebMetadata) {
	hash := c.urlHash(rawURL)
	metaBytes, _ := json.Marshal(meta)

	_, err := c.DB.Exec(
		`INSERT OR REPLACE INTO cache (url_hash, url, content, metadata) VALUES (?, ?, ?, ?)`,
		hash, rawURL, content, string(metaBytes),
	)
	if err != nil {
		log.Printf("[cache] set error: %v", err)
	}
}

// Delete 删除缓存
func (c *SQLiteCache) Delete(rawURL string) {
	hash := c.urlHash(rawURL)
	c.DB.Exec("DELETE FROM cache WHERE url_hash = ?", hash)
}

// CleanupExpired 清理过期缓存
func (c *SQLiteCache) CleanupExpired() {
	cutoff := time.Now().Add(-time.Duration(c.TTLDays) * 24 * time.Hour).Format(time.RFC3339)

	result, err := c.DB.Exec("DELETE FROM cache WHERE created_at < ?", cutoff)
	if err != nil {
		return
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		log.Printf("[cache] 已清理 %d 条过期缓存", rowsAffected)
	}
}

// Close 关闭连接
func (c *SQLiteCache) Close() { c.DB.Close() }
