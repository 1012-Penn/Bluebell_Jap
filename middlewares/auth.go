// Package middlewares 中间件包
//
// 负责实现各种HTTP中间件，提供横切关注点功能
// 主要职责：
// 1. 用户认证和授权
// 2. 请求限流和防护
// 3. 日志记录和监控
// 4. 错误处理和恢复
// 5. 跨域处理等
package middlewares

import (
	"bluebell/controller"
	"bluebell/pkg/jwt"
	"strings"

	"github.com/gin-gonic/gin"
)

// JWTAuthMiddleware JWT认证中间件
//
// 功能说明：
// 1. 从请求头中提取JWT令牌
// 2. 验证令牌的有效性
// 3. 解析用户信息并存储到上下文
// 4. 保护需要认证的接口
//
// 返回值：
// - func(c *gin.Context): Gin中间件函数
//
// 技术亮点：
// - 无状态认证，支持分布式部署
// - 统一的认证逻辑，便于维护
// - 用户信息存储到上下文，便于后续使用
// - 支持Bearer Token标准格式
func JWTAuthMiddleware() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 1. 从请求头获取Authorization字段
		// 技术亮点：支持标准的Bearer Token格式
		// 格式：Authorization: Bearer <token>
		// 其他可选格式：X-TOKEN: <token>
		authHeader := c.Request.Header.Get("Authorization")
		if authHeader == "" {
			// 未提供认证信息，返回需要登录错误
			controller.ResponseError(c, controller.CodeNeedLogin)
			c.Abort() // 终止请求处理
			return
		}

		// 2. 解析Bearer Token格式
		// 技术亮点：使用SplitN确保只分割成两部分，避免token中包含空格的问题
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			// Token格式不正确，返回无效token错误
			controller.ResponseError(c, controller.CodeInvalidToken)
			c.Abort()
			return
		}

		// 3. 解析JWT令牌
		// 技术亮点：验证令牌签名和过期时间
		mc, err := jwt.ParseToken(parts[1])
		if err != nil {
			// 令牌解析失败，返回无效token错误
			controller.ResponseError(c, controller.CodeInvalidToken)
			c.Abort()
			return
		}

		// 4. 将用户信息存储到请求上下文
		// 技术亮点：使用上下文传递用户信息，避免重复解析
		// 后续处理函数可以通过c.Get(controller.CtxUserIDKey)获取用户ID
		c.Set(controller.CtxUserIDKey, mc.UserID)

		// 5. 继续处理请求
		// 技术亮点：认证通过后，继续执行后续的中间件和处理器
		c.Next()
	}
}
