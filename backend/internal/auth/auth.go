// Package auth 提供基于内存令牌的极简会话（演示用，登录只需用户名）。
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"museum-license/internal/models"
)

const CookieName = "museum_session"

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]uint // token -> userID
	db       *gorm.DB
}

func New(db *gorm.DB) *Manager {
	return &Manager{sessions: make(map[string]uint), db: db}
}

func (m *Manager) Login(username string) (*models.User, string, error) {
	var u models.User
	if err := m.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, "", err
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	m.mu.Lock()
	m.sessions[token] = u.ID
	m.mu.Unlock()
	return &u, token, nil
}

func (m *Manager) Logout(token string) {
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// User 解析当前会话用户，未登录返回 nil
func (m *Manager) User(c *gin.Context) *models.User {
	token, err := c.Cookie(CookieName)
	if err != nil || token == "" {
		return nil
	}
	m.mu.RLock()
	uid, ok := m.sessions[token]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	var u models.User
	if err := m.db.First(&u, uid).Error; err != nil {
		return nil
	}
	return &u
}

// Middleware 注入 current_user；require 控制是否强制登录
func (m *Manager) Middleware(require bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if u := m.User(c); u != nil {
			c.Set("user", u)
		} else if require {
			c.AbortWithStatusJSON(401, gin.H{"error": "请先登录"})
			return
		}
		c.Next()
	}
}

func CurrentUser(c *gin.Context) *models.User {
	v, ok := c.Get("user")
	if !ok {
		return nil
	}
	return v.(*models.User)
}

func RequireStaff() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if u == nil || u.Role != models.RoleStaff {
			c.AbortWithStatusJSON(403, gin.H{"error": "仅馆方人员可操作"})
			return
		}
		c.Next()
	}
}
