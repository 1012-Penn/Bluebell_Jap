package redis

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
)

// 推荐阅读
// 基于用户投票的相关算法：http://www.ruanyifeng.com/blog/algorithm/

// 本项目使用简化版的投票分数
// 投一票就加432分   86400/200  --> 200张赞成票可以给你的帖子续一天

/* 投票的几种情况：
   direction=1时，有两种情况：
   	1. 之前没有投过票，现在投赞成票    --> 更新分数和投票记录  差值的绝对值：1  +432
   	2. 之前投反对票，现在改投赞成票    --> 更新分数和投票记录  差值的绝对值：2  +432*2
   direction=0时，有两种情况：
   	1. 之前投过反对票，现在要取消投票  --> 更新分数和投票记录  差值的绝对值：1  +432
	2. 之前投过赞成票，现在要取消投票  --> 更新分数和投票记录  差值的绝对值：1  -432
   direction=-1时，有两种情况：
   	1. 之前没有投过票，现在投反对票    --> 更新分数和投票记录  差值的绝对值：1  -432
   	2. 之前投赞成票，现在改投反对票    --> 更新分数和投票记录  差值的绝对值：2  -432*2

   投票的限制：
   每个贴子自发表之日起一个星期之内允许用户投票，超过一个星期就不允许再投票了。
   	1. 到期之后将redis中保存的赞成票数及反对票数存储到mysql表中
   	2. 到期之后删除那个 KeyPostVotedZSetPF
*/

// 实际生产环境下 context.Background() 按需替换

const (
	oneWeekInSeconds = 7 * 24 * 3600
	scorePerVote     = 432 // 每一票值多少分
)

var (
	ErrVoteTimeExpire = errors.New("投票时间已过")
	ErrVoteRepeated   = errors.New("不允许重复投票")
)

// CreatePost 创建帖子时同步更新Redis缓存
// 参数说明：
// - postID: 帖子ID，用于标识具体的帖子
// - communityID: 社区ID，表示帖子属于哪个社区
// 返回值：error，如果操作失败返回错误信息
func CreatePost(postID, communityID int64) error {
	// 🔍 client.TxPipeline() 是什么？
	// TxPipeline 创建一个事务管道，确保多个Redis操作的原子性
	// 要么全部成功，要么全部失败，避免数据不一致
	pipeline := client.TxPipeline()

	// 🔍 第一个操作：将帖子添加到时间排序的有序集合
	// 作用：支持按发帖时间排序查询帖子列表
	// 数据结构：ZSet（有序集合），分数是时间戳，成员是帖子ID
	pipeline.ZAdd(context.Background(), getRedisKey(KeyPostTimeZSet), &redis.Z{
		Score:  float64(time.Now().Unix()), // 🔍 分数：当前时间戳（秒）
		Member: postID,                     // 🔍 成员：帖子ID
	})

	// 🔍 第二个操作：将帖子添加到分数排序的有序集合
	// 作用：支持按投票分数排序查询帖子列表
	// 初始分数：发帖时间戳，后续会根据投票情况动态调整
	pipeline.ZAdd(context.Background(), getRedisKey(KeyPostScoreZSet), &redis.Z{
		Score:  float64(time.Now().Unix()), // 🔍 分数：当前时间戳（秒）
		Member: postID,                     // 🔍 成员：帖子ID
	})

	// 🔍 第三个操作：将帖子ID添加到对应社区的集合中
	// 作用：支持按社区查询帖子列表
	// 数据结构：Set（集合），存储该社区下所有帖子的ID

	// 🔍 getRedisKey(KeyCommunitySetPF + strconv.Itoa(int(communityID))) 详解：
	// - KeyCommunitySetPF: 社区集合的前缀，如 "bluebell:community:"
	// - strconv.Itoa(int(communityID)): 将社区ID转换为字符串
	// - 最终key: "bluebell:community:1" (假设communityID=1)
	cKey := getRedisKey(KeyCommunitySetPF + strconv.Itoa(int(communityID)))

	// 🔍 pipeline.SAdd() 是什么？
	// SAdd 向Set集合中添加元素，这里添加帖子ID到社区集合
	// 参数说明：
	// - context.Background(): 上下文，用于控制操作的生命周期
	// - cKey: 社区的Redis key
	// - postID: 要添加的帖子ID
	pipeline.SAdd(context.Background(), cKey, postID)

	// 🔍 pipeline.Exec() 是什么？
	// Exec 执行事务管道中的所有命令
	// 返回值：
	// - 第一个返回值：命令执行结果（这里用_忽略）
	// - 第二个返回值：错误信息
	_, err := pipeline.Exec(context.Background())
	//context.Background()是上下文,用于控制操作的生命周期 这里是gin框架的上下文

	// 返回执行结果
	return err
}

