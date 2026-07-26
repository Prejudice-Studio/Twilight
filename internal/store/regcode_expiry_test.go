package store

import "testing"

// TestRegCodeExpired 锁定注册码过期判定的唯一口径（纯函数，锁内锁外共用）。
// 重点回归「暂停但仍有效」一类：旧容量核算漏算 PausedSeconds/PauseStart，会把这类码
// 误判为过期 → 剩余名额算成 0 → 允许越过 emby_user_limit 超发。统一到 RegCodeExpired 后，
// 消费校验 / 展示态 / 容量核算三处必须给出一致答案。
func TestRegCodeExpired(t *testing.T) {
	const hour = int64(3600)
	// 基准创建时刻取一个较大的固定值，避免与 0 边界混淆。
	const created = int64(1_000_000)
	cases := []struct {
		name string
		code RegCode
		now  int64
		want bool
	}{
		{
			name: "永久有效(validity<=0)",
			code: RegCode{CreatedAt: created, ValidityTime: -1},
			now:  created + 100*hour,
			want: false,
		},
		{
			name: "validity=0 视为永久",
			code: RegCode{CreatedAt: created, ValidityTime: 0},
			now:  created + 100*hour,
			want: false,
		},
		{
			name: "未到期",
			code: RegCode{CreatedAt: created, ValidityTime: 10},
			now:  created + 9*hour,
			want: false,
		},
		{
			name: "恰好到期(elapsed==validity)",
			code: RegCode{CreatedAt: created, ValidityTime: 10},
			now:  created + 10*hour,
			want: true,
		},
		{
			name: "已过期",
			code: RegCode{CreatedAt: created, ValidityTime: 10},
			now:  created + 11*hour,
			want: true,
		},
		{
			name: "暂停累计后仍有效(核心回归)",
			// 有效期 10 小时，已过去 12 小时，但其中 5 小时处于暂停累计，
			// 实际消耗 7 小时 < 10 小时 → 仍有效。旧口径漏算 PausedSeconds 会误判过期。
			code: RegCode{CreatedAt: created, ValidityTime: 10, PausedSeconds: 5 * hour},
			now:  created + 12*hour,
			want: false,
		},
		{
			name: "暂停累计后到期",
			// 已过去 16 小时，暂停累计 5 小时，实际消耗 11 小时 >= 10 小时 → 过期。
			code: RegCode{CreatedAt: created, ValidityTime: 10, PausedSeconds: 5 * hour},
			now:  created + 16*hour,
			want: true,
		},
		{
			name: "正处暂停态则冻结有效期(PauseStart>0)",
			// 当前正暂停：elapsed 冻结在 PauseStart 时刻。PauseStart 距创建 6 小时、
			// 无历史累计暂停 → 冻结消耗 6 小时 < 10 小时；哪怕 now 已远超创建 100 小时也不过期。
			code: RegCode{CreatedAt: created, ValidityTime: 10, PauseStart: created + 6*hour},
			now:  created + 100*hour,
			want: false,
		},
		{
			name: "暂停态叠加历史累计暂停后到期",
			// PauseStart 距创建 18 小时，历史累计暂停 5 小时 → 冻结消耗 13 小时 >= 10 → 过期。
			code: RegCode{CreatedAt: created, ValidityTime: 10, PausedSeconds: 5 * hour, PauseStart: created + 18*hour},
			now:  created + 200*hour,
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RegCodeExpired(tc.code, tc.now); got != tc.want {
				t.Fatalf("RegCodeExpired(%+v, %d) = %v, want %v", tc.code, tc.now, got, tc.want)
			}
		})
	}
}
