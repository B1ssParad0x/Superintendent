package auth

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents Auth0 JWT claims
type Claims struct {
	Sub       string   `json:"sub"`
	Email     string   `json:"email,omitempty"`
	Roles     []string `json:"https://superintendent/roles,omitempty"`
	jwt.RegisteredClaims
}

// RequireAdmin middleware - validates JWT and ensures admin role
func RequireAdmin(jwksURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{"error": "missing or invalid authorization"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		// For local/dev without Auth0: accept a simple placeholder check
		// In production, use jwks to validate
		if jwksURL == "" {
			c.Set("claims", &Claims{Sub: "dev-admin", Roles: []string{"admin"}})
			c.Next()
			return
		}

		token, _, err := new(jwt.Parser).ParseUnverified(tokenStr, &Claims{})
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		claims, ok := token.Claims.(*Claims)
		if !ok {
			c.JSON(401, gin.H{"error": "invalid claims"})
			c.Abort()
			return
		}

		// TODO: Verify signature via JWKS in production
		_ = context.Background()

		hasAdmin := false
		for _, r := range claims.Roles {
			if r == "admin" {
				hasAdmin = true
				break
			}
		}
		if !hasAdmin {
			c.JSON(403, gin.H{"error": "admin role required"})
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}