func VoteForPost(userID, postID string, value float64) (float64, error) {
	// 1. 判断投票限制
	// 去redis取帖子发布时间
	postTime := client.ZScore(context.Background(), getRedisKey(KeyPostTimeZSet), postID).Val()
	// 这一行是取帖子发布时间,原理是zscore,zscore是取有序集合中指定成员的分数
	// client是redis的客户端,提供了Redis操作接口
	if float64(time.Now().Unix())-postTime > oneWeekInSeconds {
		return 0, ErrVoteTimeExpire
	}
	// 2和3需要放到一个pipeline事务中操作

	// 2. 更新贴子的分数
	// 先查当前用户给当前帖子的投票记录
	// 🔍 语法分析：client.ZScore().Val() 链式调用
	// 1. client.ZScore() - 调用Redis客户端的ZScore方法
	// 2. context.Background() - 上下文参数，用于控制操作生命周期
	// 3. getRedisKey(KeyPostVotedZSetPF+postID) - 构建Redis key
	// 4. userID - 要查询的成员（用户ID）
	// 5. .Val() - 获取操作结果的值部分，忽略错误信息
	ov := client.ZScore(context.Background(), getRedisKey(KeyPostVotedZSetPF+postID), userID).Val()

	// 🔍 ZSet数据结构详细说明：
	// ov = old value，即用户对该帖子的旧投票值
	//
	// 📊 Redis ZSet（有序集合）结构：
	// Key: "bluebell:post:voted:123" (123是postID)
	// Type: ZSet (Sorted Set)
	// 每个帖子的投票情况就是一个ZSet,其中每个成员就是一个用户,每个成员的分数就是该用户对该帖子的投票值
	//
	// 🗂️ 数据存储格式：
	// Member (成员)     | Score (分数)  | 含义
	// -----------------|---------------|------------------
	// userID: 456      | 1.0          | 用户456投赞成票
	// userID: 789      | -1.0         | 用户789投反对票
	// userID: 101      | 1.0          | 用户101投赞成票
	// userID: 202      | 0.0          | 用户202取消投票
	//
	// 🎯 投票值含义：
	// 1.0  = 赞成票 (支持)
	// -1.0 = 反对票 (不支持)
	// 0.0  = 未投票或取消投票
	//
	// 🔍 查询逻辑：
	// 通过userID作为member查找对应的score值
	// 如果用户未投票，ZScore返回0
	// 返回值：float64类型的投票值

	// 更新：如果这一次投票的值和之前保存的值一致，就提示不允许重复投票
	if value == ov {
		return ov, ErrVoteRepeated
	}
	var op float64
	if value > ov {
		op = 1
	} else {
		op = -1
	} //op是opertion,即操作,1是赞成,0是取消, -1是反对

	diff := math.Abs(ov - value) // 计算两次投票的差值
	pipeline := client.TxPipeline()

	// 🔍 更新帖子分数ZSet说明：
	// 操作：ZINCRBY key increment member
	// Key: "bluebell:post:score" (帖子分数排序ZSet)
	// Member: postID (帖子ID)
	// Increment: op*diff*scorePerVote (分数变化量)
	//
	// 📊 分数计算逻辑：
	// op = 1  (新投票 > 旧投票，分数增加)
	// op = -1 (新投票 < 旧投票，分数减少)
	// diff = |旧投票 - 新投票| (变化幅度)
	// scorePerVote = 432 (每票的分数权重)
	//
	// 🎯 实际效果：
	// 用户投赞成票：帖子分数 +432
	// 用户投反对票：帖子分数 -432
	// 用户取消投票：根据之前投票情况调整分数
	pipeline.ZIncrBy(context.Background(), getRedisKey(KeyPostScoreZSet), op*diff*scorePerVote, postID)

	// 3. 记录用户为该贴子投票的数据
	// 🔍 ZSet更新操作说明：
	if value == 0 {
		// 📝 情况1：用户取消投票 (value = 0)
		// 操作：从ZSet中删除该用户的投票记录
		// 命令：ZREM key member
		// 作用：清理用户投票数据，释放存储空间
		pipeline.ZRem(context.Background(), getRedisKey(KeyPostVotedZSetPF+postID), userID)
	} else {
		// 📝 情况2：用户投票或修改投票 (value = 1 或 -1)
		// 操作：向ZSet中添加或更新用户的投票记录
		// 命令：ZADD key score member
		// 作用：记录用户投票状态，支持后续查询和统计
		pipeline.ZAdd(context.Background(), getRedisKey(KeyPostVotedZSetPF+postID), &redis.Z{
			Score:  value,  // 投票值：1=赞成, -1=反对
			Member: userID, // 用户ID作为成员标识
		})
	}
	// 统一控制点赞关系的生命周期，降低冷数据占用
	pipeline.Expire(context.Background(), getRedisKey(KeyPostVotedZSetPF+postID), 15*24*time.Hour)
	_, err := pipeline.Exec(context.Background())
	return ov, err
}
