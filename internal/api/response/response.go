// Package response 统一响应与业务错误封装.
//
// 错误上抛模式 (参考 CourseSelectionMS):
//   - Response 实现 error 接口, 业务错误全部定义为包级 var 集合
//   - dao/service 层方法以 (data, rsp) 元组返回, 错误原样上抛不包装
//   - 调用方以 errors.Is(rsp, response.OperationSuccess) 判定成败
//   - handler 层统一 GenFinalResponse(rsp, data) 输出 JSON
package response

// 成功响应
var OperationSuccess = Response{Status: 20000, Info: "operation success"}

// 请求数据错误
var RevDataError = Response{Status: 70000, Info: "can't validate data"}

// 业务错误集合
var (
	// 登录错误
	UserNotExiOrWrongPwd = Response{Status: 10002, Info: "user not exist or wrong password"} // 用户名错误或不存在 (统一错误防枚举)
	WrongPassword        = Response{Status: 10001, Info: "wrong old password"}               // 旧密码错误 (改密场景)

	// 会话错误
	EmptyToken     = Response{Status: 10016, Info: "empty token"}           // 空 Token
	InvalidToken   = Response{Status: 10016, Info: "invalid token"}         // 无效/过期 Token
	InvalidRefresh = Response{Status: 10017, Info: "invalid refresh token"} // RefreshToken 无效/已使用

	// 修改密码错误
	EmptyNewPassword = Response{Status: 10015, Info: "empty new password"} // 新密码为空

	// operator 模块错误 (资产/媒介/数据源)
	OperatorNotFound    = Response{Status: 10020, Info: "operator resource not found"}  // 资产/媒介/数据源不存在
	OperatorInUse       = Response{Status: 10021, Info: "operator resource is bound"}   // 媒介/数据源仍被资产绑定
	OperatorNotRunning  = Response{Status: 10022, Info: "asset instance not running"}   // 资产实例未运行 (未启用或构建失败)
	OperatorAmbiguous   = Response{Status: 10023, Info: "multiple capable resources"}   // 同类多绑定需精确指定
	OperatorUnavailable = Response{Status: 10024, Info: "operator service unavailable"} // operator 服务未装配
)

// Response 业务层错误封装: 实现 error 接口, 可在层间作为 error 值传递与 errors.Is 判定.
type Response struct {
	Status uint   `json:"status"`
	Info   string `json:"info"`
}

// Error 实现 error 接口
func (r Response) Error() string {
	return r.Info
}

// FinalResponse 请求层最终响应结构 (HTTP JSON 输出)
type FinalResponse struct {
	Status uint        `json:"status"`
	Info   string      `json:"info"`
	Data   interface{} `json:"data,omitempty"`
}

// GenFinalResponse 组装最终响应: 业务 rsp + 业务数据
func GenFinalResponse(rsp Response, data interface{}) FinalResponse {
	return FinalResponse{Status: rsp.Status, Info: rsp.Info, Data: data}
}

// ServerInternalError 服务器内部错误封装: 携带底层错误详情
func ServerInternalError(err error) Response {
	return Response{
		Status: 500,
		Info:   err.Error(),
	}
}
