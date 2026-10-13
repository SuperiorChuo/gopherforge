package channel

import (
	"context"
	"strings"
)

// Dispatcher fans out rendered payloads to email/sms after inbox is written.
type Dispatcher struct {
	Email EmailSender
	SMS   SMSSender
	Voice VoiceSender
}

// Status reports which external channels are live.
func (d *Dispatcher) Status() map[string]any {
	emailOn, emailName := false, ""
	if d != nil && d.Email != nil && d.Email.Enabled() {
		emailOn, emailName = true, d.Email.Name()
	}
	smsOn, smsName := false, ""
	if d != nil && d.SMS != nil && d.SMS.Enabled() {
		smsOn, smsName = true, d.SMS.Name()
	}
	voiceOn, voiceName := false, ""
	if d != nil && d.Voice != nil && d.Voice.Enabled() {
		voiceOn, voiceName = true, d.Voice.Name()
	}
	return map[string]any{
		"inbox": true,
		"email": map[string]any{"enabled": emailOn, "provider": emailName},
		"sms":   map[string]any{"enabled": smsOn, "provider": smsName},
		"voice": map[string]any{"enabled": voiceOn, "provider": voiceName},
	}
}

// DeliverExternal sends email/sms for channels list; inbox is handled by store.
func (d *Dispatcher) DeliverExternal(ctx context.Context, channels []string, contact Contact, p Payload) []Result {
	var results []Result
	if d == nil {
		return results
	}
	if Has(channels, "email") {
		results = append(results, d.sendEmail(ctx, contact, p))
	}
	if Has(channels, "sms") {
		results = append(results, d.sendSMS(ctx, contact, p))
	}
	if Has(channels, "voice") {
		results = append(results, d.sendVoice(ctx, contact, p))
	}
	return results
}

func (d *Dispatcher) sendEmail(ctx context.Context, contact Contact, p Payload) Result {
	r := Result{Channel: "email"}
	if d.Email == nil || !d.Email.Enabled() {
		r.Status = "skipped"
		r.Detail = "email provider not configured"
		return r
	}
	to := strings.TrimSpace(contact.Email)
	if to == "" {
		r.Status = "skipped"
		r.Detail = "user has no email"
		return r
	}
	r.Target = to
	if err := d.Email.Send(ctx, to, p); err != nil {
		r.Status = "failed"
		r.Detail = err.Error()
		return r
	}
	r.Status = "sent"
	r.Detail = d.Email.Name()
	return r
}

func (d *Dispatcher) sendSMS(ctx context.Context, contact Contact, p Payload) Result {
	r := Result{Channel: "sms"}
	if d.SMS == nil || !d.SMS.Enabled() {
		r.Status = "skipped"
		r.Detail = "sms provider not configured"
		return r
	}
	phone := strings.TrimSpace(contact.Phone)
	if phone == "" {
		r.Status = "skipped"
		r.Detail = "user has no phone"
		return r
	}
	r.Target = phone
	if err := d.SMS.Send(ctx, phone, p); err != nil {
		r.Status = "failed"
		r.Detail = err.Error()
		return r
	}
	// stub 通道只写了一行日志，短信并没有发出去——状态与说明都必须写死这一点，
	// 否则告警链路上会有人对着一条"成功"记录以为通知已送达。
	if d.SMS.Name() == "stub" {
		r.Status = "stub"
		r.Detail = "sms stub: 仅记录日志，短信未真实投递（未接入短信服务商）"
		return r
	}
	r.Status = "sent"
	r.Detail = d.SMS.Name()
	return r
}

func (d *Dispatcher) sendVoice(ctx context.Context, contact Contact, p Payload) Result {
	r := Result{Channel: "voice"}
	if d.Voice == nil || !d.Voice.Enabled() {
		r.Status = "skipped"
		r.Detail = "voice provider not configured"
		return r
	}
	phone := strings.TrimSpace(contact.Phone)
	if phone == "" {
		r.Status = "skipped"
		r.Detail = "user has no phone"
		return r
	}
	r.Target = phone
	if err := d.Voice.Send(ctx, phone, p); err != nil {
		r.Status = "failed"
		r.Detail = err.Error()
		return r
	}
	r.Status = "queued"
	r.Detail = d.Voice.Name()
	return r
}
