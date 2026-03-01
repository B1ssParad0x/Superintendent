package auth

import (
	"fmt"
	"log"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func newJWKS(jwksURL string) keyfunc.Keyfunc {
	if jwksURL == "" {
		return nil
	}
	k, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		log.Fatalf("failed to create JWKS from %s: %v", jwksURL, err)
	}
	return k
}

func parseClaims(tokenStr string, k keyfunc.Keyfunc) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, k.Keyfunc)
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

// RequireUser validates bearer token and stores claims in context.
// In local dev mode (no Auth0 domain), Bearer dev is accepted.
func RequireUser(jwksURL string) gin.HandlerFunc {
	k := newJWKS(jwksURL)
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(401, gin.H{"error": "missing or invalid authorization"})
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")

		if jwksURL == "" {
			if tokenStr != "dev" {
				c.JSON(401, gin.H{"error": "invalid dev token"})
				c.Abort()
				return
			}
			c.Set("claims", &Claims{Sub: "dev-admin", Roles: []string{"admin"}})
			c.Next()
			return
		}

		claims, err := parseClaims(tokenStr, k)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		c.Set("claims", claims)
		c.Next()
	}
}

// Claims represents Auth0 JWT claims
type Claims struct {
	Sub   string   `json:"sub"`
	Email string   `json:"email,omitempty"`
	Roles []string `json:"https://superintendent/roles,omitempty"`
	jwt.RegisteredClaims
}

// RequireAdmin middleware - validates JWT via JWKS and ensures admin role
func RequireAdmin(jwksURL string) gin.HandlerFunc {
	k := newJWKS(jwksURL)

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

		claims, err := parseClaims(tokenStr, k)
		if err != nil {
			c.JSON(401, gin.H{"error": "invalid token"})
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
