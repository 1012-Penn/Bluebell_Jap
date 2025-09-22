package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 统一响应格式设计
//
// 所有API接口都使用统一的响应格式，便于前端处理
// 响应格式：
// {
//     "code": 1000,     // 业务状态码，1000表示成功
//     "msg": "success", // 提示信息
//     "data": {}        // 响应数据，可选
// }
//
// 技术亮点：
// - 统一的响应格式，提高开发效率
// - 业务状态码与HTTP状态码分离
// - 支持国际化错误信息
// - 便于前端统一处理

// ResponseData 统一响应数据结构
//
// 所有API接口都使用此结构返回数据
// 字段说明：
// - Code: 业务状态码，定义在code.go中
// - Msg: 提示信息，支持字符串和对象
// - Data: 响应数据，使用omitempty标签，空值时省略
type ResponseData struct {
	Code ResCode     `json:"code"`           // 业务状态码
	Msg  interface{} `json:"msg"`            // 提示信息，支持多种类型
	Data interface{} `json:"data,omitempty"` // 响应数据，空值时省略
}

// ResponseError 返回错误响应
//
// 功能说明：
// 1. 使用统一的错误码返回错误信息
// 2. 所有错误都返回HTTP 200状态码
// 3. 通过业务状态码区分不同的错误类型
//
// 参数说明：
// - c: Gin上下文
// - code: 业务错误码
//
// 技术亮点：
// - 统一的错误处理格式
// - 业务状态码与HTTP状态码分离
// - 便于前端统一处理错误
func ResponseError(c *gin.Context, code ResCode) {
	c.JSON(http.StatusOK, &ResponseData{
		Code: code,
		Msg:  code.Msg(), // 使用错误码对应的默认消息
		Data: nil,
	})
}

// ResponseErrorWithMsg 返回带自定义消息的错误响应
//
// 功能说明：
// 1. 支持自定义错误消息
// 2. 用于参数验证错误等需要详细信息的场景
// 3. 保持统一的响应格式
//
// 参数说明：
// - c: Gin上下文
// - code: 业务错误码
// - msg: 自定义错误消息
//
// 技术亮点：
// - 支持自定义错误信息
// - 保持响应格式一致性
// - 便于参数验证错误处理
func ResponseErrorWithMsg(c *gin.Context, code ResCode, msg interface{}) {
	c.JSON(http.StatusOK, &ResponseData{
		Code: code,
		Msg:  msg, // 使用自定义消息
		Data: nil,
	})
}

// ResponseSuccess 返回成功响应
//
// 功能说明：
// 1. 返回成功状态和响应数据
// 2. 使用统一的成功状态码
// 3. 支持任意类型的响应数据
//
// 参数说明：
// - c: Gin上下文
// - data: 响应数据，可以是任意类型
//
// 技术亮点：
// - 统一的成功响应格式
// - 支持任意类型数据
// - 便于前端统一处理
func ResponseSuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, &ResponseData{
		Code: CodeSuccess,       // 使用成功状态码
		Msg:  CodeSuccess.Msg(), // 使用成功消息
		Data: data,              // 返回响应数据
	})
}
