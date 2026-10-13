// Package wxmsg parses 公众号 inbound message XML and builds passive replies.
package wxmsg

import (
	"encoding/xml"
	"fmt"
	"strings"
	"time"
)

// Inbound 微信推送的消息/事件（明文/兼容模式）。字段覆盖 M1 文本 + 常见事件。
type Inbound struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`   // 公众号原始 ID（gh_...）
	FromUserName string   `xml:"FromUserName"` // 粉丝 openid
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"` // text/image/event/...
	Content      string   `xml:"Content"` // 文本内容
	MsgId        int64    `xml:"MsgId"`   // 消息 id（事件无）
	Event        string   `xml:"Event"`   // subscribe/unsubscribe/CLICK/...
	EventKey     string   `xml:"EventKey"`
	// 发布结果事件（M5 freepublish 回调）
	PublishEventInfo PublishEventInfo `xml:"PublishEventInfo"`
}

// PublishEventInfo matches WeChat's PUBLISHJOBFINISH payload. These element
// names are lowercase in the official callback XML.
type PublishEventInfo struct {
	PublishID     string `xml:"publish_id"`
	PublishStatus string `xml:"publish_status"`
}

// MsgIDString 返回字符串形式的 MsgId（事件无 MsgId 时返回空）。
func (in *Inbound) MsgIDString() string {
	if in.MsgId == 0 {
		return ""
	}
	return fmt.Sprintf("%d", in.MsgId)
}

// Parse 解析明文/兼容模式的消息 XML。
func Parse(raw []byte) (*Inbound, error) {
	var in Inbound
	if err := xml.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

// Encrypted 是安全模式下的外层信封（body 只含 Encrypt 密文）。
type Encrypted struct {
	XMLName    xml.Name `xml:"xml"`
	ToUserName string   `xml:"ToUserName"`
	Encrypt    string   `xml:"Encrypt"`
}

// ParseEncrypted 解析安全模式外层信封，取出 <Encrypt> 密文。
func ParseEncrypted(raw []byte) (*Encrypted, error) {
	var e Encrypted
	if err := xml.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// BuildEncryptedReply 组装安全模式的加密回复外层信封。encrypt=AES 密文的 base64，
// msgSignature=sha1(sort(token,timestamp,nonce,encrypt))。
func BuildEncryptedReply(encrypt, msgSignature, timestamp, nonce string) string {
	return fmt.Sprintf(
		"<xml><Encrypt><![CDATA[%s]]></Encrypt>"+
			"<MsgSignature><![CDATA[%s]]></MsgSignature>"+
			"<TimeStamp>%s</TimeStamp>"+
			"<Nonce><![CDATA[%s]]></Nonce></xml>",
		cdataSafe(encrypt), cdataSafe(msgSignature), cdataSafe(timestamp), cdataSafe(nonce),
	)
}

// BuildTextReply 组装被动回复的 text XML。to=粉丝 openid，from=公众号原始 ID。
// content 用 CDATA 包裹并转义，避免 XML 注入。
func BuildTextReply(to, from, content string) string {
	return fmt.Sprintf(
		"<xml><ToUserName><![CDATA[%s]]></ToUserName>"+
			"<FromUserName><![CDATA[%s]]></FromUserName>"+
			"<CreateTime>%d</CreateTime>"+
			"<MsgType><![CDATA[text]]></MsgType>"+
			"<Content><![CDATA[%s]]></Content></xml>",
		cdataSafe(to), cdataSafe(from), time.Now().Unix(), cdataSafe(content),
	)
}

// cdataSafe 防止内容里出现 "]]>" 破坏 CDATA 结构。
func cdataSafe(s string) string {
	return strings.ReplaceAll(s, "]]>", "]] >")
}
