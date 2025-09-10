package redis

// redis key

// redis key注意使用命名空间的方式,方便查询和拆分

const (
	Prefix             = "bluebell:"   // 项目key前缀
	KeyPostTimeZSet    = "post:time"   // zset;贴子及发帖时间
	KeyPostScoreZSet   = "post:score"  // zset;贴子及投票的分数
	KeyPostVotedZSetPF = "post:voted:" // zset;记录用户及投票类型;参数是post id, PF是post id的前缀

	KeyCommunitySetPF = "community:" // set;保存每个分区下帖子的id
)

// 给redis key加上前缀
func getRedisKey(key string) string {
	return Prefix + key
}

//这一行是给redis key加上前缀
//比如KeyPostVotedZSetPF = "post:voted:" 加上前缀后变成 bluebell:post:voted:
//这样做的目的是避免key冲突
//比如两个项目都用redis,一个项目用post:voted:,一个项目用post:voted:2,这样就会冲突
// 命名空间隔离
