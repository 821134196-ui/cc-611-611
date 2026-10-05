// Package auth 提供密码哈希、JWT 签发与 Gin 鉴权中间件。
package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"museumlicense/internal/models"
)

type contextKey string

const userContextKey contextKey = "current_user"

// Manager 封装 JWT 密钥相关操作。
type Manager struct {
	secret []byte
}

func NewManager(secret string) *Manager {
	return &Manager{secret: []byte(secret)}
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

type Claims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"name"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func (m *Manager) IssueToken(u *models.User) (string, error) {
	claims := Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(m.secret)
}

func (m *Manager) parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// ParseToken 供无法设置 Authorization 头的场景（如 <img src>）使用。
func (m *Manager) ParseToken(tokenStr string) (*Claims, error) {
	return m.parse(tokenStr)
}

// RequireAuth 校验 Bearer Token，并把用户声明写入上下文。
func (m *Manager) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer"))
		if header == "" || token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		claims, err := m.parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已失效，请重新登录"})
			return
		}
		c.Set(string(userContextKey), claims)
		c.Next()
	}
}

// RequireRole 必须在 RequireAuth 之后使用。
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := CurrentUser(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		for _, r := range roles {
			if claims.Role == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "没有执行该操作的权限"})
	}
}

// CurrentUser 从 gin 上下文取出当前登录用户。
func CurrentUser(c *gin.Context) *Claims {
	v, ok := c.Get(string(userContextKey))
	if !ok {
		return nil
	}
	claims, ok := v.(*Claims)
	if !ok {
		return nil
	}
	return claims
}
