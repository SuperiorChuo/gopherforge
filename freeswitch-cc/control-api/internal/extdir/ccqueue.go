package extdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SuperiorChuo/go-freeswitch-cc/control-api/internal/store"
)

// mod_callcenter 队列物化：
//   - 队列/坐席/tier 三段合成一个 callcenter.conf.xml 的完整配置文件
//     （落 cc_queues 卷，被 autoload_configs/callcenter.conf.xml 顶层 include），
//     reload mod_callcenter 后生效——运行时增删坐席另走 callcenter_config API
//     即时生效，本文件保证重启后状态一致。
//   - 每个队列的接入号拨号计划落 dialplan 卷。
//
// agent 命名约定：<ext>@default，contact 直呼 user/<ext>。

const ccQueuesFile = "queues.xml"

func renderCCQueues(queues []store.CCQueue, agents []store.CCQueueAgent) string {
	var b strings.Builder
	b.WriteString("<include>\n  <queues>\n")
	for _, q := range queues {
		moh := q.MOHSound
		if moh == "" {
			moh = "local_stream://moh"
		}
		strategy := q.Strategy
		if strategy == "" {
			strategy = "longest-idle-agent"
		}
		fmt.Fprintf(&b, `    <queue name="%s@default">
      <param name="strategy" value="%s"/>
      <param name="moh-sound" value="%s"/>
      <param name="time-base-score" value="system"/>
      <param name="max-wait-time" value="%d"/>
      <param name="max-wait-time-with-no-agent" value="%d"/>
      <param name="max-wait-time-with-no-agent-time-reached" value="5"/>
      <param name="tier-rules-apply" value="%t"/>
      <param name="tier-rule-wait-second" value="%d"/>
      <param name="tier-rule-no-agent-no-wait" value="true"/>
      <param name="discard-abandoned-after" value="60"/>
      <param name="abandoned-resume-allowed" value="%t"/>
`, q.Name, strategy, xmlEscaper.Replace(moh), q.MaxWaitTime, q.MaxWaitTime, q.TierRulesApply, max(q.TierRuleWaitSec, 0), q.AbandonedResumeAllowed)
		// 周期播报音：仅在配置了音源与频率时输出
		if q.AnnounceSound != "" && q.AnnounceFrequency > 0 {
			fmt.Fprintf(&b, "      <param name=\"announce-sound\" value=\"%s\"/>\n", xmlEscaper.Replace(q.AnnounceSound))
			fmt.Fprintf(&b, "      <param name=\"announce-frequency\" value=\"%d\"/>\n", q.AnnounceFrequency)
		}
		// 队列级自动录音：进队即录到 recordings 卷
		if q.RecordTemplate {
			b.WriteString("      <param name=\"record-template\" value=\"$${recordings_dir}/ccq-${strftime(%Y%m%d-%H%M%S)}-${uuid}.wav\"/>\n")
		}
		if q.RingProgressivelyDelay > 0 {
			fmt.Fprintf(&b, "      <param name=\"ring-progressively-delay\" value=\"%d\"/>\n", q.RingProgressivelyDelay)
		}
		if q.SkipAgentsWithExternalCalls {
			b.WriteString("      <param name=\"skip-agents-with-external-calls\" value=\"true\"/>\n")
		}
		if q.AgentNoAnswerStatus != "" {
			fmt.Fprintf(&b, "      <param name=\"agent-no-answer-status\" value=\"%s\"/>\n", xmlEscaper.Replace(q.AgentNoAnswerStatus))
		}
		b.WriteString("    </queue>\n")
	}
	b.WriteString("  </queues>\n  <agents>\n")
	// 同一分机可挂多个队列，agent 只声明一次
	seen := map[string]store.CCQueueAgent{}
	for _, a := range agents {
		if prev, ok := seen[a.Ext]; !ok || a.WrapUpTime > prev.WrapUpTime {
			seen[a.Ext] = a
		}
	}
	for ext, a := range seen {
		wrapUp := a.WrapUpTime
		if wrapUp <= 0 {
			wrapUp = 10
		}
		maxNoAnswer := a.MaxNoAnswer
		if maxNoAnswer <= 0 {
			maxNoAnswer = 3
		}
		// 三个 delay 非零时用坐席自定义值，否则保持队列默认
		rejectDelay := a.RejectDelayTime
		if rejectDelay <= 0 {
			rejectDelay = 10
		}
		busyDelay := a.BusyDelayTime
		if busyDelay <= 0 {
			busyDelay = 60
		}
		noAnswerDelay := a.NoAnswerDelayTime
		if noAnswerDelay <= 0 {
			noAnswerDelay = 10
		}
		fmt.Fprintf(&b, `    <agent name="%s@default" type="callback" contact="[call_timeout=15]user/%s" status="Logged Out" max-no-answer="%d" wrap-up-time="%d" reject-delay-time="%d" busy-delay-time="%d" no-answer-delay-time="%d"/>
`, ext, ext, maxNoAnswer, wrapUp, rejectDelay, busyDelay, noAnswerDelay)
	}
	b.WriteString("  </agents>\n  <tiers>\n")
	for _, a := range agents {
		level := max(a.Level, 1)
		pos := max(a.Position, 1)
		fmt.Fprintf(&b, `    <tier agent="%s@default" queue="%s@default" level="%d" position="%d"/>
`, a.Ext, a.QueueName, level, pos)
	}
	b.WriteString("  </tiers>\n</include>\n")
	return b.String()
}

