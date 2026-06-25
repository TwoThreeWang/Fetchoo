package fingerprint

import (
	"math/rand"
	"sync"
)

// BrowserProfile 浏览器档案 - 确保多个层面的指纹一致性
type BrowserProfile struct {
	UserAgent           string // User-Agent 字符串
	Platform            string // navigator.platform (Win32 或 MacIntel)
	UADataPlatform      string // navigator.userAgentData.platform (Windows 或 macOS)
	UADataPlatformVer   string // navigator.userAgentData.platformVersion
	GPURenderer         string // WebGL 渲染器标识
	Language            string // 浏览器语言
}

// 预定义的浏览器档案池 - 每个档案内部完全一致
var profiles = []BrowserProfile{
	// Windows 档案 1
	{
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
		Platform:          "Win32",
		UADataPlatform:    "Windows",
		UADataPlatformVer: "10.0.0",
		GPURenderer:       "ANGLE (Intel HD Graphics 630)",
		Language:          "zh-CN,zh,en",
	},
	// Windows 档案 2
	{
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
		Platform:          "Win32",
		UADataPlatform:    "Windows",
		UADataPlatformVer: "10.0.0",
		GPURenderer:       "ANGLE (AMD Radeon RX 580)",
		Language:          "zh-CN,zh,en",
	},
	// Windows 档案 3
	{
		UserAgent:         "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
		Platform:          "Win32",
		UADataPlatform:    "Windows",
		UADataPlatformVer: "10.0.0",
		GPURenderer:       "ANGLE (NVIDIA GeForce GTX 1060)",
		Language:          "zh-CN,zh,en",
	},
	// macOS 档案 1
	{
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
		Platform:          "MacIntel",
		UADataPlatform:    "macOS",
		UADataPlatformVer: "14.6.0",
		GPURenderer:       "ANGLE (Apple Metal)",
		Language:          "zh-CN,zh,en",
	},
	// macOS 档案 2
	{
		UserAgent:         "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
		Platform:          "MacIntel",
		UADataPlatform:    "macOS",
		UADataPlatformVer: "14.5.0",
		GPURenderer:       "ANGLE (Apple Metal)",
		Language:          "zh-CN,zh,en",
	},
}

// ProfileManager 档案管理器
type ProfileManager struct {
	mu            sync.RWMutex
	currentIdx    int
	rotateProfile bool
}

// NewProfileManager 创建新的档案管理器
func NewProfileManager(rotateProfile bool) *ProfileManager {
	return &ProfileManager{
		rotateProfile: rotateProfile,
		currentIdx:    rand.Intn(len(profiles)),
	}
}

// GetProfile 获取当前档案
func (pm *ProfileManager) GetProfile() BrowserProfile {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.rotateProfile {
		// 每次获取时随机选择不同档案
		idx := rand.Intn(len(profiles))
		return profiles[idx]
	}

	// 使用固定档案（单次会话内一致）
	return profiles[pm.currentIdx]
}

// SetProfile 手动设置特定档案（索引）
func (pm *ProfileManager) SetProfile(idx int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if idx >= 0 && idx < len(profiles) {
		pm.currentIdx = idx
	}
}

// ProfileCount 返回可用档案数量
func ProfileCount() int {
	return len(profiles)
}
