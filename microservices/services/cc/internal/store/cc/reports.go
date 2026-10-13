package store

import (
	"context"
	"sort"
	"time"

	"github.com/go-admin-kit/services/shared/pkg/fullread"
)

// 呼叫报表：数据源就是 cc_calls。为避免 Postgres(生产) 与 sqlite(测试) 的时间
// 函数/EPOCH 差异，这里只做"按时间范围拉行 + Go 内聚合"——默认一天的话单量级
// 完全够用，且报表逻辑可在 sqlite 下直接单测。

// ReportOverview 是 /reports/overview 的整包返回。
type ReportOverview struct {
	From             string           `json:"from"`
	To               string           `json:"to"`
	TotalInbound     int              `json:"total_inbound"`
	TotalOutbound    int              `json:"total_outbound"`
	Total            int              `json:"total"`
	Answered         int              `json:"answered"`
	AnswerRate       float64          `json:"answer_rate"`  // 接通率 0-100（保留 1 位）
	Abandoned        int              `json:"abandoned"`    // 呼入未接通数
	AvgTalkSec       int              `json:"avg_talk_sec"` // 接通通话的平均通话时长
	AvgWaitSec       int              `json:"avg_wait_sec"` // 接通通话的平均等待（answered-started）
	HourBuckets      []HourBucket     `json:"hour_buckets"` // 24 桶
	AgentLeaderboard []AgentReportRow `json:"agent_leaderboard"`
	QueueLeaderboard []QueueReportRow `json:"queue_leaderboard"`
}

// HourBucket 时段分布：每小时呼入量 + 接通量（0-23）。
type HourBucket struct {
	Hour     int `json:"hour"`
	Inbound  int `json:"inbound"`
	Answered int `json:"answered"`
}

// AgentReportRow 坐席榜：按 agent_ext 分组的接听数/总时长/均长。
type AgentReportRow struct {
	AgentExt   string `json:"agent_ext"`
	Answered   int    `json:"answered"`
	TotalTalk  int    `json:"total_talk_sec"`
	AvgTalkSec int    `json:"avg_talk_sec"`
}

// QueueReportRow 队列榜：按 queue 分组的进队数/接通/放弃。
type QueueReportRow struct {
	Queue     string `json:"queue"`
	Entered   int    `json:"entered"`
	Answered  int    `json:"answered"`
	Abandoned int    `json:"abandoned"`
}

// ReportOverview 拉取 [from,to) 内的话单并在 Go 内聚合出整包报表。
func (s *Store) ReportOverview(ctx context.Context, tenantID uint64, from, to time.Time) (*ReportOverview, error) {
	// 以 started_at 为准落桶；started_at 为空的（异常/未起振铃）用 created_at 兜底。
	rows, err := fullread.All[Call](s.db.WithContext(ctx).
		Where("tenant_id = ?", EnsureTenant(tenantID)).
		Where("(started_at >= ? AND started_at < ?) OR (started_at IS NULL AND created_at >= ? AND created_at < ?)",
			from, to, from, to))
	if err != nil {
		return nil, err
	}

	out := &ReportOverview{
		From:        from.Format(time.RFC3339),
		To:          to.Format(time.RFC3339),
		HourBuckets: make([]HourBucket, 24),
	}
	for h := 0; h < 24; h++ {
		out.HourBuckets[h].Hour = h
	}

	var talkSum, talkCount int
	var waitSum, waitCount int
	agents := map[string]*AgentReportRow{}
	queues := map[string]*QueueReportRow{}

	for i := range rows {
		r := rows[i]
		inbound := r.Direction != "outbound" // 缺省/inbound 都算呼入
		answered := r.AnsweredAt != nil
		if inbound {
			out.TotalInbound++
		} else {
			out.TotalOutbound++
		}
		if answered {
			out.Answered++
		} else if inbound {
			out.Abandoned++ // 呼入未接通=放弃
		}

		// 时段落桶（本地时区小时）
		bucketTime := r.CreatedAt
		if r.StartedAt != nil {
			bucketTime = *r.StartedAt
		}
		h := bucketTime.Local().Hour()
		if h >= 0 && h < 24 {
			if inbound {
				out.HourBuckets[h].Inbound++
			}
			if answered {
				out.HourBuckets[h].Answered++
			}
		}

		// 通话时长（仅接通计入均值）
		if answered && r.DurationSec > 0 {
			talkSum += r.DurationSec
			talkCount++
		}
		// 等待时长（接通且有 started/answered 时）
		if answered && r.StartedAt != nil && r.AnsweredAt != nil {
			w := int(r.AnsweredAt.Sub(*r.StartedAt).Seconds())
			if w >= 0 {
				waitSum += w
				waitCount++
			}
		}

		// 坐席榜（按分机，接听计数）
		if answered && r.AgentExt != "" {
			a := agents[r.AgentExt]
			if a == nil {
				a = &AgentReportRow{AgentExt: r.AgentExt}
				agents[r.AgentExt] = a
			}
			a.Answered++
			a.TotalTalk += r.DurationSec
		}

		// 队列榜（按队列）
		if r.Queue != "" {
			q := queues[r.Queue]
			if q == nil {
				q = &QueueReportRow{Queue: r.Queue}
				queues[r.Queue] = q
			}
			q.Entered++
			if answered {
				q.Answered++
			} else {
				q.Abandoned++
			}
		}
	}

	out.Total = out.TotalInbound + out.TotalOutbound
	if out.Total > 0 {
		out.AnswerRate = round1(float64(out.Answered) * 100 / float64(out.Total))
	}
	if talkCount > 0 {
		out.AvgTalkSec = talkSum / talkCount
	}
	if waitCount > 0 {
		out.AvgWaitSec = waitSum / waitCount
	}

	// 坐席榜：均长 + 排序（接听数降序，前 20）
	for _, a := range agents {
		if a.Answered > 0 {
			a.AvgTalkSec = a.TotalTalk / a.Answered
		}
		out.AgentLeaderboard = append(out.AgentLeaderboard, *a)
	}
	sort.Slice(out.AgentLeaderboard, func(i, j int) bool {
		if out.AgentLeaderboard[i].Answered != out.AgentLeaderboard[j].Answered {
			return out.AgentLeaderboard[i].Answered > out.AgentLeaderboard[j].Answered
		}
		return out.AgentLeaderboard[i].AgentExt < out.AgentLeaderboard[j].AgentExt
	})
	if len(out.AgentLeaderboard) > 20 {
		out.AgentLeaderboard = out.AgentLeaderboard[:20]
	}

	// 队列榜：进队数降序
	for _, q := range queues {
		out.QueueLeaderboard = append(out.QueueLeaderboard, *q)
	}
	sort.Slice(out.QueueLeaderboard, func(i, j int) bool {
		if out.QueueLeaderboard[i].Entered != out.QueueLeaderboard[j].Entered {
			return out.QueueLeaderboard[i].Entered > out.QueueLeaderboard[j].Entered
		}
		return out.QueueLeaderboard[i].Queue < out.QueueLeaderboard[j].Queue
	})

	return out, nil
}

func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
