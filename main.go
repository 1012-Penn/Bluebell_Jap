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

// @title bluebell项目接口文档
// @version 1.0
// @description Go web开发进阶项目实战课程bluebell

// @contact.name liwenzhou
// @contact.url http://www.liwenzhou.com

// @host 127.0.0.1:8084
// @BasePath /api/v1
func main() {
	if len(os.Args) < 2 {
		// 如果命令行参数小于2,则提示需要配置文件;
		// 命令行参数: 程序名 配置文件名;
		fmt.Println("need config file.eg: bluebell config.yaml")
		return
	}
	// 加载配置
	if err := setting.Init(os.Args[1]); err != nil {
		fmt.Printf("load config failed, err:%v\n", err)
		return
	}
	if err := logger.Init(setting.Conf.LogConfig, setting.Conf.Mode); err != nil {
		fmt.Printf("init logger failed, err:%v\n", err)
		return
	}
	if err := mysql.Init(setting.Conf.MySQLConfig); err != nil {
		fmt.Printf("init mysql failed, err:%v\n", err)
		return
	}
	defer mysql.Close() // 程序退出关闭数据库连接
	if err := redis.Init(setting.Conf.RedisConfig); err != nil {
		fmt.Printf("init redis failed, err:%v\n", err)
		return
	}
	defer redis.Close()

	// 启动通知异步消费，配合点赞、评论写入 MQ 的策略。
	logic.StartNotificationConsumer(context.Background())

	if err := snowflake.Init(setting.Conf.StartTime, setting.Conf.MachineID); err != nil {
		fmt.Printf("init snowflake failed, err:%v\n", err)
		return
	}
	// 初始化gin框架内置的校验器使用的翻译器
	if err := controller.InitTrans("zh"); err != nil {
		fmt.Printf("init validator trans failed, err:%v\n", err)
		return
	}
	// 注册路由
	r := router.SetupRouter(setting.Conf.Mode)
	err := r.Run(fmt.Sprintf(":%d", setting.Conf.Port))
	if err != nil {
		fmt.Printf("run server failed, err:%v\n", err)
		return
	}
}
