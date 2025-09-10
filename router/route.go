package router

import (
	"bluebell/controller"
	"bluebell/logger"
	"bluebell/middlewares"
	"net/http"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

func SetupRouter(mode string) *gin.Engine {
	if mode == gin.ReleaseMode {
		gin.SetMode(gin.ReleaseMode) // gin设置成发布模式
	}
	r := gin.New()
	//r.Use(logger.GinLogger(), logger.GinRecovery(true), middlewares.RateLimitMiddleware(2*time.Second, 1))
	r.Use(logger.GinLogger(), logger.GinRecovery(true))

	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	// 路由分组
	//所有在这个分组下的路由都会被添加前缀/api/v1
	//路由分组的好处:   1. 可以避免路由冲突 2. 可以方便地管理路由
	//添加大括号是为了让v1变量在当前作用域内有效
	v1 := r.Group("/api/v1")
	{
		// 注册
		v1.POST("/signup", controller.SignUpHandler)
		// 登录
		v1.POST("/login", controller.LoginHandler)

		// 根据时间或分数获取帖子列表
		v1.GET("/posts2", controller.GetPostListHandler2)
		v1.GET("/posts", controller.GetPostListHandler)
		v1.GET("/community", controller.CommunityHandler)
		v1.GET("/community/:id", controller.CommunityDetailHandler)
		v1.GET("/post/:id", controller.GetPostDetailHandler)

		v1.Use(middlewares.JWTAuthMiddleware()) // 应用JWT认证中间件
		// 在Use之后的所有路由都会被认证中间件保护,之前的不会被保护

		{
			v1.POST("/post", controller.CreatePostHandler)

			// 投票
			v1.POST("/vote", controller.PostVoteController)
		}
	}

	pprof.Register(r) // 注册pprof相关路由

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"msg": "404",
		})
	})
	return r
}
