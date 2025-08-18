package auth

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"distributed-job-processor/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type Role string

const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleWorker Role = "worker"
)

type Claims struct {
	UserID string `json:"user_id"`
	Role   Role   `json:"role"`
	jwt.RegisteredClaims
}

type AuthConfig struct {
	Enabled   bool
	JWTSecret string
	TokenTTL  time.Duration
}

type AuthManager struct {
	config *AuthConfig
}

func NewAuthManager(config *AuthConfig) *AuthManager {
	return &AuthManager{
		config: config,
	}
}

func (a *AuthManager) GenerateToken(userID string, role Role) (string, error) {
	if !a.config.Enabled {
		return "", nil
	}

	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.config.TokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "distributed-job-processor",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(a.config.JWTSecret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

func (a *AuthManager) ValidateToken(tokenString string) (*Claims, error) {
	if !a.config.Enabled {
		return &Claims{
			UserID: "system",
			Role:   RoleAdmin,
		}, nil
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(a.config.JWTSecret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

func (a *AuthManager) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.config.Enabled {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Bearer token required"})
			c.Abort()
			return
		}

		claims, err := a.ValidateToken(tokenString)
		if err != nil {
			logger.WithError(err).Warn("Invalid token provided")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func (a *AuthManager) RequireRole(requiredRole Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.config.Enabled {
			c.Next()
			return
		}

		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "Role not found in context"})
			c.Abort()
			return
		}

		userRole, ok := role.(Role)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid role type"})
			c.Abort()
			return
		}

		if !a.hasPermission(userRole, requiredRole) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (a *AuthManager) hasPermission(userRole, requiredRole Role) bool {
	roleHierarchy := map[Role]int{
		RoleWorker: 1,
		RoleUser:   2,
		RoleAdmin:  3,
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	if !userExists || !requiredExists {
		return false
	}

	return userLevel >= requiredLevel
}

func (a *AuthManager) LoginHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var loginRequest struct {
			Username string `json:"username" binding:"required"`
			Password string `json:"password" binding:"required"`
		}

		if err := c.ShouldBindJSON(&loginRequest); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		role := a.authenticateUser(loginRequest.Username, loginRequest.Password)
		if role == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
			return
		}

		token, err := a.GenerateToken(loginRequest.Username, role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"token": token,
			"role":  role,
		})
	}
}

func (a *AuthManager) authenticateUser(username, password string) Role {
	hardcodedUsers := map[string]struct {
		password string
		role     Role
	}{
		"admin":  {"admin123", RoleAdmin},
		"user":   {"user123", RoleUser},
		"worker": {"worker123", RoleWorker},
	}

	if user, exists := hardcodedUsers[username]; exists && user.password == password {
		return user.role
	}

	return ""
}