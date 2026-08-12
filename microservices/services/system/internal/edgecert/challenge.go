package edgecert

import "sync"

// HTTP-01 挑战内存表：token → keyAuth（Let's Encrypt 回调本机时校验）。
var challengeStore sync.Map

func putChallenge(token, keyAuth string) {
	challengeStore.Store(token, keyAuth)
}

func deleteChallenge(token string) {
	challengeStore.Delete(token)
}

// LookupChallenge 供公开路由 /.well-known/acme-challenge/:token 使用。
func LookupChallenge(token string) (keyAuth string, ok bool) {
	v, ok := challengeStore.Load(token)
	if !ok {
		return "", false
	}
	s, _ := v.(string)
	return s, s != ""
}
