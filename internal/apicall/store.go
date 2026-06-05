package apicall

import (
	"database/sql"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store 持久化每日 API 调用次数（纯 SQLite 嵌入式，无独立进程）
type Store struct {
	db        *sql.DB
	mu        sync.Mutex
	inMemory  int64
	lastFlush time.Time
}

func NewStore(db *sql.DB) (*Store, error) {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS daily_calls (
		date TEXT PRIMARY KEY,
		count INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return nil, err
	}

	s := &Store{
		db:        db,
		lastFlush: time.Now(),
	}

	// 启动定时 flush 协程
	go s.periodicFlush()

	return s, nil
}

func todayKey() string {
	return time.Now().Format("2006-01-02")
}

// Incr 增加一次调用（内存缓冲，异步落盘）
func (s *Store) Incr() {
	s.mu.Lock()
	s.inMemory++
	s.mu.Unlock()
}

// GetTodayCount 获取今日累计调用次数（含内存 + 已落盘）
func (s *Store) GetTodayCount() int64 {
	s.mu.Lock()
	mem := s.inMemory
	s.mu.Unlock()

	var dbCount int64
	err := s.db.QueryRow("SELECT count FROM daily_calls WHERE date = ?", todayKey()).Scan(&dbCount)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[apicall] query error: %v", err)
	}

	return dbCount + mem
}

// GetTotalCount 获取累计总调用次数（含内存 + 已落盘）
func (s *Store) GetTotalCount() int64 {
	s.mu.Lock()
	mem := s.inMemory
	s.mu.Unlock()

	var dbCount sql.NullInt64
	err := s.db.QueryRow("SELECT SUM(count) FROM daily_calls").Scan(&dbCount)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[apicall] query error: %v", err)
	}
	if dbCount.Valid {
		return dbCount.Int64 + mem
	}
	return mem
}

func (s *Store) flush() {
	s.mu.Lock()
	delta := s.inMemory
	if delta == 0 {
		s.mu.Unlock()
		return
	}
	s.inMemory = 0
	s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO daily_calls (date, count) VALUES (?, ?)
		 ON CONFLICT(date) DO UPDATE SET count = count + ?`,
		todayKey(), delta, delta,
	)
	if err != nil {
		log.Printf("[apicall] flush error: %v", err)
	}
}

func (s *Store) periodicFlush() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.flush()
	}
}

// Close 退出前做最终 flush
func (s *Store) Close() {
	s.flush()
}
