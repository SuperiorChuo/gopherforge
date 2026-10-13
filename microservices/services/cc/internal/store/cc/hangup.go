package store

import "strings"

// ClassifyHangup 把 FreeSWITCH Hangup-Cause 归成运营可读类别。
// 不是早媒体 ASR：只消费信令释放原因。空号/忙/无法接通等靠 SIP 原因，不靠听提示音。
func ClassifyHangup(cause string) string {
	c := strings.ToUpper(strings.TrimSpace(cause))
	if c == "" {
		return ""
	}
	switch c {
	case "NORMAL_CLEARING":
		return "ok"
	case "UNALLOCATED_NUMBER", "NO_ROUTE_DESTINATION", "INVALID_NUMBER_FORMAT",
		"NUMBER_CHANGED", "UNASSIGNED_NUMBER":
		return "empty"
	case "USER_BUSY":
		return "busy"
	case "NO_ANSWER", "NO_USER_RESPONSE", "ORIGINATOR_CANCEL", "LOSE_RACE":
		return "noanswer"
	case "SUBSCRIBER_ABSENT", "USER_NOT_REGISTERED", "NORMAL_TEMPORARY_FAILURE",
		"RECOVERY_ON_TIMER_EXPIRE", "ALLOTTED_TIMEOUT", "NORMAL_UNSPECIFIED",
		"NO_USER_SPECIFIED":
		return "unavailable"
	case "CALL_REJECTED", "DESTINATION_OUT_OF_ORDER", "BEARERCAPABILITY_NOTAUTH":
		return "rejected"
	case "MANAGER_REQUEST":
		return "local"
	default:
		return "other"
	}
}

// HangupClassLabel 中文标签（给测试与后端日志；前端用 i18n）。
func HangupClassLabel(class string) string {
	switch class {
	case "ok":
		return "正常挂断"
	case "empty":
		return "空号"
	case "busy":
		return "忙线"
	case "noanswer":
		return "无人接听"
	case "unavailable":
		return "无法接通"
	case "rejected":
		return "拒接"
	case "local":
		return "本端挂断"
	case "other":
		return "其他"
	default:
		return ""
	}
}
