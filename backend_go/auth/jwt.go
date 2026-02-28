package auth

import (
	"log"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents Auth0 JWT claims
type Claims struct {
	Sub   string   `json:"sub"`
	Email string   `json:"email,omitempty"`
	Roles []string `json:"https://superintendent/roles,omitempty"`
	jwt.RegisteredClaims
}

// RequireAdmin middleware - validates JWT via JWKS and ensures admin role
func RequireAdmin(jwksURL string) gin.HandlerFunc {
	var k keyfunc.Keyfunc
	if jwksURL != "" {
		var err error
		k, err = keyfunc.NewDefault([]string{jwksURL})
		if err != nil {
			log.Fatalf("failed to create JWKS from %s: %v", jwksURL, err)
		}
	}

	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{"error": "missing or invalid authorization"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		if jwksURL == "" {
			c.Set("claims", &Claims{Sub: "dev-admin", Roles: []string{"admin"}})
			c.Next()
			return
		}

		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, k.Keyfunc)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		claims, ok := token.Claims.(*Claims)
		if !ok || !token.Valid {
			c.JSON(401, gin.H{"error": "invalid claims"})
			c.Abort()
			return
		}

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
