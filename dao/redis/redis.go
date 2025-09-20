package redis

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"

	"bluebell/setting"
)

// 实际生产环境下 context.Background() 按需替换

var (
	client *redis.Client
	Nil    = redis.Nil
)

// Init 初始化连接
func Init(cfg *setting.RedisConfig) (err error) {
	client = redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password, // no password set
		DB:           cfg.DB,       // use default DB
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	_, err = client.Ping(context.Background()).Result()
	if err != nil {
		return err
	}
	return nil
}

func Close() {
	_ = client.Close()
}

// GetClient 返回初始化好的 Redis 客户端实例。
//
// 在热点识别、排行榜等高级功能中，我们需要直接使用底层的 Redis API
// 来完成批量管道、布隆过滤器等操作。为了避免在项目的其他位置重复
// 初始化客户端，这里提供一个安全的只读访问接口。调用方在使用时需
// 要确保在 dao/redis.Init 成功之后再调用该方法。
func GetClient() *redis.Client {
	return client
}