func renderQueueDialplan(q store.CCQueue) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!-- 队列 %s：拨 %s 进队（mod_callcenter），接通即录音 -->
<include>
  <extension name="ccq_%s">
    <condition field="destination_number" expression="^%s$">
      <action application="answer"/>
      <action application="set" data="RECORD_STEREO=true"/>
      <action application="export" data="execute_on_answer=record_session $${recordings_dir}/${uuid}.wav"/>
      <action application="set" data="cc_queue_joined=%s"/>
      <action application="callcenter" data="%s@default"/>
`, q.Name, q.AccessNumber, q.Name, q.AccessNumber, q.Name, q.Name)
	// max-wait-time 到点 callcenter 应用返回，续接超时目的地
	if q.TimeoutNumber != "" {
		fmt.Fprintf(&b, "      <action application=\"transfer\" data=\"%s XML default\"/>\n", q.TimeoutNumber)
	} else {
		b.WriteString("      <action application=\"playback\" data=\"ivr/ivr-no_one_is_available_to_take_your_call.wav\"/>\n      <action application=\"hangup\"/>\n")
	}
	b.WriteString("    </condition>\n  </extension>\n</include>\n")
	return b.String()
}

// SyncCCQueues rewrites the consolidated queues file plus per-queue dialplans.
func SyncCCQueues(ccDir, dialplanDir string, queues []store.CCQueue, agents []store.CCQueueAgent) error {
	if err := os.MkdirAll(ccDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(ccDir, ccQueuesFile), []byte(renderCCQueues(queues, agents)), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(dialplanDir, 0o755); err != nil {
		return err
	}
	// 先清旧队列拨号计划再重写（队列删除场景）
	old, _ := filepath.Glob(filepath.Join(dialplanDir, "30_ccq-*.xml"))
	for _, f := range old {
		_ = os.Remove(f)
	}
	for _, q := range queues {
		f := filepath.Join(dialplanDir, "30_ccq-"+q.Name+".xml")
		if err := os.WriteFile(f, []byte(renderQueueDialplan(q)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RenderBlacklist materializes the mod_blacklist number file (one per line).
func RenderBlacklist(dir string, numbers []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "cc_blacklist.txt"), []byte(strings.Join(numbers, "\n")+"\n"), 0o644)
}
