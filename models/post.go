// Package models 数据模型包
//
// 定义系统中所有的数据结构，包括：
// 1. 数据库实体模型
// 2. 请求参数模型
// 3. 响应数据模型
// 4. 业务逻辑模型
//
// 技术亮点：
// - 使用结构体标签进行数据绑定
// - 支持JSON序列化和反序列化
// - 支持数据库字段映射
// - 支持参数验证标签
package models

import "time"

// Post 帖子数据模型
//
// 功能说明：
// 1. 定义帖子的基本信息结构
// 2. 支持数据库操作和JSON序列化
// 3. 包含帖子内容、作者、社区等关联信息
//
// 字段说明：
// - ID: 帖子唯一标识，使用雪花算法生成
// - AuthorID: 作者ID，关联用户表
// - CommunityID: 社区ID，关联社区表
// - Status: 帖子状态（如：正常、删除、审核中等）
// - Title: 帖子标题
// - Content: 帖子内容
// - CreateTime: 帖子创建时间
//
// 技术亮点：
// - 使用db标签映射数据库字段
// - 使用json标签定义JSON序列化字段名
// - 使用binding标签进行参数验证
// - 内存对齐优化（按字段大小排序）
//
// 内存对齐概念：
// 按照字段大小从大到小排列，减少内存碎片
// 例如：int64(8字节) > int32(4字节) > string(16字节指针) > time.Time(24字节)
type Post struct {
	ID          int64     `json:"id,string" db:"post_id"`                            // 帖子ID，JSON序列化为字符串避免精度丢失
	AuthorID    int64     `json:"author_id" db:"author_id"`                          // 作者ID，关联用户表
	CommunityID int64     `json:"community_id" db:"community_id" binding:"required"` // 社区ID，关联社区表，必填
	Status      int32     `json:"status" db:"status"`                                // 帖子状态（如：1-正常，0-删除）
	Title       string    `json:"title" db:"title" binding:"required"`               // 帖子标题，必填
	Content     string    `json:"content" db:"content" binding:"required"`           // 帖子内容，必填
	CreateTime  time.Time `json:"create_time" db:"create_time"`                      // 帖子创建时间
}

// ApiPostDetail 帖子详情API响应结构体
//
// 功能说明：
// 1. 用于API响应的帖子详情数据结构
// 2. 包含帖子的完整信息（基础信息+关联信息）
// 3. 支持前端展示所需的所有数据
//
// 字段说明：
// - AuthorName: 作者用户名，便于前端显示
// - VoteNum: 投票数量，实时统计
// - Post: 嵌入帖子基础结构体，包含所有帖子字段
// - CommunityDetail: 嵌入社区信息，包含社区详情
//
// 技术亮点：
// - 使用结构体嵌入，继承Post的所有字段
// - 包含关联数据，减少前端请求次数
// - 支持JSON序列化，便于API响应
type ApiPostDetail struct {
	AuthorName       string             `json:"author_name"` // 作者用户名，便于前端显示
	VoteNum          int64              `json:"vote_num"`    // 投票数量，实时统计
	*Post                               // 嵌入帖子结构体，继承所有帖子字段
	*CommunityDetail `json:"community"` // 嵌入社区信息，包含社区详情
}
