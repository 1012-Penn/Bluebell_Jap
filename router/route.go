// Package router 路由配置包
//
// 负责配置HTTP路由和中间件，是系统的入口配置
// 主要职责：
// 1. 配置API路由
// 2. 设置中间件
// 3. 路由分组管理
// 4. 错误处理配置
// 5. 性能监控配置
package router

import (
	"bluebell/controller"
	"bluebell/logger"
	"bluebell/middlewares"
	"net/http"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

// SetupRouter 设置并返回Gin路由引擎
//
// 功能说明：
// 1. 根据运行模式配置Gin引擎
// 2. 注册全局中间件（日志、恢复）
// 3. 配置API路由分组
// 4. 设置认证中间件
// 5. 配置性能监控和错误处理
//
// 参数说明：
// - mode: 运行模式（debug/release）
//
// 返回值：
// - *gin.Engine: 配置好的Gin路由引擎
//
// 技术亮点：
// - 路由分组，便于管理和维护
// - 中间件分层，功能解耦
// - 认证保护，安全性高
// - 性能监控，便于调试
func SetupRouter(mode string) *gin.Engine {
	// 1. 根据运行模式设置Gin模式
	// 技术亮点：生产环境使用Release模式，提高性能
	if mode == gin.ReleaseMode {
		gin.SetMode(gin.ReleaseMode) // 发布模式，禁用调试信息
	}

	// 2. 创建Gin引擎实例
	// 技术亮点：使用gin.New()而不是gin.Default()，避免默认中间件
	r := gin.New()

	// 3. 注册全局中间件
	// 技术亮点：中间件顺序很重要，日志记录在恢复之前
	// 注释掉的限流中间件可以根据需要启用
	//r.Use(logger.GinLogger(), logger.GinRecovery(true), middlewares.RateLimitMiddleware(2*time.Second, 1))
	r.Use(logger.GinLogger(), logger.GinRecovery(true))

	// 4. 健康检查接口
	// 技术亮点：提供健康检查接口，便于负载均衡器检查服务状态
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	// 5. API路由分组
	// 技术亮点：使用路由分组，所有路由自动添加/api/v1前缀
	// 优势：避免路由冲突、便于版本管理、统一API前缀
	v1 := r.Group("/api/v1")
	{
		// 5.1 公开接口（无需认证）
		// 用户注册和登录接口
		v1.POST("/signup", controller.SignUpHandler) // 用户注册
		v1.POST("/login", controller.LoginHandler)   // 用户登录

		// 帖子相关公开接口
		v1.GET("/posts2", controller.GetPostListHandler2)           // 升级版帖子列表
		v1.GET("/posts", controller.GetPostListHandler)             // 基础版帖子列表
		v1.GET("/community", controller.CommunityHandler)           // 社区列表
		v1.GET("/community/:id", controller.CommunityDetailHandler) // 社区详情
		v1.GET("/post/:id", controller.GetPostDetailHandler)        // 帖子详情

		// 5.2 应用JWT认证中间件
		// 技术亮点：中间件只对后续路由生效，实现部分接口保护
		// 在Use之后的所有路由都会被认证中间件保护
		v1.Use(middlewares.JWTAuthMiddleware())

		// 5.3 需要认证的接口
		{
			v1.POST("/post", controller.CreatePostHandler)  // 创建帖子
			v1.POST("/vote", controller.PostVoteController) // 帖子投票
		}
	}

	// 6. 注册性能监控路由
	// 技术亮点：集成pprof性能分析工具，便于性能调试
	// 访问 /debug/pprof/ 查看性能数据
	pprof.Register(r)

	// 7. 404处理
	// 技术亮点：统一处理未匹配的路由，提供友好的错误信息
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"msg": "404",
		})
	})

	return r
}
