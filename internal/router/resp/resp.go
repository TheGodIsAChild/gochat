// Package resp 统一 HTTP 响应格式，供所有 handler 复用。
//
// 统一格式：{"code":0,"message":"ok","data":{...}}
// 约定：code=0 表示成功，非 0 表示失败（这里直接用 HTTP 状态码当业务码，学习项目够用；
// 生产环境一般用独立的业务错误码，如 40001、50002）。
package resp

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// OK 返回成功响应。
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

// Fail 返回失败响应。
func Fail(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"code": status, "message": msg})
}
