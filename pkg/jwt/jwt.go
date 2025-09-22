// Package jwt JWT令牌处理包
//
// 实现JWT（JSON Web Token）的生成和解析功能
// 技术原理：
// 1. 使用HMAC-SHA256算法签名
// 2. 包含用户ID、用户名等自定义信息
// 3. 支持令牌过期时间配置
// 4. 无状态认证，支持分布式部署
//
// 技术亮点：
// - 无状态认证，支持分布式部署
// - 包含用户信息，减少数据库查询
// - 可配置过期时间，提高安全性
// - 使用HMAC-SHA256签名，安全性高
package jwt

import (
	"errors"
	"time"

	"github.com/spf13/viper"

	"github.com/dgrijalva/jwt-go"
)

// JWT签名密钥
// 技术亮点：使用固定密钥，确保令牌验证的一致性
// 注意：实际项目中应该使用环境变量或配置文件管理密钥
var mySecret = []byte("夏天夏天悄悄过去")

// MyClaims 自定义JWT声明结构体
//
// 功能说明：
// 1. 继承jwt.StandardClaims标准声明
// 2. 添加自定义用户信息字段
// 3. 支持扩展更多业务字段
//
// 字段说明：
// - UserID: 用户ID，用于身份识别
// - Username: 用户名，用于显示
// - StandardClaims: 标准声明，包含过期时间、签发人等
//
// 技术亮点：
// - 结构体嵌入，复用标准声明
// - 自定义字段，满足业务需求
// - JSON标签，支持序列化
type MyClaims struct {
	UserID             int64  `json:"user_id"`  // 用户ID
	Username           string `json:"username"` // 用户名
	jwt.StandardClaims        // 标准声明
}

// GenToken 生成JWT令牌
//
// 功能说明：
// 1. 创建包含用户信息的声明
// 2. 设置令牌过期时间
// 3. 使用HMAC-SHA256算法签名
// 4. 返回完整的JWT字符串
//
// 参数说明：
// - userID: 用户ID
// - username: 用户名
//
// 返回值：
// - string: JWT令牌字符串
// - error: 生成过程中的错误
//
// 技术亮点：
// - 可配置的过期时间
// - HMAC-SHA256签名算法
// - 包含用户信息，减少查询
func GenToken(userID int64, username string) (string, error) {
	// 1. 创建自定义声明
	// 技术亮点：包含用户信息，减少后续查询
	c := MyClaims{
		UserID:   userID,
		Username: username, // 注意：这里应该使用传入的username参数
		jwt.StandardClaims{
			// 设置过期时间，从配置文件读取
			// 技术亮点：可配置的过期时间，提高灵活性
			ExpiresAt: time.Now().Add(
				time.Duration(viper.GetInt("auth.jwt_expire")) * time.Hour).Unix(),
			Issuer: "bluebell", // 签发人标识
		},
	}

	// 2. 创建JWT令牌对象
	// 技术亮点：使用HMAC-SHA256签名算法，安全性高
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)

	// 3. 使用密钥签名并返回完整令牌
	// 技术亮点：签名确保令牌的完整性和真实性
	return token.SignedString(mySecret)
}

// ParseToken 解析JWT令牌
//
// 功能说明：
// 1. 解析JWT令牌字符串
// 2. 验证令牌签名
// 3. 检查令牌有效性
// 4. 返回解析后的声明信息
//
// 参数说明：
// - tokenString: JWT令牌字符串
//
// 返回值：
// - *MyClaims: 解析后的声明信息
// - error: 解析过程中的错误
//
// 技术亮点：
// - 签名验证，确保令牌真实性
// - 过期时间检查，提高安全性
// - 返回用户信息，减少数据库查询
func ParseToken(tokenString string) (*MyClaims, error) {
	// 1. 创建声明对象用于解析
	var mc = new(MyClaims)

	// 2. 解析令牌并验证签名
	// 技术亮点：使用回调函数提供签名密钥
	token, err := jwt.ParseWithClaims(tokenString, mc, func(token *jwt.Token) (i interface{}, err error) {
		return mySecret, nil
	})
	if err != nil {
		return nil, err
	}

	// 3. 验证令牌有效性
	// 技术亮点：检查签名、过期时间等
	if token.Valid {
		return mc, nil
	}

	// 4. 令牌无效
	return nil, errors.New("invalid token")
}
