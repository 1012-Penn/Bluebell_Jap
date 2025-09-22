// Package main Bluebell社区论坛系统主程序入口
//
// 这是一个基于Go语言开发的社区论坛系统，采用经典的三层架构设计：
// - Controller层：处理HTTP请求和响应
// - Logic层：处理业务逻辑
// - DAO层：处理数据访问
//
// 技术栈：
// - Web框架：Gin
// - 数据库：MySQL + Redis
// - 认证：JWT
// - 日志：Zap
// - ID生成：雪花算法
// - 缓存：多级缓存策略
package main

import (
	"context"
	"fmt"
	"os"

	"bluebell/controller"
	"bluebell/dao/mysql"
	"bluebell/dao/redis"
	"bluebell/logger"
	"bluebell/logic"
	"bluebell/pkg/snowflake"
	"bluebell/router"
	"bluebell/setting"
)

// Swagger API文档注释
// @title bluebell项目接口文档
// @version 1.0
// @description Go web开发进阶项目实战课程bluebell
// @contact.name liwenzhou
// @contact.url http://www.liwenzhou.com
// @host 127.0.0.1:8084
// @BasePath /api/v1

// main 程序主入口函数
//
// 功能说明：
// 1. 解析命令行参数，获取配置文件路径
// 2. 初始化各个组件（配置、日志、数据库、Redis、雪花算法等）
// 3. 启动异步通知消费者
// 4. 注册路由并启动HTTP服务器
//
// 初始化顺序：
// 配置 -> 日志 -> MySQL -> Redis -> 雪花算法 -> 参数验证器 -> 路由 -> 启动服务
func main() {
	// 1. 检查命令行参数
	// 技术亮点：通过命令行参数传递配置文件路径，提高部署灵活性
	if len(os.Args) < 2 {
		// 如果命令行参数小于2，则提示需要配置文件
		// 命令行参数格式：程序名 配置文件名
		fmt.Println("need config file.eg: bluebell config.yaml")
		return
	}

	// 2. 加载配置文件
	// 技术亮点：使用Viper进行配置管理，支持多种配置格式
	if err := setting.Init(os.Args[1]); err != nil {
		fmt.Printf("load config failed, err:%v\n", err)
		return
	}

	// 3. 初始化日志系统
	// 技术亮点：使用Zap高性能日志库，支持结构化日志和日志轮转
	if err := logger.Init(setting.Conf.LogConfig, setting.Conf.Mode); err != nil {
		fmt.Printf("init logger failed, err:%v\n", err)
		return
	}

	// 4. 初始化MySQL数据库连接
	// 技术亮点：使用连接池管理数据库连接，提高性能
	if err := mysql.Init(setting.Conf.MySQLConfig); err != nil {
		fmt.Printf("init mysql failed, err:%v\n", err)
		return
	}
	defer mysql.Close() // 程序退出时关闭数据库连接，防止连接泄露

	// 5. 初始化Redis连接
	// 技术亮点：Redis作为缓存和消息队列，提高系统性能
	if err := redis.Init(setting.Conf.RedisConfig); err != nil {
		fmt.Printf("init redis failed, err:%v\n", err)
		return
	}
	defer redis.Close() // 程序退出时关闭Redis连接

	// 6. 启动异步通知消费者
	// 技术亮点：使用消息队列异步处理通知，提高响应速度
	// 配合点赞、评论写入MQ的策略，实现解耦和异步处理
	logic.StartNotificationConsumer(context.Background())

	// 7. 初始化雪花算法
	// 技术亮点：使用雪花算法生成分布式唯一ID，支持高并发
	if err := snowflake.Init(setting.Conf.StartTime, setting.Conf.MachineID); err != nil {
		fmt.Printf("init snowflake failed, err:%v\n", err)
		return
	}

	// 8. 初始化参数验证器翻译器
	// 技术亮点：使用validator进行参数验证，支持中文错误信息
	if err := controller.InitTrans("zh"); err != nil {
		fmt.Printf("init validator trans failed, err:%v\n", err)
		return
	}

	// 9. 注册路由并启动HTTP服务器
	// 技术亮点：使用Gin框架，支持中间件、路由分组等高级特性
	r := router.SetupRouter(setting.Conf.Mode)
	err := r.Run(fmt.Sprintf(":%d", setting.Conf.Port))
	if err != nil {
		fmt.Printf("run server failed, err:%v\n", err)
		return
	}
}
