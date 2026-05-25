package store

import (
	"path/filepath"
	"testing"
)

// 防回归：DeleteUser 必须级联清理所有 UID-键控派生数据。
// 旧实现仅清 5 个域，留下幽灵关联：复用 UID 创建的新用户会继承旧设备 / 旧
// 邀请关系 / 旧签到积分。这条测试覆盖每个域至少一条记录。
func TestDeleteUserCascadesAllDerivedData(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// CreateUser 自动分配 UID；test 内不假设具体值，按 username 反查。
	// 直接 mutate s.state 不行：DeleteUser → refreshLocked 会从磁盘重读 state，
	// 抹掉未持久化的直接注入。
	uTarget, err := st.CreateUser(User{Username: "target", Active: true, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	uOther, err := st.CreateUser(User{Username: "other", Active: true, PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	target := uTarget.UID
	other := uOther.UID

	// 各域种入两条数据：一条挂 target，一条挂 other。所有写入都通过 store
	// 的公开方法或 saveLocked 包裹的 helper 持久化，避免被 refreshLocked 抹掉。
	if _, err := st.CreateAPIKey(APIKey{UID: target, Name: "k1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAPIKey(APIKey{UID: other, Name: "k2"}); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.state.InviteCodes["c1"] = InviteCode{Code: "c1", InviterUID: target, Active: true}
	st.state.InviteCodes["c2"] = InviteCode{Code: "c2", InviterUID: other, Active: true}
	st.state.InviteCodes["c3"] = InviteCode{Code: "c3", InviterUID: 999, UsedByUID: target}
	st.state.InviteRelations[target] = InviteRelation{ParentUID: 999, ChildUID: target}
	st.state.InviteRelations[other] = InviteRelation{ParentUID: target, ChildUID: other}
	st.state.MediaRequests[1] = MediaRequest{ID: 1, UID: target, Title: "x"}
	st.state.MediaRequests[2] = MediaRequest{ID: 2, UID: other, Title: "y"}
	st.state.Signin[target] = Signin{UID: target, Points: 100}
	st.state.Signin[other] = Signin{UID: other, Points: 50}
	st.state.Devices["dev:target"] = Device{UID: target, DeviceID: "dev:target", Trusted: true}
	st.state.Devices["dev:other"] = Device{UID: other, DeviceID: "dev:other"}
	st.state.LoginLogs = []LoginLog{{UID: target, IP: "1.1.1.1"}, {UID: other, IP: "2.2.2.2"}}
	st.state.PlaybackRecords = []PlaybackRecord{{UID: target, ItemID: "p1"}, {UID: other, ItemID: "p2"}}
	st.state.RebindRequests[1] = RebindRequest{ID: 1, UID: target}
	st.state.RebindRequests[2] = RebindRequest{ID: 2, UID: other}
	st.state.BindCodes["b1"] = BindCode{Code: "b1", UID: target}
	st.state.BindCodes["b2"] = BindCode{Code: "b2", UID: other}
	st.state.RegCodes["rc1"] = RegCode{Code: "rc1", UsedBy: target, UsedByUIDs: []int64{target, other, 999}}
	st.state.Announcements[1] = Announcement{ID: 1, Title: "by-target", CreatedByUID: target}
	st.state.Announcements[2] = Announcement{ID: 2, Title: "by-other", CreatedByUID: other}
	if err := st.saveLocked(); err != nil {
		st.mu.Unlock()
		t.Fatal(err)
	}
	st.mu.Unlock()

	// ViolationLog 走匿名化：target 的违规记录必须保留（审计 artefact）。
	if err := st.AddViolationLog(ViolationLog{UID: target, Code: "X", Reason: "decoy"}); err != nil {
		t.Fatal(err)
	}

	if err := st.DeleteUser(target); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// 用户本体 + 全部 GDPR-删除域：target 不再出现。
	if _, ok := st.state.Users[target]; ok {
		t.Fatalf("user %d still present", target)
	}
	for id, k := range st.state.APIKeys {
		if k.UID == target {
			t.Fatalf("api key %d still references deleted user", id)
		}
	}
	if _, ok := st.state.InviteCodes["c1"]; ok {
		t.Fatalf("invite code c1 (inviter=target) should be gone")
	}
	if _, ok := st.state.InviteCodes["c3"]; ok {
		t.Fatalf("invite code c3 (used_by=target) should be gone")
	}
	if _, ok := st.state.InviteCodes["c2"]; !ok {
		t.Fatalf("invite code c2 (other user) was wrongly deleted")
	}
	if _, ok := st.state.InviteRelations[target]; ok {
		t.Fatalf("invite relation as child should be gone")
	}
	if _, ok := st.state.InviteRelations[other]; ok {
		t.Fatalf("child relation parented by target should be gone")
	}
	if _, ok := st.state.MediaRequests[1]; ok {
		t.Fatalf("media request for target should be gone")
	}
	if _, ok := st.state.Signin[target]; ok {
		t.Fatalf("signin for target should be gone")
	}
	if _, ok := st.state.Devices["dev:target"]; ok {
		t.Fatalf("device for target should be gone (trusted-flag carryover risk)")
	}
	for _, log := range st.state.LoginLogs {
		if log.UID == target {
			t.Fatalf("login log for target should be gone")
		}
	}
	for _, p := range st.state.PlaybackRecords {
		if p.UID == target {
			t.Fatalf("playback record for target should be gone")
		}
	}
	if _, ok := st.state.RebindRequests[1]; ok {
		t.Fatalf("rebind request for target should be gone")
	}
	if _, ok := st.state.BindCodes["b1"]; ok {
		t.Fatalf("bind code for target should be gone")
	}

	// 旁观 other 的派生数据完整保留。
	if _, ok := st.state.Devices["dev:other"]; !ok {
		t.Fatalf("device for other was wrongly deleted")
	}
	if _, ok := st.state.MediaRequests[2]; !ok {
		t.Fatalf("media request for other was wrongly deleted")
	}
	if _, ok := st.state.Signin[other]; !ok {
		t.Fatalf("signin for other was wrongly deleted")
	}

	// RegCode 匿名化：rc1 仍在；UsedBy 应清零；UsedByUIDs 不再含 target，但保留其他 UID。
	rc := st.state.RegCodes["rc1"]
	if rc.UsedBy != 0 {
		t.Fatalf("regcode UsedBy not anonymized: %d", rc.UsedBy)
	}
	for _, u := range rc.UsedByUIDs {
		if u == target {
			t.Fatalf("regcode UsedByUIDs still contains target")
		}
	}
	if len(rc.UsedByUIDs) != 2 { // other + 999
		t.Fatalf("regcode UsedByUIDs lost non-target entries: %v", rc.UsedByUIDs)
	}

	// 公告匿名化：ann 1 仍存在但 CreatedByUID 清零；ann 2 不变。
	if _, ok := st.state.Announcements[1]; !ok {
		t.Fatalf("announcement 1 wrongly deleted (should be anonymized)")
	}
	if st.state.Announcements[1].CreatedByUID != 0 {
		t.Fatalf("announcement 1 CreatedByUID not anonymized")
	}
	if st.state.Announcements[2].CreatedByUID != other {
		t.Fatalf("announcement 2 CreatedByUID should be unchanged")
	}

	// ViolationLogs 保留：审计 artefact 不随用户删除。
	logs := st.ListViolationLogs()
	found := false
	for _, log := range logs {
		if log.UID == target {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("violation log for target should be retained for audit (got %d logs)", len(logs))
	}
}
