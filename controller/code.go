package controller

// ResCode 业务状态码类型
//
// 使用自定义类型而不是int，提供类型安全性
// 避免与其他数字类型混淆
type ResCode int64

// 业务状态码常量定义
//
// 使用iota自动递增，从1000开始
// 1000-1999: 成功状态码
// 2000-2999: 客户端错误
// 3000-3999: 服务端错误
// 4000-4999: 认证授权错误
const (
	CodeSuccess         ResCode = 1000 + iota // 成功
	CodeInvalidParam                          // 请求参数错误
	CodeUserExist                             // 用户名已存在
	CodeUserNotExist                          // 用户名不存在
	CodeInvalidPassword                       // 用户名或密码错误
	CodeServerBusy                            // 服务繁忙

	CodeNeedLogin    // 需要登录
	CodeInvalidToken // 无效的token
)

// codeMsgMap 错误码与错误信息的映射表
//
// 技术亮点：
// - 集中管理所有错误信息
// - 支持国际化扩展
// - 便于维护和修改
// - 提供用户友好的错误提示
var codeMsgMap = map[ResCode]string{
	CodeSuccess:         "success",  // 成功
	CodeInvalidParam:    "请求参数错误",   // 参数验证失败
	CodeUserExist:       "用户名已存在",   // 用户注册时用户名重复
	CodeUserNotExist:    "用户名不存在",   // 用户登录时用户名不存在
	CodeInvalidPassword: "用户名或密码错误", // 密码验证失败
	CodeServerBusy:      "服务繁忙",     // 服务器内部错误

	CodeNeedLogin:    "需要登录",     // 需要用户登录
	CodeInvalidToken: "无效的token", // JWT令牌无效
}

// Msg 获取错误码对应的错误信息
//
// 功能说明：
// 1. 根据错误码返回对应的错误信息
// 2. 如果错误码不存在，返回默认的服务繁忙错误
// 3. 支持错误信息的国际化
//
// 返回值：
// - string: 错误信息
//
// 技术亮点：
// - 提供默认错误处理
// - 支持错误信息扩展
// - 类型安全的方法调用
func (c ResCode) Msg() string {
	msg, ok := codeMsgMap[c]
	if !ok {
		// 如果错误码不存在，返回默认错误信息
		// 技术亮点：提供兜底机制，避免返回空字符串
		msg = codeMsgMap[CodeServerBusy]
	}
	return msg
}
