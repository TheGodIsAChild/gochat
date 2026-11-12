package middleware

import (
	"gochat/global"
	"gochat/internal/auth"
	"strings"

	"github.com/gin-gonic/gin"
)

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(401, gin.H{"code": 401, "message": msg})
}

// 校验access token 并将userid与username放入gin.context
func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			abortUnauthorized(c, "缺少 Authorization")
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			abortUnauthorized(c, "Authorzation 格式错误")
			return
		}
		claims, err := auth.Parse(global.Config.JWT.Secret, parts[1])
		if err != nil {
			abortUnauthorized(c, "token已过期或token无效")
			return
		}
		// 防止将refresh token当作 access token使用------ refresh token也能使用，但是因为它是长期有效的，不能使用它调用业务接口，校验防止漏洞
		if claims.Type != auth.TypeAccess {
			abortUnauthorized(c, "token 类型错误")
			return
		}
		c.Set("userId", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}
